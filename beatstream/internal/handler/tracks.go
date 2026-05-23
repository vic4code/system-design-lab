package handler

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vic4code/system-design-lab/beatstream/internal/storage"
)

type Tracks struct {
	db    *pgxpool.Pool
	store *storage.MinIO
}

func NewTracks(db *pgxpool.Pool, store *storage.MinIO) *Tracks {
	return &Tracks{db: db, store: store}
}

type trackRow struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	ArtistID    string    `json:"artist_id"`
	DurationMs  int       `json:"duration_ms"`
	ReleaseDate *string   `json:"release_date,omitempty"`
	PlayCount   int64     `json:"play_count"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h *Tracks) Get(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, apiError("invalid track id"))
		return
	}

	var t trackRow
	err := h.db.QueryRow(c.Request.Context(),
		`SELECT id, title, artist_id, duration_ms, release_date::TEXT, play_count, created_at
		 FROM tracks WHERE id = $1`, id).
		Scan(&t.ID, &t.Title, &t.ArtistID, &t.DurationMs, &t.ReleaseDate, &t.PlayCount, &t.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, apiError("track not found"))
		return
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

	// Record play event (fire-and-forget; errors are non-fatal)
	go func() {
		h.db.Exec(c.Request.Context(),
			`INSERT INTO play_events (track_id, source) VALUES ($1, $2)`,
			id, c.GetHeader("X-Source"))
		h.db.Exec(c.Request.Context(),
			`UPDATE tracks SET play_count = play_count + 1 WHERE id = $1`, id)
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

	var t trackRow
	err = h.db.QueryRow(c.Request.Context(),
		`INSERT INTO tracks (id, title, artist_id, audio_key)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, title, artist_id, duration_ms, NULL, play_count, created_at`,
		trackID, title, artistID, audioKey).
		Scan(&t.ID, &t.Title, &t.ArtistID, &t.DurationMs, &t.ReleaseDate, &t.PlayCount, &t.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to save track"))
		return
	}
	c.JSON(http.StatusCreated, t)
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
