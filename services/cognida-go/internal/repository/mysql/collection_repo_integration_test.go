//go:build integration

package mysql

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"

	"cognida/internal/model/collection"
)

func TestCollectionRepositoryTenantScopedTaskAndRunPersistence(t *testing.T) {
	db := newIntegrationDB(t)
	prefix := fmt.Sprintf("it_collection_%d", time.Now().UnixNano())
	t.Cleanup(func() { require.NoError(t, db.Where("id LIKE ?", prefix+"%").Delete(&CollectionTaskModel{}).Error) })
	repo := NewCollectionRepository(db)
	now := time.Now()
	taskID := prefix + "_task"
	task := &collection.Task{ID: taskID, TenantID: 7001, UserID: 1, KnowledgeBaseID: "kb", IdempotencyKey: prefix, Status: collection.TaskStatusDraft, Limits: collection.Limits{MaxSources: 1, MaxResponseBytes: 1024}, CreatedAt: now, UpdatedAt: now}
	source := &collection.Source{ID: prefix + "_source", TaskID: taskID, URL: "https://example.com", CreatedAt: now}
	if err := repo.CreateTask(context.Background(), task, []*collection.Source{source}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := repo.FindTask(context.Background(), 7002, taskID); !errors.Is(err, collection.ErrTaskNotFound) {
		t.Fatalf("cross-tenant task lookup = %v", err)
	}
	run := &collection.Run{ID: prefix + "_run", TaskID: taskID, TenantID: 7001, Status: collection.RunStatusRunning, StartedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := repo.FindActiveRun(context.Background(), 7002, taskID); !errors.Is(err, collection.ErrRunNotFound) {
		t.Fatalf("cross-tenant run lookup = %v", err)
	}
	item := &collection.Item{ID: prefix + "_item", TenantID: 7001, TaskID: taskID, RunID: run.ID, SourceID: source.ID, SourceURL: source.URL, Status: collection.ItemStatusPending, CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateItem(context.Background(), item); err != nil {
		t.Fatalf("create item: %v", err)
	}
	items, err := repo.ListItems(context.Background(), 7001, run.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("items = %d, %v", len(items), err)
	}
}

func TestCollectionRepositoryUpdatesAndAtomicLifecycle(t *testing.T) {
	db := newIntegrationDB(t)
	repo := NewCollectionRepository(db)
	ctx := context.Background()
	now := time.Now().UTC()
	prefix := fmt.Sprintf("it_collection_%d", time.Now().UnixNano())
	t.Cleanup(func() { require.NoError(t, db.Where("id = ?", prefix).Delete(&CollectionTaskModel{}).Error) })
	task := &collection.Task{ID: prefix, TenantID: 7001, UserID: 1, KnowledgeBaseID: "kb", IdempotencyKey: prefix, Status: collection.TaskStatusDraft, Limits: collection.Limits{MaxSources: 1, MaxResponseBytes: 1024}, CreatedAt: now, UpdatedAt: now}
	source := &collection.Source{ID: prefix + "_src", TaskID: prefix, URL: "https://example.com", CreatedAt: now}
	require.NoError(t, repo.CreateTask(ctx, task, []*collection.Source{source}))
	task.ErrorMessage = "draft note"
	require.NoError(t, repo.UpdateTask(ctx, task))
	stored, err := repo.FindTask(ctx, 7001, prefix)
	require.NoError(t, err)
	require.Equal(t, "draft note", stored.ErrorMessage)
	// Both callers read the draft state, but only one transaction creates a run.
	runs := []*collection.Run{
		{ID: prefix + "_r1", TaskID: prefix, TenantID: 7001, Status: collection.RunStatusRunning, StartedAt: now, CreatedAt: now, UpdatedAt: now},
		{ID: prefix + "_r2", TaskID: prefix, TenantID: 7001, Status: collection.RunStatusRunning, StartedAt: now, CreatedAt: now, UpdatedAt: now},
	}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range runs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); errs[i] = repo.CreateRun(ctx, runs[i]) }(i)
	}
	wg.Wait()
	var active *collection.Run
	for i, err := range errs {
		if err == nil {
			require.Nil(t, active)
			active = runs[i]
		} else {
			require.ErrorIs(t, err, collection.ErrActiveRunExists)
		}
	}
	require.NotNil(t, active)
	stored, err = repo.FindTask(ctx, 7001, prefix)
	require.NoError(t, err)
	require.Equal(t, collection.TaskStatusRunning, stored.Status)
	active.ErrorMessage = "progress"
	require.NoError(t, repo.UpdateRun(ctx, active))
	item := &collection.Item{ID: prefix + "_item", TenantID: 7001, TaskID: prefix, RunID: active.ID, SourceID: source.ID, SourceURL: source.URL, Status: collection.ItemStatusPending, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateItem(ctx, item))
	item.RawObjectKey = "collection/7001/fixture/raw"
	item.NormalizedObjectKey = "collection/7001/fixture/normalized"
	item.RawHash = "rawhash"
	item.RawSize = 22
	item.NormalizedSize = 16
	item.Status = collection.ItemStatusAccepted
	item.ContentHash = "hash"
	item.KnowledgeID = uuid.NewString()
	require.NoError(t, db.Exec("INSERT INTO knowledge (id, tenant_id, knowledge_base_id, user_id, type, title, source, parse_status) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", item.KnowledgeID, 7001, "kb", 1, "text", "fixture", "collection", "completed").Error)
	t.Cleanup(func() { require.NoError(t, db.Exec("DELETE FROM knowledge WHERE id = ?", item.KnowledgeID).Error) })
	require.NoError(t, repo.UpdateItem(ctx, item))
	found, err := repo.FindAcceptedContent(ctx, 7001, prefix, "hash")
	require.NoError(t, err)
	require.Equal(t, item.KnowledgeID, found.KnowledgeID)
	require.Equal(t, item.RawObjectKey, found.RawObjectKey)
	require.Equal(t, item.NormalizedObjectKey, found.NormalizedObjectKey)
	require.Equal(t, item.RawSize, found.RawSize)
	require.Equal(t, item.NormalizedSize, found.NormalizedSize)
	require.Equal(t, item.RawHash, found.RawHash)
	_, err = repo.FindAcceptedContent(ctx, 7002, prefix, "hash")
	require.ErrorIs(t, err, collection.ErrItemNotFound)
	require.NoError(t, db.Exec("UPDATE knowledge SET enable_status = 'disabled' WHERE id = ?", item.KnowledgeID).Error)
	_, err = repo.FindAcceptedContent(ctx, 7001, prefix, "hash")
	require.ErrorIs(t, err, collection.ErrItemNotFound)
	require.NoError(t, db.Exec("UPDATE knowledge SET enable_status = 'enabled' WHERE id = ?", item.KnowledgeID).Error)
	require.NoError(t, db.Exec("UPDATE knowledge SET parse_status = 'failed' WHERE id = ?", item.KnowledgeID).Error)
	_, err = repo.FindAcceptedContent(ctx, 7001, prefix, "hash")
	require.ErrorIs(t, err, collection.ErrItemNotFound)
	require.NoError(t, db.Exec("UPDATE knowledge SET parse_status = 'completed', deleted_at = ? WHERE id = ?", now, item.KnowledgeID).Error)
	_, err = repo.FindAcceptedContent(ctx, 7001, prefix, "hash")
	require.ErrorIs(t, err, collection.ErrItemNotFound)
	require.NoError(t, db.Exec("DELETE FROM knowledge WHERE id = ?", item.KnowledgeID).Error)
	_, err = repo.FindAcceptedContent(ctx, 7001, prefix, "hash")
	require.ErrorIs(t, err, collection.ErrItemNotFound)

	active.Status = collection.RunStatusCompleted
	active.CompletedAt = &now
	require.NoError(t, repo.FinishRun(ctx, active))
	stored, err = repo.FindTask(ctx, 7001, prefix)
	require.NoError(t, err)
	require.Equal(t, collection.TaskStatusCompleted, stored.Status)
	retry := &collection.Run{ID: prefix + "_retry", TaskID: prefix, TenantID: 7001, Status: collection.RunStatusRunning, StartedAt: now, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateRun(ctx, retry))
	stored, err = repo.FindTask(ctx, 7001, prefix)
	require.NoError(t, err)
	require.NoError(t, stored.Cancel())
	require.NoError(t, repo.UpdateTask(ctx, stored))
	retry.Status = collection.RunStatusCompleted
	retry.CompletedAt = &now
	require.NoError(t, repo.FinishRun(ctx, retry))
	latest, err := repo.FindLatestRun(ctx, 7001, prefix)
	require.NoError(t, err)
	// Timestamps can tie; read the exact cancelled run.
	var model CollectionRunModel
	require.NoError(t, db.Where("id = ?", retry.ID).First(&model).Error)
	require.Equal(t, string(collection.RunStatusCancelled), model.Status)
	_ = latest
}
