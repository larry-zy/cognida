package adapters

import (
	"context"
	"testing"

	agentctx "link/internal/model/agent"
	"link/internal/model/knowledge"
	"link/internal/model/rag"
	"link/internal/service/agent/tools"
)

// ========================================
// mock rag.Retriever
// ========================================

// recordingRetriever 记录每次检索命中的 kbID，并对每个 kb 返回一条固定分数的文档。
type recordingRetriever struct {
	calledKBs []string
	scores    map[string]float32 // kbID -> score
}

func (r *recordingRetriever) respond(_ context.Context, kbID string) (*rag.RetrieveResponse, error) {
	r.calledKBs = append(r.calledKBs, kbID)
	score := r.scores[kbID]
	return &rag.RetrieveResponse{
		Results: []*rag.Document{
			{Content: "chunk-" + kbID, Score: score, KnowledgeBaseID: kbID, ChunkIndex: 0},
		},
	}, nil
}

func (r *recordingRetriever) Retrieve(ctx context.Context, _, kbID, _ string, _ *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return r.respond(ctx, kbID)
}
func (r *recordingRetriever) RetrieveWithEmbedding(ctx context.Context, _, kbID, _ string, _ []float32, _ *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return r.respond(ctx, kbID)
}
func (r *recordingRetriever) VectorRetrieve(ctx context.Context, _, kbID, _ string, _ *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return r.respond(ctx, kbID)
}
func (r *recordingRetriever) BM25Retrieve(ctx context.Context, _, kbID, _ string, _ *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return r.respond(ctx, kbID)
}
func (r *recordingRetriever) HybridRetrieve(ctx context.Context, _, kbID, _ string, _ *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return r.respond(ctx, kbID)
}
func (r *recordingRetriever) GraphRetrieve(ctx context.Context, _, kbID, _ string, _ *rag.RetrieveOptions) (*rag.RetrieveResponse, error) {
	return r.respond(ctx, kbID)
}

// ========================================
// mock knowledge.KnowledgeBaseRepository（仅 FindByTenantID 有行为）
// ========================================

type fakeKBRepo struct {
	kbs []*knowledge.KnowledgeBase
}

func (f *fakeKBRepo) FindByTenantID(_ context.Context, _ int64, _, _ int) ([]*knowledge.KnowledgeBase, int64, error) {
	return f.kbs, int64(len(f.kbs)), nil
}
func (f *fakeKBRepo) Create(context.Context, *knowledge.KnowledgeBase) error { return nil }
func (f *fakeKBRepo) FindByID(context.Context, string) (*knowledge.KnowledgeBase, error) {
	return nil, nil
}
func (f *fakeKBRepo) FindByUser(context.Context, int64, int, int) ([]*knowledge.KnowledgeBase, int64, error) {
	return nil, 0, nil
}
func (f *fakeKBRepo) Update(context.Context, *knowledge.KnowledgeBase) error         { return nil }
func (f *fakeKBRepo) UpdateStats(context.Context, string, int, int, int64) error     { return nil }
func (f *fakeKBRepo) Delete(context.Context, string) error                           { return nil }
func (f *fakeKBRepo) HardDelete(context.Context, string) error                       { return nil }
func (f *fakeKBRepo) Exists(context.Context, string) (bool, error)                   { return true, nil }
func (f *fakeKBRepo) GetStorageStats(context.Context, int64) (int64, int64, error)   { return 0, 0, nil }

// ========================================
// 测试
// ========================================

func baseCtx() context.Context {
	return agentctx.WithTenantID(context.Background(), 7)
}

