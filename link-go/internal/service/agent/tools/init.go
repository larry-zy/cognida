// Package tools 提供工具自动初始化
package tools

import (
	"log"
)

// init 包初始化时自动注册所有工具
func init() {
	if err := InitializeTools(); err != nil {
		log.Printf("[警告] 工具初始化失败: %v", err)
	}
}

// InitializeTools 初始化所有工具到全局注册表
func InitializeTools() error {
	// RAG 工具
	if err := registerRAGTools(); err != nil {
		return err
	}

	// SQL 工具
	if err := registerSQLTools(); err != nil {
		return err
	}

	// Web 工具
	if err := registerWebTools(); err != nil {
		return err
	}

	// 知识库工具
	if err := registerKBTools(); err != nil {
		return err
	}

	// 数据查询工具
	if err := registerDataTools(); err != nil {
		return err
	}

	// 指标语义层工具（NL2Semantics）
	if err := registerSemanticTools(); err != nil {
		return err
	}

	// 图谱工具
	if err := registerGraphTools(); err != nil {
		return err
	}

	// 数据分析工具（经 MCP 调用 Python analytics 引擎）
	if err := registerAnalyticsTools(); err != nil {
		return err
	}

	// 渲染工具（render_ui：结果集 → A2UI 规格）
	if err := registerRenderTools(); err != nil {
		return err
	}

	// 操作工具（sql_mutate / etl_run / data_export：写、派生、导出）
	if err := registerOperationTools(); err != nil {
		return err
	}

	// Skill 工具（工具发现和推荐）
	if err := registerSkillTools(); err != nil {
		return err
	}

	log.Printf("[工具注册] 已注册 %d 个工具，分组: %v",
		GlobalRegistry.Size(), GlobalRegistry.ListGroups())

	return nil
}

// registerRAGTools 注册 RAG 工具
func registerRAGTools() error {
	ragTool := NewRAGQueryTool()
	if ragTool == nil {
		return nil // RAG 工具可能未配置
	}
	return GlobalRegistry.Register("rag", ragTool)
}

// registerSQLTools 注册 SQL 工具
func registerSQLTools() error {
	// get_schema
	getSchemaTool := NewGetSchemaTool()
	if getSchemaTool != nil {
		if err := GlobalRegistry.Register("sql", getSchemaTool); err != nil {
			return err
		}
	}

	// sql_execute
	sqlExecuteTool := NewSQLExecuteTool()
	if sqlExecuteTool != nil {
		if err := GlobalRegistry.Register("sql", sqlExecuteTool); err != nil {
			return err
		}
	}

	return nil
}

// registerWebTools 注册 Web 工具
func registerWebTools() error {
	// web_search
	webSearchTool := NewWebSearchTool()
	if webSearchTool != nil {
		if err := GlobalRegistry.Register("web", webSearchTool); err != nil {
			return err
		}
	}

	// fetch_url
	fetchURLTool := NewFetchURLTool()
	if fetchURLTool != nil {
		if err := GlobalRegistry.Register("web", fetchURLTool); err != nil {
			return err
		}
	}

	// search_multi
	searchMultiTool := NewSearchMultiTool()
	if searchMultiTool != nil {
		if err := GlobalRegistry.Register("web", searchMultiTool); err != nil {
			return err
		}
	}

	return nil
}

// registerKBTools 注册知识库工具
//
// 检索范围由用户在会话入口选定并经 ctx 强制透传，rag_query 会自动在选定范围内多库检索，
// 因此不再需要 kb_select（LLM 选库）与 rag_query_multi（LLM 传库列表）。仅保留 kb_list 供查看/说明。
func registerKBTools() error {
	// kb_list
	kbListTool := NewKbListTool()
	if kbListTool != nil {
		if err := GlobalRegistry.Register("kb", kbListTool); err != nil {
			return err
		}
	}

	// kb_route：让 Agent 在结合/智能模式下自主聚焦检索范围（写入 ctx 路由 holder，无需外部服务）。
	kbRouteTool := NewKbRouteTool()
	if kbRouteTool != nil {
		if err := GlobalRegistry.Register("kb", kbRouteTool); err != nil {
			return err
		}
	}

	return nil
}

