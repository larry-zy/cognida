package semantic

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	model "link/internal/model/semantic"
)

// --- 测试替身 --------------------------------------------------------------

// fakeRepo 内存版 model.Repository：以 modelID 存 bundle，供 Create/Update/Publish/Deprecate 回环校验。
type fakeRepo struct {
	bundles map[string]*model.ModelBundle
	// 计数器：断言 UpsertBundle/BumpVersion 被调用次数（校验版本推进副作用）。
	upserts int
	bumps   int
	// 注入错误：非 nil 时对应方法直接返回该错误。
	getErr    error
	upsertErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{bundles: map[string]*model.ModelBundle{}}
}

func (r *fakeRepo) GetActiveModel(_ context.Context, tenantID int64, name string) (*model.ModelBundle, error) {
	for _, b := range r.bundles {
		if b.Model.TenantID == tenantID && b.Model.Name == name && b.Model.Status == model.ModelStatusActive {
			return b, nil
		}
	}
	return nil, model.ErrModelNotFound
}

func (r *fakeRepo) ListActiveModels(_ context.Context, tenantID int64) ([]*model.SemanticModel, error) {
	out := make([]*model.SemanticModel, 0)
	for _, b := range r.bundles {
		if b.Model.TenantID == tenantID && b.Model.Status == model.ModelStatusActive {
			out = append(out, b.Model)
		}
	}
	return out, nil
}

func (r *fakeRepo) GetModelBundle(_ context.Context, modelID string) (*model.ModelBundle, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	b, ok := r.bundles[modelID]
	if !ok {
		return nil, model.ErrModelNotFound
	}
	return b, nil
}

func (r *fakeRepo) UpsertBundle(_ context.Context, bundle *model.ModelBundle) error {
	if r.upsertErr != nil {
		return r.upsertErr
	}
	r.upserts++
	// 深浅无所谓：服务每次全量组装新 bundle，这里直接存指针即可。
	r.bundles[bundle.Model.ID] = bundle
	return nil
}

func (r *fakeRepo) BumpVersion(_ context.Context, modelID string) (int, error) {
	r.bumps++
	b, ok := r.bundles[modelID]
	if !ok {
		return 0, model.ErrModelNotFound
	}
	b.Model.Version++
	return b.Model.Version, nil
}

// seqIDGen 顺序 ID 生成器，产出确定性 ID 便于断言。
type seqIDGen struct{ n int }

func (g *seqIDGen) Generate() string {
	g.n++
	return "gen-" + strconv.Itoa(g.n)
}

// fakeCoverage 覆盖率读侧替身。
type fakeCoverage struct {
	stats []model.CoverageModelStat
	err   error
}

func (c *fakeCoverage) Record(_ context.Context, _ model.CoverageEvent) error { return nil }
func (c *fakeCoverage) Stats(_ context.Context, _ int64) ([]model.CoverageModelStat, error) {
	return c.stats, c.err
}

// --- 夹具 ------------------------------------------------------------------

const tenant = int64(1)

// validInput 返回一份最小但结构完整的建模输入（一表一维一度一指标）。
func validInput(name string) *SaveInput {
	return &SaveInput{
		Name: name,
		LogicalTables: []*model.LogicalTable{
			{ID: "lt_order", Name: "orders", PhysicalTable: "t_order"},
		},
		Dimensions: []*model.Dimension{
			{LogicalTableID: "lt_order", Name: "区域", Expr: "region"},
		},
		Measures: []*model.Measure{
			{LogicalTableID: "lt_order", Name: "金额", Expr: "amount", Aggregation: model.AggSum},
		},
		Metrics: []*model.Metric{
			{Name: "营收", Expr: "SUM(orders.amount)"},
		},
	}
}

func newService(repo model.Repository, cov model.CoverageReporter) *Service {
	return NewService(repo, &seqIDGen{}, cov)
}

// --- Create ----------------------------------------------------------------

