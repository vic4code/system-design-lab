package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vic4code/system-design-lab/beatstream/internal/queue"
)

const TopicUploads = "track.uploads"

// UploadEvent is published by POST /tracks and consumed by UploadWorker.
type UploadEvent struct {
	TrackID  string `json:"track_id"`
	AudioKey string `json:"audio_key"`
}

// UploadWorker consumes track.uploads events and simulates transcoding.
type UploadWorker struct {
	consumer *queue.Consumer
	db       *pgxpool.Pool
}

func NewUploadWorker(brokers, kafkaAuth, awsRegion string, db *pgxpool.Pool) (*UploadWorker, error) {
	var c *queue.Consumer
	var err error
	if kafkaAuth == "iam" {
		c, err = queue.NewConsumerIAM(brokers, awsRegion, "upload-workers", TopicUploads)
	} else {
		c, err = queue.NewConsumer(brokers, "upload-workers", TopicUploads)
	}
	if err != nil {
		return nil, fmt.Errorf("upload worker consumer: %w", err)
	}
	return &UploadWorker{consumer: c, db: db}, nil
}

func (w *UploadWorker) Close() { w.consumer.Close() }

func (w *UploadWorker) Run(ctx context.Context) {
	log.Println("upload worker: listening on", TopicUploads)
	w.consumer.Consume(ctx, func(data []byte) error {
		var ev UploadEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return fmt.Errorf("unmarshal upload event: %w", err)
		}
		return w.process(ctx, ev)
	})
}

func (w *UploadWorker) process(ctx context.Context, ev UploadEvent) error {
	// Idempotency guard: skip if already processed.
	// This handles re-delivery after a consumer restart.
	var status string
	err := w.db.QueryRow(ctx, `SELECT status FROM tracks WHERE id = $1`, ev.TrackID).Scan(&status)
	if err != nil {
		return fmt.Errorf("status check: %w", err)
	}
	if status != "pending" {
		log.Printf("upload worker: track %s already in status=%s, skipping", ev.TrackID, status)
		return nil
	}

	// Mark as processing so a second consumer doesn't race.
	if _, err := w.db.Exec(ctx,
		`UPDATE tracks SET status = 'processing' WHERE id = $1 AND status = 'pending'`, ev.TrackID,
	); err != nil {
		return fmt.Errorf("set processing: %w", err)
	}

	log.Printf("upload worker: transcoding track %s", ev.TrackID)

	// Simulate transcoding: analyse audio, compute waveform, encode formats.
	// In production this calls ffmpeg; here we sleep and derive a fake duration.
	time.Sleep(2 * time.Second)
	durationMs := 120_000 + rand.Intn(180_000) // 2–5 min

	if _, err := w.db.Exec(ctx,
		`UPDATE tracks SET status = 'ready', duration_ms = $1 WHERE id = $2`,
		durationMs, ev.TrackID,
	); err != nil {
		// Best-effort rollback to error state so clients can see the failure.
		w.db.Exec(ctx, `UPDATE tracks SET status = 'error' WHERE id = $1`, ev.TrackID)
		return fmt.Errorf("set ready: %w", err)
	}

	log.Printf("upload worker: track %s ready (%dms)", ev.TrackID, durationMs)
	return nil
}
