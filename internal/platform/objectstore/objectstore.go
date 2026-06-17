// Package objectstore wraps the S3-compatible, EU-region object storage
// (ADR-0003) behind a small surface. MinIO is the dev/CI implementation; the
// same code targets any S3-compatible provider via endpoint + path-style
// configuration. Phase 0 needs only a reachability ping (HeadBucket); put/get/
// presign land in Phase 3.
package objectstore

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pubkraal/paddock/internal/platform/config"
)

// api is the subset of the S3 client the store depends on. Defined here, at the
// consumer, so it can be faked in tests.
type api interface {
	HeadBucket(ctx context.Context, in *s3.HeadBucketInput, opts ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
}

// Store is the object-storage handle bound to a single bucket.
type Store struct {
	client api
	bucket string
}

// NewWith wraps an arbitrary S3 API (used in tests).
func NewWith(client api, bucket string) *Store {
	return &Store{client: client, bucket: bucket}
}

// Open builds a Store from config. Construction performs no network I/O; call
// Ping to verify the bucket is reachable.
func Open(cfg config.ObjectStore) *Store {
	awsCfg := aws.Config{
		Region:      cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}

		o.UsePathStyle = cfg.UsePathStyle
	})

	return NewWith(client, cfg.Bucket)
}

// Bucket returns the configured bucket name.
func (s *Store) Bucket() string {
	return s.bucket
}

// Ping verifies the configured bucket is reachable.
func (s *Store) Ping(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})

	return err
}
