// Package semantic 指标语义层「建模」应用服务：为受治理的语义模型提供写入进料线
// （Create/Update/Publish/Deprecate）与读取（List/Get）。
//
// 定位：补齐语义层的建模写入口——此前 UpsertBundle 仅在测试中被调用，线上无建模入口，
// 导致语义层「空壳」、semantic_query 恒回退词法 NL2SQL。本服务把 UpsertBundle/BumpVersion
// 收敛为受校验的应用用例，接入 HTTP handler + 组合根。
//
// 信任边界：Metric/Measure/Dimension.Expr 与 Relation.JoinCondition 是「可信管理员 SQL」，
// 逐字进入引擎生成的 SELECT/ON（见 metricsql/engine.go 顶部注释）。本服务仅做结构完整性
// 与引用一致性校验，不解析表达式语义；若日后放开由低权限角色编辑，必须在此加表达式白名单校验。
package semantic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	model "cognida/internal/model/semantic"
)

// IDGenerator 生成语义模型/构件 ID（与 infrastructure/id.IDGenerator 同构，便于复用注入）。
type IDGenerator interface {
	Generate() string
}

var (
	// ErrValidation 建模输入结构校验失败（缺字段/引用悬挂等）。
	ErrValidation = errors.New("semantic model: 校验失败")
	// ErrNameConflict 同租户下已存在同名生效模型。
	ErrNameConflict = errors.New("semantic model: 生效模型名已存在")
)

// Service 语义模型建模应用服务。
type Service struct {
	repo     model.Repository
	idGen    IDGenerator
	coverage model.CoverageReporter // 覆盖率读侧（可选，nil 时 Coverage 返回未启用）
	profiles ProfileReader          // 列画像读侧（可选，nil 时 DiagnoseValueMaps 返回未启用）
}

// NewService 创建建模服务。coverage 覆盖率读侧、profiles 列画像读侧端口均可为 nil
// （对应埋点未启用 / 画像未接线时，相关只读诊断返回「未启用」而非报错）。
func NewService(repo model.Repository, idGen IDGenerator, coverage model.CoverageReporter, profiles ProfileReader) *Service {
	return &Service{repo: repo, idGen: idGen, coverage: coverage, profiles: profiles}
}

// ErrCoverageDisabled 覆盖埋点未启用（未注入 CoverageReporter）。
var ErrCoverageDisabled = errors.New("semantic model: 覆盖埋点未启用")

// ErrProfileDisabled 列画像未接线（未注入 ProfileReader），值映射诊断不可用。
var ErrProfileDisabled = errors.New("semantic model: 列画像未接线")

// Coverage 返回租户下各语义模型的治理命中率（covered/cache_hit/fallback 聚合）。
func (s *Service) Coverage(ctx context.Context, tenantID int64) ([]model.CoverageModelStat, error) {
	if s.coverage == nil {
		return nil, ErrCoverageDisabled
	}
	return s.coverage.Stats(ctx, tenantID)
}

// SaveInput 建模写入载荷（Create/Update 复用）。模型头的 ID/TenantID/Version/时间戳
// 由服务掌管，客户端不可设定；逻辑表 ID 由客户端提供并被维度/度量/关系引用，
// 其余构件 ID 缺省则服务端生成。
type SaveInput struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Status      string                `json:"status"` // draft/active，空则 Create 默认 draft、Update 保持原状态
	LogicalTables []*model.LogicalTable `json:"logical_tables"`
	Dimensions    []*model.Dimension    `json:"dimensions"`
	Measures      []*model.Measure      `json:"measures"`
	Metrics       []*model.Metric       `json:"metrics"`
	Relations     []*model.Relation     `json:"relations"`
}

// List 列出租户下全部「生效」语义模型（仅模型头）。
//
// 注：受仓储接口限制，当前仅列生效模型；新建的 draft 需以 Create 返回的 ID 经 Get 取用，
// Publish 后方进入本列表。建模控制台若需管理草稿列表，另需扩仓储 ListModels(any status)。
func (s *Service) List(ctx context.Context, tenantID int64) ([]*model.SemanticModel, error) {
	return s.repo.ListActiveModels(ctx, tenantID)
}

// Get 按 ID 取完整 bundle（任意状态），并做租户隔离校验。
func (s *Service) Get(ctx context.Context, tenantID int64, modelID string) (*model.ModelBundle, error) {
	b, err := s.repo.GetModelBundle(ctx, modelID)
	if err != nil {
		return nil, err
	}
	if b.Model.TenantID != tenantID {
		// 跨租户不可见：对外等同不存在，避免探测。
		return nil, model.ErrModelNotFound
	}
	return b, nil
}

