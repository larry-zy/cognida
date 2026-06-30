# Deep Research Agent 深度研究助手

## 概述

Deep Research Agent 是一个专业的深度研究助手，基于 **StateGraph（状态图）** 构建多阶段的 AI 研究流程。核心思想是将复杂的研究任务拆解为多个节点，通过边连接形成有向图，实现灵活的研究流程编排。

## 架构设计

### 整体架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Deep Research Agent                                   │
│                       (基于 Eino StateGraph)                                  │
│                                                                              │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                           StateGraph                                │   │
│   │                                                                    │   │
│   │   ┌──────┐    ┌──────┐    ┌──────┐    ┌──────┐    ┌──────┐        │   │
│   │   │ Node │───▶│ Node │───▶│ Node │───▶│ Node │───▶│ Node │        │   │
│   │   └──────┘    └──────┘    └──────┘    └──────┘    └──────┘        │   │
│   │        │            │            │            │            │        │   │
│   │        ▼            ▼            ▼            ▼            ▼        │   │
│   │   ┌──────┐    ┌──────┐    ┌──────┐    ┌──────┐    ┌──────┐        │   │
│   │   │ Edge │    │ Edge │    │ Edge │    │ Edge │    │ Edge │        │   │
│   │   └──────┘    └──────┘    └──────┘    └──────┘    └──────┘        │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                         DeepResearchState                            │   │
│   │   - Query, Plan, StepResults, ResearchFindings, FinalReport          │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 工作流程图

```
                    ┌──────────────────┐
                    │      START       │
                    └────────┬─────────┘
                             ▼
                    ┌──────────────────┐
                    │   Coordinator    │  入口节点，判断是否需要深度研究
                    └────────┬─────────┘
                             ▼
                    ┌──────────────────┐
                    │ RewriteMultiQuery│  查询重写，生成多个优化的子查询
                    └────────┬─────────┘
                             ▼
                    ┌──────────────────┐
                    │  Background      │  背景调查，初步收集信息
                    │  Investigator    │
                    └────────┬─────────┘
                             ▼
                    ┌──────────────────┐
                    │     Planner      │  规划研究计划，分解为步骤
                    └────────┬─────────┘
                             ▼
                    ┌──────────────────┐
                    │  Information     │  ◀───┐
                    │  (决策节点)       │      │ 反馈循环
                    └──────┬───────────┘      │
                           │                  │
            ┌──────────────┼──────────────┐   │
            ▼              ▼              ▼   │
        ┌────────┐    ┌────────┐    ┌────────┐ │
        │Reporter│    │Human   │    │Research│ │
        │        │    │Feedback│    │Team    │─┘
        └───┬────┘    └───┬────┘    └───┬────┘
            │             │             │
            ▼             │             ▼
        ┌────────┐       │    ┌────────────┐
        │  END   │       │    │  Parallel  │
        └────────┘       │    │  Executor  │
                         │    └─────┬──────┘
                         │          │
                         │          ▼
                         │    ┌────────────────┐
                         │    │  Researcher_N  │ (并行多个)
                         │    │  Researcher_N  │
                         │    └───────┬────────┘
                         │            │
                         └────────────┘
```

## 模块结构

### 代码组织

```
internal/
├── types/
│   └── deep_research.go              # 类型定义 (State, Plan, Step, Report 等)
│
└── application/service/agent/deep_research/
    │
    ├── agent.go                      # 对外接口，提供 Run/RunStream 方法
    ├── graph.go                      # Graph 编排，构建 StateGraph
    ├── state.go                      # 状态管理，DeepResearchState 定义
    ├── config.go                     # 配置管理
    │
    ├── nodes/                        # 节点目录 (每个节点独立文件)
    │   ├── coordinator.go            # CoordinatorNode - 入口节点
    │   ├── rewrite_multi_query.go    # RewriteAndMultiQueryNode - 查询重写
    │   ├── background_investigator.go # BackgroundInvestigationNode - 背景调查
    │   ├── planner.go                # PlannerNode - 研究规划
    │   ├── information.go            # InformationNode - 决策节点
    │   ├── research_team.go          # ResearchTeamNode - 研究团队协调
    │   ├── parallel_executor.go      # ParallelExecutorNode - 并行执行调度
    │   ├── researcher.go             # ResearcherNode - 研究执行节点
    │   ├── reporter.go               # ReporterNode - 报告生成
    │   └── human_feedback.go         # HumanFeedbackNode - 人工反馈
    │
    └── prompts/                      # 提示词管理
        ├── prompts.go                # 提示词加载和渲染
        └── templates/                # 提示词模板文件
            ├── coordinator.md
            ├── rewrite.md
            ├── planner.md
            ├── researcher.md
            └── reporter.md
```

