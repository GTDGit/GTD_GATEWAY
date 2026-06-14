// Package storage provides private object storage for the QRIS document portal.
//
// Files are ALWAYS private: the S3 bucket must block all public access, and
// objects are only ever read back by streaming through a token-validating
// handler (see the files-qris portal service). Nothing here ever produces a
// public URL. The interface lets the byte backend be swapped (S3 today, local
// disk or MinIO for dev) without touching callers.
package storage

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	appconfig "github.com/GTDGit/gtd_gateway/internal/config"
)

// Storage is the minimal object-storage contract used by the QRIS doc portal.
type Storage interface {
	// Put stores bytes under key with the given content type.
	Put(ctx context.Context, key, contentType string, data []byte) error
	// Get retrieves the bytes stored under key.
	Get(ctx context.Context, key string) ([]byte, string, error)
	// Delete removes the object at key (used by retention cleanup).
	Delete(ctx context.Context, key string) error
}

// S3Storage is an S3-backed Storage. The bucket is expected to live in the
// Jakarta region (ap-southeast-3) for Indonesian data residency and to have all
// public access blocked; objects are written with private ACL semantics
// (bucket-owner enforced — no ACL header is sent).
type S3Storage struct {
	client *s3.Client
	bucket string
}

// NewS3Storage builds an S3 client from StorageConfig. When AccessKey/SecretKey
// are provided they are used as static credentials; otherwise the default AWS
// credential chain (IAM role, env, shared config) applies. A custom Endpoint
// (e.g. MinIO) enables path-style addressing for local development.
func NewS3Storage(ctx context.Context, cfg appconfig.StorageConfig) (*S3Storage, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("storage: S3_BUCKET is required")
	}

	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(cfg.Region))
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("storage: load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		}
	})

	return &S3Storage{client: client, bucket: cfg.Bucket}, nil
}

// Put uploads data privately. No ACL is set, so the object inherits the
// bucket's (public-access-blocked) policy.
func (s *S3Storage) Put(ctx context.Context, key, contentType string, data []byte) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	return nil
}

// Get downloads the object bytes and its stored content type.
func (s *S3Storage) Get(ctx context.Context, key string) ([]byte, string, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", fmt.Errorf("storage: get %s: %w", key, err)
	}
	defer out.Body.Close()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(out.Body); err != nil {
		return nil, "", fmt.Errorf("storage: read %s: %w", key, err)
	}
	ct := ""
	if out.ContentType != nil {
		ct = *out.ContentType
	}
	return buf.Bytes(), ct, nil
}

// Delete removes the object at key.
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage: delete %s: %w", key, err)
	}
	return nil
}
