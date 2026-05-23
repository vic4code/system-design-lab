package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vic4code/system-design-lab/beatstream/internal/cache"
	"github.com/vic4code/system-design-lab/beatstream/internal/metrics"
)

type Search struct {
	db    *pgxpool.Pool
	cache *cache.Redis
}

func NewSearch(db *pgxpool.Pool, c *cache.Redis) *Search {
	return &Search{db: db, cache: c}
}

type searchResponse struct {
	Items []trackRow `json:"items"`
	Total int        `json:"total"`
}

func (h *Search) Search(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, apiError("q parameter is required"))
		return
	}

	// Return cached search results if available (1min TTL — balances freshness vs. DB load).
	cacheKey := "search:" + q
	if cached, err := h.cache.Get(c.Request.Context(), cacheKey); err == nil {
		var resp searchResponse
		if json.Unmarshal([]byte(cached), &resp) == nil {
			metrics.CacheHits.WithLabelValues("search").Inc()
			c.JSON(http.StatusOK, resp)
			return
		}
	}
	metrics.CacheMisses.WithLabelValues("search").Inc()

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

	resp := searchResponse{Items: results, Total: len(results)}
	if data, err := json.Marshal(resp); err == nil {
		h.cache.Set(c.Request.Context(), cacheKey, string(data), time.Minute)
	}

	c.JSON(http.StatusOK, resp)
}

// apiError is a shared helper for structured JSON error responses.
func apiError(msg string) gin.H {
	return gin.H{"error": msg}
}
