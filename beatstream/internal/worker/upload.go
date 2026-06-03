package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vic4code/system-design-lab/beatstream/internal/queue"
	"github.com/vic4code/system-design-lab/beatstream/internal/search"
	"github.com/vic4code/system-design-lab/beatstream/internal/storage"
)

const TopicUploads = "track.uploads"

type UploadEvent struct {
	TrackID  string `json:"track_id"`
	AudioKey string `json:"audio_key"`
}

type UploadWorker struct {
	consumer *queue.Consumer
	db       *pgxpool.Pool
	store    *storage.Storage
	search   *search.Client
}

func NewUploadWorker(brokers, kafkaAuth, awsRegion string, db *pgxpool.Pool, store *storage.Storage, sc *search.Client) (*UploadWorker, error) {
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
	return &UploadWorker{consumer: c, db: db, store: store, search: sc}, nil
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
	var status string
	err := w.db.QueryRow(ctx, `SELECT status FROM tracks WHERE id = $1`, ev.TrackID).Scan(&status)
	if err != nil {
		return fmt.Errorf("status check: %w", err)
	}
	if status != "pending" {
		log.Printf("upload worker: track %s already in status=%s, skipping", ev.TrackID, status)
		return nil
	}

	if _, err := w.db.Exec(ctx,
		`UPDATE tracks SET status = 'processing' WHERE id = $1 AND status = 'pending'`, ev.TrackID,
	); err != nil {
		return fmt.Errorf("set processing: %w", err)
	}

	log.Printf("upload worker: transcoding track %s", ev.TrackID)

	tmpDir, err := os.MkdirTemp("", "transcode-"+ev.TrackID)
	if err != nil {
		w.markError(ctx, ev.TrackID)
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input")
	if err := w.downloadFromS3(ctx, ev.AudioKey, inputPath); err != nil {
		w.markError(ctx, ev.TrackID)
		return fmt.Errorf("download original: %w", err)
	}

	durationMs, err := ProbeDuration(inputPath)
	if err != nil {
		w.markError(ctx, ev.TrackID)
		return fmt.Errorf("probe duration: %w", err)
	}

	for _, bitrate := range Bitrates {
		result, err := Transcode(inputPath, ev.TrackID, bitrate, tmpDir)
		if err != nil {
			w.markError(ctx, ev.TrackID)
			return fmt.Errorf("transcode %dk: %w", bitrate, err)
		}

		data, err := os.ReadFile(result.LocalPath)
		if err != nil {
			w.markError(ctx, ev.TrackID)
			return fmt.Errorf("read transcoded file: %w", err)
		}

		if err := w.store.Upload(ctx, result.OutputKey, bytes.NewReader(data), int64(len(data)), "audio/ogg"); err != nil {
			w.markError(ctx, ev.TrackID)
			return fmt.Errorf("upload %dk to s3: %w", bitrate, err)
		}

		if _, err := w.db.Exec(ctx,
			`INSERT INTO track_formats (track_id, bitrate, codec, s3_key, size_bytes)
			 VALUES ($1, $2, 'ogg', $3, $4)
			 ON CONFLICT (track_id, bitrate, codec) DO UPDATE SET s3_key = $3, size_bytes = $4`,
			ev.TrackID, bitrate, result.OutputKey, result.SizeBytes,
		); err != nil {
			w.markError(ctx, ev.TrackID)
			return fmt.Errorf("insert track_format: %w", err)
		}

		log.Printf("upload worker: track %s encoded %dk (%d bytes)", ev.TrackID, bitrate, result.SizeBytes)
	}

	if _, err := w.db.Exec(ctx,
		`UPDATE tracks SET status = 'ready', duration_ms = $1 WHERE id = $2`,
		durationMs, ev.TrackID,
	); err != nil {
		w.markError(ctx, ev.TrackID)
		return fmt.Errorf("set ready: %w", err)
	}

	log.Printf("upload worker: track %s ready (duration=%dms)", ev.TrackID, durationMs)

	// Index into OpenSearch for fuzzy search (best-effort).
	if w.search != nil {
		var title, artistName string
		w.db.QueryRow(ctx,
			`SELECT t.title, a.name FROM tracks t JOIN artists a ON a.id = t.artist_id WHERE t.id = $1`,
			ev.TrackID).Scan(&title, &artistName)
		if title != "" {
			w.search.Index(ctx, search.TrackDocument{
				ID:         ev.TrackID,
				Title:      title,
				ArtistName: artistName,
				Status:     "ready",
			})
		}
	}

	return nil
}

func (w *UploadWorker) downloadFromS3(ctx context.Context, key, destPath string) error {
	reader, err := w.store.Download(ctx, key)
	if err != nil {
		return err
	}
	defer reader.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	return err
}

func (w *UploadWorker) markError(ctx context.Context, trackID string) {
	w.db.Exec(ctx, `UPDATE tracks SET status = 'error' WHERE id = $1`, trackID)
}
