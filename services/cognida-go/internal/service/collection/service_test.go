package collection

import (
	storage "cognida/internal/infrastructure/objectstore"
	knowledge "cognida/internal/service/knowledge"
	"context"
	"errors"
	"fmt"
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
	r.run = *run
	r.task.Status, r.task.ErrorMessage = domain.TaskStatus(run.Status), run.ErrorMessage
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
	return &workflowRepo{task: domain.Task{ID: "task", TenantID: 1, UserID: 2, KnowledgeBaseID: "kb", Status: domain.TaskStatusRunning}, run: domain.Run{CandidateOnly: true, ID: "run", TaskID: "task", TenantID: 1, Status: domain.RunStatusRunning}, sources: []*domain.Source{{ID: "source", TaskID: "task", URL: "https://example.com"}}}
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
	svc := candidateTestService(t, repo, fetch, ingest, TextNormalizer{})
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
	svc := candidateTestService(t, repo, fetch, ingest, TextNormalizer{})
	require.ErrorIs(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}), repo.updateErr)
	require.Empty(t, repo.finished, "failed persistence must not acknowledge completion")
	// A redelivery with a committed accepted item skips re-fetch/ingestion.
	repo.updateErr = nil
	require.NoError(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}))
	require.Zero(t, calls, "candidate collection cannot call direct ingestion")
	require.Equal(t, domain.RunStatusCompleted, repo.finished)
}
func TestCollectionNewRunDoesNotReuseLegacyPublishedReceipt(t *testing.T) {
	repo := workflowFixture()
	repo.accepted = &domain.Item{KnowledgeID: "existing-doc"}
	fetch := fetchFunc(func(context.Context, domain.FetchRequest) (*domain.FetchResponse, error) {
		return &domain.FetchResponse{ContentType: "text/plain", Body: []byte("result")}, nil
	})
	ingest := ingestFunc(func(context.Context, domain.IngestRequest) (*domain.IngestResult, error) {
		t.Fatal("duplicate ingested")
		return nil, nil
	})
	svc := candidateTestService(t, repo, fetch, ingest, TextNormalizer{})
	require.NoError(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}))
	require.Equal(t, domain.ItemStatusCandidate, repo.items[0].Status)
	require.Empty(t, repo.items[0].KnowledgeID)
	require.Equal(t, domain.RunStatusCompleted, repo.finished)
	require.Empty(t, repo.run.ErrorMessage)
}

type validatingCollectionIngestor struct {
	ingestFunc
	err error
}

func (i validatingCollectionIngestor) ValidateTarget(context.Context, int64, string) error {
	return i.err
}

func TestCollectionValidatesTargetBeforeReusingDuplicate(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		repo := workflowFixture()
		repo.accepted = &domain.Item{KnowledgeID: "previous-doc"}
		ingestor := validatingCollectionIngestor{ingestFunc: func(context.Context, domain.IngestRequest) (*domain.IngestResult, error) {
			t.Fatal("duplicate must not be ingested again")
			return nil, nil
		}}
		if unavailable {
			ingestor.err = errors.New("target no longer active")
		}
		svc := candidateTestService(t, repo, fetchFunc(func(context.Context, domain.FetchRequest) (*domain.FetchResponse, error) {
			return &domain.FetchResponse{ContentType: "text/plain", Body: []byte("unchanged")}, nil
		}), ingestor, TextNormalizer{})
		require.NoError(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}))
		require.Equal(t, domain.RunStatusCompleted, repo.finished)
		require.Equal(t, domain.ItemStatusCandidate, repo.items[0].Status)
		require.Empty(t, repo.items[0].KnowledgeID)

	}
}

func TestCollectionFinishReflectsEverySourceOutcome(t *testing.T) {
	for _, tc := range []struct {
		name, outcome string
		sources       []string
		status        domain.RunStatus
		usable        int
	}{
		{"all accepted", "success", []string{"accepted", "accepted"}, domain.RunStatusCompleted, 2},
		{"all fetch failures", "failed", []string{"fetch_failed", "fetch_failed"}, domain.RunStatusFailed, 0},
		{"all rejected", "failed", []string{"rejected"}, domain.RunStatusFailed, 0},
		{"ingestor unavailable does not publish", "success", []string{"ingest_failed"}, domain.RunStatusCompleted, 1},
		{"no ingestion receipt required for candidate", "success", []string{"missing_receipt"}, domain.RunStatusCompleted, 1},
		{"partial", "partial", []string{"accepted", "fetch_failed", "rejected", "ingest_failed"}, domain.RunStatusCompleted, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := workflowFixture()
			repo.sources = nil
			byURL := map[string]string{}
			for i, outcome := range tc.sources {
				url := fmt.Sprintf("https://example.com/%d", i)
				byURL[url] = outcome
				repo.sources = append(repo.sources, &domain.Source{ID: fmt.Sprint(i), TaskID: "task", URL: url})
			}
			fetch := fetchFunc(func(_ context.Context, req domain.FetchRequest) (*domain.FetchResponse, error) {
				if byURL[req.URL] == "fetch_failed" {
					return nil, errors.New("fetch unavailable")
				}
				body := "unique source " + req.URL
				if byURL[req.URL] == "rejected" {
					body = "  "
				}
				return &domain.FetchResponse{CanonicalURL: req.URL, ContentType: "text/plain", Body: []byte(body)}, nil
			})
			ingest := ingestFunc(func(_ context.Context, req domain.IngestRequest) (*domain.IngestResult, error) {
				switch byURL[req.SourceURL] {
				case "ingest_failed":
					return nil, errors.New("index unavailable")
				case "missing_receipt":
					return &domain.IngestResult{}, nil
				default:
					return &domain.IngestResult{KnowledgeID: "doc-" + req.SourceURL}, nil
				}
			})
			svc := candidateTestService(t, repo, fetch, ingest, TextNormalizer{})
			require.NoError(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}))
			require.Equal(t, tc.status, repo.finished)
			summary := domain.SummarizeItems(len(repo.sources), repo.items)
			require.Equal(t, tc.outcome, summary.Outcome)
			require.Equal(t, tc.usable, summary.Usable)
			if tc.outcome == "success" {
				require.Empty(t, repo.run.ErrorMessage)
			} else {
				require.Contains(t, repo.run.ErrorMessage, "failed=")
				require.Equal(t, repo.run.ErrorMessage, repo.task.ErrorMessage)
			}
		})
	}
}

