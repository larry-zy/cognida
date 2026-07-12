// Package tools 提供指标语义层（NL2Semantics）工具。
//
// 查询主路径：先经 semantic_models 了解受治理的指标/维度（含口径/同义词），
// 再用 semantic_query 提交结构化取数请求，由指标引擎按既定口径生成 SQL；
// 语义模型未覆盖时返回 covered=false，调用方回退词法 get_schema + sql_execute。
package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentctx "link/internal/model/agent"
	"link/internal/model/semantic"
	"link/internal/service/agent/metricsql"
	"link/internal/service/agent/semanticcache"
	"link/internal/service/agent/termgrounding"
)

// ========================================
// semantic_models：受治理指标/维度目录（grounding 面）
// ========================================

// SemanticModelsRequest 语义模型目录请求。
type SemanticModelsRequest struct {
	// Model 语义模型名（可选，空则列出租户全部生效模型的目录）
	Model string `json:"model" jsonschema:"description=语义模型名，可选，空则列出全部生效模型"`
}

// MetricCard 指标卡（供 LLM 选用的治理口径）。
type MetricCard struct {
	Name     string   `json:"name"`
	Caliber  string   `json:"caliber,omitempty"`
	Format   string   `json:"format,omitempty"`
	Synonyms []string `json:"synonyms,omitempty"`
}

// DimensionCard 维度卡。
type DimensionCard struct {
	Name     string   `json:"name"`
	DataType string   `json:"data_type,omitempty"`
	Synonyms []string `json:"synonyms,omitempty"`
}

// SemanticModelCard 一个语义模型的目录视图。
type SemanticModelCard struct {
	Name        string          `json:"name"`
	Version     int             `json:"version"`
	Description string          `json:"description,omitempty"`
	Metrics     []MetricCard    `json:"metrics"`
	Dimensions  []DimensionCard `json:"dimensions"`
}

// SemanticModelsResult 语义模型目录结果。
type SemanticModelsResult struct {
	Models []SemanticModelCard `json:"models"`
	Note   string              `json:"note,omitempty"`
}

// NewSemanticModelsTool 创建语义模型目录工具；repo 语义仓储（可为 nil）经参数注入。
func NewSemanticModelsTool(repo semantic.Repository) *TypedBaseTool[SemanticModelsRequest, SemanticModelsResult] {
	handler := func(ctx context.Context, req *SemanticModelsRequest) (*SemanticModelsResult, error) {
		return semanticModels(ctx, req, repo)
	}
	return NewTypedBaseTool("semantic_models",
		`列出受治理的指标语义模型目录（指标 + 维度 + 口径 + 同义词）。

用于在取数前了解有哪些「已治理」的指标/维度可直接按口径调用，替代对物理表的猜测。
- model: 语义模型名（可选，空则列出全部生效模型）

拿到目录后，用 semantic_query 提交结构化取数请求（指定 metrics/dimensions 的语义名）。
若目标不在目录中，改用 get_schema + sql_execute 走词法 NL2SQL。`,
		handler,
	)
}

