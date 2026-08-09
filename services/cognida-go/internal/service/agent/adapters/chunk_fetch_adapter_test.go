package adapters

import (
	"context"
	"strings"
	"testing"

	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/knowledge"
	"cognida/internal/repository/milvus/retriever"
	"cognida/internal/service/agent/tools"
)

// fakeChunkQuerier 记录收到的过滤表达式并回放固定文档，替代真实 Milvus 查询。
type fakeChunkQuerier struct {
	lastFilter string
	lastLimit  int64
	lastOffset int64
	docs       []*retriever.DocumentData
	err        error
}

func (f *fakeChunkQuerier) Query(_ context.Context, _ int64, opts *retriever.QueryOptions) ([]*retriever.DocumentData, error) {
	if opts != nil {
		if len(opts.Expr) > 0 {
			f.lastFilter = opts.Expr[0]
		}
		f.lastLimit = opts.Limit
		f.lastOffset = opts.Offset
	}
	return f.docs, f.err
}

// newChunkAdapter 构造受测适配器（注入 fake 底层查询器 + KB 仓储）。
func newChunkAdapter(q chunkQuerier, repo knowledge.KnowledgeBaseRepository) *ChunkFetchAdapter {
	return &ChunkFetchAdapter{retriever: q, kbRepo: repo}
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
	q := &fakeChunkQuerier{docs: []*retriever.DocumentData{
		{ChunkID: "c1", KnowledgeID: "k1", KnowledgeBaseID: "kb1", ChunkIndex: 3, Content: "hello", IsEnabled: true, TokenCount: 12},
	}}
	repo := &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{enabledKB("kb1"), enabledKB("kb2")}}
	a := newChunkAdapter(q, repo)

	ctx := chunkCtx(7, "kb1", "kb2")
	res, err := a.FetchChunks(ctx, &tools.FetchChunksRequest{ChunkIDs: []string{"c1"}, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(q.lastFilter, "tenant_id == 7") {
		t.Fatalf("filter must force tenant from ctx, got %q", q.lastFilter)
	}
	if !strings.Contains(q.lastFilter, `kb_id in ["kb1", "kb2"]`) {
		t.Fatalf("filter must scope kb_id to authorized set, got %q", q.lastFilter)
	}
	if !strings.Contains(q.lastFilter, `chunk_id in ["c1"]`) {
		t.Fatalf("filter must carry chunk_id predicate, got %q", q.lastFilter)
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
	if !strings.Contains(q.lastFilter, `kb_id in ["kb1"]`) {
		t.Fatalf("out-of-scope kb9 must be dropped, got %q", q.lastFilter)
	}
	if strings.Contains(q.lastFilter, "kb9") {
		t.Fatalf("kb9 must not leak into filter, got %q", q.lastFilter)
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
	if q.lastFilter != "" {
		t.Fatalf("underlying query must not run for fully out-of-scope, got %q", q.lastFilter)
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
	if res.Count != 0 || q.lastFilter != "" {
		t.Fatalf("expected empty without tenant and no underlying query, got %+v filter=%q", res, q.lastFilter)
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
	if !strings.Contains(q.lastFilter, "is_enabled == true") {
		t.Fatalf("enabled_only must add predicate, got %q", q.lastFilter)
	}
	if q.lastLimit != 50 || q.lastOffset != 10 {
		t.Fatalf("limit/offset must thread through, got limit=%d offset=%d", q.lastLimit, q.lastOffset)
	}
}

// TestBuildChunkFilter_QuotesAndEscapes 验证表达式字面量的引号与转义（防注入/防表达式破坏）。
func TestBuildChunkFilter_QuotesAndEscapes(t *testing.T) {
	got := buildChunkFilter(42, []string{`kb"x`, `kb\y`}, &tools.FetchChunksRequest{
		ChunkIDs: []string{"c1"},
	})
	if !strings.HasPrefix(got, "tenant_id == 42 and ") {
		t.Fatalf("tenant clause must lead, got %q", got)
	}
	if !strings.Contains(got, `kb_id in ["kb\"x", "kb\\y"]`) {
		t.Fatalf("kb ids must be quoted/escaped, got %q", got)
	}
}

// TestNewChunkFetchAdapter_TypedNilRetriever 验证 typed-nil *VectorRetriever 被规约为真正的 nil 接口，
// 使 FetchChunks 命中未接线分支而非在接口方法上 panic。
func TestNewChunkFetchAdapter_TypedNilRetriever(t *testing.T) {
	a := NewChunkFetchAdapter(nil, &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{enabledKB("kb1")}})
	if a.retriever != nil {
		t.Fatalf("typed-nil retriever must be normalized to nil interface")
	}
	_, err := a.FetchChunks(chunkCtx(7, "kb1"), &tools.FetchChunksRequest{ChunkIDs: []string{"c1"}, Limit: 20})
	if err == nil {
		t.Fatalf("expected '未接线' error when retriever is nil")
	}
}
