package collection

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	storage "cognida/internal/infrastructure/objectstore"
	domain "cognida/internal/model/collection"
	object "cognida/internal/model/objectstore"
	"github.com/stretchr/testify/require"
)

type failingArtifactStore struct {
	object.Store
	fail bool
}

func (s *failingArtifactStore) Put(ctx context.Context, request object.PutRequest) (object.ObjectInfo, error) {
	if s.fail && strings.HasSuffix(request.Key, "/normalized") {
		return object.ObjectInfo{}, errors.New("object store temporarily unavailable")
	}
	return s.Store.Put(ctx, request)
}
func TestCollectionArtifactsRetainSnapshotAcrossRetry(t *testing.T) {
	repo := workflowFixture()
	repo.task.Limits.MaxResponseBytes = 1024
	local, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	store := &failingArtifactStore{Store: local, fail: true}
	fetches := 0
	svc := NewService(repo, fetchFunc(func(context.Context, domain.FetchRequest) (*domain.FetchResponse, error) {
		fetches++
		return &domain.FetchResponse{CanonicalURL: "https://example.com/final", StatusCode: 200, ContentType: "text/html", Body: []byte("<p>Original content</p>")}, nil
	}), ingestFunc(func(_ context.Context, r domain.IngestRequest) (*domain.IngestResult, error) {
		require.Equal(t, "Original content", r.Content)
		return &domain.IngestResult{KnowledgeID: "doc"}, nil
	}), TextNormalizer{})
	svc.SetArtifactStore(store)
	payload := RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}
	require.Error(t, svc.ExecuteRun(context.Background(), payload))
	require.Empty(t, repo.finished)
	require.NotEmpty(t, repo.items[0].RawObjectKey)
	require.Equal(t, domain.ItemStatusPending, repo.items[0].Status)
	store.fail = false
	require.NoError(t, svc.ExecuteRun(context.Background(), payload))
	require.Equal(t, 1, fetches, "retry restores the exact fetched bytes instead of a changed page")
	item := repo.items[0]
	for key, want := range map[string]string{item.RawObjectKey: "<p>Original content</p>", item.NormalizedObjectKey: "Original content"} {
		reader, info, err := local.Get(context.Background(), key)
		require.NoError(t, err)
		content, err := io.ReadAll(reader)
		require.NoError(t, reader.Close())
		require.NoError(t, err)
		if key == item.RawObjectKey {
			var envelope rawSnapshot
			require.NoError(t, json.Unmarshal(content, &envelope))
			require.Equal(t, want, string(envelope.Body))
			require.Equal(t, "text/html", envelope.ContentType)
		} else {
			require.Equal(t, want, string(content))
		}
		require.Equal(t, int64(len(content)), info.Size)
	}
	require.Equal(t, domain.RunStatusCompleted, repo.finished)
}
func TestCollectionStoredArtifactFailureIsRetryable(t *testing.T) {
	repo := workflowFixture()
	repo.items = []*domain.Item{{ID: "item", SourceID: "source", Status: domain.ItemStatusPending, RawObjectKey: "missing"}}
	local, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	svc := NewService(repo, nil, nil, TextNormalizer{})
	svc.SetArtifactStore(local)
	require.Error(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}))
	require.Equal(t, domain.ItemStatusPending, repo.items[0].Status)
	require.Empty(t, repo.finished)
}

func TestCollectionRawWriteBeforeDatabaseFailureRestoresEnvelope(t *testing.T) {
	repo := workflowFixture()
	repo.task.Limits.MaxResponseBytes = 1024
	local, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	calls := 0
	svc := NewService(repo, fetchFunc(func(context.Context, domain.FetchRequest) (*domain.FetchResponse, error) {
		calls++
		return &domain.FetchResponse{CanonicalURL: "https://example.com/final", ContentType: "text/plain", StatusCode: 200, Body: []byte("Original snapshot")}, nil
	}), ingestFunc(func(_ context.Context, r domain.IngestRequest) (*domain.IngestResult, error) {
		require.Equal(t, "Original snapshot", r.Content)
		return &domain.IngestResult{KnowledgeID: "doc"}, nil
	}), TextNormalizer{})
	svc.SetArtifactStore(local)
	repo.updateErr = errors.New("database down")
	payload := RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}
	require.Error(t, svc.ExecuteRun(context.Background(), payload))
	// Emulate the database state before failed UpdateItem, followed by process restart.
	old := repo.items[0]
	repo.items = []*domain.Item{{ID: old.ID, TaskID: old.TaskID, RunID: old.RunID, SourceID: old.SourceID, TenantID: old.TenantID, SourceURL: old.SourceURL, Status: domain.ItemStatusPending}}
	repo.updateErr = nil
	require.NoError(t, svc.ExecuteRun(context.Background(), payload))
	require.Equal(t, 1, calls)
	require.Equal(t, "https://example.com/final", repo.items[0].CanonicalURL)
	require.NotEmpty(t, repo.items[0].RawObjectKey)
}
