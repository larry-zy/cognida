package tools

import (
	"context"
	"strings"
	"testing"

	agentctx "link/internal/model/agent"
	"link/internal/model/semantic"
	"link/internal/service/agent/metricsql"
	"link/internal/service/agent/semanticcache"
	"link/internal/service/agent/termgrounding"
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

// recordingCoverageSink 捕获覆盖埋点事件，供断言 semantic_query 每条路径的 outcome。
type recordingCoverageSink struct {
	events []semantic.CoverageEvent
}

var _ semantic.CoverageSink = (*recordingCoverageSink)(nil)

func (s *recordingCoverageSink) Record(_ context.Context, ev semantic.CoverageEvent) error {
	s.events = append(s.events, ev)
	return nil
}

// last 返回最后一条埋点；无则 t.Fatalf。
func (s *recordingCoverageSink) last(t *testing.T) semantic.CoverageEvent {
	t.Helper()
	if len(s.events) == 0 {
		t.Fatalf("expected a coverage event to be recorded, got none")
	}
	return s.events[len(s.events)-1]
}

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

// withSemantic 装配测试用仓储/缓存并注入租户上下文，返回可直接传入 semanticQuery/groundTerms 的依赖。
func withSemantic(t *testing.T, repo semantic.Repository, cache semanticcache.Cache) (context.Context, semantic.Repository, semanticcache.Cache) {
	t.Helper()
	ctx := agentctx.WithTenantID(context.Background(), 1)
	return ctx, repo, cache
}

func TestSemanticQuery_CoveredGeneratesSQLAndCaches(t *testing.T) {
	cache := semanticcache.NewMemoryCache()
	ctx, repo, c := withSemantic(t, &stubSemanticRepo{bundle: salesBundle()}, cache)

	sink := &recordingCoverageSink{}
	res, err := semanticQuery(ctx, &SemanticQueryRequest{
		Metrics:    []string{"营收"},
		Dimensions: []string{"区域"},
	}, repo, c, sink)
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
	// 覆盖埋点：治理直出应记 covered，模型名钉在 sales。
	if ev := sink.last(t); ev.Outcome != semantic.CoverageCovered || ev.Model != "sales" {
		t.Errorf("expected covered event on 'sales', got %+v", ev)
	}

	// 覆盖命中应写入受信缓存：同签名请求键应命中。
	key := semanticcache.BuildKey(1, "sales", 3, metricsql.Query{Metrics: []string{"营收"}, Dimensions: []string{"区域"}})
	if _, ok, _ := cache.Get(ctx, key); !ok {
		t.Errorf("covered SQL should have been cached under key %s", key)
	}
}

func TestSemanticQuery_UncoveredFallsBackToLexical(t *testing.T) {
	ctx, repo, c := withSemantic(t, &stubSemanticRepo{bundle: salesBundle()}, semanticcache.NewMemoryCache())

	sink := &recordingCoverageSink{}
	res, err := semanticQuery(ctx, &SemanticQueryRequest{Metrics: []string{"利润"}}, repo, c, sink)
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
	// 覆盖埋点：未覆盖应记 fallback，并带上未覆盖名称供定位建模缺口。
	if ev := sink.last(t); ev.Outcome != semantic.CoverageFallback || len(ev.Uncovered) == 0 || ev.Uncovered[0] != "利润" {
		t.Errorf("expected fallback event carrying 利润, got %+v", ev)
	}
}

func TestSemanticQuery_CacheHitReturnsTrustedSQL(t *testing.T) {
	cache := semanticcache.NewMemoryCache()
	ctx, repo, c := withSemantic(t, &stubSemanticRepo{bundle: salesBundle()}, cache)

	q := metricsql.Query{Metrics: []string{"营收"}, Dimensions: []string{"区域"}}
	key := semanticcache.BuildKey(1, "sales", 3, q)
	if err := cache.Put(ctx, key, &semanticcache.Entry{SQL: "SELECT /*golden*/ 1", ModelVersion: 3, Verified: true, Golden: true}, semanticcache.DefaultTTL); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	sink := &recordingCoverageSink{}
	res, err := semanticQuery(ctx, &SemanticQueryRequest{Metrics: []string{"营收"}, Dimensions: []string{"区域"}}, repo, c, sink)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.CacheHit || !res.Golden {
		t.Fatalf("expected golden cache hit, got %+v", res)
	}
	// 覆盖埋点：缓存命中亦属治理命中，应记 cache_hit。
	if ev := sink.last(t); ev.Outcome != semantic.CoverageCacheHit || ev.Model != "sales" {
		t.Errorf("expected cache_hit event on 'sales', got %+v", ev)
	}
	if !strings.Contains(res.SQL, "/*golden*/") {
		t.Errorf("cache hit must return the trusted cached SQL, got %s", res.SQL)
	}
}

func TestBundleDatabaseID(t *testing.T) {
	cases := []struct {
		name   string
		tables []*semantic.LogicalTable
		want   string
	}{
		{"nil bundle tables", nil, ""},
		{"all unbound → 回落", []*semantic.LogicalTable{{ID: "a"}, {ID: "b"}}, ""},
		{"single bound → 取该库", []*semantic.LogicalTable{{ID: "a", DatabaseID: "ds1"}, {ID: "b", DatabaseID: "ds1"}}, "ds1"},
		{"部分绑定、取值一致 → 取该库", []*semantic.LogicalTable{{ID: "a"}, {ID: "b", DatabaseID: "ds1"}}, "ds1"},
		{"跨库不一致 → 不硬猜、回落", []*semantic.LogicalTable{{ID: "a", DatabaseID: "ds1"}, {ID: "b", DatabaseID: "ds2"}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := bundleDatabaseID(&semantic.ModelBundle{LogicalTables: c.tables})
			if got != c.want {
				t.Errorf("bundleDatabaseID(%s) = %q, want %q", c.name, got, c.want)
			}
		})
	}
	if got := bundleDatabaseID(nil); got != "" {
		t.Errorf("bundleDatabaseID(nil) = %q, want empty", got)
	}
}

// boundSalesBundle 在 salesBundle 基础上把逻辑表绑定到数据源 ds-ecom，
// 用于验证治理 SQL 会显式带上数据源 ID。
func boundSalesBundle() *semantic.ModelBundle {
	b := salesBundle()
	for _, lt := range b.LogicalTables {
		lt.DatabaseID = "ds-ecom"
	}
	return b
}

func TestSemanticQuery_CoveredPassesThroughDatabaseID(t *testing.T) {
	ctx, repo, c := withSemantic(t, &stubSemanticRepo{bundle: boundSalesBundle()}, semanticcache.NewMemoryCache())

	res, err := semanticQuery(ctx, &SemanticQueryRequest{
		Metrics:    []string{"营收"},
		Dimensions: []string{"区域"},
	}, repo, c, &recordingCoverageSink{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Covered {
		t.Fatalf("expected covered, got %+v", res)
	}
	if res.DatabaseID != "ds-ecom" {
		t.Errorf("covered result must carry model-bound database_id, got %q", res.DatabaseID)
	}
}

func TestSemanticQuery_CacheHitPassesThroughDatabaseID(t *testing.T) {
	cache := semanticcache.NewMemoryCache()
	ctx, repo, c := withSemantic(t, &stubSemanticRepo{bundle: boundSalesBundle()}, cache)

	q := metricsql.Query{Metrics: []string{"营收"}, Dimensions: []string{"区域"}}
	key := semanticcache.BuildKey(1, "sales", 3, q)
	if err := cache.Put(ctx, key, &semanticcache.Entry{SQL: "SELECT /*v*/ 1", ModelVersion: 3, Verified: true}, semanticcache.DefaultTTL); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	res, err := semanticQuery(ctx, &SemanticQueryRequest{Metrics: []string{"营收"}, Dimensions: []string{"区域"}}, repo, c, &recordingCoverageSink{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.CacheHit {
		t.Fatalf("expected cache hit, got %+v", res)
	}
	if res.DatabaseID != "ds-ecom" {
		t.Errorf("cache-hit result must also carry model-bound database_id, got %q", res.DatabaseID)
	}
}

// TestSemanticQuery_UnboundModelOmitsDatabaseID 旧模型（逻辑表未绑定数据源）
// 应返回空 database_id，回落会话选定库——保证向后兼容不破坏既有行为。
func TestSemanticQuery_UnboundModelOmitsDatabaseID(t *testing.T) {
	ctx, repo, c := withSemantic(t, &stubSemanticRepo{bundle: salesBundle()}, semanticcache.NewMemoryCache())

	res, err := semanticQuery(ctx, &SemanticQueryRequest{
		Metrics:    []string{"营收"},
		Dimensions: []string{"区域"},
	}, repo, c, &recordingCoverageSink{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.DatabaseID != "" {
		t.Errorf("unbound model must omit database_id (会话回落), got %q", res.DatabaseID)
	}
}

func TestGroundTerms_NeedsClarificationOnAmbiguity(t *testing.T) {
	b := salesBundle()
	// 追加一个与 营收 共享同义词 "revenue" 的指标 → 触发歧义。
	b.Metrics = append(b.Metrics, &semantic.Metric{ID: "mt_rev2", ModelID: "m1", Name: "净收入", Synonyms: []string{"revenue"}})
	ctx, repo, _ := withSemantic(t, &stubSemanticRepo{bundle: b}, semanticcache.NewMemoryCache())
	grounder := termgrounding.NewGrounder(nil)

	res, err := groundTerms(ctx, &GroundTermsRequest{Terms: []string{"revenue", "大区", "客单价"}}, repo, grounder)
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
