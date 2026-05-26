package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vic4code/system-design-lab/beatstream/internal/queue"
)

const TopicPlayEvents = "play.events"

// PlayEvent is published by GET /tracks/:id/stream and consumed by AnalyticsWorker.
type PlayEvent struct {
	TrackID   string `json:"track_id"`
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
}

// AnalyticsWorker consumes play.events and persists them to PostgreSQL.
// At-least-once delivery means a crashed consumer may double-count a play;
// that's acceptable for analytics (vs. billing). Use deduplication by
// event ID for stricter requirements.
type AnalyticsWorker struct {
	consumer *queue.Consumer
	db       *pgxpool.Pool
}

func NewAnalyticsWorker(brokers, kafkaAuth, awsRegion string, db *pgxpool.Pool) (*AnalyticsWorker, error) {
	var c *queue.Consumer
	var err error
	if kafkaAuth == "iam" {
		c, err = queue.NewConsumerIAM(brokers, awsRegion, "analytics-workers", TopicPlayEvents)
	} else {
		c, err = queue.NewConsumer(brokers, "analytics-workers", TopicPlayEvents)
	}
	if err != nil {
		return nil, fmt.Errorf("analytics worker consumer: %w", err)
	}
	return &AnalyticsWorker{consumer: c, db: db}, nil
}

func (w *AnalyticsWorker) Close() { w.consumer.Close() }

func (w *AnalyticsWorker) Run(ctx context.Context) {
	log.Println("analytics worker: listening on", TopicPlayEvents)
	w.consumer.Consume(ctx, func(data []byte) error {
		var ev PlayEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return fmt.Errorf("unmarshal play event: %w", err)
		}
		return w.record(ctx, ev)
	})
}

func (w *AnalyticsWorker) record(ctx context.Context, ev PlayEvent) error {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO play_events (track_id, source) VALUES ($1, $2)`,
		ev.TrackID, ev.Source,
	); err != nil {
		return fmt.Errorf("insert play_event: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE tracks SET play_count = play_count + 1 WHERE id = $1`,
		ev.TrackID,
	); err != nil {
		return fmt.Errorf("update play_count: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	log.Printf("analytics worker: recorded play for track %s", ev.TrackID)
	return nil
}
