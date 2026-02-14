// Package s3blob provides an S3-compatible BlobStore for use with registry.NewS3Registry.
// Use: go get github.com/aws/aws-sdk-go-v2/config github.com/aws/aws-sdk-go-v2/service/s3
package s3blob

import (
	"bytes"
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klejdi94/loom/registry"
)

// Store implements registry.BlobStore using AWS S3 (or S3-compatible endpoints).
type Store struct {
	client *s3.Client
	bucket string
	prefix string
}

// New creates a BlobStore that uses the given S3 client, bucket, and key prefix.
func New(client *s3.Client, bucket, prefix string) *Store {
	return &Store{client: client, bucket: bucket, prefix: prefix}
}

// NewFromConfig creates a BlobStore using default AWS config (credentials, region from env).
func NewFromConfig(ctx context.Context, bucket, prefix string) (*Store, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return New(s3.NewFromConfig(cfg), bucket, prefix), nil
}

func (s *Store) fullKey(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + key
}

// Get implements registry.BlobStore.
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(key)),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

// Put implements registry.BlobStore.
func (s *Store) Put(ctx context.Context, key string, body []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(key)),
		Body:   bytes.NewReader(body),
	})
	return err
}

// List implements registry.BlobStore. Returns object keys (with prefix stripped if using prefix).
func (s *Store) List(ctx context.Context, prefix string) ([]string, error) {
	fullPrefix := s.fullKey(prefix)
	var keys []string
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(fullPrefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			k := *obj.Key
			if s.prefix != "" && len(k) >= len(s.prefix) {
				k = k[len(s.prefix):]
			}
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// Delete implements registry.BlobStore.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.fullKey(key)),
	})
	return err
}

// Ensure Store implements registry.BlobStore at compile time.
var _ registry.BlobStore = (*Store)(nil)
