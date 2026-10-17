package adapters

import (
	"context"
	"reflect"
	"testing"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/knowledge"
	"cognida/internal/service/agent/tools"
)

// fakeChunkQuerier records the domain query and returns fixed indexed chunks.
type fakeChunkQuerier struct {
	lastQuery knowledge.ChunkIndexQuery
	calls     int
	docs      []*knowledge.IndexedChunk
	err       error
}

func (f *fakeChunkQuerier) QueryChunks(_ context.Context, query knowledge.ChunkIndexQuery) ([]*knowledge.IndexedChunk, error) {
	f.lastQuery = query
	f.calls++
	return f.docs, f.err
}

func newChunkAdapter(q knowledge.ChunkIndexReader, repo knowledge.KnowledgeBaseRepository) *ChunkFetchAdapter {
	return NewChunkFetchAdapter(q, repo)
}

func enabledKB(id string) *knowledge.KnowledgeBase {
	return &knowledge.KnowledgeBase{ID: id, Status: 1}
}

// chunkCtx 造带租户 + 手动模式勾选 KB 的 ctx（手动模式下范围=勾选∩已启用）。
func chunkCtx(tenant int64, selected ...string) context.Context {
	ctx := agentctx.WithTenantID(context.Background(), tenant)
	ctx = agentctx.WithKBScopeMode(ctx, agentctx.KBScopeModeManual)
	ctx = agentctx.WithAllowedKBIDs(ctx, selected)
	return ctx
}

// TestChunkFetch_ForcesTenantAndScope 验证：租户从 ctx 强制进过滤，kb_id 限定在授权范围内。
func TestChunkFetch_ForcesTenantAndScope(t *testing.T) {
	q := &fakeChunkQuerier{docs: []*knowledge.IndexedChunk{
		{ChunkID: "c1", KnowledgeID: "k1", KnowledgeBaseID: "kb1", ChunkIndex: 3, Content: "hello", IsEnabled: true, TokenCount: 12},
	}}
	repo := &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{enabledKB("kb1"), enabledKB("kb2")}}
	a := newChunkAdapter(q, repo)

	ctx := chunkCtx(7, "kb1", "kb2")
	res, err := a.FetchChunks(ctx, &tools.FetchChunksRequest{ChunkIDs: []string{"c1"}, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if q.lastQuery.TenantID != 7 || !reflect.DeepEqual(q.lastQuery.KBIDs, []string{"kb1", "kb2"}) || !reflect.DeepEqual(q.lastQuery.ChunkIDs, []string{"c1"}) {
		t.Fatalf("query must preserve authorized scope and exact selection: %+v", q.lastQuery)
	}
	if res.Count != 1 || res.Chunks[0].ChunkID != "c1" || res.Chunks[0].KBID != "kb1" || res.Chunks[0].ChunkIndex != 3 {
		t.Fatalf("unexpected mapping: %+v", res)
	}
}

// TestChunkFetch_DropsOutOfScopeKB 验证：调用方传入的越权 kb_id 被交集丢弃。
func TestChunkFetch_DropsOutOfScopeKB(t *testing.T) {
	q := &fakeChunkQuerier{}
	repo := &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{enabledKB("kb1")}}
	a := newChunkAdapter(q, repo)

	// 授权范围仅 kb1；调用方却要 kb1 + kb9(越权)。
	ctx := chunkCtx(7, "kb1")
	res, err := a.FetchChunks(ctx, &tools.FetchChunksRequest{KBIDs: []string{"kb1", "kb9"}, KnowledgeIDs: []string{"k1"}, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !reflect.DeepEqual(q.lastQuery.KBIDs, []string{"kb1"}) || !reflect.DeepEqual(q.lastQuery.KnowledgeIDs, []string{"k1"}) {
		t.Fatalf("query includes unauthorized knowledge base: %+v", q.lastQuery)
	}
	_ = res
}

// TestChunkFetch_FullyOutOfScopeReturnsEmpty 验证：全部 kb_id 越权 → 直接空结果，不触底层查询。
func TestChunkFetch_FullyOutOfScopeReturnsEmpty(t *testing.T) {
	q := &fakeChunkQuerier{}
	repo := &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{enabledKB("kb1")}}
	a := newChunkAdapter(q, repo)

	ctx := chunkCtx(7, "kb1")
	res, err := a.FetchChunks(ctx, &tools.FetchChunksRequest{KBIDs: []string{"kb9"}, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Count != 0 || len(res.Chunks) != 0 {
		t.Fatalf("expected empty, got %+v", res)
	}
	if q.calls != 0 {
		t.Fatalf("underlying query must not run for fully out-of-scope, got %+v", q.lastQuery)
	}
}

// TestChunkFetch_NoTenantReturnsEmpty 验证：缺租户 → 空结果，绝不落到 tenant=0 查询。
func TestChunkFetch_NoTenantReturnsEmpty(t *testing.T) {
	q := &fakeChunkQuerier{}
	repo := &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{enabledKB("kb1")}}
	a := newChunkAdapter(q, repo)

	ctx := agentctx.WithKBScopeMode(context.Background(), agentctx.KBScopeModeManual) // 无租户
	res, err := a.FetchChunks(ctx, &tools.FetchChunksRequest{ChunkIDs: []string{"c1"}, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Count != 0 || q.calls != 0 {
		t.Fatalf("expected empty without tenant and no underlying query, got %+v query=%+v", res, q.lastQuery)
	}
}

// TestChunkFetch_EnabledOnlyAndPaging 验证：enabled_only 追加 is_enabled 子句，limit/offset 透传。
func TestChunkFetch_EnabledOnlyAndPaging(t *testing.T) {
	q := &fakeChunkQuerier{}
	repo := &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{enabledKB("kb1")}}
	a := newChunkAdapter(q, repo)

	ctx := chunkCtx(7, "kb1")
	_, err := a.FetchChunks(ctx, &tools.FetchChunksRequest{
		KnowledgeIDs: []string{"k1"}, EnabledOnly: true, Limit: 50, Offset: 10,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !q.lastQuery.EnabledOnly || q.lastQuery.Limit != 50 || q.lastQuery.Offset != 10 {
		t.Fatalf("enabled and paging must thread through: %+v", q.lastQuery)
	}
}

// TestNewChunkFetchAdapter_TypedNilRetriever verifies a typed-nil port remains unwired.
// 使 FetchChunks 命中未接线分支而非在接口方法上 panic。
func TestNewChunkFetchAdapter_TypedNilRetriever(t *testing.T) {
	var reader *fakeChunkQuerier
	a := NewChunkFetchAdapter(reader, &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{enabledKB("kb1")}})
	if a.retriever != nil {
		t.Fatalf("typed-nil retriever must be normalized to nil interface")
	}
	_, err := a.FetchChunks(chunkCtx(7, "kb1"), &tools.FetchChunksRequest{ChunkIDs: []string{"c1"}, Limit: 20})
	if err == nil {
		t.Fatalf("expected '未接线' error when retriever is nil")
	}
}
