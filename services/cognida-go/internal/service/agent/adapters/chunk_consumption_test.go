package adapters

import (
	"cognida/internal/model/knowledge"
	"cognida/internal/model/rag"
	"cognida/internal/repository/milvus/retriever"
	"cognida/internal/service/agent/tools"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type chunkConsumptionGate struct{ err error }

func (chunkConsumptionGate) Prepare(context.Context, string, string) error { return nil }
func (g chunkConsumptionGate) Filter(context.Context, string, string, []*rag.Document) ([]*rag.Document, error) {
	return nil, g.err
}

func TestChunkFetchAppliesGuardEvenWithEnabledOnlyFalse(t *testing.T) {
	for _, err := range []error{nil, errors.New("publication unavailable")} {
		q := &fakeChunkQuerier{docs: []*retriever.DocumentData{{ChunkID: "old", KnowledgeID: "retired", KnowledgeBaseID: "kb", Content: "private"}}}
		a := newChunkAdapter(q, &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{enabledKB("kb")}}).WithConsumptionGuard(chunkConsumptionGate{err})
		result, e := a.FetchChunks(chunkCtx(7, "kb"), &tools.FetchChunksRequest{KnowledgeIDs: []string{"retired"}, Limit: 20, EnabledOnly: false})
		if err != nil {
			require.Error(t, e)
			require.Nil(t, result)
		} else {
			require.NoError(t, e)
			require.Empty(t, result.Chunks)
		}
	}
}
