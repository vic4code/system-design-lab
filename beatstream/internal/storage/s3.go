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
//
// PresignEndpoint overrides the host in pre-signed URLs.
// Use this in local dev so browsers receive localhost:9000 URLs instead of minio:9000.
type Storage struct {
	uploadClient  *s3.Client
	presignClient *s3.PresignClient
	bucket        string
}

type Config struct {
	Bucket          string
	Region          string
	Endpoint        string // optional: internal endpoint (e.g. "http://minio:9000")
	AccessKey       string // optional: static credentials for local dev
	SecretKey       string
	PresignEndpoint string // optional: browser-accessible endpoint for pre-signed URLs
}

func New(ctx context.Context, cfg Config) (*Storage, error) {
	uploadClient, err := buildClient(ctx, cfg.Region, cfg.Endpoint, cfg.AccessKey, cfg.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("upload client: %w", err)
	}

	// If PresignEndpoint differs from Endpoint, create a separate client for presigning
	// so that generated URLs point to the browser-reachable address.
	presignSrc := uploadClient
	if cfg.PresignEndpoint != "" && cfg.PresignEndpoint != cfg.Endpoint {
		presignSrc, err = buildClient(ctx, cfg.Region, cfg.PresignEndpoint, cfg.AccessKey, cfg.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("presign client: %w", err)
		}
	}

	return &Storage{
		uploadClient:  uploadClient,
		presignClient: s3.NewPresignClient(presignSrc),
		bucket:        cfg.Bucket,
	}, nil
}

func buildClient(ctx context.Context, region, endpoint, accessKey, secretKey string) (*s3.Client, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}
	if accessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // MinIO requires path-style addressing
		})
	}

	return s3.NewFromConfig(awsCfg, clientOpts...), nil
}

// EnsureBucket creates the bucket if it does not exist.
// Called in local dev only — AWS buckets are provisioned by Terraform.
func (s *Storage) EnsureBucket(ctx context.Context) error {
	_, err := s.uploadClient.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}
	_, err = s.uploadClient.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)})
	return err
}

// Upload stores an audio file at key.
func (s *Storage) Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.uploadClient.PutObject(ctx, &s3.PutObjectInput{
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
	req, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return nil, err
	}
	return url.Parse(req.URL)
}
