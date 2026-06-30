// Package text2sql 提供语义驱动的 Text2SQL Agent
package text2sql

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"

	domainagent "link/internal/model/agent"
	infraagent "link/internal/service/agent/framework"
	agentorch "link/internal/service/agent/orchestration"
	toolregistry "link/internal/service/agent/tools"
)

// ========================================
// Prompt 定义
// ========================================

const (
	// planPrompt 规划 Agent 的提示词
	planPrompt = `你是查询规划专家，负责将用户问题转换为 SQL 查询计划。

【重要】工作流程：
1. 首先判断用户输入类型：
   - 如果是问候语（hello、你好、在吗等）：直接友好回复，不要调用任何工具
   - 如果是闲聊：礼貌回复说明你的功能是 Text2SQL 查询
   - 如果是数据查询：继续执行规划流程

2. 对于数据查询，执行以下分析：
   - 识别查询类型：aggregate（聚合）、filter（过滤）、sort（排序）、simple（简单查询）
   - 识别相关表和列
   - 推断 JOIN 关系

3. 工具使用规则：
   - 仅在需要了解表结构时使用 get_schema 工具
   - 不要为简单问候调用任何工具

回复格式要求：
- 问候/闲聊：直接用自然语言回复
- 数据查询：输出规划 JSON
{
  "query_type": "aggregate|filter|sort|simple",
  "tables": ["table1", "table2"],
  "columns": ["col1", "col2"],
  "joins": ["table1.col1 = table2.col2"],
  "notes": "其他说明"
}

【注意事项】
- 用户说 "hello"、"你好" 时，回复问候语并介绍功能
- 用户说 "完成"、"ok"、"好的" 时，友好回复并等待下一步指示
- 不要过度使用工具，只在必要时调用`

	// executePrompt 执行 Agent 的提示词
	executePrompt = `你是 SQL 执行专家，负责根据计划生成并执行 SQL 查询。

【重要】输入处理：
- 如果上游传递的是问候语或闲聊：直接友好回复，不要执行任何 SQL
- 如果上游传递的是计划 JSON：执行 SQL 查询

SQL 执行流程：
1. 仅在需要详细了解表结构时使用 get_schema 工具
2. 根据计划生成 SQL：
   - 只生成 SELECT 查询
   - 使用反引号包裹表名和列名
   - 添加 LIMIT 限制结果（默认100，最多1000）
3. 使用 sql_execute 执行 SQL
4. 取数→分析→结论：当查询返回多行数值/时间序列数据，且用户意图涉及趋势、对比、
   异常、相关性或需要给出有依据的结论/建议时，把 sql_execute 的行集（{columns, rows}）
   作为 data 传给 data_analysis 工具做真实量化分析：
   - 趋势/增长 → analysis_type=trend（options.value_col，可选 time_col）
   - 异常点 → analysis_type=anomaly（options.value_col）
   - 多指标关系 → analysis_type=correlation
   - 分布/统计概览 → analysis_type=describe
   - 综合洞察+行动建议 → analysis_type=insight
   分析结论必须基于 data_analysis 的计算输出，不要凭空叙述。

结果输出格式：
- 成功：说明查询结果，提供数据摘要；若已做量化分析，给出有计算依据的结论与建议
- 失败：说明错误原因和改进建议

【工具使用原则】
- 不要盲目调用 get_schema，仅在不确定表结构时使用
- 简单查询可以直接生成 SQL，无需先查 schema
- 单值/单行结果（如 COUNT、单条记录）无需调用 data_analysis`

	// reflectPrompt 反思 Agent 的提示词
	reflectPrompt = `你是结果审核专家，负责检查查询结果的合理性。

【重要】输入处理：
- 如果上游是问候/闲聊回复：直接返回原内容
- 如果上游是 SQL 执行结果：进行质量审核

SQL 结果检查项：
1. 空结果 → 可能查询条件有问题，或数据确实不存在
2. 结果数量异常（如超过1000条被截断）→ 可能需要更精确的条件
3. 执行失败（语法错误、表不存在等）→ 需要重新规划
4. 执行超时 → 可能需要优化查询

审核结果格式：
{
  "status": "success 或 need_retry",
  "reason": "判断理由",
  "suggestion": "如果需要重试的建议（可选）"
}

【判断规则】
- 执行成功且有合理结果 → status = "success"
- 执行失败或结果不合理 → status = "need_retry"
- 有疑问时倾向于 "need_retry"，让系统有机会重试`
)

