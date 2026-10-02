package collection

import (
	knowledge "cognida/internal/service/knowledge"
	"context"
	"errors"
	"strings"
	"testing"

	domain "cognida/internal/model/collection"
	"github.com/stretchr/testify/require"
)

type workflowRepo struct {
	domain.Repository
	task      domain.Task
	run       domain.Run
	sources   []*domain.Source
	items     []*domain.Item
	accepted  *domain.Item
	updateErr error
	finished  domain.RunStatus
}

func (r *workflowRepo) FindTask(context.Context, int64, string) (*domain.Task, error) {
	copy := r.task
	return &copy, nil
}
func (r *workflowRepo) FindActiveRun(context.Context, int64, string) (*domain.Run, error) {
	copy := r.run
	return &copy, nil
}
func (r *workflowRepo) ListSources(context.Context, int64, string) ([]*domain.Source, error) {
	return r.sources, nil
}
func (r *workflowRepo) ListItems(context.Context, int64, string) ([]*domain.Item, error) {
	return r.items, nil
}
func (r *workflowRepo) CreateItem(_ context.Context, item *domain.Item) error {
	r.items = append(r.items, item)
	return nil
}
func (r *workflowRepo) UpdateItem(context.Context, *domain.Item) error { return r.updateErr }
func (r *workflowRepo) FindAcceptedContent(context.Context, int64, string, string) (*domain.Item, error) {
	if r.accepted == nil {
		return nil, domain.ErrItemNotFound
	}
	return r.accepted, nil
}
func (r *workflowRepo) FinishRun(_ context.Context, run *domain.Run) error {
	r.finished = run.Status
	return nil
}

type fetchFunc func(context.Context, domain.FetchRequest) (*domain.FetchResponse, error)

func (f fetchFunc) Fetch(ctx context.Context, req domain.FetchRequest) (*domain.FetchResponse, error) {
	return f(ctx, req)
}

type ingestFunc func(context.Context, domain.IngestRequest) (*domain.IngestResult, error)

func (f ingestFunc) Ingest(ctx context.Context, req domain.IngestRequest) (*domain.IngestResult, error) {
	return f(ctx, req)
}
func workflowFixture() *workflowRepo {
	return &workflowRepo{task: domain.Task{ID: "task", TenantID: 1, UserID: 2, KnowledgeBaseID: "kb", Status: domain.TaskStatusRunning}, run: domain.Run{ID: "run", TaskID: "task", TenantID: 1, Status: domain.RunStatusRunning}, sources: []*domain.Source{{ID: "source", TaskID: "task", URL: "https://example.com"}}}
}
func TestCollectionCancelDuringFinalFetchDoesNotIngest(t *testing.T) {
	repo := workflowFixture()
	calls := 0
	fetch := fetchFunc(func(context.Context, domain.FetchRequest) (*domain.FetchResponse, error) {
		repo.task.Status = domain.TaskStatusCancelled
		return &domain.FetchResponse{ContentType: "text/plain", Body: []byte("result")}, nil
	})
	ingest := ingestFunc(func(context.Context, domain.IngestRequest) (*domain.IngestResult, error) {
		calls++
		return &domain.IngestResult{}, nil
	})
	svc := NewService(repo, fetch, ingest, TextNormalizer{})
	require.NoError(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}))
	require.Zero(t, calls)
	require.Equal(t, domain.RunStatusCancelled, repo.finished)
}
func TestCollectionRetriesPersistenceFailureAndResumesCommittedItems(t *testing.T) {
	repo := workflowFixture()
	repo.updateErr = errors.New("database unavailable")
	calls := 0
	fetch := fetchFunc(func(context.Context, domain.FetchRequest) (*domain.FetchResponse, error) {
		return &domain.FetchResponse{ContentType: "text/plain", Body: []byte("result")}, nil
	})
	ingest := ingestFunc(func(context.Context, domain.IngestRequest) (*domain.IngestResult, error) {
		calls++
		return &domain.IngestResult{KnowledgeID: "doc"}, nil
	})
	svc := NewService(repo, fetch, ingest, TextNormalizer{})
	require.ErrorIs(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}), repo.updateErr)
	require.Empty(t, repo.finished, "failed persistence must not acknowledge completion")
	// A redelivery with a committed accepted item skips re-fetch/ingestion.
	repo.updateErr = nil
	require.NoError(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}))
	require.Equal(t, 1, calls)
	require.Equal(t, domain.RunStatusCompleted, repo.finished)
}
func TestCollectionDeduplicatesPreviouslyAcceptedContent(t *testing.T) {
	repo := workflowFixture()
	repo.accepted = &domain.Item{KnowledgeID: "existing-doc"}
	fetch := fetchFunc(func(context.Context, domain.FetchRequest) (*domain.FetchResponse, error) {
		return &domain.FetchResponse{ContentType: "text/plain", Body: []byte("result")}, nil
	})
	ingest := ingestFunc(func(context.Context, domain.IngestRequest) (*domain.IngestResult, error) {
		t.Fatal("duplicate ingested")
		return nil, nil
	})
	svc := NewService(repo, fetch, ingest, TextNormalizer{})
	require.NoError(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}))
	require.Equal(t, domain.ItemStatusSkipped, repo.items[0].Status)
	require.Equal(t, "existing-doc", repo.items[0].KnowledgeID)
}
func TestKnowledgeIngestorRejectsShortHash(t *testing.T) {
	_, err := NewKnowledgeIngestorAdapter(nil).Ingest(context.Background(), domain.IngestRequest{ContentHash: "short"})
	require.Error(t, err)
}

type captureDocumentProcessor struct {
	knowledge.DocumentProcessorService
	request *knowledge.ProcessDocumentRequest
	status  string
}

func (p *captureDocumentProcessor) ProcessDocument(_ context.Context, _ int64, _ int64, req *knowledge.ProcessDocumentRequest) (*knowledge.ProcessDocumentResponse, error) {
	p.request = req
	status := p.status
	if status == "" {
		status = "completed"
	}
	return &knowledge.ProcessDocumentResponse{KnowledgeID: "stable-doc", ParseStatus: status}, nil
}
func TestCollectionIngestionPassesStableContentKey(t *testing.T) {
	processor := &captureDocumentProcessor{}
	adapter := NewKnowledgeIngestorAdapter(processor)
	hash := strings.Repeat("a", 64)
	result, err := adapter.Ingest(context.Background(), domain.IngestRequest{TenantID: 1, UserID: 2, KnowledgeBaseID: "kb", CanonicalURL: "https://example.com/docs", ContentHash: hash, Content: "normalized content"})
	require.NoError(t, err)
	require.Equal(t, hash, processor.request.IdempotencyKey)
	require.Equal(t, []byte("normalized content"), processor.request.Content)
	require.Equal(t, "stable-doc", result.KnowledgeID)
}

func TestCollectionIngestionRejectsFailedVectorization(t *testing.T) {
	adapter := NewKnowledgeIngestorAdapter(&captureDocumentProcessor{status: "failed"})
	result, err := adapter.Ingest(context.Background(), domain.IngestRequest{ContentHash: strings.Repeat("a", 64), CanonicalURL: "https://example.com/", Content: "data"})
	require.Error(t, err)
	require.Nil(t, result)
}