// Create 新建语义模型（全量落库）。status 缺省 draft；置 active 时校验同名生效模型不冲突。
func (s *Service) Create(ctx context.Context, tenantID int64, in *SaveInput) (*model.ModelBundle, error) {
	if err := validate(in); err != nil {
		return nil, err
	}
	status := normalizeStatus(in.Status, model.ModelStatusDraft)
	if status == model.ModelStatusActive {
		if err := s.ensureNoActiveNameConflict(ctx, tenantID, in.Name, ""); err != nil {
			return nil, err
		}
	}

	modelID := s.idGen.Generate()
	bundle := s.assemble(modelID, tenantID, 1, status, in)
	if err := s.repo.UpsertBundle(ctx, bundle); err != nil {
		return nil, err
	}
	return s.repo.GetModelBundle(ctx, modelID)
}

// Update 全量替换一个语义模型的构件并 bump 版本（驱动受信查询缓存失效）。
func (s *Service) Update(ctx context.Context, tenantID int64, modelID string, in *SaveInput) (*model.ModelBundle, error) {
	existing, err := s.repo.GetModelBundle(ctx, modelID)
	if err != nil {
		return nil, err
	}
	if existing.Model.TenantID != tenantID {
		return nil, model.ErrModelNotFound
	}
	if err := validate(in); err != nil {
		return nil, err
	}
	status := normalizeStatus(in.Status, existing.Model.Status)
	if status == model.ModelStatusActive {
		if err := s.ensureNoActiveNameConflict(ctx, tenantID, in.Name, modelID); err != nil {
			return nil, err
		}
	}

	bundle := s.assemble(modelID, tenantID, existing.Model.Version, status, in)
	if err := s.repo.UpsertBundle(ctx, bundle); err != nil {
		return nil, err
	}
	if _, err := s.repo.BumpVersion(ctx, modelID); err != nil {
		return nil, err
	}
	return s.repo.GetModelBundle(ctx, modelID)
}

// Publish 将模型置为生效并 bump 版本（进入查询主路径）。
func (s *Service) Publish(ctx context.Context, tenantID int64, modelID string) (*model.ModelBundle, error) {
	return s.setStatus(ctx, tenantID, modelID, model.ModelStatusActive, true)
}

// Deprecate 弃用模型（退出查询主路径），bump 版本使其残留缓存失效。
func (s *Service) Deprecate(ctx context.Context, tenantID int64, modelID string) (*model.ModelBundle, error) {
	return s.setStatus(ctx, tenantID, modelID, model.ModelStatusDeprecated, true)
}

// setStatus 载入既有 bundle、改状态、全量回写并按需 bump。
func (s *Service) setStatus(ctx context.Context, tenantID int64, modelID string, status model.ModelStatus, bump bool) (*model.ModelBundle, error) {
	bundle, err := s.repo.GetModelBundle(ctx, modelID)
	if err != nil {
		return nil, err
	}
	if bundle.Model.TenantID != tenantID {
		return nil, model.ErrModelNotFound
	}
	if status == model.ModelStatusActive {
		if err := s.ensureNoActiveNameConflict(ctx, tenantID, bundle.Model.Name, modelID); err != nil {
			return nil, err
		}
	}
	bundle.Model.Status = status
	if err := s.repo.UpsertBundle(ctx, bundle); err != nil {
		return nil, err
	}
	if bump {
		if _, err := s.repo.BumpVersion(ctx, modelID); err != nil {
			return nil, err
		}
	}
	return s.repo.GetModelBundle(ctx, modelID)
}