func semanticModels(ctx context.Context, req *SemanticModelsRequest, semanticRepo semantic.Repository) (*SemanticModelsResult, error) {
	if semanticRepo == nil {
		return &SemanticModelsResult{Note: "语义层未启用，请改用 get_schema 走词法 NL2SQL"}, nil
	}
	tenantID := agentctx.MustGetTenantID(ctx)

	var bundles []*semantic.ModelBundle
	if name := strings.TrimSpace(req.Model); name != "" {
		b, err := semanticRepo.GetActiveModel(ctx, tenantID, name)
		if err != nil {
			if errors.Is(err, semantic.ErrModelNotFound) {
				return &SemanticModelsResult{Note: fmt.Sprintf("未找到生效语义模型 %q，请改用 get_schema", name)}, nil
			}
			return nil, err
		}
		bundles = append(bundles, b)
	} else {
		heads, err := semanticRepo.ListActiveModels(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		if len(heads) == 0 {
			return &SemanticModelsResult{Note: "无生效语义模型，请改用 get_schema 走词法 NL2SQL"}, nil
		}
		for _, h := range heads {
			b, err := semanticRepo.GetModelBundle(ctx, h.ID)
			if err != nil {
				continue
			}
			bundles = append(bundles, b)
		}
	}

	result := &SemanticModelsResult{Models: make([]SemanticModelCard, 0, len(bundles))}
	for _, b := range bundles {
		result.Models = append(result.Models, toModelCard(b))
	}
	return result, nil
}

// toModelCard 把 bundle 收敛为面向 LLM 的目录卡。
func toModelCard(b *semantic.ModelBundle) SemanticModelCard {
	card := SemanticModelCard{
		Name:        b.Model.Name,
		Version:     b.Model.Version,
		Description: b.Model.Description,
		Metrics:     make([]MetricCard, 0, len(b.Metrics)),
		Dimensions:  make([]DimensionCard, 0, len(b.Dimensions)),
	}
	for _, m := range b.Metrics {
		card.Metrics = append(card.Metrics, MetricCard{Name: m.Name, Caliber: m.Caliber, Format: m.Format, Synonyms: m.Synonyms})
	}
	// 度量也作为可选指标暴露（默认聚合已在引擎内处理）。
	for _, ms := range b.Measures {
		card.Metrics = append(card.Metrics, MetricCard{Name: ms.Name})
	}
	for _, d := range b.Dimensions {
		card.Dimensions = append(card.Dimensions, DimensionCard{Name: d.Name, DataType: d.DataType, Synonyms: d.Synonyms})
	}
	return card
}

// ========================================
// semantic_query：结构化取数 → 治理口径 SQL
// ========================================

// SemanticQueryRequest 结构化取数请求。
type SemanticQueryRequest struct {
	// Model 语义模型名（可选，空且仅一个生效模型时自动选用）
	Model string `json:"model" jsonschema:"description=语义模型名，可选，空且仅一个生效模型时自动选用"`
	// Metrics 指标语义名列表（可含度量名/同义词）
	Metrics []string `json:"metrics" jsonschema:"description=指标语义名列表（可用 semantic_models 中的 name 或 synonyms）"`
	// Dimensions 维度语义名列表（分组/明细）
	Dimensions []string `json:"dimensions" jsonschema:"description=维度语义名列表，用于分组"`
	// Filters 过滤条件（field=维度名，op 取 = != > >= < <= like in，values 为值）
	Filters []metricsql.Filter `json:"filters" jsonschema:"description=过滤条件，field 为维度名，op 取 = != > >= < <= like in"`
	// OrderBy 排序（field 须为已选指标/维度名）
	OrderBy []metricsql.OrderKey `json:"order_by" jsonschema:"description=排序键，field 须为已选的指标或维度名"`
	// Limit 行数上限
	Limit int `json:"limit" jsonschema:"description=返回行数上限"`
}

// SemanticQueryResult 结构化取数结果：治理口径 SQL + 覆盖报告。
type SemanticQueryResult struct {
	// Covered 是否被语义模型完整覆盖；false 表示应回退词法 NL2SQL
	Covered bool `json:"covered"`
	// SQL 治理口径下生成的 SQL（Covered=true 时非空），交由 sql_execute 执行
	SQL string `json:"sql,omitempty"`
	// DatabaseID 治理 SQL 应打向的数据源 ID（取自语义模型逻辑表的绑定）。
	// Covered=true 时非空即代表模型已显式绑定数据源，调用方 MUST 原样传给
	// sql_execute 的 database_id，避免依赖「会话隐式选库」而静默打错库。为空表示
	// 模型未绑定数据源，回落到会话选定数据源 / 业务库（向后兼容旧模型）。
	DatabaseID string `json:"database_id,omitempty"`
	// Model / ModelVersion 命中的语义模型与版本（版本供 Verified Query 缓存键，见 2a.3）
	Model        string `json:"model,omitempty"`
	ModelVersion int    `json:"model_version,omitempty"`
	// ResolvedMetrics / ResolvedDimensions 已解析的语义名
	ResolvedMetrics    []string `json:"resolved_metrics,omitempty"`
	ResolvedDimensions []string `json:"resolved_dimensions,omitempty"`
	// Uncovered 未被覆盖的名称
	Uncovered []string `json:"uncovered,omitempty"`
	// FallbackHint 未覆盖时的回退指引（可观测）
	FallbackHint string `json:"fallback_hint,omitempty"`
	// CacheHit 命中受信查询缓存（Verified/Golden）
	CacheHit bool `json:"cache_hit,omitempty"`
	// Golden 命中的是人工策展的黄金查询
	Golden bool `json:"golden,omitempty"`
	// Note 说明
	Note string `json:"note,omitempty"`
}

// NewSemanticQueryTool 创建结构化取数工具；repo 语义仓储、cache 受信缓存、grounder 术语接地器、
// sink 覆盖埋点端口经参数注入（均可为 nil）。
func NewSemanticQueryTool(repo semantic.Repository, cache semanticcache.Cache, grounder *termgrounding.Grounder, sink semantic.CoverageSink) *TypedBaseTool[SemanticQueryRequest, SemanticQueryResult] {
	handler := func(ctx context.Context, req *SemanticQueryRequest) (*SemanticQueryResult, error) {
		return semanticQuery(ctx, req, repo, cache, sink)
	}
	return NewTypedBaseTool("semantic_query",
		`按受治理的指标语义模型生成 SQL（NL2Semantics 查询主路径）。

用法：
- 先用 semantic_models 查看可用的指标/维度语义名。
- 提交 metrics（指标/度量语义名）+ dimensions（维度语义名）+ 可选 filters/order_by/limit。
- 返回治理口径下的 SQL，再交给 sql_execute 执行取数。

执行取数（重要）：covered=true 时，若返回了 database_id，调用 sql_execute 必须原样把它
传给 database_id 参数——治理 SQL 是裸物理 SQL，须显式打向该数据源，不能依赖会话隐式选库，
否则会静默查错库、拿到空结果或报表不存在。database_id 为空才回落到会话选定的数据源。

覆盖判定：任一指标/维度/过滤字段无法在语义模型中解析时返回 covered=false，
此时应回退到 get_schema + sql_execute 走词法 NL2SQL（结果会在 fallback_hint 标注）。

参数：
- model: 语义模型名（可选）
- metrics / dimensions: 语义名列表
- filters: [{field, op, values}]，op ∈ = != > >= < <= like in
- order_by: [{field, desc}]；limit: 行数上限`,
		handler,
	)
}

func semanticQuery(ctx context.Context, req *SemanticQueryRequest, semanticRepo semantic.Repository, semanticQueryCache semanticcache.Cache, sink semantic.CoverageSink) (*SemanticQueryResult, error) {
	if semanticRepo == nil {
		return &SemanticQueryResult{
			Covered:      false,
			FallbackHint: "语义层未启用，改用 get_schema + sql_execute 走词法 NL2SQL",
		}, nil
	}
	if len(req.Metrics) == 0 && len(req.Dimensions) == 0 {
		return nil, fmt.Errorf("metrics 与 dimensions 不能同时为空")
	}
	tenantID := agentctx.MustGetTenantID(ctx)

	bundle, note, err := resolveBundle(ctx, tenantID, req.Model, semanticRepo)
	if err != nil {
		return nil, err
	}
	if bundle == nil {
		// 无可用语义模型也是一次「治理未命中」：记 fallback（模型名空），使命中率分母完整。
		recordCoverage(ctx, sink, tenantID, "", semantic.CoverageFallback, nil)
		return &SemanticQueryResult{
			Covered:      false,
			Note:         note,
			FallbackHint: "无可用语义模型，改用 get_schema + sql_execute 走词法 NL2SQL",
		}, nil
	}

	// 数据源绑定：治理 SQL 是裸物理 SQL，须显式带上模型绑定的数据源 ID，
	// 让 sql_execute 打向 ecommerce_demo 等外部库，而非依赖「会话隐式选库」。
	dbID := bundleDatabaseID(bundle)

	query := metricsql.Query{
		Metrics:    req.Metrics,
		Dimensions: req.Dimensions,
		Filters:    req.Filters,
		OrderBy:    req.OrderBy,
		Limit:      req.Limit,
	}

	// 受信查询缓存：键含模型版本，版本 bump 后旧键自然失效。命中即返回受信 SQL，
	// 跳过引擎重建（黄金查询亦经此优先返回），后续仍由 sql_execute 沿用 result_id 信封。
	cacheKey := ""
	if semanticQueryCache != nil {
		cacheKey = semanticcache.BuildKey(tenantID, bundle.Model.Name, bundle.Model.Version, query)
		if entry, ok, cerr := semanticQueryCache.Get(ctx, cacheKey); cerr == nil && ok {
			recordCoverage(ctx, sink, tenantID, bundle.Model.Name, semantic.CoverageCacheHit, nil)
			return &SemanticQueryResult{
				Covered:      true,
				SQL:          entry.SQL,
				DatabaseID:   dbID,
				Model:        bundle.Model.Name,
				ModelVersion: bundle.Model.Version,
				CacheHit:     true,
				Golden:       entry.Golden,
				Note:         "命中受信查询缓存",
			}, nil
		}
	}

	out, err := metricsql.Build(bundle, query)
	if err != nil {
		return nil, err
	}

	res := &SemanticQueryResult{
		Covered:            out.Coverage.Covered,
		Model:              bundle.Model.Name,
		ModelVersion:       bundle.Model.Version,
		ResolvedMetrics:    out.Coverage.ResolvedMetrics,
		ResolvedDimensions: out.Coverage.ResolvedDimensions,
		Uncovered:          out.Coverage.Uncovered,
		Note:               strings.Join(out.Notes, "；"),
	}
	if out.Coverage.Covered {
		res.SQL = out.SQL
		res.DatabaseID = dbID
		recordCoverage(ctx, sink, tenantID, bundle.Model.Name, semantic.CoverageCovered, nil)
		// 覆盖命中的治理 SQL 写入受信缓存（Verified），供后续同签名请求复用。
		if semanticQueryCache != nil && cacheKey != "" {
			_ = semanticQueryCache.Put(ctx, cacheKey, &semanticcache.Entry{
				SQL:          out.SQL,
				ModelVersion: bundle.Model.Version,
				Verified:     true,
			}, semanticcache.DefaultTTL)
		}
	} else {
		recordCoverage(ctx, sink, tenantID, bundle.Model.Name, semantic.CoverageFallback, out.Coverage.Uncovered)
		res.FallbackHint = "存在未覆盖名称，改用 get_schema + sql_execute 走词法 NL2SQL"
	}
	return res, nil
}

// recordCoverage 以 best-effort 记一条覆盖埋点：sink 缺省则跳过，落库失败静默吞掉，
// 绝不影响 semantic_query 主路径。request_id 取全链路 rid（缺失则空），与审计/Loki 关联。
func recordCoverage(ctx context.Context, sink semantic.CoverageSink, tenantID int64, model string, outcome semantic.CoverageOutcome, uncovered []string) {
	if sink == nil {
		return
	}
	rid, _ := agentctx.GetRequestID(ctx)
	_ = sink.Record(ctx, semantic.CoverageEvent{
		TenantID:  tenantID,
		RequestID: rid,
		Model:     model,
		Outcome:   outcome,
		Uncovered: uncovered,
	})
}

// bundleDatabaseID 从语义模型的逻辑表推导本模型绑定的数据源 ID。
//
// 治理引擎生成的是单库物理 SQL（表名不带库前缀），故一个模型的全部逻辑表必须绑定
// 同一数据源，SQL 才成立。据此：取全部逻辑表中「非空且唯一」的 DatabaseID——
//   - 恰有一个非空取值 → 返回它（模型已显式绑定，sql_execute 据此显式选库）；
//   - 全空 → 返回 ""（未绑定，回落会话选定库/业务库，向后兼容旧模型）；
//   - 多个不同取值 → 返回 ""（跨库模型属建模错误，不硬猜，交由回落而非静默打错库）。
func bundleDatabaseID(bundle *semantic.ModelBundle) string {
	if bundle == nil {
		return ""
	}
	seen := ""
	for _, lt := range bundle.LogicalTables {
		if lt == nil || lt.DatabaseID == "" {
			continue
		}
		if seen == "" {
			seen = lt.DatabaseID
			continue
		}
		if seen != lt.DatabaseID {
			return "" // 跨数据源绑定不一致：不硬选，交回落
		}
	}
	return seen
}

// resolveBundle 解析目标语义模型：指定名则精确取；未指定且恰有一个生效模型则自动选用；
// 无生效模型返回 (nil, note, nil)（触发回退），多个未指定则返回 (nil, note, nil) 并提示指定。
func resolveBundle(ctx context.Context, tenantID int64, name string, semanticRepo semantic.Repository) (*semantic.ModelBundle, string, error) {
	if n := strings.TrimSpace(name); n != "" {
		b, err := semanticRepo.GetActiveModel(ctx, tenantID, n)
		if err != nil {
			if errors.Is(err, semantic.ErrModelNotFound) {
				return nil, fmt.Sprintf("未找到生效语义模型 %q", n), nil
			}
			return nil, "", err
		}
		return b, "", nil
	}
	heads, err := semanticRepo.ListActiveModels(ctx, tenantID)
	if err != nil {
		return nil, "", err
	}
	switch len(heads) {
	case 0:
		return nil, "无生效语义模型", nil
	case 1:
		b, err := semanticRepo.GetModelBundle(ctx, heads[0].ID)
		if err != nil {
			return nil, "", err
		}
		return b, "", nil
	default:
		names := make([]string, 0, len(heads))
		for _, h := range heads {
			names = append(names, h.Name)
		}
		return nil, "存在多个生效语义模型，请在 model 参数指定其一：" + strings.Join(names, ", "), nil
	}
}

// ========================================
// ground_terms：业务术语接地（意图识别消歧）
// ========================================

// GroundTermsRequest 术语接地请求。
type GroundTermsRequest struct {
	// Model 语义模型名（可选，空且仅一个生效模型时自动选用）
	Model string `json:"model" jsonschema:"description=语义模型名，可选，空且仅一个生效模型时自动选用"`
	// Terms 用户口语中的业务术语列表（如 GMV、成交额、大区）
	Terms []string `json:"terms" jsonschema:"description=用户口语中的业务术语列表，将被接地到受治理的指标/维度名"`
}

// GroundedTerm 单个术语的接地结果。
type GroundedTerm struct {
	Term       string                    `json:"term"`
	Resolved   string                    `json:"resolved,omitempty"`
	Kind       string                    `json:"kind,omitempty"`
	Ambiguous  bool                      `json:"ambiguous,omitempty"`
	Unresolved bool                      `json:"unresolved,omitempty"`
	Candidates []termgrounding.Candidate `json:"candidates,omitempty"`
}

// GroundTermsResult 术语接地结果。
type GroundTermsResult struct {
	Model        string         `json:"model,omitempty"`
	ModelVersion int            `json:"model_version,omitempty"`
	Terms        []GroundedTerm `json:"terms"`
	// NeedsClarification 存在歧义/未命中术语，应先反问用户澄清，不要硬猜
	NeedsClarification bool `json:"needs_clarification,omitempty"`
	// ClarifyPrompt 供反问的提示（列出歧义候选/未命中术语）
	ClarifyPrompt string `json:"clarify_prompt,omitempty"`
	Note          string `json:"note,omitempty"`
}

// NewGroundTermsTool 创建术语接地工具；repo 语义仓储、grounder 术语接地器经参数注入（均可为 nil）。
func NewGroundTermsTool(repo semantic.Repository, grounder *termgrounding.Grounder) *TypedBaseTool[GroundTermsRequest, GroundTermsResult] {
	handler := func(ctx context.Context, req *GroundTermsRequest) (*GroundTermsResult, error) {
		return groundTerms(ctx, req, repo, grounder)
	}
	return NewTypedBaseTool("ground_terms",
		`把用户口语中的业务术语接地到受治理的指标/维度语义名（意图识别消歧）。

在 semantic_query 取数前，用它把「GMV/成交额/大区」等口语术语映射到语义模型里的规范名：
- 先匹配模型内同义词，未命中再经知识图谱/血缘解析近义规范名。
- 唯一命中 → resolved 给出规范名，可直接用于 semantic_query。
- 命中多个不同概念（ambiguous=true）或未命中（unresolved=true）→ needs_clarification=true，
  应按 clarify_prompt 反问用户澄清，不要硬猜。

参数：
- model: 语义模型名（可选）
- terms: 业务术语列表`,
		handler,
	)
}

func groundTerms(ctx context.Context, req *GroundTermsRequest, semanticRepo semantic.Repository, termGrounder *termgrounding.Grounder) (*GroundTermsResult, error) {
	if semanticRepo == nil {
		return &GroundTermsResult{Note: "语义层未启用，无法接地，请改用 get_schema 走词法 NL2SQL"}, nil
	}
	if len(req.Terms) == 0 {
		return nil, fmt.Errorf("terms 不能为空")
	}
	tenantID := agentctx.MustGetTenantID(ctx)

	bundle, note, err := resolveBundle(ctx, tenantID, req.Model, semanticRepo)
	if err != nil {
		return nil, err
	}
	if bundle == nil {
		return &GroundTermsResult{Note: note}, nil
	}

	groundings := termGrounder.Ground(ctx, tenantID, bundle, req.Terms)

	res := &GroundTermsResult{
		Model:        bundle.Model.Name,
		ModelVersion: bundle.Model.Version,
		Terms:        make([]GroundedTerm, 0, len(groundings)),
	}
	var ambiguous, unresolved []string
	for _, g := range groundings {
		res.Terms = append(res.Terms, GroundedTerm{
			Term:       g.Term,
			Resolved:   g.Resolved,
			Kind:       string(g.Kind),
			Ambiguous:  g.Ambiguous,
			Unresolved: g.Unresolved,
			Candidates: g.Candidates,
		})
		if g.Ambiguous {
			names := make([]string, 0, len(g.Candidates))
			for _, c := range g.Candidates {
				names = append(names, c.Name)
			}
			ambiguous = append(ambiguous, fmt.Sprintf("%q 可能指：%s", g.Term, strings.Join(names, " / ")))
		}
		if g.Unresolved {
			unresolved = append(unresolved, g.Term)
		}
	}

	if len(ambiguous) > 0 || len(unresolved) > 0 {
		res.NeedsClarification = true
		var parts []string
		if len(ambiguous) > 0 {
			parts = append(parts, "以下术语存在歧义，请确认你指的是哪个：\n"+strings.Join(ambiguous, "\n"))
		}
		if len(unresolved) > 0 {
			parts = append(parts, "以下术语未能在语义模型中找到对应指标/维度，请换个说法或确认口径："+strings.Join(unresolved, "、"))
		}
		res.ClarifyPrompt = strings.Join(parts, "\n\n")
	}
	return res, nil
}
