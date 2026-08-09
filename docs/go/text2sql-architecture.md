# Text2SQL Agent 方案（基于现有编排模式）

## 一、设计思路

**利用现有编排模式，组合出 Plan-Execute-Reflect-Retry 流程**

```
┌─────────────────────────────────────────────────────────────────┐
│                    Text2SQL Agent                               │
│                  (Sequential + Retry)                           │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
        ┌───────────────────────────────────────┐
        │         Sequential Pipeline           │
        └───────────────────────────────────────┘
                │           │           │
                ▼           ▼           ▼
        ┌───────────┐ ┌───────────┐ ┌───────────┐
        │   Plan    │ │  Execute  │ │  Reflect  │
        │   Agent   │ │   Agent   │ │   Agent   │
        └───────────┘ └───────────┘ └───────────┘
                │           │           │
                └───────────┴───────────┘
                            │
                            ▼
                    ┌───────────────┐
                    │ Retry Wrapper │
                    │ (最多3次重试)  │
                    └───────────────┘
```

---

## 二、架构设计

### 2.1 核心流程

```
用户问题
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ Phase 1: Plan Agent                                             │
├─────────────────────────────────────────────────────────────────┤
│ 任务：分析问题，制定执行计划                                     │
│ 输出：{                                                        │
│   tables: ["products", "sales"],                               │
│   columns: ["name", "quantity", "amount"],                     │
│   joins: ["sales.product_id = products.id"],                   │
│   query_type: "aggregate"                                      │
│ }                                                              │
└─────────────────────────────────────────────────────────────────┘
    │ (传递计划给 Execute)
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ Phase 2: Execute Agent                                          │
├─────────────────────────────────────────────────────────────────┤
│ 任务：根据计划生成并执行 SQL                                    │
│ 流程：                                                         │
│   1. get_schema 工具获取详细结构                                │
│   2. 生成 SQL                                                   │
│   3. sql_execute 工具执行查询                                   │
│ 输出：{sql: "...", rows: [...], count: 10}                     │
└─────────────────────────────────────────────────────────────────┘
    │ (传递结果给 Reflect)
    ▼
┌─────────────────────────────────────────────────────────────────┐
│ Phase 3: Reflect Agent                                          │
├─────────────────────────────────────────────────────────────────┤
│ 任务：检查结果是否合理                                          │
│ 检查项：                                                        │
│   - 结果是否为空？                                              │
│   - 数据量是否异常？                                            │
│   - SQL 是否有问题？                                            │
│ 输出：{                                                       │
│   status: "success" | "need_retry"                             │
│   reason: "..."                                               │
│ }                                                              │
└─────────────────────────────────────────────────────────────────┘
    │
    ▼
    如果 need_retry → 返回给 Plan 重新制定计划
    如果 success → 返回结果
```

### 2.2 使用现有编排模式

```go
// 组合编排模式
text2sqlAgent := Retry(                          // 重试层
    Sequential(                                 // 顺序执行
        PlanAgent,                              // 1. 计划
        ExecuteAgent,                           // 2. 执行
        ReflectAgent,                           // 3. 反思
    ),
    maxRetries: 3,
    shouldRetry: func(resp *agent.Response) bool {
        return resp.Metadata["status"] == "need_retry"
    },
)
```

---

## 三、Agent 设计

### 3.1 Plan Agent（规划器）

```go
// internal/application/agent/text2sql/plan_agent.go

func CreatePlanAgent(toolModel model.ToolCallingChatModel, getSchemaTool tool.BaseTool) agentuc.Agent {
    return agentuc.New(toolModel).
        Name("PlanAgent").
        Prompt(`你是查询规划专家。

任务：分析用户问题，制定查询计划。

分析步骤：
1. 识别查询意图（统计/过滤/排序/聚合）
2. 识别相关表和列
3. 推断需要的 JOIN 关系

输出格式（JSON）：
{
  "query_type": "aggregate|filter|sort|simple",
  "tables": ["table1", "table2"],
  "columns": ["col1", "col2"],
  "joins": ["table1.col1 = table2.col2"],
  "suggestions": "具体建议..."
}

如果不确定表结构，使用 get_schema 工具查询。`).
        Tools(getSchemaTool).
        WithMaxIterations(3).
        Build(nil)
}
```

### 3.2 Execute Agent（执行器）

```go
// internal/application/agent/text2sql/execute_agent.go

func CreateExecuteAgent(toolModel model.ToolCallingChatModel, tools ...tool.BaseTool) agentuc.Agent {
    return agentuc.New(toolModel).
        Name("ExecuteAgent").
        Prompt(`你是 SQL 执行专家。

任务：根据查询计划生成并执行 SQL。

输入：上游的计划信息（JSON格式）
流程：
1. 解析上游计划
2. 生成 SQL（只用 SELECT，加 LIMIT）
3. 使用 sql_execute 执行
4. 返回结果

输出格式：
{
  "sql": "SELECT ...",
  "rows": [...],
  "count": 10,
  "execution_time_ms": 100
}`).
        Tools(tools...).
        WithMaxIterations(5).
        Build(nil)
}
```

