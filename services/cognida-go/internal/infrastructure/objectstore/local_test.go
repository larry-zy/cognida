package objectstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "cognida/internal/model/objectstore"
)

func TestLocalStoreRoundTrip(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	put, err := store.Put(context.Background(), domain.PutRequest{
		Key:         "collection/tenant-1/raw/page.html",
		Body:        bytes.NewBufferString("hello object storage"),
		Size:        int64(len("hello object storage")),
		ContentType: "text/html",
	})
	if err != nil {
		t.Fatal(err)
	}
	if put.Size != int64(len("hello object storage")) || put.ETag == "" || put.ContentType != "text/html" {
		t.Fatalf("unexpected put info: %+v", put)
	}

	body, got, err := store.Get(context.Background(), put.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello object storage" || got.Size != put.Size {
		t.Fatalf("unexpected object: %q %+v", data, got)
	}
	if err := store.Delete(context.Background(), put.Key); err != nil {
		t.Fatal(err)
	}
	_, err = store.Stat(context.Background(), put.Key)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Stat error = %v, want ErrNotFound", err)
	}
}

func TestLocalStoreRejectsSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	store, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, _, err := store.Get(ctx, "escape/secret"); err == nil {
		t.Fatal("read escaped root")
	}
	if _, err := store.Stat(ctx, "escape/secret"); err == nil {
		t.Fatal("stat escaped root")
	}
	if err := store.Delete(ctx, "escape/secret"); err == nil {
		t.Fatal("delete escaped root")
	}
	if _, err := store.Put(ctx, domain.PutRequest{Key: "escape/new", Body: strings.NewReader("x"), Size: 1}); err == nil {
		t.Fatal("write escaped root")
	}
	if _, err := os.Stat(filepath.Join(outside, "secret")); err != nil {
		t.Fatal("outside file changed")
	}
}

func TestLocalStoreRejectsEscapingKeys(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", "../secret", "/absolute", "dir\\secret"} {
		_, err := store.Put(context.Background(), domain.PutRequest{Key: key, Body: bytes.NewBufferString("x"), Size: 1})
		if !errors.Is(err, domain.ErrInvalidKey) {
			t.Errorf("Put(%q) error = %v, want ErrInvalidKey", key, err)
		}
	}
}

func TestLocalStoreRejectsUnexpectedSize(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Put(context.Background(), domain.PutRequest{Key: "a.txt", Body: bytes.NewBufferString("x"), Size: 2})
	if err == nil {
		t.Fatal("Put unexpectedly succeeded")
	}
}
