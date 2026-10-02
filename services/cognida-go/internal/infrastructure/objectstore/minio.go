package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	domain "cognida/internal/model/objectstore"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOOptions describes one S3-compatible MinIO namespace. Credentials are
// intentionally supplied by configuration/environment, never persisted by the
// collection domain.
type MinIOOptions struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// MinIOStore is a MinIO/S3-compatible Store implementation. Bucket creation
// is lazy so constructing the application does not require a running MinIO
// service; this keeps local-disk development entirely dependency-free.
type MinIOStore struct {
	client      *minio.Client
	bucket      string
	ensureMu    sync.Mutex
	bucketReady bool
}

func NewMinIO(options MinIOOptions) (*MinIOStore, error) {
	if strings.TrimSpace(options.Endpoint) == "" {
		return nil, fmt.Errorf("MinIO endpoint is required")
	}
	if strings.TrimSpace(options.Bucket) == "" {
		return nil, fmt.Errorf("MinIO bucket is required")
	}
	if strings.TrimSpace(options.AccessKey) == "" || strings.TrimSpace(options.SecretKey) == "" {
		return nil, fmt.Errorf("MinIO access and secret keys are required")
	}
	client, err := minio.New(options.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(options.AccessKey, options.SecretKey, ""),
		Secure: options.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create MinIO client: %w", err)
	}
	return &MinIOStore{client: client, bucket: options.Bucket}, nil
}

func (s *MinIOStore) Put(ctx context.Context, request domain.PutRequest) (domain.ObjectInfo, error) {
	if err := domain.ValidateKey(request.Key); err != nil {
		return domain.ObjectInfo{}, fmt.Errorf("%w: %q", domain.ErrInvalidKey, request.Key)
	}
	if request.Body == nil {
		return domain.ObjectInfo{}, fmt.Errorf("object body is required")
	}
	if err := s.ensureBucket(ctx); err != nil {
		return domain.ObjectInfo{}, err
	}
	info, err := s.client.PutObject(ctx, s.bucket, request.Key, request.Body, request.Size, minio.PutObjectOptions{ContentType: request.ContentType})
	if err != nil {
		return domain.ObjectInfo{}, fmt.Errorf("put MinIO object: %w", err)
	}
	return domain.ObjectInfo{Key: request.Key, Size: info.Size, ETag: info.ETag, ContentType: request.ContentType, ModifiedAt: info.LastModified}, nil
}

func (s *MinIOStore) Get(ctx context.Context, key string) (io.ReadCloser, domain.ObjectInfo, error) {
	info, err := s.stat(ctx, key)
	if err != nil {
		return nil, domain.ObjectInfo{}, err
	}
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, domain.ObjectInfo{}, fmt.Errorf("get MinIO object: %w", err)
	}
	return object, info, nil
}

func (s *MinIOStore) Stat(ctx context.Context, key string) (domain.ObjectInfo, error) {
	return s.stat(ctx, key)
}

func (s *MinIOStore) stat(ctx context.Context, key string) (domain.ObjectInfo, error) {
	if err := domain.ValidateKey(key); err != nil {
		return domain.ObjectInfo{}, fmt.Errorf("%w: %q", domain.ErrInvalidKey, key)
	}
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		var response minio.ErrorResponse
		if minio.ToErrorResponse(err).Code == "NoSuchKey" || errors.As(err, &response) && response.Code == "NoSuchKey" {
			return domain.ObjectInfo{}, fmt.Errorf("%w: %s", domain.ErrNotFound, key)
		}
		return domain.ObjectInfo{}, fmt.Errorf("stat MinIO object: %w", err)
	}
	return domain.ObjectInfo{Key: key, Size: info.Size, ETag: info.ETag, ContentType: info.ContentType, ModifiedAt: info.LastModified}, nil
}

func (s *MinIOStore) Delete(ctx context.Context, key string) error {
	if err := domain.ValidateKey(key); err != nil {
		return fmt.Errorf("%w: %q", domain.ErrInvalidKey, key)
	}
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("delete MinIO object: %w", err)
	}
	return nil
}

func (s *MinIOStore) ensureBucket(ctx context.Context) error {
	s.ensureMu.Lock()
	defer s.ensureMu.Unlock()
	if s.bucketReady {
		return nil
	}
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check MinIO bucket: %w", err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			// Another worker may have created the same bucket after our check.
			code := minio.ToErrorResponse(err).Code
			if code != "BucketAlreadyOwnedByYou" {
				return fmt.Errorf("create MinIO bucket: %w", err)
			}
		}
	}
	s.bucketReady = true
	return nil
}

var _ domain.Store = (*MinIOStore)(nil)
