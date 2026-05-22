package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
}

type MinIO struct {
	client *minio.Client
	bucket string
}

func NewMinIO(cfg Config) (*MinIO, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &MinIO{client: client, bucket: cfg.Bucket}, nil
}

func (m *MinIO) EnsureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{})
	}
	return nil
}

// Upload stores an audio file and returns the object key.
func (m *MinIO) Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := m.client.PutObject(ctx, m.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

// PresignedURL returns a time-limited URL for direct client download.
func (m *MinIO) PresignedURL(ctx context.Context, key string, ttl time.Duration) (*url.URL, error) {
	return m.client.PresignedGetObject(ctx, m.bucket, key, ttl, nil)
}