### 设计原则

| 原则 | 说明 |
|------|------|
| **节点独立** | 每个节点独立文件，职责单一 |
| **状态驱动** | 节点间通过 State 传递数据，不直接依赖 |
| **条件路由** | 支持动态决策下一个节点 |
| **并行执行** | 支持 Researcher 并行工作 |
| **可测试** | 每个节点可独立测试 |
| **可扩展** | 新增节点只需实现 NodeHandler |

## 核心类型

### State 状态管理

```go
// state/state.go
package deep_research

type DeepResearchState struct {
    mu sync.RWMutex

    // === 输入 ===
    Query    string `json:"query"`
    SessionID string `json:"session_id"`

    // === 路由控制 ===
    NextNode string `json:"next_node"`

    // === 查询优化 ===
    RewrittenQueries []string `json:"rewritten_queries"`

    // === 背景调查 ===
    BackgroundInfo []SearchResult `json:"background_info"`

    // === 研究计划 ===
    Plan          *Plan       `json:"plan"`
    PlanAttempt   int         `json:"plan_attempt"`

    // === 执行状态 ===
    StepResults map[string]*StepResult `json:"step_results"`

    // === 研究发现 ===
    ResearchFindings []Finding `json:"research_findings"`

    // === 最终报告 ===
    FinalReport string `json:"final_report"`

    // === 进度 ===
    Progress    *Progress  `json:"progress"`

    // === 元数据 ===
    CurrentNode string    `json:"current_node"`
    StartTime   time.Time `json:"start_time"`
    EndTime     time.Time `json:"end_time"`
}
```

### Plan 计划类型

```go
// types/plan.go
type Plan struct {
    ID          string   `json:"id"`
    Title       string   `json:"title"`
    Description string   `json:"description"`
    Steps       []*Step  `json:"steps"`
    CreatedAt   time.Time `json:"created_at"`
}

type Step struct {
    ID          string     `json:"id"`
    Title       string     `json:"title"`
    Description string     `json:"description"`
    QueryTerms  []string   `json:"query_terms"`
    Priority    int        `json:"priority"`
    Status      StepStatus `json:"status"`
    AssignedTo  int        `json:"assigned_to"`
    Result      *StepResult `json:"result,omitempty"`
    Reflection  *Reflection `json:"reflection,omitempty"`
}

type StepStatus string

const (
    StepStatusPending      StepStatus = "pending"
    StepStatusAssigned     StepStatus = "assigned"
    StepStatusProcessing   StepStatus = "processing"
    StepStatusCompleted    StepStatus = "completed"
    StepStatusWaitingReflect StepStatus = "waiting_reflect"
    StepStatusFailed       StepStatus = "failed"
)
```

## 节点详解

### 1. CoordinatorNode (协调器节点)

**职责**: 入口节点，判断用户查询是否需要触发深度研究流程

**输入**: 用户原始查询
**输出**: 决策结果 (chat/research)

```go
// nodes/coordinator.go
type CoordinatorNode struct {
    config *Config
}

func (n *CoordinatorNode) Execute(ctx context.Context, s *DeepResearchState) (*DeepResearchState, error) {
    // 使用 LLM 判断意图
    // - 简单对话 → 直接返回
    // - 复杂问题 → 进入深度研究流程
}
```

**决策依据**:
- 查询复杂度
- 是否需要多轮检索
- 是否需要综合分析

### 2. RewriteAndMultiQueryNode (查询重写节点)

**职责**: 将用户查询重写为多个优化的子查询

**输入**: 原始查询
**输出**: 重写后的查询列表

```go
// nodes/rewrite_multi_query.go
type RewriteAndMultiQueryNode struct {
    config *Config
}

func (n *RewriteAndMultiQueryNode) Execute(ctx context.Context, s *DeepResearchState) (*DeepResearchState, error) {
    // LLM 生成多个优化的子查询
    // 覆盖不同角度和关键词
}
```

**策略**:
- 同义词扩展
- 不同角度表述
- 技术术语拆解

### 3. BackgroundInvestigationNode (背景调查节点)

**职责**: 初步收集背景信息，为后续规划提供上下文

**输入**: 重写后的查询列表
**输出**: 背景信息摘要

```go
// nodes/background_investigator.go
type BackgroundInvestigationNode struct {
    retriever Retriever
}

func (n *BackgroundInvestigationNode) Execute(ctx context.Context, s *DeepResearchState) (*DeepResearchState, error) {
    // 使用 WebSearch 或 RAG 快速检索
    // 生成背景信息摘要
}
```

