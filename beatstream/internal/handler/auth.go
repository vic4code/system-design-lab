package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Auth struct {
	db     *pgxpool.Pool
	secret string
}

func NewAuth(db *pgxpool.Pool, secret string) *Auth {
	return &Auth{db: db, secret: secret}
}

type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Register godoc  POST /v1/auth/register
func (h *Auth) Register(c *gin.Context) {
	var body struct {
		Email    string `json:"email"    binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
		Name     string `json:"name"     binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, apiError(err.Error()))
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to hash password"))
		return
	}

	var user userResponse
	err = h.db.QueryRow(c.Request.Context(),
		`INSERT INTO users (email, password_hash, name)
		 VALUES ($1, $2, $3)
		 RETURNING id, email, name, created_at`,
		body.Email, string(hash), body.Name,
	).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt)
	if err != nil {
		// email conflict
		c.JSON(http.StatusConflict, apiError("email already registered"))
		return
	}

	token, err := h.generateToken(user.ID, user.Email, user.Name)
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
		`SELECT id, email, name, created_at, password_hash FROM users WHERE email = $1`,
		body.Email,
	).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt, &passwordHash)
	if err != nil {
		c.JSON(http.StatusUnauthorized, apiError("invalid email or password"))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(body.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, apiError("invalid email or password"))
		return
	}

	token, err := h.generateToken(user.ID, user.Email, user.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to generate token"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

// Me godoc  GET /v1/auth/me  (protected)
func (h *Auth) Me(c *gin.Context) {
	userID, _ := c.Get("user_id")
	var user userResponse
	err := h.db.QueryRow(c.Request.Context(),
		`SELECT id, email, name, created_at FROM users WHERE id = $1`, userID,
	).Scan(&user.ID, &user.Email, &user.Name, &user.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, apiError("user not found"))
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *Auth) generateToken(userID, email, name string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"name":    name,
		"exp":     time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.secret))
}
