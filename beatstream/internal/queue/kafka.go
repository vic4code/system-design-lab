package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Publisher is implemented by *Producer and allows handler injection without a concrete type.
type Publisher interface {
	Publish(ctx context.Context, topic, key string, payload any) error
}

// Producer wraps a franz-go client for synchronous event publishing.
type Producer struct {
	client *kgo.Client
}

func NewProducer(brokerList string) (*Producer, error) {
	brokers := strings.Split(brokerList, ",")
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.LeaderAck()),
		kgo.DisableIdempotentWrite(),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka producer: %w", err)
	}
	return &Producer{client: client}, nil
}

func (p *Producer) Close() { p.client.Close() }

// Publish serialises payload as JSON and produces synchronously.
func (p *Producer) Publish(ctx context.Context, topic, key string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	results := p.client.ProduceSync(ctx, &kgo.Record{
		Topic: topic,
		Key:   []byte(key),
		Value: data,
	})
	return results.FirstErr()
}

// Consumer wraps a franz-go client for at-least-once event consumption.
// Records are only committed after the handler returns nil; errors leave the
// offset uncommitted so the record is redelivered after a restart.
type Consumer struct {
	client *kgo.Client
}

func NewConsumer(brokerList, group, topic string) (*Consumer, error) {
	brokers := strings.Split(brokerList, ",")
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.AutoCommitMarks(),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka consumer: %w", err)
	}
	return &Consumer{client: client}, nil
}

func (c *Consumer) Close() { c.client.Close() }

// Consume polls indefinitely until ctx is cancelled.
// Records are marked for commit only on successful handler execution.
func (c *Consumer) Consume(ctx context.Context, handler func([]byte) error) {
	for {
		fetches := c.client.PollFetches(ctx)
		if ctx.Err() != nil {
			return
		}
		fetches.EachError(func(t string, p int32, err error) {
			log.Printf("consumer fetch error topic=%s partition=%d: %v", t, p, err)
		})
		fetches.EachRecord(func(r *kgo.Record) {
			if err := handler(r.Value); err != nil {
				log.Printf("handler error topic=%s offset=%d: %v", r.Topic, r.Offset, err)
				// do not mark — record will be redelivered after restart (at-least-once)
				return
			}
			c.client.MarkCommitRecords(r)
		})
	}
}
