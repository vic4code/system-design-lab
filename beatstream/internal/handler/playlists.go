package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Playlists struct {
	db *pgxpool.Pool
}

func NewPlaylists(db *pgxpool.Pool) *Playlists {
	return &Playlists{db: db}
}

type playlistRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *Playlists) Get(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, apiError("invalid playlist id"))
		return
	}

	var p playlistRow
	err := h.db.QueryRow(c.Request.Context(),
		`SELECT id, name, created_at FROM playlists WHERE id = $1`, id).
		Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, apiError("playlist not found"))
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Playlists) Create(c *gin.Context) {
	var body struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, apiError("name is required"))
		return
	}

	var p playlistRow
	err := h.db.QueryRow(c.Request.Context(),
		`INSERT INTO playlists (name) VALUES ($1) RETURNING id, name, created_at`,
		body.Name).Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to create playlist"))
		return
	}
	c.JSON(http.StatusCreated, p)
}

// ListTracks uses cursor-based pagination (after= param is the last seen track_id).
func (h *Playlists) ListTracks(c *gin.Context) {
	id := c.Param("id")
	limit := 50
	after := c.Query("after")

	var rows []trackRow
	var query string
	var args []any

	if after == "" {
		query = `
			SELECT t.id, t.title, t.artist_id, t.duration_ms, t.release_date::TEXT, t.play_count, t.created_at
			FROM playlist_tracks pt
			JOIN tracks t ON t.id = pt.track_id
			WHERE pt.playlist_id = $1
			ORDER BY pt.position, pt.added_at
			LIMIT $2`
		args = []any{id, limit + 1}
	} else {
		// Cursor: return tracks after the given position
		query = `
			SELECT t.id, t.title, t.artist_id, t.duration_ms, t.release_date::TEXT, t.play_count, t.created_at
			FROM playlist_tracks pt
			JOIN tracks t ON t.id = pt.track_id
			WHERE pt.playlist_id = $1
			  AND pt.added_at > (SELECT added_at FROM playlist_tracks WHERE playlist_id = $1 AND track_id = $2)
			ORDER BY pt.position, pt.added_at
			LIMIT $3`
		args = []any{id, after, limit + 1}
	}

	dbRows, err := h.db.Query(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to list tracks"))
		return
	}
	defer dbRows.Close()

	for dbRows.Next() {
		var t trackRow
		if err := dbRows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.DurationMs,
			&t.ReleaseDate, &t.PlayCount, &t.CreatedAt); err != nil {
			continue
		}
		rows = append(rows, t)
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	var nextCursor string
	if hasMore && len(rows) > 0 {
		nextCursor = rows[len(rows)-1].ID
	}

	c.JSON(http.StatusOK, gin.H{
		"items":       rows,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
	})
}

func (h *Playlists) AddTrack(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		TrackID string `json:"track_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, apiError("track_id is required"))
		return
	}

	_, err := h.db.Exec(c.Request.Context(),
		`INSERT INTO playlist_tracks (playlist_id, track_id)
		 VALUES ($1, $2)
		 ON CONFLICT (playlist_id, track_id) DO NOTHING`,
		id, body.TrackID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to add track"))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Playlists) RemoveTrack(c *gin.Context) {
	id := c.Param("id")
	trackID := c.Param("track_id")

	_, err := h.db.Exec(c.Request.Context(),
		`DELETE FROM playlist_tracks WHERE playlist_id = $1 AND track_id = $2`,
		id, trackID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to remove track"))
		return
	}
	c.Status(http.StatusNoContent)
}