func TestCollectionResumedFailureSummaryIncludesCommittedItems(t *testing.T) {
	repo := workflowFixture()
	repo.sources = append(repo.sources, &domain.Source{ID: "other", TaskID: "task"})
	repo.items = []*domain.Item{
		{SourceID: "source", Status: domain.ItemStatusSkipped, KnowledgeID: "existing"},
		{SourceID: "other", Status: domain.ItemStatusFailed, ErrorReason: "previous failure"},
	}
	svc := candidateTestService(t, repo, nil, nil, TextNormalizer{})
	require.NoError(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}))
	require.Equal(t, domain.RunStatusCompleted, repo.finished)
	require.Contains(t, repo.run.ErrorMessage, "Partial collection")
	require.Contains(t, repo.run.ErrorMessage, "skipped=1")
	require.Equal(t, "existing", repo.items[0].KnowledgeID)
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

type submitFunc func(context.Context, *domain.Run) error

func (f submitFunc) Submit(ctx context.Context, run *domain.Run) error { return f(ctx, run) }
func TestCollectionQueueFailureKeepsPersistedRunRecoverable(t *testing.T) {
	repo := workflowFixture()
	svc := candidateTestService(t, repo, nil, nil, TextNormalizer{})
	want := errors.New("redis unavailable")
	calls := 0
	svc.SetRunSubmitter(submitFunc(func(_ context.Context, run *domain.Run) error {
		calls++
		require.Equal(t, "run", run.ID)
		if calls == 1 {
			return want
		}
		return nil
	}))
	require.ErrorIs(t, svc.Start(context.Background(), 1, "task"), want)
	require.Empty(t, repo.finished)
	require.NoError(t, svc.Start(context.Background(), 1, "task"))
	require.Equal(t, 2, calls)
}

func (r *workflowRepo) ListRunningRuns(context.Context, int) ([]*domain.Run, error) {
	return []*domain.Run{&r.run}, nil
}
func TestCollectionPeriodicRecoveryResubmitsPersistedRun(t *testing.T) {
	repo := workflowFixture()
	svc := candidateTestService(t, repo, nil, nil, TextNormalizer{})
	calls := 0
	want := errors.New("redis unavailable")
	svc.SetRunSubmitter(submitFunc(func(context.Context, *domain.Run) error {
		calls++
		if calls == 1 {
			return want
		}
		return nil
	}))
	require.ErrorIs(t, svc.RecoverPendingRuns(context.Background(), 100), want)
	require.NoError(t, svc.RecoverPendingRuns(context.Background(), 100))
	require.Equal(t, 2, calls)
	require.Empty(t, repo.finished)
}

func candidateTestService(t *testing.T, r domain.Repository, f domain.Fetcher, i domain.DocumentIngestor, n Normalizer) *Service {
	t.Helper()
	s := NewService(r, f, i, n)
	store, e := storage.NewLocal(t.TempDir())
	require.NoError(t, e)
	s.SetArtifactStore(store)
	return s
}
func (r *workflowRepo) FindLegacyReceipt(context.Context, int64, string, string) (string, error) {
	if r.accepted != nil {
		return r.accepted.KnowledgeID, nil
	}
	return "", nil
}
func TestCollectionLegacyRecoveryOnlyBindsExistingReceipt(t *testing.T) {
	repo := workflowFixture()
	repo.run.CandidateOnly = false
	repo.accepted = &domain.Item{KnowledgeID: "historical-doc"}
	svc := candidateTestService(t, repo, fetchFunc(func(context.Context, domain.FetchRequest) (*domain.FetchResponse, error) {
		return &domain.FetchResponse{ContentType: "text/plain", Body: []byte("legacy content")}, nil
	}), ingestFunc(func(context.Context, domain.IngestRequest) (*domain.IngestResult, error) {
		t.Fatal("legacy recovery must never write knowledge")
		return nil, nil
	}), TextNormalizer{})
	require.NoError(t, svc.ExecuteRun(context.Background(), RunJobPayload{RunID: "run", TaskID: "task", TenantID: 1}))
	require.Equal(t, "historical-doc", repo.items[0].KnowledgeID)
	require.Equal(t, domain.ItemStatusAccepted, repo.items[0].Status)
}
