package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Search struct {
	db *pgxpool.Pool
}

func NewSearch(db *pgxpool.Pool) *Search {
	return &Search{db: db}
}

func (h *Search) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, apiError("q parameter is required"))
		return
	}

	// Full-text search using PostgreSQL tsvector, ranked by play_count.
	// ts_rank weights recent plays more — play_count DESC is the tiebreaker.
	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, title, artist_id, duration_ms, release_date::TEXT, play_count, created_at
		FROM tracks
		WHERE search_vector @@ plainto_tsquery('english', $1)
		ORDER BY ts_rank(search_vector, plainto_tsquery('english', $1)) DESC,
		         play_count DESC
		LIMIT 20`, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("search failed"))
		return
	}
	defer rows.Close()

	var results []trackRow
	for rows.Next() {
		var t trackRow
		if err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.DurationMs,
			&t.ReleaseDate, &t.PlayCount, &t.CreatedAt); err != nil {
			continue
		}
		results = append(results, t)
	}
	if results == nil {
		results = []trackRow{}
	}

	c.JSON(http.StatusOK, gin.H{"items": results, "total": len(results)})
}

// apiError is a shared helper for structured JSON error responses.
func apiError(msg string) gin.H {
	return gin.H{"error": msg}
}
