// Package tools 提供工具注册装配：由 NewToolRegistry 在构造期按序调用各 registerXxx 方法，
// 每个方法从 r.deps 读取显式注入的依赖并构造工具，取代原先的 init() 自动注册 + 包级全局注入。
package tools

import (
	"context"

	"link/internal/model/agent/operations"
	"link/internal/service/agent/framework"
	"link/internal/service/agent/termgrounding"
)

// registerRAGTools 注册 RAG 工具
func (r *ToolRegistry) registerRAGTools() error {
	ragTool := NewRAGQueryTool(r.deps.RAGService)
	if ragTool == nil {
		return nil // RAG 工具可能未配置
	}
	return r.Register("rag", ragTool)
}

// registerSQLTools 注册 SQL 工具
func (r *ToolRegistry) registerSQLTools() error {
	// get_schema
	getSchemaTool := NewGetSchemaTool(r.deps.GetSchemaDB, r.deps.DatasourceProvider)
	if getSchemaTool != nil {
		if err := r.Register("sql", getSchemaTool); err != nil {
			return err
		}
	}

	// sql_execute
	sqlExecuteTool := NewSQLExecuteTool(r.deps.SQLDB, r.deps.DatasourceProvider, r.deps.ResultStore)
	if sqlExecuteTool != nil {
		if err := r.Register("sql", sqlExecuteTool); err != nil {
			return err
		}
	}

	return nil
}

// registerWebTools 注册 Web 工具
func (r *ToolRegistry) registerWebTools() error {
	// web_search
	webSearchTool := NewWebSearchTool(r.deps.MetasoClient)
	if webSearchTool != nil {
		if err := r.Register("web", webSearchTool); err != nil {
			return err
		}
	}

	// fetch_url
	fetchURLTool := NewFetchURLTool()
	if fetchURLTool != nil {
		if err := r.Register("web", fetchURLTool); err != nil {
			return err
		}
	}

	// search_multi
	searchMultiTool := NewSearchMultiTool(r.deps.MetasoClient)
	if searchMultiTool != nil {
		if err := r.Register("web", searchMultiTool); err != nil {
			return err
		}
	}

	return nil
}

// registerKBTools 注册知识库工具
//
// 检索范围由用户在会话入口选定并经 ctx 强制透传，rag_query 会自动在选定范围内多库检索，
// 因此不再需要 kb_select（LLM 选库）与 rag_query_multi（LLM 传库列表）。仅保留 kb_list 供查看/说明。
func (r *ToolRegistry) registerKBTools() error {
	// kb_list
	kbListTool := NewKbListTool(r.deps.KnowledgeBaseRepo)
	if kbListTool != nil {
		if err := r.Register("kb", kbListTool); err != nil {
			return err
		}
	}

	// kb_route：让 Agent 在结合/智能模式下自主聚焦检索范围（写入 ctx 路由 holder，无需外部服务）。
	kbRouteTool := NewKbRouteTool()
	if kbRouteTool != nil {
		if err := r.Register("kb", kbRouteTool); err != nil {
			return err
		}
	}

	return nil
}

// registerDataTools 注册数据查询工具
// data_query 已移除，使用 sql_execute 代替
func (r *ToolRegistry) registerDataTools() error {
	// 数据查询功能由 sql_execute 工具提供
	return nil
}

// registerSemanticTools 注册指标语义层工具（semantic_models / semantic_query）
//
// 工具无需仓储即可注册；真实语义模型仓储由组合根经 ToolDeps.SemanticRepo 注入。
// 未注入时工具报告语义层未启用并提示回退词法 NL2SQL。
func (r *ToolRegistry) registerSemanticTools() error {
	// 词条对齐器：图谱端口缺省时以 nil 图谱降级构造（仍可跑无图谱兜底逻辑）。
	grounder := termgrounding.NewGrounder(r.deps.TermGrounding)

	modelsTool := NewSemanticModelsTool(r.deps.SemanticRepo)
	if modelsTool != nil {
		if err := r.Register("semantic", modelsTool); err != nil {
			return err
		}
	}
	queryTool := NewSemanticQueryTool(r.deps.SemanticRepo, r.deps.SemanticCache, grounder, r.deps.CoverageSink, r.deps.DatasourceProvider)
	if queryTool != nil {
		if err := r.Register("semantic", queryTool); err != nil {
			return err
		}
	}
	groundTool := NewGroundTermsTool(r.deps.SemanticRepo, grounder)
	if groundTool != nil {
		if err := r.Register("semantic", groundTool); err != nil {
			return err
		}
	}
	return nil
}

// registerGraphTools 注册图谱工具
func (r *ToolRegistry) registerGraphTools() error {
	graphQueryTool := NewGraphQueryTool(r.deps.GraphService)
	if graphQueryTool != nil {
		return r.Register("graph", graphQueryTool)
	}
	return nil
}

// registerAnalyticsTools 注册数据分析工具（data_analysis）
//
// 工具本身无需 MCP 客户端即可注册；真实 MCP 调用器由组合根经
// ToolDeps.DataAnalysisInvoker 注入。未注入时调用返回非致命错误结果。
func (r *ToolRegistry) registerAnalyticsTools() error {
	dataAnalysisTool, err := NewDataAnalysisTool(r.deps.DataAnalysisInvoker, r.deps.ResultStore)
	if err != nil {
		return err
	}
	if dataAnalysisTool != nil {
		if err := r.Register("analytics", dataAnalysisTool); err != nil {
			return err
		}
	}
	return nil
}

