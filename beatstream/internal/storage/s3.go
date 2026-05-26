package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Storage wraps an S3-compatible object store.
// Set Endpoint + AccessKey + SecretKey for local MinIO.
// Leave Endpoint empty on AWS — the ECS task role provides credentials automatically.
type Storage struct {
	client *s3.Client
	bucket string
}

type Config struct {
	Bucket    string
	Region    string
	Endpoint  string // optional: set for local MinIO (e.g. "http://minio:9000")
	AccessKey string // optional: static credentials for local dev
	SecretKey string
}

func New(ctx context.Context, cfg Config) (*Storage, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}
	if cfg.AccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true // MinIO requires path-style addressing
		})
	}

	client := s3.NewFromConfig(awsCfg, clientOpts...)
	return &Storage{client: client, bucket: cfg.Bucket}, nil
}

// EnsureBucket creates the bucket if it does not exist.
// Called in local dev only — AWS buckets are provisioned by Terraform.
func (s *Storage) EnsureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}
	_, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)})
	return err
}

// Upload stores an audio file at key.
func (s *Storage) Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	return err
}

// PresignedURL returns a time-limited URL for direct client download.
func (s *Storage) PresignedURL(ctx context.Context, key string, ttl time.Duration) (*url.URL, error) {
	presigner := s3.NewPresignClient(s.client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return nil, err
	}
	return url.Parse(req.URL)
}