func TestCreate_DefaultsToDraftAndAssignsIDs(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, nil)

	got, err := svc.Create(context.Background(), tenant, validInput("sales"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Model.Status != model.ModelStatusDraft {
		t.Errorf("status 缺省应为 draft, got %q", got.Model.Status)
	}
	if got.Model.Version != 1 {
		t.Errorf("首版版本应为 1, got %d", got.Model.Version)
	}
	if got.Model.TenantID != tenant {
		t.Errorf("租户应回填为 %d, got %d", tenant, got.Model.TenantID)
	}
	if got.Model.ID == "" {
		t.Error("模型 ID 应由服务生成")
	}
	// 逻辑表沿用客户端 ID；其余构件缺省 ID 应被生成并挂 ModelID。
	if got.LogicalTables[0].ID != "lt_order" {
		t.Errorf("逻辑表应沿用客户端 id, got %q", got.LogicalTables[0].ID)
	}
	if got.Metrics[0].ID == "" || got.Metrics[0].ModelID != got.Model.ID {
		t.Errorf("指标应补 id 并挂 modelID: %+v", got.Metrics[0])
	}
	if repo.upserts != 1 {
		t.Errorf("Create 应调用一次 UpsertBundle, got %d", repo.upserts)
	}
}

func TestCreate_ActiveStatusChecksNameConflict(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, nil)

	// 先建一个生效的 sales。
	in := validInput("sales")
	in.Status = string(model.ModelStatusActive)
	if _, err := svc.Create(context.Background(), tenant, in); err != nil {
		t.Fatalf("first create: %v", err)
	}

	// 再建同名生效模型应冲突。
	dup := validInput("sales")
	dup.Status = string(model.ModelStatusActive)
	_, err := svc.Create(context.Background(), tenant, dup)
	if !errors.Is(err, ErrNameConflict) {
		t.Fatalf("同名生效模型应报 ErrNameConflict, got %v", err)
	}
}

func TestCreate_DraftSameNameAllowed(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, nil)

	// 生效 sales 存在时，再建 draft 同名不应冲突（冲突仅约束生效态）。
	active := validInput("sales")
	active.Status = string(model.ModelStatusActive)
	if _, err := svc.Create(context.Background(), tenant, active); err != nil {
		t.Fatalf("active create: %v", err)
	}
	if _, err := svc.Create(context.Background(), tenant, validInput("sales")); err != nil {
		t.Errorf("draft 同名不应冲突, got %v", err)
	}
}

func TestCreate_ValidationErrors(t *testing.T) {
	cases := map[string]func(*SaveInput){
		"空 name":       func(in *SaveInput) { in.Name = "  " },
		"无逻辑表":         func(in *SaveInput) { in.LogicalTables = nil },
		"逻辑表缺 id":      func(in *SaveInput) { in.LogicalTables[0].ID = "" },
		"逻辑表缺物理表":      func(in *SaveInput) { in.LogicalTables[0].PhysicalTable = "" },
		"维度引用不存在的逻辑表":  func(in *SaveInput) { in.Dimensions[0].LogicalTableID = "nope" },
		"度量缺 expr":     func(in *SaveInput) { in.Measures[0].Expr = "" },
		"指标缺 name":     func(in *SaveInput) { in.Metrics[0].Name = "" },
	}
	svc := newService(newFakeRepo(), nil)
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			in := validInput("sales")
			mut(in)
			_, err := svc.Create(context.Background(), tenant, in)
			if !errors.Is(err, ErrValidation) {
				t.Errorf("应返回 ErrValidation, got %v", err)
			}
		})
	}
}

func TestCreate_DuplicateLogicalTableID(t *testing.T) {
	svc := newService(newFakeRepo(), nil)
	in := validInput("sales")
	in.LogicalTables = append(in.LogicalTables, &model.LogicalTable{ID: "lt_order", Name: "o2", PhysicalTable: "t2"})
	_, err := svc.Create(context.Background(), tenant, in)
	if !errors.Is(err, ErrValidation) {
		t.Errorf("重复逻辑表 id 应报 ErrValidation, got %v", err)
	}
}

// --- Update ----------------------------------------------------------------

func TestUpdate_ReplacesAndBumpsVersion(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, nil)

	created, err := svc.Create(context.Background(), tenant, validInput("sales"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	repo.bumps = 0 // 只观察 Update 的 bump。

	in := validInput("sales-v2")
	updated, err := svc.Update(context.Background(), tenant, created.Model.ID, in)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Model.Name != "sales-v2" {
		t.Errorf("名称应被替换, got %q", updated.Model.Name)
	}
	if updated.Model.Version != 2 {
		t.Errorf("Update 应 bump 到版本 2, got %d", updated.Model.Version)
	}
	if repo.bumps != 1 {
		t.Errorf("Update 应调用一次 BumpVersion, got %d", repo.bumps)
	}
}

func TestUpdate_CrossTenantIsNotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, nil)
	created, _ := svc.Create(context.Background(), tenant, validInput("sales"))

	_, err := svc.Update(context.Background(), int64(999), created.Model.ID, validInput("x"))
	if !errors.Is(err, model.ErrModelNotFound) {
		t.Errorf("跨租户更新应等同不存在, got %v", err)
	}
}

func TestUpdate_MissingModel(t *testing.T) {
	svc := newService(newFakeRepo(), nil)
	_, err := svc.Update(context.Background(), tenant, "does-not-exist", validInput("sales"))
	if !errors.Is(err, model.ErrModelNotFound) {
		t.Errorf("不存在的模型应报 ErrModelNotFound, got %v", err)
	}
}

