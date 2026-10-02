package objectstore

import (
	"testing"

	"cognida/internal/config"
)

func TestNewFromConfigLocal(t *testing.T) {
	store, err := NewFromConfig(&config.ObjectStorageConfig{Backend: "local", LocalDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.(*LocalStore); !ok {
		t.Fatalf("store = %T, want *LocalStore", store)
	}
}

func TestNewFromConfigMinIORequiresCredentials(t *testing.T) {
	_, err := NewFromConfig(&config.ObjectStorageConfig{Backend: "minio", Endpoint: "localhost:9000", Bucket: "assets"})
	if err == nil {
		t.Fatal("NewFromConfig unexpectedly succeeded")
	}
}

func TestNewFromConfigRejectsUnknownBackend(t *testing.T) {
	_, err := NewFromConfig(&config.ObjectStorageConfig{Backend: "unknown"})
	if err == nil {
		t.Fatal("NewFromConfig unexpectedly succeeded")
	}
}
