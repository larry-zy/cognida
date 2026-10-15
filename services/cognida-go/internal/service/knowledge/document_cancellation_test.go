package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	"cognida/internal/infrastructure/id"
	"cognida/internal/model/docreader"
	domain "cognida/internal/model/knowledge"
	"github.com/stretchr/testify/require"
)

type cancellationStatusRepo struct {
	domain.KnowledgeRepository
	ctx                 context.Context
	id, status, message string
	err                 error
}

type cancelledPipelineRepo struct {
	domain.KnowledgeRepository
	doc    *domain.Knowledge
	cancel context.CancelFunc
	at     string
}

func (r *cancelledPipelineRepo) Create(_ context.Context, doc *domain.Knowledge) error {
	r.doc = doc
	return nil
}
func (r *cancelledPipelineRepo) UpdateChunkCount(ctx context.Context, _ string, _ int) error {
	if r.at == "chunk_count" {
		r.cancel()
	}
	return ctx.Err()
}
func (r *cancelledPipelineRepo) UpdateParseStatus(ctx context.Context, _ string, state, message string) error {
	if r.at == "parse_status" {
		r.at = ""
		r.cancel()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.doc.ParseStatus, r.doc.ErrorMessage = state, message
	return nil
}

type cancelledPipelineChunks struct {
	domain.ChunkRepository
	cancel context.CancelFunc
	at     string
}

func (r cancelledPipelineChunks) CreateBatch(ctx context.Context, _ []*domain.Chunk) error {
	if r.at == "chunk_batch" {
		r.cancel()
	}
	return ctx.Err()
}

type cancelledPipelineReader struct{ captureDocumentReader }

func (cancelledPipelineReader) ChunkDocument(context.Context, *docreader.ChunkDocumentRequest) (*docreader.ChunkDocumentResponse, error) {
	return &docreader.ChunkDocumentResponse{Chunks: []*docreader.Chunk{{Text: "source", Index: 0}}}, nil
}

func TestProcessDocumentCancellationAtPersistenceBoundariesFailsReceipt(t *testing.T) {
	for _, at := range []string{"chunk_batch", "chunk_count", "parse_status"} {
		t.Run(at, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			kb := newTenantFakeKBRepo()
			kb.kbs["kb"] = &domain.KnowledgeBase{ID: "kb", TenantID: 1, Status: 1}
			repo := &cancelledPipelineRepo{cancel: cancel, at: at}
			svc := &documentProcessorService{kbRepo: kb, knowledgeRepo: repo, chunkRepo: cancelledPipelineChunks{cancel: cancel, at: at}, grpcClient: &cancelledPipelineReader{}, idGenerator: id.NewIDGenerator()}
			response, err := svc.ProcessDocument(ctx, 1, 2, &ProcessDocumentRequest{KnowledgeBaseID: "kb", Title: "cancel fixture", Content: []byte("source")})
			require.ErrorIs(t, err, context.Canceled)
			require.Nil(t, response)
			require.NotNil(t, repo.doc)
			require.Equal(t, domain.ParseStatusFailed, repo.doc.ParseStatus)
			require.Contains(t, repo.doc.ErrorMessage, "context canceled")
		})
	}
}

func (r *cancellationStatusRepo) UpdateParseStatus(ctx context.Context, id, status, message string) error {
	r.ctx, r.id, r.status, r.message = ctx, id, status, message
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.err
}
func TestCancelledDocumentPersistsBoundedFailedReceipt(t *testing.T) {
	type traceKey struct{}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		repo := &cancellationStatusRepo{}
		svc := &documentProcessorService{knowledgeRepo: repo}
		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), traceKey{}, "request-trace"))
		cancel()
		err := svc.recordCancelledProcessing(ctx, "owned-document", cause)
		require.ErrorIs(t, err, cause)
		require.Equal(t, "owned-document", repo.id)
		require.Equal(t, domain.ParseStatusFailed, repo.status)
		require.Contains(t, repo.message, cause.Error())
		require.Equal(t, "request-trace", repo.ctx.Value(traceKey{}))
		deadline, ok := repo.ctx.Deadline()
		require.True(t, ok)
		require.WithinDuration(t, time.Now().Add(5*time.Second), deadline, time.Second)
		repo.err = errors.New("storage unavailable")
		err = svc.recordCancelledProcessing(ctx, "owned-document", cause)
		require.ErrorIs(t, err, cause)
		require.ErrorIs(t, err, repo.err)
	}
}