### 3.3 Reflect Agent（反思器）

```go
// internal/application/agent/text2sql/reflect_agent.go

func CreateReflectAgent(toolModel model.ToolCallingChatModel) agentuc.Agent {
    return agentuc.New(toolModel).
        Name("ReflectAgent").
        Prompt(`你是结果审核专家。

任务：检查查询结果是否合理。

检查项：
1. 空结果 → 可能 SQL 有问题
2. 结果数量异常（0 或极大）→ 可能条件不对
3. 数据格式异常 → 可能列选择错误
4. 执行时间过长 → 可能需要优化

输出格式：
{
  "status": "success|need_retry",
  "reason": "判断理由...",
  "suggestion": "如果需要重试的建议"
}

规则：
- 如果结果看起来合理，status = "success"
- 如果明显有问题，status = "need_retry"`).
        WithMaxIterations(1).
        Build(nil)
}
```

---

## 四、状态传递

### 4.1 Sequential 自动传递

```go
// Sequential 模式下，前一个 Agent 的输出会传递给下一个

// Plan Agent 输出
{
  "query_type": "aggregate",
  "tables": ["sales", "products"],
  ...
}
    ↓ (自动传递)
// Execute Agent 输入 = Plan Agent 输出
// Execute Agent 输出
{
  "sql": "SELECT ...",
  "rows": [...],
  ...
}
    ↓ (自动传递)
// Reflect Agent 输入 = Execute Agent 输出
// Reflect Agent 输出
{
  "status": "success",
  "reason": "..."
}
```

### 4.2 增强版（带上下文）

```go
// 使用 Conditional 模式根据状态决定行为

text2sqlAgent := Sequential(
    PlanAgent,
    ExecuteAgent,
    Conditional(                                 // 条件分支
        func(resp *agent.Response) bool {
            return resp.Metadata["status"] == "need_retry"
        },
        PlanAgent,  // 重试时回到 Plan
        FinalAgent,  // 成功时返回结果
    ),
)
```

---

## 五、完整实现

### 5.1 注册函数

```go
// internal/application/agent/text2sql.go

package agentinit

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"

	agentuc "link/internal/application/usecases/agent"
	agentorch "link/internal/application/usecases/agent/orchestration"
	ragtool "link/internal/application/usecases/agent/tools"
)

// RegisterText2SQLAgentV2 注册 Plan-Execute-Reflect 版本
func RegisterText2SQLAgentV2(
	ctx context.Context,
	registry agent.AgentRegistry,
	toolModel model.ToolCallingChatModel,
) error {
	// 1. 获取工具
	getSchemaTool, _ := ragtool.NewGetSchemaTool()
	sqlExecuteTool, _ := ragtool.NewSQLExecuteTool()

	// 2. 创建 Agents
	planAgent := CreatePlanAgent(toolModel, getSchemaTool)
	executeAgent := CreateExecuteAgent(toolModel, getSchemaTool, sqlExecuteTool)
	reflectAgent := CreateReflectAgent(toolModel)

	// 3. 组合编排
	sequentialAgent := agentorch.Sequential(
		planAgent,
		executeAgent,
		reflectAgent,
	)

	// 4. 包装重试逻辑
	text2sqlAgent := agentorch.Retry(
		sequentialAgent,
		3, // 最多重试3次
		func(resp *agent.Response, err error) bool {
			if err != nil {
				return true // 有错误就重试
			}
			// 检查 Reflect Agent 的判断
			if status, ok := resp.Metadata["status"].(string); ok {
				return status == "need_retry"
			}
			return false
		},
	)

	// 5. 注册
	def := &agent.AgentDefinition{
		ID:          "agent-text2sql-v2",
		Name:        "text2sql_per",
		Description: "Text2SQL Agent (Plan-Execute-Reflect)",
		Type:        agent.AgentTypeAgenticRAG,
		Status:      agent.AgentStatusIdle,
		Metadata: map[string]string{
			"version": "2.0.0",
			"pattern": "sequential+retry",
			"phases":  "plan,execute,reflect",
		},
	}

	if err := registry.Register(ctx, def); err != nil {
		return err
	}

	SetText2SQLAgent(text2sqlAgent)
	return nil
}
```

### 5.2 创建 Agent 的具体实现

```go
// internal/application/agent/text2sql/agents.go

package text2sql

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"

	agentuc "link/internal/application/usecases/agent"
)

// CreatePlanAgent 创建规划 Agent
func CreatePlanAgent(toolModel model.ToolCallingChatModel, schemaTool tool.BaseTool) agentuc.Agent {
	return agentuc.New(toolModel).
		Name("PlanAgent").
		Prompt(planPrompt).
		Tools(schemaTool).
		WithMaxIterations(3).
		Build(nil)
}

const planPrompt = `你是查询规划专家。

任务：分析用户问题，制定查询计划。

分析：
1. 识别查询类型：统计/过滤/排序/聚合/简单查询
2. 识别相关表和列
3. 推断 JOIN 关系