// registerDataTools 注册数据查询工具
// data_query 已移除，使用 sql_execute 代替
func registerDataTools() error {
	// 数据查询功能由 sql_execute 工具提供
	return nil
}

// registerSemanticTools 注册指标语义层工具（semantic_models / semantic_query）
//
// 工具无需仓储即可注册；真实语义模型仓储由组合根通过 InitSemanticTools 注入。
// 未注入时工具报告语义层未启用并提示回退词法 NL2SQL。
func registerSemanticTools() error {
	modelsTool := NewSemanticModelsTool()
	if modelsTool != nil {
		if err := GlobalRegistry.Register("semantic", modelsTool); err != nil {
			return err
		}
	}
	queryTool := NewSemanticQueryTool()
	if queryTool != nil {
		if err := GlobalRegistry.Register("semantic", queryTool); err != nil {
			return err
		}
	}
	groundTool := NewGroundTermsTool()
	if groundTool != nil {
		if err := GlobalRegistry.Register("semantic", groundTool); err != nil {
			return err
		}
	}
	return nil
}

// registerGraphTools 注册图谱工具
func registerGraphTools() error {
	graphQueryTool := NewGraphQueryTool()
	if graphQueryTool != nil {
		return GlobalRegistry.Register("graph", graphQueryTool)
	}
	return nil
}

// registerAnalyticsTools 注册数据分析工具（data_analysis）
//
// 工具本身无需 MCP 客户端即可注册；真实 MCP 调用器由组合根通过
// InitDataAnalysisTool 注入。未注入时调用返回非致命错误结果。
func registerAnalyticsTools() error {
	dataAnalysisTool, err := NewDataAnalysisTool()
	if err != nil {
		return err
	}
	if dataAnalysisTool != nil {
		if err := GlobalRegistry.Register("analytics", dataAnalysisTool); err != nil {
			return err
		}
	}
	return nil
}

// registerRenderTools 注册渲染工具（render_ui）
//
// 工具无需 Result Store 即可注册；真实存储由组合根经 InitResultStore 注入
// （与 sql_execute 共享）。未注入时调用返回"结果存储未启用"错误。
func registerRenderTools() error {
	renderUITool := NewRenderUITool()
	if renderUITool != nil {
		if err := GlobalRegistry.Register("render", renderUITool); err != nil {
			return err
		}
	}
	return nil
}

// registerOperationTools 注册操作工具（sql_mutate / etl_run / data_export）
//
// 工具无需配置即可注册；写库、审计仓储、待确认存储由组合根经
// InitOperationTools 注入。未注入时调用返回不可用错误（宁拒不闯）。
func registerOperationTools() error {
	mutateTool := NewSQLMutateTool()
	if mutateTool != nil {
		if err := GlobalRegistry.Register("operation", mutateTool); err != nil {
			return err
		}
	}
	etlTool := NewETLRunTool()
	if etlTool != nil {
		if err := GlobalRegistry.Register("operation", etlTool); err != nil {
			return err
		}
	}
	exportTool := NewDataExportTool()
	if exportTool != nil {
		if err := GlobalRegistry.Register("operation", exportTool); err != nil {
			return err
		}
	}
	return nil
}

// registerSkillTools 注册 Skill 工具（工具发现和推荐）
func registerSkillTools() error {
	// skill_list - 列出所有可用工具
	skillListTool, err := NewSkillListTool()
	if err != nil {
		return err
	}
	if skillListTool != nil {
		if err := GlobalRegistry.Register("skill", skillListTool); err != nil {
			return err
		}
	}

	// skill_invoke - 智能工具调用（根据任务找到合适的工具）
	skillInvokeTool, err := NewSkillInvokeTool()
	if err != nil {
		return err
	}
	if skillInvokeTool != nil {
		if err := GlobalRegistry.Register("skill", skillInvokeTool); err != nil {
			return err
		}
	}

	return nil
}
