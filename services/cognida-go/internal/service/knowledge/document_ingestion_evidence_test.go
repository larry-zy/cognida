package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"cognida/internal/infrastructure/id"
	"cognida/internal/model/docreader"
	d "cognida/internal/model/knowledge"
	"github.com/stretchr/testify/require"
)

type evidenceRepo struct {
	*cancelledPipelineRepo
	completion d.IngestionCompletion
	err        error
}

func (r *evidenceRepo) FinalizeIngestion(_ context.Context, in d.IngestionCompletion) error {
	r.completion = in
	return r.err
}

type evidenceChunks struct {
	d.ChunkRepository
	chunks []*d.Chunk
}

func (r *evidenceChunks) CreateBatch(_ context.Context, chunks []*d.Chunk) error {
	r.chunks = chunks
	return nil
}

type evidenceReader struct{ captureDocumentReader }

func (evidenceReader) ChunkDocument(context.Context, *docreader.ChunkDocumentRequest) (*docreader.ChunkDocumentResponse, error) {
	return &docreader.ChunkDocumentResponse{Chunks: []*docreader.Chunk{{Text: "first overlap", Index: 0}, {Text: "overlap second", Index: 1}}}, nil
}

func TestProcessDocumentReturnsOnlyPersistedIngestionEvidence(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "atomic write fails"}[fail], func(t *testing.T) {
			kb := newTenantFakeKBRepo()
			kb.kbs["kb"] = &d.KnowledgeBase{ID: "kb", TenantID: 1, Status: 1}
			repo := &evidenceRepo{cancelledPipelineRepo: &cancelledPipelineRepo{}}
			if fail {
				repo.err = errors.New("evidence unavailable")
			}
			chunks := &evidenceChunks{}
			s := &documentProcessorService{kbRepo: kb, knowledgeRepo: repo, chunkRepo: chunks, grpcClient: &evidenceReader{}, idGenerator: id.NewIDGenerator()}
			input := []byte("first overlap second")
			out, err := s.ProcessDocument(context.Background(), 1, 2, &ProcessDocumentRequest{KnowledgeBaseID: "kb", Title: "evidence", Content: input})
			if fail {
				require.ErrorIs(t, err, repo.err)
				require.Nil(t, out)
				return
			}
			require.NoError(t, err)
			sum := sha256.Sum256(input)
			require.Equal(t, hex.EncodeToString(sum[:]), out.ContentDigest)
			require.Equal(t, d.IngestionChunkDigest(chunks.chunks), out.ChunkDigest)
			require.NotEqual(t, out.ContentDigest, out.ChunkDigest, "overlap and label newline serialization are not the raw input")
			require.Equal(t, out.ChunkDigest, repo.completion.ChunkDigest)
			require.Equal(t, repo.doc.FileHash, repo.completion.ContentDigest)
			require.EqualValues(t, 2, repo.completion.UserID)
		})
	}
}
