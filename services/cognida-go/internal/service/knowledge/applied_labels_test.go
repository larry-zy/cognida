package knowledge

import (
	"cognida/internal/model/rag"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type appliedLabelStub struct {
	fail   bool
	tenant int64
	labels []string
}

func (f *appliedLabelStub) FilterKnowledgeLabels(_ context.Context, tenant int64, _ []string, labels []string) (map[string]bool, error) {
	f.tenant = tenant
	f.labels = labels
	if f.fail {
		return nil, errors.New("database unavailable")
	}
	return map[string]bool{"approved": true}, nil
}
func TestAppliedKnowledgeLabelsFilterBeforeResultsFailClosed(t *testing.T) {
	ret := &capFakeRetriever{hybrid: []*rag.Document{{KnowledgeID: "approved", ChunkID: "a", Content: "proof", Score: 1}, {KnowledgeID: "unlabelled", ChunkID: "b", Content: "other", Score: 1}, {ChunkID: "c", Content: "unknown", Score: 1}}}
	filter := &appliedLabelStub{}
	cap := NewRetrievalCapability(ret, nil).WithAppliedLabelFilter(filter)
	q := GovernedQuery{Query: "proof", Mode: "auto", AppliedLabels: []string{"sales"}}
	out, e := cap.Retrieve(context.Background(), 123, []string{"kb"}, q)
	require.NoError(t, e)
	require.Len(t, out.Chunks, 1)
	require.Equal(t, "approved", out.Chunks[0].KnowledgeID)
	require.Equal(t, int64(123), filter.tenant)
	require.Equal(t, []string{"sales"}, filter.labels)
	filter.fail = true
	_, e = cap.Retrieve(context.Background(), 123, []string{"kb"}, q)
	require.Error(t, e)
	_, e = NewRetrievalCapability(ret, nil).Retrieve(context.Background(), 123, []string{"kb"}, q)
	require.Error(t, e)
}
