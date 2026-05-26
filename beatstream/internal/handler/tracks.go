package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vic4code/system-design-lab/beatstream/internal/cache"
	"github.com/vic4code/system-design-lab/beatstream/internal/metrics"
	"github.com/vic4code/system-design-lab/beatstream/internal/queue"
	"github.com/vic4code/system-design-lab/beatstream/internal/storage"
	"github.com/vic4code/system-design-lab/beatstream/internal/worker"
)

type Tracks struct {
	db       *pgxpool.Pool
	store    *storage.Storage
	cache    *cache.Redis
	producer queue.Publisher
}

func NewTracks(db *pgxpool.Pool, store *storage.Storage, c *cache.Redis, p queue.Publisher) *Tracks {
	return &Tracks{db: db, store: store, cache: c, producer: p}
}

type trackRow struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	ArtistID    string    `json:"artist_id"`
	DurationMs  int       `json:"duration_ms"`
	ReleaseDate *string   `json:"release_date,omitempty"`
	PlayCount   int64     `json:"play_count"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h *Tracks) List(c *gin.Context) {
	rows, err := h.db.Query(c.Request.Context(),
		`SELECT id, title, artist_id, duration_ms, release_date::TEXT, play_count, status, created_at
		 FROM tracks ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to list tracks"))
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

func (h *Tracks) Get(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, apiError("invalid track id"))
		return
	}

	// Cache-aside: return cached metadata if available (1h TTL).
	cacheKey := fmt.Sprintf("track:%s", id)
	if cached, err := h.cache.Get(c.Request.Context(), cacheKey); err == nil {
		var t trackRow
		if json.Unmarshal([]byte(cached), &t) == nil {
			metrics.CacheHits.WithLabelValues("track").Inc()
			c.JSON(http.StatusOK, t)
			return
		}
	}
	metrics.CacheMisses.WithLabelValues("track").Inc()

	var t trackRow
	err := h.db.QueryRow(c.Request.Context(),
		`SELECT id, title, artist_id, duration_ms, release_date::TEXT, play_count, status, created_at
		 FROM tracks WHERE id = $1`, id).
		Scan(&t.ID, &t.Title, &t.ArtistID, &t.DurationMs, &t.ReleaseDate, &t.PlayCount, &t.Status, &t.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, apiError("track not found"))
		return
	}

	// Only cache ready tracks — pending/processing status changes and must not be frozen in cache.
	if t.Status == "ready" {
		if data, err := json.Marshal(t); err == nil {
			h.cache.Set(c.Request.Context(), cacheKey, string(data), time.Hour)
		}
	}
	c.JSON(http.StatusOK, t)
}

func (h *Tracks) Stream(c *gin.Context) {
	id := c.Param("id")

	var audioKey string
	err := h.db.QueryRow(c.Request.Context(),
		`SELECT audio_key FROM tracks WHERE id = $1`, id).Scan(&audioKey)
	if err != nil {
		c.JSON(http.StatusNotFound, apiError("track not found"))
		return
	}

	// Publish play event to Redpanda; analytics worker persists it asynchronously.
	// Fire-and-forget: losing a play event is acceptable for analytics.
	go func() {
		ev := worker.PlayEvent{
			TrackID:   id,
			Source:    c.GetHeader("X-Source"),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
		if err := h.producer.Publish(context.Background(), worker.TopicPlayEvents, id, ev); err != nil {
			log.Printf("play event publish: %v", err)
		}
	}()

	// Return a pre-signed URL valid for 1 hour — client streams directly from MinIO/S3.
	// This avoids proxying large audio bytes through the API server.
	u, err := h.store.PresignedURL(c.Request.Context(), audioKey, time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("could not generate stream URL"))
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, u.String())
}

func (h *Tracks) Create(c *gin.Context) {
	title := c.PostForm("title")
	artistID := c.PostForm("artist_id")
	if title == "" || artistID == "" {
		c.JSON(http.StatusBadRequest, apiError("title and artist_id are required"))
		return
	}

	file, header, err := c.Request.FormFile("audio")
	if err != nil {
		c.JSON(http.StatusBadRequest, apiError("audio file is required"))
		return
	}
	defer file.Close()

	// Validate artist exists
	var exists bool
	h.db.QueryRow(c.Request.Context(),
		`SELECT EXISTS(SELECT 1 FROM artists WHERE id = $1)`, artistID).Scan(&exists)
	if !exists {
		c.JSON(http.StatusBadRequest, apiError("artist not found"))
		return
	}

	trackID := uuid.New().String()
	audioKey := fmt.Sprintf("tracks/%s/audio", trackID)

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}

	// Read into memory to get size (fine for Phase 0; use multipart upload for large files)
	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to read file"))
		return
	}

	if err := h.store.Upload(c.Request.Context(), audioKey,
		newBytesReader(data), int64(len(data)), contentType); err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to upload audio"))
		return
	}

	// Insert with status='pending'; upload worker will transcode and flip to 'ready'.
	var t trackRow
	err = h.db.QueryRow(c.Request.Context(),
		`INSERT INTO tracks (id, title, artist_id, audio_key, status)
		 VALUES ($1, $2, $3, $4, 'pending')
		 RETURNING id, title, artist_id, duration_ms, NULL, play_count, status, created_at`,
		trackID, title, artistID, audioKey).
		Scan(&t.ID, &t.Title, &t.ArtistID, &t.DurationMs, &t.ReleaseDate, &t.PlayCount, &t.Status, &t.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to save track"))
		return
	}

	// Publish upload event; if this fails the track is stuck in 'pending'.
	// Production fix: use the transactional outbox pattern to avoid this split-brain.
	ev := worker.UploadEvent{TrackID: trackID, AudioKey: audioKey}
	if err := h.producer.Publish(c.Request.Context(), worker.TopicUploads, trackID, ev); err != nil {
		log.Printf("upload event publish: %v", err)
		c.JSON(http.StatusInternalServerError, apiError("failed to queue upload"))
		return
	}

	// 202 Accepted: the track is saved and queued for processing.
	// Poll GET /tracks/:id until status == "ready".
	c.JSON(http.StatusAccepted, t)
}

// bytesReader wraps []byte to implement io.Reader without importing bytes package name conflict.
type bytesReader struct {
	data   []byte
	offset int
}

func newBytesReader(data []byte) *bytesReader { return &bytesReader{data: data} }

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}
