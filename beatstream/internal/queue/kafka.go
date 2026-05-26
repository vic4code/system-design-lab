package queue

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/twmb/franz-go/pkg/kgo"
	franzsasl "github.com/twmb/franz-go/pkg/sasl/aws"
)

// Publisher is implemented by *Producer and allows handler injection without a concrete type.
type Publisher interface {
	Publish(ctx context.Context, topic, key string, payload any) error
}

// Producer wraps a franz-go client for synchronous event publishing.
type Producer struct {
	client *kgo.Client
}

// NewProducer connects with plaintext auth (local Redpanda / no-auth brokers).
func NewProducer(brokerList string) (*Producer, error) {
	return newProducer(brokerList, nil)
}

// NewProducerIAM connects with AWS MSK IAM SASL authentication.
// Use this on ECS where the task role provides credentials.
func NewProducerIAM(brokerList, region string) (*Producer, error) {
	mechanism, err := mskIAMMechanism(context.Background(), region)
	if err != nil {
		return nil, err
	}
	return newProducer(brokerList, mechanism)
}

func newProducer(brokerList string, saslMechanism kgo.Opt) (*Producer, error) {
	brokers := strings.Split(brokerList, ",")
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.LeaderAck()),
		kgo.DisableIdempotentWrite(),
		kgo.AllowAutoTopicCreation(),
	}
	if saslMechanism != nil {
		opts = append(opts, saslMechanism, kgo.DialTLSConfig(&tls.Config{}))
	}
	client, err := kgo.NewClient(opts...)
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

// NewConsumer connects with plaintext auth (local Redpanda / no-auth brokers).
func NewConsumer(brokerList, group, topic string) (*Consumer, error) {
	return newConsumer(brokerList, group, topic, nil)
}

// NewConsumerIAM connects with AWS MSK IAM SASL authentication.
func NewConsumerIAM(brokerList, region, group, topic string) (*Consumer, error) {
	mechanism, err := mskIAMMechanism(context.Background(), region)
	if err != nil {
		return nil, err
	}
	return newConsumer(brokerList, group, topic, mechanism)
}

func newConsumer(brokerList, group, topic string, saslMechanism kgo.Opt) (*Consumer, error) {
	brokers := strings.Split(brokerList, ",")
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topic),
		kgo.AutoCommitMarks(),
	}
	if saslMechanism != nil {
		opts = append(opts, saslMechanism, kgo.DialTLSConfig(&tls.Config{}))
	}
	client, err := kgo.NewClient(opts...)
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

// mskIAMMechanism returns a franz-go SASL option using AWS MSK IAM auth.
// The ECS task role provides credentials automatically via the instance metadata service.
func mskIAMMechanism(ctx context.Context, region string) (kgo.Opt, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("aws config for MSK IAM: %w", err)
	}
	mechanism := franzsasl.ManagedStreamingIAM(func(ctx context.Context) (franzsasl.Auth, error) {
		creds, err := awsCfg.Credentials.Retrieve(ctx)
		if err != nil {
			return franzsasl.Auth{}, fmt.Errorf("retrieve aws credentials: %w", err)
		}
		return franzsasl.Auth{
			AccessKey:    creds.AccessKeyID,
			SecretKey:    creds.SecretAccessKey,
			SessionToken: creds.SessionToken,
			UserAgent:    "beatstream/1.0",
		}, nil
	})
	return kgo.SASL(mechanism), nil
}
