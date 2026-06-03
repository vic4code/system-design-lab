package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vic4code/system-design-lab/beatstream/internal/cache"
	"github.com/vic4code/system-design-lab/beatstream/internal/metrics"
	"github.com/vic4code/system-design-lab/beatstream/internal/search"
)

type Search struct {
	db    *pgxpool.Pool
	cache *cache.Redis
	os    *search.Client
}

func NewSearch(db *pgxpool.Pool, c *cache.Redis, os *search.Client) *Search {
	return &Search{db: db, cache: c, os: os}
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

	// If OpenSearch is available, use fuzzy multi-field search.
	if h.os != nil {
		results, err := h.os.Search(c.Request.Context(), q, 20)
		if err == nil {
			resp := searchResponse{
				Items: make([]trackRow, 0, len(results)),
				Total: len(results),
			}
			for _, r := range results {
				resp.Items = append(resp.Items, trackRow{
					ID:       r.ID,
					Title:    r.Title,
					ArtistID: r.ArtistName,
				})
			}
			if data, err := json.Marshal(resp); err == nil {
				h.cache.Set(c.Request.Context(), cacheKey, string(data), time.Minute)
			}
			c.JSON(http.StatusOK, resp)
			return
		}
		// OpenSearch failed — fall through to PostgreSQL tsvector.
	}

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

func apiError(msg string) gin.H {
	return gin.H{"error": msg}
}

func (h *Search) Autocomplete(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, apiError("q parameter is required"))
		return
	}

	if h.os == nil {
		c.JSON(http.StatusServiceUnavailable, apiError("autocomplete requires OpenSearch"))
		return
	}

	suggestions, err := h.os.Autocomplete(c.Request.Context(), q, 5)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("autocomplete failed"))
		return
	}
	if suggestions == nil {
		suggestions = []string{}
	}

	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
}