### 4. PlannerNode (规划节点)

**职责**: 根据查询和背景信息生成研究计划

**输入**: 查询、背景信息
**输出**: 结构化的研究计划 (Plan)

```go
// nodes/planner.go
type PlannerNode struct {
    config *Config
}

func (n *PlannerNode) Execute(ctx context.Context, s *DeepResearchState) (*DeepResearchState, error) {
    // LLM 生成 JSON 格式计划
    // 包含多个 Step，每个 Step 分配给 Researcher
}
```

**计划格式**:
```json
{
  "title": "研究计划标题",
  "description": "计划描述",
  "steps": [
    {
      "id": "step_1",
      "title": "步骤标题",
      "description": "详细描述",
      "query_terms": ["关键词1", "关键词2"],
      "priority": 1
    }
  ]
}
```

### 5. InformationNode (信息决策节点)

**职责**: 决策节点，判断下一步行动

**决策分支**:
- 研究充分 → 生成报告 (Reporter)
- 需要更多信息 → 继续研究 (ResearchTeam)
- 需要人工反馈 → 请求输入 (HumanFeedback)

```go
// nodes/information.go
type InformationNode struct {
    config *Config
}

func (n *InformationNode) Execute(ctx context.Context, s *DeepResearchState) (*DeepResearchState, error) {
    // 评估当前研究结果
    // 决定: 报告生成 / 继续研究 / 请求反馈
}
```

**评估维度**:
- 结果完整性
- 信息深度
- 来源多样性

### 6. ResearchTeamNode (研究团队节点)

**职责**: 研究团队协调节点，将任务分配给具体的研究员

**输入**: 研究计划
**输出**: 分配后的任务状态

```go
// nodes/research_team.go
type ResearchTeamNode struct {
    config *Config
}

func (n *ResearchTeamNode) Execute(ctx context.Context, s *DeepResearchState) (*DeepResearchState, error) {
    // 分配 Step 给 Researcher
    // 管理研究进度
}
```

### 7. ParallelExecutorNode (并行执行节点)

**职责**: 并行执行调度器，管理多个 Researcher 的并行执行

**输入**: 已分配的任务
**输出**: 执行结果

```go
// nodes/parallel_executor.go
type ParallelExecutorNode struct {
    config *Config
}

func (n *ParallelExecutorNode) Execute(ctx context.Context, s *DeepResearchState) (*DeepResearchState, error) {
    // 并发执行多个 Researcher
    // 收集执行结果
}
```

**并发控制**:
- 最大并发数限制
- 超时控制
- 错误处理

### 8. ResearcherNode (研究执行节点)

**职责**: 具体的研究执行节点，可并行多个实例

**输入**: 分配的 Step
**输出**: StepResult

```go
// nodes/researcher.go
type ResearcherNode struct {
    config *Config
    index  int
}

func (n *ResearcherNode) Execute(ctx context.Context, s *DeepResearchState) (*DeepResearchState, error) {
    // 使用内部 ReAct Agent
    // 自主搜索和整合信息
}
```

**可用工具**:
- `rag_query` - 知识库检索
- `web_search` - 网络搜索
- `graph_query` - 图谱查询

### 9. ReporterNode (报告生成节点)

**职责**: 生成最终研究报告

**输入**: 所有研究结果
**输出**: 结构化报告

```go
// nodes/reporter.go
type ReporterNode struct {
    config *Config
}

func (n *ReporterNode) Execute(ctx context.Context, s *DeepResearchState) (*DeepResearchState, error) {
    // 整合所有研究结果
    // 生成结构化报告
}
```

**报告结构**:
- 核心答案 (ExecutiveSummary)
- 详细分析 (DetailedAnalysis)
- 关键发现 (KeyFindings)
- 参考资料 (Sources)

### 10. HumanFeedbackNode (人工反馈节点)

**职责**: 人工反馈介入点

**输入**: 当前研究状态
**输出**: 用户反馈

```go
// nodes/human_feedback.go
type HumanFeedbackNode struct {
    config *Config
}

func (n *HumanFeedbackNode) Execute(ctx context.Context, s *DeepResearchState) (*DeepResearchState, error) {
    // 等待用户输入
    // 根据反馈调整研究方向
}
```

## Graph 编排

### 构建 StateGraph

