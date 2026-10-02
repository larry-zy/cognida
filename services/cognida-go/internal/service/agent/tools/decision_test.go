package tools

import (
	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/service/agent/resultstore"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func decisionContext() context.Context {
	return agentctx.WithUserID(agentctx.WithSessionID(agentctx.WithTenantID(context.Background(), 7), "decision-session"), 3)
}
func comparisonFixture(t *testing.T, s resultstore.Store) (context.Context, VerifyResultsRequest) {
	t.Helper()
	ctx := decisionContext()
	owner := resultstore.OwnerKey(7, "decision-session")
	put := func(rows []map[string]any) string {
		id, e := s.Put(ctx, &resultstore.Result{Owner: owner, Columns: []string{"region", "revenue"}, Rows: rows}, time.Minute)
		require.NoError(t, e)
		return id
	}
	left := put([]map[string]any{{"region": "A", "revenue": 100.0}, {"region": "B", "revenue": 200.0}})
	right := put([]map[string]any{{"region": "B", "revenue": 200.1}, {"region": "A", "revenue": 100.0}})
	return ctx, VerifyResultsRequest{LeftResultID: left, RightResultID: right, KeyColumns: []string{"region"}, ValueColumns: []string{"revenue"}, AbsoluteTolerance: 0.2}
}
func TestVerifyResultsAgreementMismatchAndIsolation(t *testing.T) {
	s := resultstore.NewMemoryStore()
	ctx, req := comparisonFixture(t, s)
	out, e := verifyResults(ctx, s, &req)
	require.NoError(t, e)
	require.Equal(t, 2, out.ComparedCells)
	require.False(t, out.RequiresClarification)
	require.Nil(t, out.Decision.Confidence)
	req.AbsoluteTolerance = 0
	out, e = verifyResults(ctx, s, &req)
	require.NoError(t, e)
	require.True(t, out.RequiresClarification)
	require.Equal(t, 1, out.DifferenceCount)
	require.Equal(t, "numeric_mismatch", out.Differences[0].Reason)
	_, e = verifyResults(agentctx.WithSessionID(ctx, "other"), s, &req)
	require.ErrorIs(t, e, resultstore.ErrUnauthorized)
	_, e = verifyResults(agentctx.WithTenantID(ctx, 8), s, &req)
	require.ErrorIs(t, e, resultstore.ErrUnauthorized)
	req.RightResultID = req.LeftResultID
	_, e = verifyResults(ctx, s, &req)
	require.Error(t, e)
}
func TestVerifyResultsMissingNullAndDuplicate(t *testing.T) {
	s := resultstore.NewMemoryStore()
	ctx, req := comparisonFixture(t, s)
	replace := func(rows []map[string]any) {
		id, e := s.Put(ctx, &resultstore.Result{Owner: resultstore.OwnerKey(7, "decision-session"), Columns: []string{"region", "revenue"}, Rows: rows}, time.Minute)
		require.NoError(t, e)
		req.RightResultID = id
	}
	replace([]map[string]any{{"region": "A", "revenue": nil}, {"region": "C", "revenue": 10}})
	out, e := verifyResults(ctx, s, &req)
	require.NoError(t, e)
	require.Equal(t, 3, out.DifferenceCount)
	require.True(t, out.RequiresClarification)
	replace([]map[string]any{{"region": "A", "revenue": 100}, {"region": "A", "revenue": 100}})
	_, e = verifyResults(ctx, s, &req)
	require.ErrorContains(t, e, "duplicate comparison key")
	req.ValueColumns = []string{"unknown"}
	_, e = verifyResults(ctx, s, &req)
	require.ErrorContains(t, e, "comparison column")
}

type decisionAssets struct {
	ref l.DataAssetRef
	err error
}

func (a decisionAssets) Resolve(_ context.Context, tenant, user int64, id l.RefID) (l.DataAssetRef, json.RawMessage, error) {
	if tenant != 7 || user != 3 || id != a.ref.RefID {
		return l.DataAssetRef{}, nil, l.ErrForbidden
	}
	return a.ref, json.RawMessage(`{"method":"descriptive_statistics","record_count":2}`), a.err
}

type decisionRAG struct{ calls int }

func (r *decisionRAG) Query(ctx interface{}, req *RAGQueryRequest) (*RAGQueryResult, error) {
	r.calls++
	return &RAGQueryResult{KnowledgeBaseID: "kb-authorized", Chunks: []DocumentChunk{{Content: "Check the source totals before changing the budget.", DocumentID: 5, ChunkIndex: 2}}}, nil
}
func TestDecisionRealEvidenceAndClarification(t *testing.T) {
	s := resultstore.NewMemoryStore()
	ctx, verify := comparisonFixture(t, s)
	verify.AbsoluteTolerance = 0
	ref := l.DataAssetRef{RefID: l.RefID{AssetID: uuid.NewString(), Version: 1}, Kind: l.Insight}
	rag := &decisionRAG{}
	req := DecisionRequest{EvidenceRefIDs: []string{ref.RefID.String()}, Goal: "Review revenue", KnowledgeQuery: "budget SOP", Actions: []string{"Increase the budget"}, Verification: &verify}
	out, e := synthesizeDecision(ctx, decisionAssets{ref: ref}, s, rag, &req)
	require.NoError(t, e)
	require.Equal(t, "blocked_pending_clarification", out.ActionStatus)
	require.Equal(t, "requires_clarification", out.Decision.VerificationStatus)
	require.Len(t, out.Knowledge, 1)
	require.Equal(t, 1, rag.calls)
	require.Contains(t, out.Decision.EvidenceRefs, "kb:kb-authorized:document:5:chunk:2")
	_, e = synthesizeDecision(ctx, decisionAssets{ref: ref, err: l.ErrForbidden}, s, rag, &req)
	require.ErrorIs(t, e, l.ErrForbidden)
	ref.Kind = l.RawAsset
	_, e = synthesizeDecision(ctx, decisionAssets{ref: ref}, s, rag, &req)
	require.ErrorContains(t, e, "insight ref")
}

func TestComparisonPreservesDecimalPrecision(t *testing.T) {
	a, ok := comparisonNumber(json.Number("0.100000000000000001"))
	require.True(t, ok)
	b, ok := comparisonNumber(json.Number("0.1"))
	require.True(t, ok)
	require.NotZero(t, a.Cmp(b))
	_, ok = comparisonNumber(float64(9007199254740992))
	require.False(t, ok)
	_, ok = comparisonNumber(json.Number("0x1p999999999"))
	require.False(t, ok)
	_, ok = comparisonNumber(json.Number("1e999999999999"))
	require.False(t, ok)
}
