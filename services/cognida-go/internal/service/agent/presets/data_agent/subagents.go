// 数据域子代理预设（Phase 7 任务 8.1/8.1a/8.2）：orchestrator-worker 分工。
//
// 叶子子代理各自只挂最小工具集（SchemaExplorer/SQLAuthor/Analysis/Viz 只读，
// Operation 唯一持写）；Insight/Report 为分层编排者，不持任何直接工具，
// 仅通过委派信封调度下层子代理。全部子代理经 RegisterGoverned 登记治理目录
// {purpose, data_scope, tools, risk_class}，与 agent_operation_audit 委派留痕串联。
package dataagent

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"

	domainagent "cognida/internal/model/agent"
	infraagent "cognida/internal/service/agent/framework"
	toolregistry "cognida/internal/service/agent/tools"
)

// subAgentReturnContract 所有子代理共用的回传契约（只回传 handle/摘要，防上下文膨胀）。
const subAgentReturnContract = `

## 回传契约（必须遵守）
- 只回传紧凑结论：result_id（如有）+ 一段结论摘要。
- 不要罗列内部执行过程、中间推理或工具调用细节。
- 不要粘贴大规模原始数据；数据一律以 result_id 句柄引用。`

// subAgentSpec 单个子代理的声明式规格。
type subAgentSpec struct {
	id          string
	description string
	prompt      string
	toolNames   []string // 最小工具集（按名解析，present-if-registered）
	maxIter     int
	mode        domainagent.CollaborationContextMode
	purpose     string
	dataScope   string
	riskClass   string
	// delegates 为 true 时不挂直接工具，改挂委派工具（分层编排者）。
	delegates bool
}

// leafSubAgentSpecs 叶子子代理：各自最小工具集，仅 Operation 持写（任务 8.2）。
var leafSubAgentSpecs = []subAgentSpec{
	{
		id:          "SchemaExplorer",
		description: "表结构探查专家：按业务问题定位相关表并解读结构，只读元数据",
		prompt: `你是表结构探查专家（SchemaExplorer）。
职责：根据委派任务定位相关业务表，解读字段含义、主外键与关联关系，给出选表建议。
你只有 get_schema 一个工具，不能查询业务数据，也不能修改任何东西。`,
		toolNames: []string{"get_schema"},
		maxIter:   4,
		mode:      domainagent.ContextModeIsolated,
		purpose:   "探查表结构与选表建议",
		dataScope: "业务库表结构元数据（只读）",
		riskClass: infraagent.ScopeRead,
	},
	{
		id:          "SQLAuthor",
		description: "SQL 撰写与执行专家：将问题转成只读查询并执行，回传 result_id",
		prompt: `你是 SQL 撰写与执行专家（SQLAuthor）。
职责：根据委派任务撰写只读 SELECT 查询并用 sql_execute 执行，必要时先用 get_schema 确认表结构。
遵守委派约束中的行数上限（LIMIT）。你不能执行任何写操作。`,
		toolNames: []string{"get_schema", "sql_execute"},
		maxIter:   6,
		mode:      domainagent.ContextModeIsolated,
		purpose:   "自然语言转只读 SQL 并执行",
		dataScope: "业务库数据（只读查询）",
		riskClass: infraagent.ScopeRead,
	},
	{
		id:          "Analysis",
		description: "数据分析专家：按 result_id 对既有结果做统计/趋势/对比分析",
		prompt: `你是数据分析专家（Analysis）。
职责：对委派输入中的 result_id 指向的既有结果执行 data_analysis（统计、趋势、对比、异常检测），
提炼可直接使用的分析结论。你不能执行 SQL，只能基于既有结果分析。`,
		toolNames: []string{"data_analysis"},
		maxIter:   6,
		mode:      domainagent.ContextModeSummary,
		purpose:   "对既有结果做统计分析",
		dataScope: "Result Store 既有结果（按 result_id 引用）",
		riskClass: infraagent.ScopeRead,
	},
	{
		id:          "Operation",
		description: "数据操作专家：DML 写库、ETL 派生、数据导出（唯一持写子代理，高危受审计与确认约束）",
		prompt: `你是数据操作专家（Operation），数据域内唯一持写工具的子代理。
职责：按委派任务执行 sql_mutate（DML 写库）、etl_run（派生 agent_etl_ 新对象）、data_export（按 result_id 导出）。
红线：不碰原始业务表结构，ETL 只派生新对象；危险操作会被暂停等待人工确认，如实回传该状态。
若委派授予的 scope 不足以执行请求的操作，直接说明并拒绝，不要绕过。`,
		toolNames: []string{"sql_mutate", "etl_run", "data_export"},
		maxIter:   6,
		mode:      domainagent.ContextModeSummary,
		purpose:   "受控执行写/ETL/导出操作",
		dataScope: "业务库 DML + agent_etl_ 派生对象 + 结果导出",
		riskClass: infraagent.ScopeWrite,
	},
	{
		id:          "Viz",
		description: "可视化专家：按 result_id 生成 A2UI 图表/表格界面",
		prompt: `你是可视化专家（Viz）。
职责：对委派输入中的 result_id 指向的结果调用 render_ui，生成「有解读」的生成式 UI，而不是只堆指标卡+表格。
渲染要求：
- 传自定义 components（扁平邻接表，唯一 root），别留空退回默认模板；
- 首个组件用 Callout 写一句最关键的结论/洞察（tone 按语气取 info/success/warning/error），必要时用 Text 补充叙述或小标题；
- 用 MetricCard/LineChart/BarChart/ScatterChart/Table 承载数字，所有数字一律用 {"path":"/..."} 绑定，禁止内联具体数值；
- 结果行多或可按维度筛选时，加 Filter/Pagination 交互组件。
你不能查询或修改数据，只能渲染既有结果。`,
		toolNames: []string{"render_ui"},
		maxIter:   4,
		mode:      domainagent.ContextModeSummary,
		purpose:   "既有结果的生成式 UI 渲染",
		dataScope: "Result Store 既有结果（渲染只读）",
		riskClass: infraagent.ScopeRead,
	},
}