// --- Publish / Deprecate ---------------------------------------------------

func TestPublish_ActivatesAndBumps(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, nil)
	created, _ := svc.Create(context.Background(), tenant, validInput("sales"))

	got, err := svc.Publish(context.Background(), tenant, created.Model.ID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got.Model.Status != model.ModelStatusActive {
		t.Errorf("Publish 应置 active, got %q", got.Model.Status)
	}
	if got.Model.Version != 2 {
		t.Errorf("Publish 应 bump 版本, got %d", got.Model.Version)
	}
}

func TestPublish_NameConflictBlocks(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, nil)

	// 已有生效 sales。
	active := validInput("sales")
	active.Status = string(model.ModelStatusActive)
	if _, err := svc.Create(context.Background(), tenant, active); err != nil {
		t.Fatalf("seed active: %v", err)
	}
	// 另有一个 draft 的同名 sales，发布应因命名冲突被拦。
	draft, _ := svc.Create(context.Background(), tenant, validInput("sales"))

	_, err := svc.Publish(context.Background(), tenant, draft.Model.ID)
	if !errors.Is(err, ErrNameConflict) {
		t.Errorf("发布同名应报 ErrNameConflict, got %v", err)
	}
}

func TestDeprecate_RemovesFromActive(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, nil)
	active := validInput("sales")
	active.Status = string(model.ModelStatusActive)
	created, _ := svc.Create(context.Background(), tenant, active)

	got, err := svc.Deprecate(context.Background(), tenant, created.Model.ID)
	if err != nil {
		t.Fatalf("deprecate: %v", err)
	}
	if got.Model.Status != model.ModelStatusDeprecated {
		t.Errorf("Deprecate 应置 deprecated, got %q", got.Model.Status)
	}
	heads, _ := svc.List(context.Background(), tenant)
	if len(heads) != 0 {
		t.Errorf("弃用后不应再出现在生效列表, got %d", len(heads))
	}
}

// --- Get / List ------------------------------------------------------------

func TestGet_CrossTenantHidden(t *testing.T) {
	svc := newService(newFakeRepo(), nil)
	created, _ := svc.Create(context.Background(), tenant, validInput("sales"))

	if _, err := svc.Get(context.Background(), tenant, created.Model.ID); err != nil {
		t.Fatalf("owner get: %v", err)
	}
	_, err := svc.Get(context.Background(), int64(2), created.Model.ID)
	if !errors.Is(err, model.ErrModelNotFound) {
		t.Errorf("跨租户 Get 应等同不存在, got %v", err)
	}
}

func TestList_OnlyActive(t *testing.T) {
	svc := newService(newFakeRepo(), nil)
	// 一个 draft，一个 active。
	if _, err := svc.Create(context.Background(), tenant, validInput("draft-model")); err != nil {
		t.Fatalf("draft: %v", err)
	}
	active := validInput("active-model")
	active.Status = string(model.ModelStatusActive)
	if _, err := svc.Create(context.Background(), tenant, active); err != nil {
		t.Fatalf("active: %v", err)
	}

	heads, err := svc.List(context.Background(), tenant)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(heads) != 1 || heads[0].Name != "active-model" {
		t.Errorf("List 仅应返回生效模型, got %+v", heads)
	}
}

// --- Coverage --------------------------------------------------------------

func TestCoverage_DisabledWhenNoReporter(t *testing.T) {
	svc := newService(newFakeRepo(), nil)
	_, err := svc.Coverage(context.Background(), tenant)
	if !errors.Is(err, ErrCoverageDisabled) {
		t.Errorf("未注入覆盖读侧应报 ErrCoverageDisabled, got %v", err)
	}
}

func TestCoverage_DelegatesToReporter(t *testing.T) {
	want := []model.CoverageModelStat{{Model: "sales", Covered: 8, Fallback: 2, Total: 10, HitRatio: 0.8}}
	svc := newService(newFakeRepo(), &fakeCoverage{stats: want})
	got, err := svc.Coverage(context.Background(), tenant)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if len(got) != 1 || got[0].Model != "sales" || got[0].HitRatio != 0.8 {
		t.Errorf("应透传 reporter 统计, got %+v", got)
	}
}

func TestCoverage_PropagatesReporterError(t *testing.T) {
	boom := fmt.Errorf("db down")
	svc := newService(newFakeRepo(), &fakeCoverage{err: boom})
	_, err := svc.Coverage(context.Background(), tenant)
	if !errors.Is(err, boom) {
		t.Errorf("应透传 reporter 错误, got %v", err)
	}
}
