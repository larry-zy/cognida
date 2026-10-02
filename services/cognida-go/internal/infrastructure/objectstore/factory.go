package objectstore

import (
	"fmt"
	"strings"

	"cognida/internal/config"
	domain "cognida/internal/model/objectstore"
)

// NewFromConfig selects an object store without exposing vendor selection to
// collection services. A local backend remains the safe default.
func NewFromConfig(cfg *config.ObjectStorageConfig) (domain.Store, error) {
	if cfg == nil {
		return nil, fmt.Errorf("object storage config is required")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case "", "local":
		return NewLocal(cfg.LocalDir)
	case "minio":
		return NewMinIO(MinIOOptions{
			Endpoint: cfg.Endpoint, Bucket: cfg.Bucket,
			AccessKey: cfg.AccessKey, SecretKey: cfg.SecretKey, UseSSL: cfg.UseSSL,
		})
	default:
		return nil, fmt.Errorf("unsupported object storage backend %q", cfg.Backend)
	}
}