// 用户选定 KB 时，检索严格限定在选定范围内（且需属于本租户），不检索未选中的 KB。
func TestRAGAdapter_AllowedKBsScope(t *testing.T) {
	ret := &recordingRetriever{scores: map[string]float32{"kb1": 0.9, "kb2": 0.5}}
	// 租户下有 kb1/kb2/kb3；用户只选了 kb1/kb2，kb3 不应被检索。
	repo := &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{
		{ID: "kb1", Status: 1}, {ID: "kb2", Status: 1}, {ID: "kb3", Status: 1},
	}}
	a := NewRAGRetrieverAdapter(ret, repo)

	ctx := agentctx.WithAllowedKBIDs(baseCtx(), []string{"kb1", "kb2"})
	res, err := a.Query(ctx, &tools.RAGQueryRequest{Query: "hi", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ret.calledKBs) != 2 {
		t.Fatalf("expected 2 kb calls, got %v", ret.calledKBs)
	}
	for _, id := range ret.calledKBs {
		if id == "kb3" {
			t.Fatalf("kb3 must not be retrieved when scope is kb1/kb2")
		}
	}
	// 跨库合并后按分数降序：kb1(0.9) 在前。
	if res.Count != 2 || len(res.Chunks) != 2 {
		t.Fatalf("expected 2 merged chunks, got %d", res.Count)
	}
	if res.Chunks[0].Score < res.Chunks[1].Score {
		t.Fatalf("chunks not sorted by score desc: %+v", res.Chunks)
	}
	if !res.HasAnswer {
		t.Fatalf("expected HasAnswer=true")
	}
}

// 用户选定集内混入不属于本租户/已禁用的 ID 时，越权 ID 被丢弃，只检索合法 KB。
func TestRAGAdapter_DropsForeignAndDisabledKBs(t *testing.T) {
	ret := &recordingRetriever{scores: map[string]float32{"kb1": 0.9}}
	repo := &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{
		{ID: "kb1", Status: 1},
		{ID: "kbDisabled", Status: 0}, // 本租户但禁用
		// "kbForeign" 不在本租户列表内
	}}
	a := NewRAGRetrieverAdapter(ret, repo)

	ctx := agentctx.WithAllowedKBIDs(baseCtx(), []string{"kb1", "kbDisabled", "kbForeign"})
	res, err := a.Query(ctx, &tools.RAGQueryRequest{Query: "hi", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ret.calledKBs) != 1 || ret.calledKBs[0] != "kb1" {
		t.Fatalf("expected only kb1 retrieved, got %v", ret.calledKBs)
	}
	if res.Count != 1 {
		t.Fatalf("expected 1 chunk, got %d", res.Count)
	}
}

