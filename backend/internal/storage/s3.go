package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Config holds connection options for S3 / MinIO / Cloudflare R2 / GCP Cloud Storage.
type S3Config struct {
	Endpoint        string
	Bucket          string
	Region          string
	AccessKey       string
	SecretKey       string
	UsePathStyle    bool
	UseSSL          bool
	CustomKeyPrefix string
}

// S3API abstracts the s3.Client operations for testing/mocking.
type S3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

// S3Storage implements the Storage interface for S3-compatible object storage.
type S3Storage struct {
	client S3API
	bucket string
	prefix string
}

// NewS3Storage creates an S3Storage instance connected to AWS S3, Cloudflare R2, MinIO, or GCP.
func NewS3Storage(ctx context.Context, cfg S3Config) (*S3Storage, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("s3 storage: bucket name is required")
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "auto" // Default for R2 / MinIO if not specified
	}

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			endpoint := strings.TrimSpace(cfg.Endpoint)
			if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
				scheme := "https://"
				if !cfg.UseSSL {
					scheme = "http://"
				}
				endpoint = scheme + endpoint
			}
			o.BaseEndpoint = aws.String(endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	})

	return &S3Storage{
		client: client,
		bucket: cfg.Bucket,
		prefix: strings.Trim(cfg.CustomKeyPrefix, "/"),
	}, nil
}

// NewS3StorageWithClient creates an S3Storage with an injected S3API client (useful for testing).
func NewS3StorageWithClient(client S3API, bucket, prefix string) *S3Storage {
	return &S3Storage{
		client: client,
		bucket: bucket,
		prefix: strings.Trim(prefix, "/"),
	}
}

func (s *S3Storage) resolveKey(rawKey string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(strings.TrimSpace(rawKey), "\\", "/"))
	if clean == "" || clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", ErrInvalidPath
	}
	if s.prefix != "" {
		return path.Join(s.prefix, clean), nil
	}
	return clean, nil
}

// Put uploads an object to the S3 bucket.
func (s *S3Storage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	resolvedKey, err := s.resolveKey(key)
	if err != nil {
		return err
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(resolvedKey),
		Body:   r,
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	_, err = s.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("s3 put object: %w", err)
	}
	return nil
}

// Get retrieves an object from the S3 bucket.
func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, StorageMetadata, error) {
	resolvedKey, err := s.resolveKey(key)
	if err != nil {
		return nil, StorageMetadata{}, err
	}

	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(resolvedKey),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, StorageMetadata{}, ErrNotFound
		}
		return nil, StorageMetadata{}, fmt.Errorf("s3 get object: %w", err)
	}

	meta := StorageMetadata{
		ContentType:   aws.ToString(output.ContentType),
		ContentLength: aws.ToInt64(output.ContentLength),
	}
	if output.LastModified != nil {
		meta.ModTime = *output.LastModified
	}

	return output.Body, meta, nil
}

// Delete removes an object from the S3 bucket.
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	resolvedKey, err := s.resolveKey(key)
	if err != nil {
		return err
	}

	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(resolvedKey),
	})
	if err != nil && !isS3NotFound(err) {
		return fmt.Errorf("s3 delete object: %w", err)
	}
	return nil
}

// Exists checks if an object exists in the S3 bucket using HeadObject.
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	resolvedKey, err := s.resolveKey(key)
	if err != nil {
		return false, err
	}

	_, err = s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(resolvedKey),
	})
	if err != nil {
		if isS3NotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("s3 head object: %w", err)
	}
	return true, nil
}

func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	var nsk *s3types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var notFound *s3types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		if code == "NoSuchKey" || code == "NotFound" || code == "404" {
			return true
		}
	}
	return false
}