// ========================================
// Agent 创建函数
// ========================================

// createPlanAgent 创建规划 Agent
// 职责：分析用户问题，制定查询计划
func createPlanAgent(toolModel model.ToolCallingChatModel, schemaTool tool.BaseTool) (infraagent.Agent, error) {
	return infraagent.New(toolModel).
		Name("PlanAgent").
		Prompt(planPrompt).
		Tools(schemaTool).
		WithMaxIterations(3).
		Build(nil)
}

// createExecuteAgent 创建执行 Agent
// 职责：根据计划生成并执行 SQL
func createExecuteAgent(toolModel model.ToolCallingChatModel, tools ...tool.BaseTool) (infraagent.Agent, error) {
	return infraagent.New(toolModel).
		Name("ExecuteAgent").
		Prompt(executePrompt).
		Tools(tools...).
		WithMaxIterations(5).
		Build(nil)
}

// createReflectAgent 创建反思 Agent
// 职责：检查结果是否合理，决定是否重试
func createReflectAgent(toolModel model.ToolCallingChatModel) (infraagent.Agent, error) {
	return infraagent.New(toolModel).
		Name("ReflectAgent").
		Prompt(reflectPrompt).
		WithMaxIterations(1).
		Build(nil)
}

// ========================================
// Agent 注册
// ========================================

// RegisterText2SQLAgent 注册语义驱动的 Text2SQL Agent
func RegisterText2SQLAgent(
	ctx context.Context,
	registry domainagent.AgentRegistry,
	toolModel model.ToolCallingChatModel,
) error {
	// 1. 从全局注册表获取工具
	getSchemaTool, ok := toolregistry.GetTool("get_schema")
	if !ok || getSchemaTool == nil {
		return fmt.Errorf("获取 get_schema 工具失败: 工具未注册")
	}

	sqlExecuteTool, ok := toolregistry.GetTool("sql_execute")
	if !ok || sqlExecuteTool == nil {
		return fmt.Errorf("获取 sql_execute 工具失败: 工具未注册")
	}

	// data_analysis 为可选增强：取数后做量化分析，缺失时降级为纯取数。
	executeTools := []tool.BaseTool{getSchemaTool, sqlExecuteTool}
	if dataAnalysisTool, ok := toolregistry.GetTool("data_analysis"); ok && dataAnalysisTool != nil {
		executeTools = append(executeTools, dataAnalysisTool)
	}

	// 2. 创建 PER 查询管道（Plan → Execute → Reflect）
	planAgent, err := createPlanAgent(toolModel, getSchemaTool)
	if err != nil {
		return fmt.Errorf("创建 Plan Agent 失败: %w", err)
	}

	executeAgent, err := createExecuteAgent(toolModel, executeTools...)
	if err != nil {
		return fmt.Errorf("创建 Execute Agent 失败: %w", err)
	}

	reflectAgent, err := createReflectAgent(toolModel)
	if err != nil {
		return fmt.Errorf("创建 Reflect Agent 失败: %w", err)
	}

	// 3. 串联为查询管道
	queryPipeline := agentorch.Sequential(
		planAgent,
		executeAgent,
		reflectAgent,
	)

	// 4. 注册到注册中心
	def := &domainagent.AgentDefinition{
		ID:          "agent-text2sql-per",
		Name:        "text2sql_per",
		Description: "Text2SQL Agent with semantic intent detection (Plan-Execute-Reflect pattern)",
		Type:        domainagent.AgentTypeAgenticRAG,
		Status:      domainagent.AgentStatusIdle,
		Metadata: map[string]string{
			"version": "4.0.0",
			"pattern": "sequential+semantic",
			"phases":  "plan,execute,reflect",
		},
	}

	if err := registry.Register(ctx, def); err != nil {
		return fmt.Errorf("注册 Text2SQL Agent 失败: %w", err)
	}

	// 5. 保存 Agent 实例
	return SetAgent(queryPipeline)
}

// ========================================
// Agent 实例管理
// ========================================

var (
	text2sqlAgentInstance infraagent.Agent
)

// SetAgent 设置 Agent 实例
func SetAgent(agent infraagent.Agent) error {
	text2sqlAgentInstance = agent
	return nil
}

// GetAgent 获取 Agent 实例
func GetAgent() infraagent.Agent {
	return text2sqlAgentInstance
}