// orchestratorSubAgentSpecs 分层编排者（任务 8.1a）：不持直接工具，只委派下层。
// 注册顺序在叶子之后：委派工具的可用代理清单在 Info 时动态生成，不依赖构建顺序，
// 但 Insight 先于 Report 注册可保证 Report 可见 Insight。
var orchestratorSubAgentSpecs = []subAgentSpec{
	{
		id:          "Insight",
		description: "洞察编排者：拆解分析目标，委派 SQLAuthor 取数、Analysis 分析，综合出洞察结论",
		prompt: `你是洞察编排者（Insight）。你没有任何直接数据工具，只能通过委派工具调度子代理：
- 需要取数时委派 SQLAuthor（回传 result_id）；
- 需要对结果做统计/趋势分析时委派 Analysis（输入携 result_id）；
- 相互独立的取数/分析子任务用 delegate_parallel 并行委派。
每次委派必须携完整信封（goal + constraints.scope），只授予完成该子任务所需的最小 scope（通常 read）。
最后综合各子结论，输出结构化洞察。`,
		toolNames: []string{"delegate_to_agent", "delegate_parallel"},
		maxIter:   8,
		mode:      domainagent.ContextModeSummary,
		purpose:   "编排取数与分析，产出数据洞察",
		dataScope: "经由子代理间接访问（自身无直接数据权限）",
		riskClass: infraagent.ScopeRead,
		delegates: true,
	},
	{
		id:          "Report",
		description: "报告编排者：拆解报告章节，委派 Insight 产出各节洞察，汇总成完整报告",
		prompt: `你是报告编排者（Report）。你没有任何直接数据工具，只能通过委派工具调度子代理：
- 将报告拆成若干洞察主题，逐一（或对独立主题用 delegate_parallel 并行）委派 Insight；
- 每次委派携完整信封（goal + constraints.scope=read）。
最后把各节洞察组织成结构清晰的报告（含结论与依据 result_id 引用）。`,
		toolNames: []string{"delegate_to_agent", "delegate_parallel"},
		maxIter:   8,
		mode:      domainagent.ContextModeSummary,
		purpose:   "编排多主题洞察，汇总数据报告",
		dataScope: "经由 Insight 间接访问（自身无直接数据权限）",
		riskClass: infraagent.ScopeRead,
		delegates: true,
	},
}

