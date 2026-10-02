// Package objectstore defines the application boundary for binary object
// persistence. It intentionally has no dependency on collection, HTTP, or a
// concrete storage vendor so collection artefacts can move between local disk
// and S3-compatible storage without changing domain services.
package objectstore

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"time"
)

var (
	ErrInvalidKey = errors.New("invalid object key")
	ErrNotFound   = errors.New("object not found")
)

// ObjectInfo is portable metadata returned by every backend.
type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
	ETag        string
	ModifiedAt  time.Time
}

// PutRequest contains an object payload. Callers retain ownership of Body and
// may close it after Put returns.
type PutRequest struct {
	Key         string
	Body        io.Reader
	Size        int64 // -1 means unknown
	ContentType string
}

// Store is the narrow port for durable binary content. Object keys are
// logical, slash-separated identifiers; implementations must reject paths
// that escape their configured namespace.
type Store interface {
	Put(ctx context.Context, request PutRequest) (ObjectInfo, error)
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	Delete(ctx context.Context, key string) error
}

// ValidateKey enforces logical object keys that are safe in both filesystem
// and bucket namespaces. Keys always use forward slashes, even on Windows.
func ValidateKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return ErrInvalidKey
	}
	clean := path.Clean(key)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ErrInvalidKey
	}
	return nil
}