// ensureNoActiveNameConflict 校验目标名在租户内无「其它」生效模型占用（excludeID 为自身）。
func (s *Service) ensureNoActiveNameConflict(ctx context.Context, tenantID int64, name, excludeID string) error {
	heads, err := s.repo.ListActiveModels(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, h := range heads {
		if h.ID != excludeID && strings.EqualFold(strings.TrimSpace(h.Name), strings.TrimSpace(name)) {
			return fmt.Errorf("%w: %q", ErrNameConflict, name)
		}
	}
	return nil
}

// assemble 由输入组装完整 bundle：服务掌管模型头字段，构件统一挂 ModelID，缺省 ID 生成。
func (s *Service) assemble(modelID string, tenantID int64, version int, status model.ModelStatus, in *SaveInput) *model.ModelBundle {
	b := &model.ModelBundle{
		Model: &model.SemanticModel{
			ID:          modelID,
			TenantID:    tenantID,
			Name:        strings.TrimSpace(in.Name),
			Description: in.Description,
			Version:     version,
			Status:      status,
		},
		LogicalTables: make([]*model.LogicalTable, 0, len(in.LogicalTables)),
		Dimensions:    make([]*model.Dimension, 0, len(in.Dimensions)),
		Measures:      make([]*model.Measure, 0, len(in.Measures)),
		Metrics:       make([]*model.Metric, 0, len(in.Metrics)),
		Relations:     make([]*model.Relation, 0, len(in.Relations)),
	}
	for _, t := range in.LogicalTables {
		t.ModelID = modelID
		t.ID = s.ensureID(t.ID)
		b.LogicalTables = append(b.LogicalTables, t)
	}
	for _, d := range in.Dimensions {
		d.ModelID = modelID
		d.ID = s.ensureID(d.ID)
		b.Dimensions = append(b.Dimensions, d)
	}
	for _, m := range in.Measures {
		m.ModelID = modelID
		m.ID = s.ensureID(m.ID)
		b.Measures = append(b.Measures, m)
	}
	for _, m := range in.Metrics {
		m.ModelID = modelID
		m.ID = s.ensureID(m.ID)
		b.Metrics = append(b.Metrics, m)
	}
	for _, r := range in.Relations {
		r.ModelID = modelID
		r.ID = s.ensureID(r.ID)
		b.Relations = append(b.Relations, r)
	}
	return b
}

func (s *Service) ensureID(id string) string {
	if strings.TrimSpace(id) != "" {
		return id
	}
	return s.idGen.Generate()
}

// normalizeStatus 归一状态：合法则取之，否则回落默认。
func normalizeStatus(raw string, def model.ModelStatus) model.ModelStatus {
	switch model.ModelStatus(strings.TrimSpace(raw)) {
	case model.ModelStatusDraft:
		return model.ModelStatusDraft
	case model.ModelStatusActive:
		return model.ModelStatusActive
	case model.ModelStatusDeprecated:
		return model.ModelStatusDeprecated
	default:
		return def
	}
}

// validate 结构完整性 + 引用一致性校验（不校验表达式语义——表达式为可信管理员 SQL）。
func validate(in *SaveInput) error {
	if in == nil {
		return fmt.Errorf("%w: 空请求", ErrValidation)
	}
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: name 不能为空", ErrValidation)
	}
	if len(in.LogicalTables) == 0 {
		return fmt.Errorf("%w: 至少需要一张逻辑表", ErrValidation)
	}

	tableIDs := make(map[string]struct{}, len(in.LogicalTables))
	for i, t := range in.LogicalTables {
		if t == nil {
			return fmt.Errorf("%w: logical_tables[%d] 为空", ErrValidation, i)
		}
		if strings.TrimSpace(t.ID) == "" {
			return fmt.Errorf("%w: logical_tables[%d].id 不能为空（供维度/度量/关系引用）", ErrValidation, i)
		}
		if strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.PhysicalTable) == "" {
			return fmt.Errorf("%w: 逻辑表 %q 缺 name 或 physical_table", ErrValidation, t.ID)
		}
		if _, dup := tableIDs[t.ID]; dup {
			return fmt.Errorf("%w: 逻辑表 id %q 重复", ErrValidation, t.ID)
		}
		tableIDs[t.ID] = struct{}{}
	}

	hasTable := func(id string) bool { _, ok := tableIDs[id]; return ok }

	for i, d := range in.Dimensions {
		if d == nil || strings.TrimSpace(d.Name) == "" || strings.TrimSpace(d.Expr) == "" {
			return fmt.Errorf("%w: dimensions[%d] 缺 name 或 expr", ErrValidation, i)
		}
		if !hasTable(d.LogicalTableID) {
			return fmt.Errorf("%w: 维度 %q 的 logical_table_id %q 不存在", ErrValidation, d.Name, d.LogicalTableID)
		}
	}
	for i, m := range in.Measures {
		if m == nil || strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.Expr) == "" {
			return fmt.Errorf("%w: measures[%d] 缺 name 或 expr", ErrValidation, i)
		}
		if !hasTable(m.LogicalTableID) {
			return fmt.Errorf("%w: 度量 %q 的 logical_table_id %q 不存在", ErrValidation, m.Name, m.LogicalTableID)
		}
	}
	for i, m := range in.Metrics {
		if m == nil || strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.Expr) == "" {
			return fmt.Errorf("%w: metrics[%d] 缺 name 或 expr", ErrValidation, i)
		}
	}
	for i, r := range in.Relations {
		if r == nil || strings.TrimSpace(r.JoinCondition) == "" {
			return fmt.Errorf("%w: relations[%d] 缺 join_condition", ErrValidation, i)
		}
		if !hasTable(r.LeftTableID) || !hasTable(r.RightTableID) {
			return fmt.Errorf("%w: 关系[%d] 引用了不存在的逻辑表（left=%q right=%q）", ErrValidation, i, r.LeftTableID, r.RightTableID)
		}
	}
	return nil
}
