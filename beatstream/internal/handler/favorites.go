package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vic4code/system-design-lab/beatstream/internal/middleware"
)

type Favorites struct {
	db *pgxpool.Pool
}

func NewFavorites(db *pgxpool.Pool) *Favorites {
	return &Favorites{db: db}
}

func (h *Favorites) Add(c *gin.Context) {
	userID, _ := c.Get(middleware.UserIDKey)
	uid, _ := userID.(string)

	var body struct {
		TrackID string `json:"track_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, apiError("track_id is required"))
		return
	}

	_, err := h.db.Exec(c.Request.Context(),
		`INSERT INTO favorites (user_id, track_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		uid, body.TrackID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to add favorite"))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Favorites) Remove(c *gin.Context) {
	userID, _ := c.Get(middleware.UserIDKey)
	uid, _ := userID.(string)
	trackID := c.Param("track_id")

	_, err := h.db.Exec(c.Request.Context(),
		`DELETE FROM favorites WHERE user_id = $1 AND track_id = $2`, uid, trackID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to remove favorite"))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Favorites) List(c *gin.Context) {
	userID, _ := c.Get(middleware.UserIDKey)
	uid, _ := userID.(string)

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT t.id, t.title, t.artist_id, t.duration_ms, t.release_date::TEXT, t.play_count, t.status, t.created_at
		FROM favorites f JOIN tracks t ON t.id = f.track_id
		WHERE f.user_id = $1
		ORDER BY f.created_at DESC
		LIMIT 200`, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to list favorites"))
		return
	}
	defer rows.Close()

	tracks := []trackRow{}
	for rows.Next() {
		var t trackRow
		if err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.DurationMs,
			&t.ReleaseDate, &t.PlayCount, &t.Status, &t.CreatedAt); err != nil {
			continue
		}
		tracks = append(tracks, t)
	}
	c.JSON(http.StatusOK, gin.H{"items": tracks, "total": len(tracks)})
}

func (h *Favorites) IDs(c *gin.Context) {
	userID, _ := c.Get(middleware.UserIDKey)
	uid, _ := userID.(string)

	rows, err := h.db.Query(c.Request.Context(),
		`SELECT track_id FROM favorites WHERE user_id = $1`, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to get favorites"))
		return
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	c.JSON(http.StatusOK, gin.H{"ids": ids})
}
