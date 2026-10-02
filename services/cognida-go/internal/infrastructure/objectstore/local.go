// Package objectstore contains concrete adapters for the objectstore port.
package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	domain "cognida/internal/model/objectstore"
	"github.com/google/uuid"
)

// LocalStore persists objects under one configured root. The write path is
// atomic: callers never observe a partially-written final object.
type LocalStore struct {
	root string
}

func NewLocal(root string) (*LocalStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("local object storage root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local object storage root: %w", err)
	}
	if err := os.MkdirAll(absRoot, 0o750); err != nil {
		return nil, fmt.Errorf("create local object storage root: %w", err)
	}
	return &LocalStore{root: absRoot}, nil
}

func (s *LocalStore) Put(ctx context.Context, request domain.PutRequest) (domain.ObjectInfo, error) {
	path, err := s.pathFor(request.Key)
	if err != nil {
		return domain.ObjectInfo{}, err
	}
	if request.Body == nil {
		return domain.ObjectInfo{}, fmt.Errorf("object body is required")
	}
	if err := ctx.Err(); err != nil {
		return domain.ObjectInfo{}, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return domain.ObjectInfo{}, err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return domain.ObjectInfo{}, fmt.Errorf("create object directory: %w", err)
	}

	tempPath := filepath.Join(filepath.Dir(path), ".object-"+uuid.NewString())
	temp, err := root.OpenFile(tempPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return domain.ObjectInfo{}, fmt.Errorf("create temporary object: %w", err)
	}
	defer root.Remove(tempPath)

	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temp, hash), &contextReader{ctx: ctx, reader: request.Body})
	if closeErr := temp.Close(); copyErr == nil && closeErr != nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return domain.ObjectInfo{}, fmt.Errorf("write object: %w", copyErr)
	}
	if request.Size >= 0 && request.Size != written {
		return domain.ObjectInfo{}, fmt.Errorf("object size mismatch: got %d, expected %d", written, request.Size)
	}
	if err := root.Rename(tempPath, path); err != nil {
		return domain.ObjectInfo{}, fmt.Errorf("commit object: %w", err)
	}
	info, err := root.Stat(path)
	if err != nil {
		return domain.ObjectInfo{}, fmt.Errorf("stat committed object: %w", err)
	}
	return domain.ObjectInfo{Key: request.Key, Size: written, ContentType: request.ContentType, ETag: hex.EncodeToString(hash.Sum(nil)), ModifiedAt: info.ModTime()}, nil
}

func (s *LocalStore) Get(ctx context.Context, key string) (io.ReadCloser, domain.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, domain.ObjectInfo{}, err
	}
	path, err := s.pathFor(key)
	if err != nil {
		return nil, domain.ObjectInfo{}, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, domain.ObjectInfo{}, err
	}
	defer root.Close()
	file, err := root.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, domain.ObjectInfo{}, fmt.Errorf("%w: %s", domain.ErrNotFound, key)
		}
		return nil, domain.ObjectInfo{}, fmt.Errorf("open object: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, domain.ObjectInfo{}, fmt.Errorf("stat object: %w", err)
	}
	return file, domain.ObjectInfo{Key: key, Size: info.Size(), ModifiedAt: info.ModTime()}, nil
}

func (s *LocalStore) Stat(ctx context.Context, key string) (domain.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return domain.ObjectInfo{}, err
	}
	path, err := s.pathFor(key)
	if err != nil {
		return domain.ObjectInfo{}, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return domain.ObjectInfo{}, err
	}
	defer root.Close()
	info, err := root.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.ObjectInfo{}, fmt.Errorf("%w: %s", domain.ErrNotFound, key)
		}
		return domain.ObjectInfo{}, fmt.Errorf("stat object: %w", err)
	}
	return domain.ObjectInfo{Key: key, Size: info.Size(), ModifiedAt: info.ModTime()}, nil
}

func (s *LocalStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.pathFor(key)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", domain.ErrNotFound, key)
		}
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func (s *LocalStore) pathFor(key string) (string, error) {
	if err := domain.ValidateKey(key); err != nil {
		return "", fmt.Errorf("%w: %q", domain.ErrInvalidKey, key)
	}
	clean := filepath.Clean(key)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", domain.ErrInvalidKey, key)
	}
	path := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", domain.ErrInvalidKey, key)
	}
	return rel, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

var _ domain.Store = (*LocalStore)(nil)
