package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vic4code/system-design-lab/beatstream/internal/cache"
	"github.com/vic4code/system-design-lab/beatstream/internal/metrics"
	"github.com/vic4code/system-design-lab/beatstream/internal/middleware"
)

type Recommendations struct {
	db    *pgxpool.Pool
	cache *cache.Redis
}

func NewRecommendations(db *pgxpool.Pool, c *cache.Redis) *Recommendations {
	return &Recommendations{db: db, cache: c}
}

func (h *Recommendations) TopTracks(c *gin.Context) {
	cacheKey := "recs:top"
	if cached, err := h.cache.Get(c.Request.Context(), cacheKey); err == nil {
		var resp searchResponse
		if json.Unmarshal([]byte(cached), &resp) == nil {
			metrics.CacheHits.WithLabelValues("recs_top").Inc()
			c.JSON(http.StatusOK, resp)
			return
		}
	}
	metrics.CacheMisses.WithLabelValues("recs_top").Inc()

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, title, artist_id, duration_ms, release_date::TEXT, play_count, status, created_at
		FROM tracks WHERE status = 'ready'
		ORDER BY play_count DESC LIMIT 20`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to fetch top tracks"))
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

	resp := searchResponse{Items: tracks, Total: len(tracks)}
	if data, err := json.Marshal(resp); err == nil {
		h.cache.Set(c.Request.Context(), cacheKey, string(data), 5*time.Minute)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Recommendations) ForYou(c *gin.Context) {
	userID, _ := c.Get(middleware.UserIDKey)
	uid, ok := userID.(string)
	if !ok || uid == "" {
		c.JSON(http.StatusUnauthorized, apiError("authentication required"))
		return
	}

	cacheKey := "recs:foryou:" + uid
	if cached, err := h.cache.Get(c.Request.Context(), cacheKey); err == nil {
		var resp searchResponse
		if json.Unmarshal([]byte(cached), &resp) == nil {
			metrics.CacheHits.WithLabelValues("recs_foryou").Inc()
			c.JSON(http.StatusOK, resp)
			return
		}
	}
	metrics.CacheMisses.WithLabelValues("recs_foryou").Inc()

	rows, err := h.db.Query(c.Request.Context(), `
		WITH user_top_artists AS (
			SELECT t.artist_id, COUNT(*) as plays
			FROM play_events pe JOIN tracks t ON t.id = pe.track_id
			WHERE pe.user_id = $1
			GROUP BY t.artist_id ORDER BY plays DESC LIMIT 5
		)
		SELECT t.id, t.title, t.artist_id, t.duration_ms, t.release_date::TEXT, t.play_count, t.status, t.created_at
		FROM tracks t JOIN user_top_artists uta ON t.artist_id = uta.artist_id
		WHERE t.id NOT IN (SELECT track_id FROM play_events WHERE user_id = $1)
		  AND t.status = 'ready'
		ORDER BY t.play_count DESC LIMIT 20`, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to fetch recommendations"))
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

	resp := searchResponse{Items: tracks, Total: len(tracks)}
	if data, err := json.Marshal(resp); err == nil {
		h.cache.Set(c.Request.Context(), cacheKey, string(data), 10*time.Minute)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Recommendations) Similar(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, apiError("invalid track id"))
		return
	}

	cacheKey := "recs:similar:" + id
	if cached, err := h.cache.Get(c.Request.Context(), cacheKey); err == nil {
		var resp searchResponse
		if json.Unmarshal([]byte(cached), &resp) == nil {
			metrics.CacheHits.WithLabelValues("recs_similar").Inc()
			c.JSON(http.StatusOK, resp)
			return
		}
	}
	metrics.CacheMisses.WithLabelValues("recs_similar").Inc()

	rows, err := h.db.Query(c.Request.Context(), `
		SELECT id, title, artist_id, duration_ms, release_date::TEXT, play_count, status, created_at
		FROM tracks
		WHERE artist_id = (SELECT artist_id FROM tracks WHERE id = $1)
		  AND id != $1 AND status = 'ready'
		ORDER BY play_count DESC LIMIT 10`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to fetch similar tracks"))
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

	resp := searchResponse{Items: tracks, Total: len(tracks)}
	if data, err := json.Marshal(resp); err == nil {
		h.cache.Set(c.Request.Context(), cacheKey, string(data), time.Hour)
	}
	c.JSON(http.StatusOK, resp)
}
