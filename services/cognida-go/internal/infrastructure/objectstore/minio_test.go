package objectstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMinIOBucketInitializationCanRecover(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := minio.New(strings.TrimPrefix(server.URL, "http://"), &minio.Options{
		Creds: credentials.NewStaticV4("test", "test-secret", ""), Region: "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &MinIOStore{client: client, bucket: "test-bucket"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.ensureBucket(ctx); err == nil {
		t.Fatal("canceled initialization succeeded")
	}
	if err := store.ensureBucket(context.Background()); err != nil {
		t.Fatalf("transient failure poisoned store: %v", err)
	}
	if err := store.ensureBucket(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("expected successful initialization cached, got %d requests", requests)
	}
}