```go
// graph.go
package deep_research

func NewGraph(ctx context.Context, cfg *Config) (*Graph, error) {
    g := compose.NewStateGraph()

    // 添加节点
    g.AddNode("coordinator", nodes.NewCoordinatorNode(cfg))
    g.AddNode("rewrite_multi_query", nodes.NewRewriteMultiQueryNode(cfg))
    g.AddNode("background_investigator", nodes.NewBackgroundInvestigatorNode(cfg))
    g.AddNode("planner", nodes.NewPlannerNode(cfg))
    g.AddNode("information", nodes.NewInformationNode(cfg))
    g.AddNode("research_team", nodes.NewResearchTeamNode(cfg))
    g.AddNode("parallel_executor", nodes.NewParallelExecutorNode(cfg))
    g.AddNode("reporter", nodes.NewReporterNode(cfg))
    g.AddNode("human_feedback", nodes.NewHumanFeedbackNode(cfg))

    // 添加并行 researcher 节点
    for i := 0; i < cfg.ResearcherCount; i++ {
        g.AddNode(fmt.Sprintf("researcher_%d", i), nodes.NewResearcherNode(cfg, i))
    }

    // 添加边
    setupEdges(g, cfg)

    // 编译
    runnable, err := g.Compile(ctx)
    // ...

    return &Graph{stateGraph: runnable, config: cfg}, nil
}
```

### 边连接

```go
func setupEdges(g *compose.StateGraph, cfg *Config) {
    // 主流程边
    g.AddEdge(compose.START, "coordinator")

    // coordinator → (rewrite_multi_query | END)
    g.AddConditionalEdge("coordinator", routeByNextNode("rewrite_multi_query"))

    // rewrite_multi_query → background_investigator
    g.AddEdge("rewrite_multi_query", "background_investigator")

    // background_investigator → planner
    g.AddEdge("background_investigator", "planner")

    // planner → information
    g.AddEdge("planner", "information")

    // information → (reporter | research_team | human_feedback)
    g.AddConditionalEdge("information", func(ctx context.Context, s *DeepResearchState) (string, error) {
        if isResearchSufficient(s) {
            return "reporter", nil
        }
        if needHumanFeedback(s) {
            return "human_feedback", nil
        }
        return "research_team", nil
    })

    // research_team → parallel_executor
    g.AddEdge("research_team", "parallel_executor")

    // parallel_executor → information (循环)
    g.AddEdge("parallel_executor", "information")

    // human_feedback → planner (重新规划)
    g.AddEdge("human_feedback", "planner")

    // reporter → END
    g.AddEdge("reporter", compose.END)
}
```

## 配置管理

```go
// config.go
type Config struct {
    // 模型
    ChatModel model.ChatModel

    // 研究者数量
    ResearcherCount int

    // 计划配置
    PlanConfig PlanConfig

    // 执行配置
    ExecutionConfig ExecutionConfig

    // 报告配置
    ReportConfig ReportConfig
}

type PlanConfig struct {
    MaxIterations int    // 最大生成尝试次数
    MaxSteps      int    // 最大步骤数
    AutoAccept    bool   // 是否自动接受计划
}

type ExecutionConfig struct {
    MaxConcurrency int     // 最大并发数
    Timeout        time.Duration  // 超时时间
    MaxRetries     int     // 最大重试次数
}
```

## 提示词管理

```
prompts/
├── prompts.go              # 提示词加载
└── templates/
    ├── coordinator.md      # 协调器提示词
    ├── rewrite.md          # 查询重写提示词
    ├── planner.md          # 规划提示词
    ├── researcher.md       # 研究员提示词
    └── reporter.md         # 报告生成提示词
```

## 使用示例

```go
// 创建 Agent
cfg := &deep_research.Config{
    ResearcherCount: 3,
    PlanConfig: deep_research.PlanConfig{
        MaxSteps: 5,
    },
}

agent, err := deep_research.NewAgent(ctx, chatModel, cfg)
if err != nil {
    return err
}

// 执行研究
report, err := agent.Run(ctx, "分析 React 和 Vue 在企业级应用中的差异")
if err != nil {
    return err
}

// 输出报告
fmt.Println(report.ExecutiveSummary)
```

## 设计优势

| 特性 | 说明 |
|------|------|
| **状态驱动** | 节点间通过 State 传递数据，松耦合 |
| **动态路由** | 支持根据结果动态决定下一步 |
| **并行执行** | Researcher 可并行工作，提高效率 |
| **人工介入** | 支持人工反馈调整研究方向 |
| **可观测** | 每个节点状态可追踪 |
| **可扩展** | 新增节点不影响现有流程 |

## 相关文档

- [Agent 框架说明](./agent-framework.md)
- [项目概览](./project-overview.md)
- [架构文档](./architecture.md)
