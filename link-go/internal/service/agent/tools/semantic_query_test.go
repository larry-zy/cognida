package tools

import (
	"context"
	"strings"
	"testing"

	agentctx "link/internal/model/agent"
	"link/internal/model/semantic"
	"link/internal/service/agent/metricsql"
	"link/internal/service/agent/semanticcache"
)

// stubSemanticRepo 是 semantic.Repository 打桩：只回单个生效模型的固定 bundle。
type stubSemanticRepo struct {
	bundle *semantic.ModelBundle
}

var _ semantic.Repository = (*stubSemanticRepo)(nil)

func (s *stubSemanticRepo) GetActiveModel(_ context.Context, _ int64, name string) (*semantic.ModelBundle, error) {
	if name != "" && name != s.bundle.Model.Name {
		return nil, semantic.ErrModelNotFound
	}
	return s.bundle, nil
}

func (s *stubSemanticRepo) ListActiveModels(_ context.Context, _ int64) ([]*semantic.SemanticModel, error) {
	return []*semantic.SemanticModel{s.bundle.Model}, nil
}

func (s *stubSemanticRepo) GetModelBundle(_ context.Context, _ string) (*semantic.ModelBundle, error) {
	return s.bundle, nil
}

func (s *stubSemanticRepo) UpsertBundle(_ context.Context, _ *semantic.ModelBundle) error { return nil }
func (s *stubSemanticRepo) BumpVersion(_ context.Context, _ string) (int, error)          { return 0, nil }

func salesBundle() *semantic.ModelBundle {
	return &semantic.ModelBundle{
		Model: &semantic.SemanticModel{ID: "m1", Name: "sales", Version: 3, Status: semantic.ModelStatusActive},
		LogicalTables: []*semantic.LogicalTable{
			{ID: "lt_order", ModelID: "m1", Name: "orders", PhysicalTable: "t_order"},
		},
		Dimensions: []*semantic.Dimension{
			{ID: "d_region", ModelID: "m1", LogicalTableID: "lt_order", Name: "区域", Expr: "region", Synonyms: []string{"大区"}},
		},
		Measures: []*semantic.Measure{
			{ID: "ms_amt", ModelID: "m1", LogicalTableID: "lt_order", Name: "金额", Expr: "amount", Aggregation: semantic.AggSum},
		},
		Metrics: []*semantic.Metric{
			{ID: "mt_rev", ModelID: "m1", Name: "营收", Expr: "SUM(orders.amount)", Synonyms: []string{"revenue"}},
		},
	}
}

// withSemantic 在测试内装配全局仓储/缓存并注入租户上下文，返回还原函数。
func withSemantic(t *testing.T, repo semantic.Repository, cache semanticcache.Cache) (context.Context, func()) {
	t.Helper()
	prevRepo, prevCache := semanticRepo, semanticQueryCache
	semanticRepo, semanticQueryCache = repo, cache
	ctx := agentctx.WithTenantID(context.Background(), 1)
	return ctx, func() { semanticRepo, semanticQueryCache = prevRepo, prevCache }
}

func TestSemanticQuery_CoveredGeneratesSQLAndCaches(t *testing.T) {
	cache := semanticcache.NewMemoryCache()
	ctx, restore := withSemantic(t, &stubSemanticRepo{bundle: salesBundle()}, cache)
	defer restore()

	res, err := semanticQuery(ctx, &SemanticQueryRequest{
		Metrics:    []string{"营收"},
		Dimensions: []string{"区域"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Covered || res.SQL == "" {
		t.Fatalf("expected covered SQL, got %+v", res)
	}
	if res.Model != "sales" || res.ModelVersion != 3 {
		t.Errorf("wrong model/version: %+v", res)
	}
	if res.CacheHit {
		t.Errorf("first call must not be a cache hit")
	}

	// 覆盖命中应写入受信缓存：同签名请求键应命中。
	key := semanticcache.BuildKey(1, "sales", 3, metricsql.Query{Metrics: []string{"营收"}, Dimensions: []string{"区域"}})
	if _, ok, _ := cache.Get(ctx, key); !ok {
		t.Errorf("covered SQL should have been cached under key %s", key)
	}
}

func TestSemanticQuery_UncoveredFallsBackToLexical(t *testing.T) {
	ctx, restore := withSemantic(t, &stubSemanticRepo{bundle: salesBundle()}, semanticcache.NewMemoryCache())
	defer restore()

	res, err := semanticQuery(ctx, &SemanticQueryRequest{Metrics: []string{"利润"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Covered || res.SQL != "" {
		t.Fatalf("expected uncovered with no SQL, got %+v", res)
	}
	if res.FallbackHint == "" {
		t.Errorf("uncovered must carry an observable fallback hint")
	}
	if len(res.Uncovered) == 0 || res.Uncovered[0] != "利润" {
		t.Errorf("expected 利润 uncovered, got %v", res.Uncovered)
	}
}

func TestSemanticQuery_CacheHitReturnsTrustedSQL(t *testing.T) {
	cache := semanticcache.NewMemoryCache()
	ctx, restore := withSemantic(t, &stubSemanticRepo{bundle: salesBundle()}, cache)
	defer restore()

	q := metricsql.Query{Metrics: []string{"营收"}, Dimensions: []string{"区域"}}
	key := semanticcache.BuildKey(1, "sales", 3, q)
	if err := cache.Put(ctx, key, &semanticcache.Entry{SQL: "SELECT /*golden*/ 1", ModelVersion: 3, Verified: true, Golden: true}, semanticcache.DefaultTTL); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	res, err := semanticQuery(ctx, &SemanticQueryRequest{Metrics: []string{"营收"}, Dimensions: []string{"区域"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.CacheHit || !res.Golden {
		t.Fatalf("expected golden cache hit, got %+v", res)
	}
	if !strings.Contains(res.SQL, "/*golden*/") {
		t.Errorf("cache hit must return the trusted cached SQL, got %s", res.SQL)
	}
}

func TestGroundTerms_NeedsClarificationOnAmbiguity(t *testing.T) {
	b := salesBundle()
	// 追加一个与 营收 共享同义词 "revenue" 的指标 → 触发歧义。
	b.Metrics = append(b.Metrics, &semantic.Metric{ID: "mt_rev2", ModelID: "m1", Name: "净收入", Synonyms: []string{"revenue"}})
	ctx, restore := withSemantic(t, &stubSemanticRepo{bundle: b}, semanticcache.NewMemoryCache())
	defer restore()

	res, err := groundTerms(ctx, &GroundTermsRequest{Terms: []string{"revenue", "大区", "客单价"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.NeedsClarification || res.ClarifyPrompt == "" {
		t.Fatalf("ambiguous+unresolved terms must request clarification: %+v", res)
	}
	byTerm := map[string]GroundedTerm{}
	for _, g := range res.Terms {
		byTerm[g.Term] = g
	}
	if !byTerm["revenue"].Ambiguous {
		t.Errorf("revenue should be ambiguous: %+v", byTerm["revenue"])
	}
	if byTerm["大区"].Resolved != "区域" {
		t.Errorf("大区 should resolve to 区域: %+v", byTerm["大区"])
	}
	if !byTerm["客单价"].Unresolved {
		t.Errorf("客单价 should be unresolved: %+v", byTerm["客单价"])
	}
}