// 缺少有效租户信息时不检索，避免落到 tenant=0。
func TestRAGAdapter_NoTenant(t *testing.T) {
	ret := &recordingRetriever{scores: map[string]float32{"kb1": 0.9}}
	repo := &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{{ID: "kb1", Status: 1}}}
	a := NewRAGRetrieverAdapter(ret, repo)

	// 不注入 tenant，MustGetTenantID 返回 0
	ctx := agentctx.WithAllowedKBIDs(context.Background(), []string{"kb1"})
	res, err := a.Query(ctx, &tools.RAGQueryRequest{Query: "hi"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.HasAnswer || len(ret.calledKBs) != 0 {
		t.Fatalf("expected no retrieval without tenant, got %v", ret.calledKBs)
	}
}

// 未选定 KB 时，回退到全部已启用（Status!=0 且未删除）知识库。
func TestRAGAdapter_FallbackEnabledKBs(t *testing.T) {
	ret := &recordingRetriever{scores: map[string]float32{"kbA": 0.8, "kbB": 0.7}}
	repo := &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{
		{ID: "kbA", Status: 1},
		{ID: "kbB", Status: 1},
		{ID: "kbDisabled", Status: 0}, // 禁用，应跳过
	}}
	a := NewRAGRetrieverAdapter(ret, repo)

	res, err := a.Query(baseCtx(), &tools.RAGQueryRequest{Query: "hi", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ret.calledKBs) != 2 {
		t.Fatalf("expected only 2 enabled kb calls, got %v", ret.calledKBs)
	}
	for _, id := range ret.calledKBs {
		if id == "kbDisabled" {
			t.Fatalf("disabled kb must be skipped")
		}
	}
	if res.Count != 2 {
		t.Fatalf("expected 2 chunks, got %d", res.Count)
	}
}

// 无可用 KB 时返回空结果而非报错。
func TestRAGAdapter_NoKBs(t *testing.T) {
	ret := &recordingRetriever{scores: map[string]float32{}}
	repo := &fakeKBRepo{kbs: nil}
	a := NewRAGRetrieverAdapter(ret, repo)

	res, err := a.Query(baseCtx(), &tools.RAGQueryRequest{Query: "hi"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.HasAnswer || res.Count != 0 {
		t.Fatalf("expected empty result, got %+v", res)
	}
	if len(ret.calledKBs) != 0 {
		t.Fatalf("no retrieval should happen, got %v", ret.calledKBs)
	}
}

// ========================================
// 选库模式（manual / hybrid / auto）测试
// ========================================

// tenantRepo 三库租户，均已启用。
func tenantRepo() *fakeKBRepo {
	return &fakeKBRepo{kbs: []*knowledge.KnowledgeBase{
		{ID: "kb1", Status: 1}, {ID: "kb2", Status: 1}, {ID: "kb3", Status: 1},
	}}
}

// withRoute 注入一个已写入 ids 的 RouteSelection。
func withRoute(ctx context.Context, ids []string) context.Context {
	sel := agentctx.NewRouteSelection()
	sel.Set(ids)
	return agentctx.WithRouteSelection(ctx, sel)
}

// 手动模式：即便存在 route，也一律忽略，范围严格等于用户勾选。
func TestResolveScope_ManualIgnoresRoute(t *testing.T) {
	ret := &recordingRetriever{scores: map[string]float32{"kb1": 0.9, "kb2": 0.8}}
	a := NewRAGRetrieverAdapter(ret, tenantRepo())

	ctx := agentctx.WithAllowedKBIDs(baseCtx(), []string{"kb1", "kb2"})
	ctx = agentctx.WithKBScopeMode(ctx, agentctx.KBScopeModeManual)
	ctx = withRoute(ctx, []string{"kb1"}) // route 应被忽略

	if _, err := a.Query(ctx, &tools.RAGQueryRequest{Query: "hi", TopK: 5}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ret.calledKBs) != 2 {
		t.Fatalf("manual should retrieve both selected kbs, got %v", ret.calledKBs)
	}
}

// 结合模式：route ∩ 勾选，route 聚焦到勾选池内的子集。
func TestResolveScope_HybridIntersectsSelected(t *testing.T) {
	ret := &recordingRetriever{scores: map[string]float32{"kb1": 0.9}}
	a := NewRAGRetrieverAdapter(ret, tenantRepo())

	ctx := agentctx.WithAllowedKBIDs(baseCtx(), []string{"kb1", "kb2"})
	ctx = agentctx.WithKBScopeMode(ctx, agentctx.KBScopeModeHybrid)
	ctx = withRoute(ctx, []string{"kb1"})

	if _, err := a.Query(ctx, &tools.RAGQueryRequest{Query: "hi", TopK: 5}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ret.calledKBs) != 1 || ret.calledKBs[0] != "kb1" {
		t.Fatalf("hybrid should focus to kb1 only, got %v", ret.calledKBs)
	}
}

// 结合模式：route 全部越出候选池（勾选∩启用）→ 交集为空，不检索。
func TestResolveScope_HybridOutOfPoolEmpty(t *testing.T) {
	ret := &recordingRetriever{scores: map[string]float32{"kb3": 0.9}}
	a := NewRAGRetrieverAdapter(ret, tenantRepo())

	ctx := agentctx.WithAllowedKBIDs(baseCtx(), []string{"kb1", "kb2"})
	ctx = agentctx.WithKBScopeMode(ctx, agentctx.KBScopeModeHybrid)
	ctx = withRoute(ctx, []string{"kb3"}) // kb3 属租户但不在候选池

	res, err := a.Query(ctx, &tools.RAGQueryRequest{Query: "hi", TopK: 5})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ret.calledKBs) != 0 || res.Count != 0 {
		t.Fatalf("hybrid out-of-pool route should retrieve nothing, got %v", ret.calledKBs)
	}
}

// 智能模式：忽略勾选，route ∩ 全部已启用；即使 route 库不在勾选内也生效。
func TestResolveScope_AutoIgnoresCheckboxes(t *testing.T) {
	ret := &recordingRetriever{scores: map[string]float32{"kb2": 0.9}}
	a := NewRAGRetrieverAdapter(ret, tenantRepo())

	ctx := agentctx.WithAllowedKBIDs(baseCtx(), []string{"kb1"}) // 勾选 kb1
	ctx = agentctx.WithKBScopeMode(ctx, agentctx.KBScopeModeAuto)
	ctx = withRoute(ctx, []string{"kb2"}) // AI 选了未勾选的 kb2

	if _, err := a.Query(ctx, &tools.RAGQueryRequest{Query: "hi", TopK: 5}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ret.calledKBs) != 1 || ret.calledKBs[0] != "kb2" {
		t.Fatalf("auto should honor route kb2 ignoring checkbox, got %v", ret.calledKBs)
	}
}

// 智能模式：无 route → 回退到全部已启用库。
func TestResolveScope_AutoNoRouteAllEnabled(t *testing.T) {
	ret := &recordingRetriever{scores: map[string]float32{"kb1": 0.9, "kb2": 0.8, "kb3": 0.7}}
	a := NewRAGRetrieverAdapter(ret, tenantRepo())

	ctx := agentctx.WithAllowedKBIDs(baseCtx(), []string{"kb1"})
	ctx = agentctx.WithKBScopeMode(ctx, agentctx.KBScopeModeAuto)
	// 不注入 route

	if _, err := a.Query(ctx, &tools.RAGQueryRequest{Query: "hi", TopK: 5}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ret.calledKBs) != 3 {
		t.Fatalf("auto without route should retrieve all 3 enabled kbs, got %v", ret.calledKBs)
	}
}

// 智能模式：AI 越权到租户外的库被丢弃（安全底线）。
func TestResolveScope_AutoDropsForeign(t *testing.T) {
	ret := &recordingRetriever{scores: map[string]float32{"kb1": 0.9}}
	a := NewRAGRetrieverAdapter(ret, tenantRepo())

	ctx := agentctx.WithKBScopeMode(baseCtx(), agentctx.KBScopeModeAuto)
	ctx = withRoute(ctx, []string{"kb1", "kbForeign"})

	if _, err := a.Query(ctx, &tools.RAGQueryRequest{Query: "hi", TopK: 5}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(ret.calledKBs) != 1 || ret.calledKBs[0] != "kb1" {
		t.Fatalf("auto must drop foreign kb, got %v", ret.calledKBs)
	}
}

func TestExtractQueryTerms(t *testing.T) {
	got := extractQueryTerms("张三 负责，项目A")
	if len(got) != 3 {
		t.Fatalf("expected 3 terms, got %v", got)
	}
	// 无分隔符时退回整句
	whole := extractQueryTerms("单个词")
	if len(whole) != 1 || whole[0] != "单个词" {
		t.Fatalf("expected whole-query fallback, got %v", whole)
	}
	// 空白/空串
	if len(extractQueryTerms("   ")) != 0 {
		t.Fatalf("blank query should yield no terms")
	}
}

func TestStringMapToAny(t *testing.T) {
	if stringMapToAny(nil) != nil {
		t.Fatalf("nil map should map to nil")
	}
	out := stringMapToAny(map[string]string{"k": "v"})
	if out["k"] != "v" {
		t.Fatalf("conversion failed: %+v", out)
	}
}
