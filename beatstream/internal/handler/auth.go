package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/vic4code/system-design-lab/beatstream/internal/middleware"
)

type Auth struct {
	db     *pgxpool.Pool
	secret string
}

func NewAuth(db *pgxpool.Pool, secret string) *Auth {
	return &Auth{db: db, secret: secret}
}

type userResponse struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	TermsVersion int       `json:"terms_version"`
	CreatedAt    time.Time `json:"created_at"`
}

// Register godoc  POST /v1/auth/register
func (h *Auth) Register(c *gin.Context) {
	var body struct {
		Email        string `json:"email"         binding:"required,email"`
		Password     string `json:"password"      binding:"required,min=6"`
		Name         string `json:"name"          binding:"required"`
		TermsVersion int    `json:"terms_version"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, apiError(err.Error()))
		return
	}
	if body.TermsVersion == 0 {
		body.TermsVersion = 1
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to hash password"))
		return
	}

	var user userResponse
	err = h.db.QueryRow(c.Request.Context(),
		`INSERT INTO users (email, password_hash, name, terms_version)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, email, name, role, terms_version, created_at`,
		body.Email, string(hash), body.Name, body.TermsVersion,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.TermsVersion, &user.CreatedAt)
	if err != nil {
		c.JSON(http.StatusConflict, apiError("email already registered"))
		return
	}

	token, err := h.generateToken(user.ID, user.Email, user.Name, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to generate token"))
		return
	}

	c.JSON(http.StatusCreated, gin.H{"token": token, "user": user})
}

// Login godoc  POST /v1/auth/login
func (h *Auth) Login(c *gin.Context) {
	var body struct {
		Email    string `json:"email"    binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, apiError(err.Error()))
		return
	}

	var user userResponse
	var passwordHash string
	err := h.db.QueryRow(c.Request.Context(),
		`SELECT id, email, name, role, terms_version, created_at, password_hash
		 FROM users WHERE email = $1 AND deleted_at IS NULL`,
		body.Email,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.TermsVersion, &user.CreatedAt, &passwordHash)
	if err != nil {
		c.JSON(http.StatusUnauthorized, apiError("invalid email or password"))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(body.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, apiError("invalid email or password"))
		return
	}

	token, err := h.generateToken(user.ID, user.Email, user.Name, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to generate token"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

// Me godoc  GET /v1/auth/me  (protected)
func (h *Auth) Me(c *gin.Context) {
	userID, _ := c.Get(middleware.UserIDKey)
	var user userResponse
	err := h.db.QueryRow(c.Request.Context(),
		`SELECT id, email, name, role, terms_version, created_at
		 FROM users WHERE id = $1 AND deleted_at IS NULL`, userID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.TermsVersion, &user.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, apiError("user not found"))
		return
	}
	c.JSON(http.StatusOK, user)
}

// DeleteMe godoc  DELETE /v1/me  (protected) — GDPR erasure
// Soft-deletes the account: sets deleted_at, anonymises name/email.
// Tracks and playlists are kept but ownership is preserved for 30-day grace period.
// audit_logs.user_id goes NULL via ON DELETE SET NULL on hard delete (future job).
func (h *Auth) DeleteMe(c *gin.Context) {
	userID, _ := c.Get(middleware.UserIDKey)

	tag, err := h.db.Exec(c.Request.Context(),
		`UPDATE users
		 SET deleted_at = NOW(),
		     email      = 'deleted-' || id || '@removed.invalid',
		     name       = '[deleted]',
		     password_hash = ''
		 WHERE id = $1 AND deleted_at IS NULL`,
		userID,
	)
	if err != nil || tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, apiError("user not found or already deleted"))
		return
	}
	c.Status(http.StatusNoContent)
}

// ExportMe godoc  GET /v1/me/export  (protected) — GDPR data export
func (h *Auth) ExportMe(c *gin.Context) {
	userID, _ := c.Get(middleware.UserIDKey)
	ctx := c.Request.Context()

	var user userResponse
	err := h.db.QueryRow(ctx,
		`SELECT id, email, name, role, terms_version, created_at
		 FROM users WHERE id = $1 AND deleted_at IS NULL`, userID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.Role, &user.TermsVersion, &user.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, apiError("user not found"))
		return
	}

	// Tracks owned by user (tracks table has no uploaded_by yet — return empty for now)
	type trackExport struct {
		ID        string    `json:"id"`
		Title     string    `json:"title"`
		Status    string    `json:"status"`
		CreatedAt time.Time `json:"created_at"`
	}
	var tracks []trackExport

	// Playlists owned by user
	playlistRows, err := h.db.Query(ctx,
		`SELECT id, name, created_at FROM playlists WHERE owner_id = $1`, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to export playlists"))
		return
	}
	defer playlistRows.Close()

	type playlistExport struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at"`
	}
	var playlists []playlistExport
	for playlistRows.Next() {
		var p playlistExport
		if err := playlistRows.Scan(&p.ID, &p.Name, &p.CreatedAt); err == nil {
			playlists = append(playlists, p)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"exported_at": time.Now().UTC(),
		"user":        user,
		"tracks":      tracks,
		"playlists":   playlists,
	})
}

func (h *Auth) generateToken(userID, email, name, role string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"name":    name,
		"role":    role,
		"exp":     time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.secret))
}
