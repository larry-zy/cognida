//go:build integration

package mysql

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDocumentLockSerializesIndependentConnectionsAndReleasesOnFailure(t *testing.T) {
	db := newIntegrationDB(t)
	repo := &knowledgeRepository{db: db}
	key := "review-" + uuid.NewString()
	var active atomic.Int32
	entered := make(chan struct{})
	done := make(chan error, 2)
	go func() {
		done <- repo.WithDocumentLock(context.Background(), key, func() error {
			active.Add(1)
			defer active.Add(-1)
			close(entered)
			time.Sleep(80 * time.Millisecond)
			return errors.New("simulated ingestion failure")
		})
	}()
	<-entered
	go func() {
		done <- repo.WithDocumentLock(context.Background(), key, func() error {
			if active.Add(1) != 1 {
				t.Error("same ingestion executing concurrently")
			}
			defer active.Add(-1)
			return nil
		})
	}()
	failures := 0
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err != nil {
				failures++
			}
		case <-time.After(3 * time.Second):
			t.Fatal("lock was not released after failure")
		}
	}
	if failures != 1 {
		t.Fatalf("expected one simulated failure, got %d", failures)
	}
}
