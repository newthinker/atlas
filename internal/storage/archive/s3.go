// internal/storage/archive/s3.go
package archive

import (
	"bytes"
	"context"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config holds S3 connection configuration
type S3Config struct {
	Bucket    string
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
	Prefix    string
}

// S3Storage implements Storage for S3-compatible backends
type S3Storage struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewS3 creates a new S3 storage client
func NewS3(cfg S3Config) (*S3Storage, error) {
	opts := s3.Options{
		Region:      cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
	}

	if cfg.Endpoint != "" {
		opts.BaseEndpoint = aws.String(cfg.Endpoint)
		opts.UsePathStyle = true // Required for MinIO and most S3-compatible services
	}

	client := s3.New(opts)

	return &S3Storage{
		client: client,
		bucket: cfg.Bucket,
		prefix: strings.TrimSuffix(cfg.Prefix, "/"),
	}, nil
}

func (s *S3Storage) key(path string) string {
	if s.prefix == "" {
		return path
	}
	return s.prefix + "/" + path
}

func (s *S3Storage) Write(ctx context.Context, path string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
		Body:   bytes.NewReader(data),
	})
	return err
}

func (s *S3Storage) Read(ctx context.Context, path string) ([]byte, error) {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
	})
	if err != nil {
		return nil, err
	}
	defer output.Body.Close()
	return io.ReadAll(output.Body)
}

func (s *S3Storage) List(ctx context.Context, prefix string) ([]string, error) {
	var paths []string
	fullPrefix := s.key(prefix)

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
			// Remove prefix to return relative paths
			relPath := strings.TrimPrefix(*obj.Key, s.prefix+"/")
			paths = append(paths, relPath)
		}
	}

	return paths, nil
}

func (s *S3Storage) Delete(ctx context.Context, path string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
	})
	return err
}

func (s *S3Storage) Exists(ctx context.Context, path string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.key(path)),
	})
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