// resolveSubAgentTools 按名解析最小工具集（present-if-registered：未注册的工具跳过）。
func resolveSubAgentTools(registry *toolregistry.ToolRegistry, names []string) (resolved []tool.BaseTool, missing []string) {
	for _, name := range names {
		t, ok := registry.Get(name)
		if !ok {
			missing = append(missing, name)
			continue
		}
		resolved = append(resolved, t)
	}
	return resolved, missing
}

// buildSubAgent 按规格构建一个子代理实例。
func buildSubAgent(
	ctx context.Context,
	spec *subAgentSpec,
	collabRegistry *infraagent.CollaborationRegistry,
	toolModel model.ToolCallingChatModel,
	registry *toolregistry.ToolRegistry,
) (infraagent.Agent, error) {
	builder := infraagent.New(nil).
		Name(spec.id).
		Prompt(spec.prompt + subAgentReturnContract).
		WithToolModel(toolModel).
		WithMaxIterations(spec.maxIter)

	if spec.delegates {
		// 分层编排者：只挂委派工具（delegate_to_agent + delegate_parallel）
		builder = builder.WithCollaboration(collabRegistry, infraagent.EnableDelegate())
	} else {
		resolved, missing := resolveSubAgentTools(registry, spec.toolNames)
		if len(resolved) == 0 {
			return nil, fmt.Errorf("子代理 %s 的最小工具集均未注册: %v", spec.id, spec.toolNames)
		}
		if len(missing) > 0 {
			log.Printf("[Agent] 子代理 %s 部分工具未注册（跳过）: %v", spec.id, missing)
		}
		builder = builder.Tools(resolved...)
	}

	return builder.Build(ctx)
}

// RegisterDataSubAgents 构建并注册全部数据域子代理到协作注册表（任务 8.1）。
// 叶子先注册（最小工具集），再注册 Insight/Report 分层编排者；每个子代理
// 携治理目录项与默认上下文模式（SchemaExplorer/SQLAuthor isolated，其余 summary）。
func RegisterDataSubAgents(
	ctx context.Context,
	collabRegistry *infraagent.CollaborationRegistry,
	toolModel model.ToolCallingChatModel,
	registry *toolregistry.ToolRegistry,
) error {
	if collabRegistry == nil {
		return fmt.Errorf("数据域子代理需要协作注册表")
	}
	if toolModel == nil {
		return fmt.Errorf("数据域子代理需要 ToolCallingChatModel")
	}
	if registry == nil {
		return fmt.Errorf("数据域子代理需要工具注册表")
	}

	register := func(spec *subAgentSpec) error {
		subAgent, err := buildSubAgent(ctx, spec, collabRegistry, toolModel, registry)
		if err != nil {
			return fmt.Errorf("构建子代理 %s 失败: %w", spec.id, err)
		}
		collabRegistry.RegisterGoverned(spec.id, subAgent, spec.description,
			&infraagent.AgentGovernance{
				Purpose:   spec.purpose,
				DataScope: spec.dataScope,
				Tools:     spec.toolNames,
				RiskClass: spec.riskClass,
			}, spec.mode)
		return nil
	}

	for i := range leafSubAgentSpecs {
		spec := &leafSubAgentSpecs[i]
		if err := register(spec); err != nil {
			// 叶子工具分组可能未初始化（如渲染/操作组未注入）：跳过该子代理，
			// 不阻断其余子代理注册（与 commander present-if-registered 同策略）。
			log.Printf("[Agent] %v（已跳过）", err)
			continue
		}
	}
	for i := range orchestratorSubAgentSpecs {
		if err := register(&orchestratorSubAgentSpecs[i]); err != nil {
			return err
		}
	}
	return nil
}