输出 JSON 格式：
{
  "query_type": "类型",
  "tables": ["表名列表"],
  "columns": ["需要的列"],
  "joins": ["JOIN 条件"],
  "notes": "其他说明"
}

如需确认表结构，使用 get_schema 工具。`

// CreateExecuteAgent 创建执行 Agent
func CreateExecuteAgent(toolModel model.ToolCallingChatModel, tools ...tool.BaseTool) agentuc.Agent {
	return agentuc.New(toolModel).
		Name("ExecuteAgent").
		Prompt(executePrompt).
		Tools(tools...).
		WithMaxIterations(5).
		Build(nil)
}

const executePrompt = `你是 SQL 执行专家。

任务：根据上游计划生成并执行 SQL。

流程：
1. 解析上游传递的计划信息
2. 使用 get_schema 确认表结构（如需要）
3. 生成 SQL（SELECT only，加 LIMIT）
4. 使用 sql_execute 执行

输出 JSON：
{
  "sql": "执行的SQL",
  "rows": "结果数据",
  "count": "行数"
}`

// CreateReflectAgent 创建反思 Agent
func CreateReflectAgent(toolModel model.ToolCallingChatModel) agentuc.Agent {
	return agentuc.New(toolModel).
		Name("ReflectAgent").
		Prompt(reflectPrompt).
		WithMaxIterations(1).
		Build(nil)
}

const reflectPrompt = `你是结果审核专家。

任务：检查查询结果是否合理。

检查：
- 空结果可能表示查询条件有问题
- 结果数量异常多可能有条件遗漏
- 数据格式不对可能列选错了

输出 JSON：
{
  "status": "success 或 need_retry",
  "reason": "判断理由"
}

规则：有疑问就标记 need_retry，让系统重试。`
```

---

## 六、多轮对话支持

### 6.1 使用现有 Session 机制

```go
// ChatService 自动管理 Session
// 每个 request 带 session_id 即可

// 第一轮
POST /api/v1/chat
{
  "agent_id": "agent-text2sql-v2",
  "session_id": "session-001",
  "messages": [{"role": "user", "content": "查询所有用户"}]
}

// 第二轮（追问）
POST /api/v1/chat
{
  "agent_id": "agent-text2sql-v2",
  "session_id": "session-001",  // 同一个 session
  "messages": [
    {"role": "user", "content": "按年龄排序"}
  ]
}

// Plan Agent 可以从历史中获取上次查询
// "上次查询了所有用户，现在需要按年龄排序"
// → 生成新 SQL
```

### 6.2 增强版 Prompt

```go
// Plan Agent 增加多轮支持
const planPromptWithHistory = `你是查询规划专家，支持多轮对话。

任务：分析用户问题，制定查询计划。

多轮对话规则：
- 如果用户说"排序X"、"只看X"、"统计X"，这是追问
- 从对话历史中找到上次查询
- 在上次查询基础上修改

输出格式不变。`
```

---

## 七、目录结构

```
cognida-go/internal/application/agent/text2sql/
├── text2sql.go              # 注册函数
├── agents.go                # Agent 创建函数
└── prompts.go               # Prompt 定义（可选）

修改文件：
├── init.go                  # 添加 RegisterText2SQLAgentV2 调用
```

---

## 八、实现计划

| 任务 | 内容 | 预计时间 |
|------|------|---------|
| 1 | 创建 text2sql 包结构 | 0.5小时 |
| 2 | 实现 Plan/Execute/Reflect Agent | 2小时 |
| 3 | 实现注册函数 | 1小时 |
| 4 | 修改 init.go | 0.5小时 |
| 5 | 测试基本流程 | 1小时 |
| 6 | 测试重试逻辑 | 1小时 |
| 7 | 测试多轮对话 | 1小时 |
| **总计** | | **7小时** |

---

## 九、核心优势

| 特性 | 说明 |
|------|------|
| **零新增框架** | 使用现有 Sequential + Retry |
| **清晰流程** | Plan → Execute → Reflect |
| **自动重试** | Reflect 发现问题自动重试 |
| **状态传递** | Sequential 自动传递结果 |
| **多轮支持** | 基于现有 Session |
| **易扩展** | 可插入更多 Agent |

---

## 十、流程图

```
┌─────────────────────────────────────────────────────────────────┐
│                        用户请求                                   │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Retry Wrapper (max=3)                         │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ▼
        ┌───────────────────────────────────────────────┐
        │           Sequential Pipeline                 │
        └───────────────────────────────────────────────┘
    ┌───────────┐   ┌───────────┐   ┌───────────┐
    │   Plan    │ → │  Execute  │ → │  Reflect  │
    │   Agent   │   │   Agent   │   │   Agent   │
    └───────────┘   └───────────┘   └───────────┘
            │               │               │
            ▼               ▼               ▼
        制定计划      生成并执行SQL    检查结果
            │               │               │
            └───────────────┴───────────────┘
                            │
            ┌───────────────┴───────────────┐
            ▼                               ▼
        status=success                  status=need_retry
            │                               │
            ▼                               ▼
        返回结果                      回到 Retry (重试)
```