// registerRenderTools 注册渲染工具（render_ui）
//
// 工具无需 Result Store 即可注册；真实存储由组合根经 ToolDeps.ResultStore 注入
// （与 sql_execute 共享）。未注入时调用返回"结果存储未启用"错误。
func (r *ToolRegistry) registerRenderTools() error {
	renderUITool := NewRenderUITool(r.deps.ResultStore, r.deps.UIBinding)
	if renderUITool != nil {
		if err := r.Register("render", renderUITool); err != nil {
			return err
		}
	}
	return nil
}

// registerOperationTools 注册操作工具（sql_mutate / etl_run / data_export）
//
// 工具无需配置即可注册；写库、审计仓储、待确认存储由组合根经 ToolDeps.Operation 注入。
// 未注入时调用返回不可用错误（宁拒不闯）。同时挂接硬工具门/委派审计钩子（闭包持有 o）。
func (r *ToolRegistry) registerOperationTools() error {
	// 阈值归一化（<=0 用默认）后构造操作工具依赖载体，供三件操作工具的 handler 闭包共享。
	cfg := r.deps.Operation
	if cfg.DangerRowThreshold <= 0 {
		cfg.DangerRowThreshold = DefaultDangerRowThreshold
	}
	o := &opTools{cfg: cfg, resultStore: r.deps.ResultStore}
	r.ops = o // 存到注册表，供 handler 层 confirm-resume 经注册表方法复用

	// 挂接硬工具门拦截审计（Phase 6 任务 7.4）：被拒调用与写/ETL/导出共用
	// agent_operation_audit 留痕（type=tool_gate, status=rejected, result=原因）。
	framework.SetToolBlockRecorder(func(ctx context.Context, call framework.BlockedToolCall) {
		o.recordAudit(ctx, operations.OpToolGate, call.Tool, "", "",
			operations.StatusRejected, call.Reason, map[string]interface{}{
				"skill": call.Skill,
				"scope": call.Scope,
			})
	})

	// 挂接子代理委派审计（Phase 7 任务 8.6）：委派与治理目录串联留痕
	// （type=delegate, target=子代理, params 携 goal/授予 scope/风险级）。
	framework.SetDelegationRecorder(func(ctx context.Context, rec framework.DelegationRecord) {
		o.recordAudit(ctx, operations.OpDelegate, rec.Agent, "", "",
			rec.Status, rec.Detail, map[string]interface{}{
				"goal":       rec.Goal,
				"scope":      rec.Scope,
				"risk_class": rec.RiskClass,
			})
	})

	// 挂接策略级写审批处理器（guardrail-runtime 任务 4）：被标记审批的写类工具
	// 经框架门转本处理器生成 pending_action，复用 sql_mutate 危险级同一确认通道。
	framework.SetToolApprovalHandler(o.handleWriteApproval)

	// 挂接护栏审计记录器（guardrail-runtime 任务 6/7）：输入拦截/越狱/输出脱敏/
	// 逐工具脱敏/写审批各类护栏事件统一落 agent_operation_audit（type=guardrail），
	// 携 request_id/session_id/tenant_id 与事件类型、命中工具、原因。
	framework.SetGuardrailRecorder(o.recordGuardrailAudit)

	mutateTool := NewSQLMutateTool(o)
	if mutateTool != nil {
		if err := r.Register("operation", mutateTool); err != nil {
			return err
		}
	}
	etlTool := NewETLRunTool(o)
	if etlTool != nil {
		if err := r.Register("operation", etlTool); err != nil {
			return err
		}
	}
	exportTool := NewDataExportTool(o)
	if exportTool != nil {
		if err := r.Register("operation", exportTool); err != nil {
			return err
		}
	}
	return nil
}

// guardrailAuditStatus 把护栏事件类型映射到审计状态：
// 拦截类（输入/越狱）→ rejected；写审批 → pending_confirm；脱敏类 → success（脱敏后放行）。
func guardrailAuditStatus(eventType string) string {
	switch eventType {
	case framework.GuardrailEventInputBlocked, framework.GuardrailEventJailbreakBlocked:
		return operations.StatusRejected
	case framework.GuardrailEventWriteApprovalNeeded:
		return operations.StatusPendingConfirm
	default:
		// input_redacted / output_redacted / tool_output_redacted：脱敏成功放行
		return operations.StatusSuccess
	}
}

// registerSkillTools 注册 Skill 工具（工具发现和推荐）
func (r *ToolRegistry) registerSkillTools() error {
	// skill_list - 列出所有可用工具
	skillListTool, err := NewSkillListTool(r)
	if err != nil {
		return err
	}
	if skillListTool != nil {
		if err := r.Register("skill", skillListTool); err != nil {
			return err
		}
	}

	// skill_invoke - 智能工具调用（根据任务找到合适的工具）
	skillInvokeTool, err := NewSkillInvokeTool(r)
	if err != nil {
		return err
	}
	if skillInvokeTool != nil {
		if err := r.Register("skill", skillInvokeTool); err != nil {
			return err
		}
	}

	return nil
}
