# Deep Research 重新设计方案

## 1. 架构概览

基于现有的 Agent 编排模式（Supervisor、Parallel、Sequential、Loop），重新设计 Deep Research 为一个**多 Agent 协作系统**。

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Deep Research Agent                              │
│                        (Supervisor Orchestrator)                           │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                    ┌───────────────┼───────────────┐
                    ▼               ▼               ▼
        ┌──────────────────┐ ┌─────────────┐ ┌─────────────────┐
        │  Planning Agent  │ │  Research   │ │  Synthesis      │
        │  (问题分解)      │ │  Agents     │ │  Agent          │
        │                  │ │  (并行研究) │ │  (综合报告)     │
        │ - 子问题生成     │ │             │ │                 │
        │ - 搜索策略制定   │ │ ┌─────┐     │ │ - 信息融合     │
        │ - 资源分配       │ │ │Web  │     │ │ - 事实核查     │
        └──────────────────┘ │ │Search│    │ │ - 报告生成     │
                            │ ├─────┤     │ └─────────────────┘
                            │ │RAG  │     │
                            │ ├─────┤     │
                            │ │Graph│     │
                            │ └─────┘     │
                            └─────────────┘
```

## 2. 核心组件设计

### 2.1 研究配置

```go
// ResearchConfig 深度研究配置
type ResearchConfig struct {
    // 基础配置
    MaxSubQuestions     int           // 最大子问题数 (默认: 5)
    MaxConcurrency     int           // 并发研究数 (默认: 3)
    Timeout            time.Duration // 总超时时间 (默认: 10分钟)

    // 研究深度配置
    ResearchDepth      int           // 研究深度: 1=浅层, 2=标准, 3=深度
    MaxSourcesPerQuery int           // 每个查询最大来源数 (默认: 10)

    // 验证配置
    EnableFactCheck    bool          // 启用事实核查
    FactCheckSample    int           // 事实核查抽样数量
    EnableCrossValidation bool       // 启用交叉验证

    // 输出配置
    ReportFormat       string        // 报告格式: markdown/json/html
    IncludeSources     bool          // 包含来源列表
    IncludeMetadata    bool          // 包含元数据
}

// DefaultResearchConfig 默认配置
func DefaultResearchConfig() *ResearchConfig {
    return &ResearchConfig{
        MaxSubQuestions:      5,
        MaxConcurrency:       3,
        Timeout:              10 * time.Minute,
        ResearchDepth:        2,
        MaxSourcesPerQuery:   10,
        EnableFactCheck:      true,
        FactCheckSample:      5,
        EnableCrossValidation: true,
        ReportFormat:         "markdown",
        IncludeSources:       true,
        IncludeMetadata:      true,
    }
}
```

### 2.2 研究阶段定义

```go
// ResearchStage 研究阶段
type ResearchStage string

const (
    StageInitializing   ResearchStage = "initializing"    // 初始化
    StagePlanning       ResearchStage = "planning"        // 规划
    StageResearching    ResearchStage = "researching"     // 研究中
    StageSynthesizing   ResearchStage = "synthesizing"    // 综合
    StageFactChecking   ResearchStage = "fact_checking"   // 事实核查
    StageCompleted      ResearchStage = "completed"       // 完成
    StageFailed         ResearchStage = "failed"          // 失败
)

// ResearchProgress 研究进度
type ResearchProgress struct {
    Stage          ResearchStage  `json:"stage"`
    Progress       float64        `json:"progress"`
    Detail         string         `json:"detail"`
    SubQuestions   []string       `json:"sub_questions,omitempty"`
    CompletedTasks int            `json:"completed_tasks"`
    TotalTasks     int            `json:"total_tasks"`
    Sources        []SourceInfo   `json:"sources,omitempty"`
    Timestamp      time.Time      `json:"timestamp"`
}
```

### 2.3 使用编排模式构建

```go
// NewDeepResearchAgent 创建深度研究 Agent
func NewDeepResearchAgent(
    model model.ChatModel,
    tools []tool.BaseTool,
    config *ResearchConfig,
) Agent {
    // 1. 创建各阶段 Agent
    planningAgent := NewPlanningAgent(model)
    researchAgent := NewResearchAgent(model, tools)
    synthesisAgent := NewSynthesisAgent(model)
    factCheckAgent := NewFactCheckAgent(model, tools)

    // 2. 使用 Sequential 编排整个流程
    return orchestration.Sequential(
        planningAgent,
        orchestration.Loop(
            researchAgent,
            func(resp *Response) bool {
                // 继续研究直到收集足够信息
                return resp.Metadata["sufficient"] == false
            },
        ),
        synthesisAgent,
        factCheckAgent,
    )
}
```

## 3. 子 Agent 设计

### 3.1 Planning Agent（规划）

```go
// PlanningAgent 规划 Agent - 负责问题分解
type PlanningAgent struct {
    model model.ChatModel
}

func (a *PlanningAgent) Chat(ctx context.Context, query string) (*Response, error) {
    prompt := fmt.Sprintf(`
你是一个研究规划专家。请分析用户查询并制定研究计划。

用户查询: %s

请提供:
1. 子问题列表（3-5个）
2. 每个子问题的搜索关键词
3. 需要使用的工具类型
4. 预估的优先级

以 JSON 格式响应:
{
    "sub_questions": [
        {
            "question": "...",
            "keywords": ["...", "..."],
            "tools": ["web_search", "rag", "graph_query"],
            "priority": 1
        }
    ]
}
`, query)

    resp, err := a.model.Generate(ctx, []*schema.Message{
        schema.SystemMessage(prompt),
    })
    if err != nil {
        return nil, err
    }

    // 解析并返回规划结果
    return &Response{
        Content: resp.Content,
        Metadata: map[string]interface{}{
            "stage": "planning",
            "plan": parsePlan(resp.Content),
        },
    }, nil
}
```

### 3.2 Research Agent（研究）

```go
// ResearchAgent 研究 Agent - 使用 Parallel 并发执行多个搜索
type ResearchAgent struct {
    model   model.ChatModel
    tools   []tool.BaseTool
}

func (a *ResearchAgent) Chat(ctx context.Context, message string) (*Response, error) {
    // 从上下文获取研究任务
    tasks := getResearchTasks(ctx)

    // 为每个任务创建专门的搜索 Agent
    var agents []Agent
    for _, task := range tasks {
        agent := NewSearchAgent(a.model, a.tools, task)
        agents = append(agents, agent)
    }

    // 使用 Parallel 并发执行
    parallelAgent := orchestration.Parallel(agents...)
    return parallelAgent.Chat(ctx, message)
}

// SearchAgent 搜索 Agent - 执行单个搜索任务
type SearchAgent struct {
    model model.ChatModel
    tools []tool.BaseTool
    task  ResearchTask
}

func (a *SearchAgent) Chat(ctx context.Context, _ string) (*Response, error) {
    // 根据任务选择工具
    var selectedTools []tool.BaseTool
    for _, t := range a.tools {
        if shouldUseTool(t, a.task.ToolTypes) {
            selectedTools = append(selectedTools, t)
        }
    }

    // 创建 Agent 并执行搜索
    agent := New(a.model).
        Name(fmt.Sprintf("search-%s", a.task.ID)).
        Tools(selectedTools...).
        Prompt(a.buildSearchPrompt()).
        Build()

    return agent.Chat(ctx, a.task.Query)
}
```

### 3.3 Synthesis Agent（综合）

```go
// SynthesisAgent 综合 Agent - 负责信息融合和报告生成
type SynthesisAgent struct {
    model model.ChatModel
}

func (a *SynthesisAgent) Chat(ctx context.Context, _ string) (*Response, error) {
    // 收集所有研究结果
    findings := collectFindings(ctx)

    prompt := fmt.Sprintf(`
你是一个研究报告撰写专家。请根据以下研究结果生成综合报告。

原始查询: %s

研究发现:
%s

请生成包含以下部分的报告:
1. 执行摘要 (100-200字)
2. 关键发现 (3-5点)
3. 详细分析
4. 结论

报告应:
- 结构清晰、逻辑严密
- 引用所有重要来源
- 客观呈现不同观点
- 标注不确定信息
`, getOriginalQuery(ctx), formatFindings(findings))

    resp, err := a.model.Generate(ctx, []*schema.Message{
        schema.SystemMessage(prompt),
    })

    return &Response{
        Content: resp.Content,
        Metadata: map[string]interface{}{
            "stage": "synthesis",
            "sources": extractSources(findings),
        },
    }, nil
}
```

### 3.4 Fact Check Agent（事实核查）

```go
// FactCheckAgent 事实核查 Agent
type FactCheckAgent struct {
    model model.ChatModel
    tools []tool.BaseTool
}

func (a *FactCheckAgent) Chat(ctx context.Context, _ string) (*Response, error) {
    report := getSynthesizedReport(ctx)

    // 提取需要核查的事实
    facts := a.extractFacts(report)

    // 为每个事实创建验证 Agent 并发执行
    var agents []Agent
    for _, fact := range facts {
        agent := NewFactValidationAgent(a.model, a.tools, fact)
        agents = append(agents, agent)
    }

    parallelAgent := orchestration.Parallel(agents...)
    validations, _ := parallelAgent.Chat(ctx, "")

    // 整合验证结果
    return a.integrateValidationResults(ctx, report, validations)
}

// FactValidationAgent 单个事实验证 Agent
type FactValidationAgent struct {
    model model.ChatModel
    tools []tool.BaseTool
    fact  Fact
}

func (a *FactValidationAgent) Chat(ctx context.Context, _ string) (*Response, error) {
    prompt := fmt.Sprintf(`
验证以下事实的准确性:

事实: %s

请使用 web_search 工具查找至少3个可靠来源，并评估:
1. 事实是否准确
2. 支持证据的强度
3. 是否存在矛盾信息

返回验证结果 JSON。
`, a.fact.Statement)

    agent := New(a.model).
        Tools(a.tools...).
        Prompt(prompt).
        Build()

    return agent.Chat(ctx, "")
}
```

## 4. 进度推送设计

```go
// ProgressCallback 进度回调函数
type ProgressCallback func(progress *ResearchProgress)

// DeepResearchAgentWithProgress 带进度推送的研究 Agent
type DeepResearchAgentWithProgress struct {
    agent         Agent
    progressCB    ProgressCallback
    progressChan  chan *ResearchProgress
}

func (a *DeepResearchAgentWithProgress) Chat(ctx context.Context, query string) (*Response, error) {
    // 初始化进度
    a.sendProgress(&ResearchProgress{
        Stage:     StageInitializing,
        Progress:  0.0,
        Detail:    "开始研究",
        Timestamp: time.Now(),
    })

    // 创建带进度追踪的上下文
    progressCtx := WithProgressTracker(ctx, a.sendProgress)

    // 执行研究
    result, err := a.agent.Chat(progressCtx, query)

    // 完成进度
    a.sendProgress(&ResearchProgress{
        Stage:     StageCompleted,
        Progress:  1.0,
        Detail:    "研究完成",
        Timestamp: time.Now(),
    })

    return result, err
}

// Stream 带进度的流式响应
func (a *DeepResearchAgentWithProgress) Stream(ctx context.Context, query string) (<-chan *Chunk, error) {
    out := make(chan *Chunk, 10)

    go func() {
        defer close(out)

        // 发送进度事件
        a.sendProgress(&ResearchProgress{
            Stage:     StageInitializing,
            Progress:  0.0,
            Timestamp: time.Now(),
        })

        // 获取内部流
        innerChan, err := a.agent.Stream(ctx, query)
        if err != nil {
            a.sendProgress(&ResearchProgress{
                Stage:     StageFailed,
                Progress:  0.0,
                Detail:    err.Error(),
                Timestamp: time.Now(),
            })
            return
        }

        // 转发流式数据并追踪进度
        for chunk := range innerChan {
            if stage, ok := chunk.Metadata["stage"].(string); ok {
                a.sendProgress(&ResearchProgress{
                    Stage:     ResearchStage(stage),
                    Progress:  chunk.Metadata["progress"].(float64),
                    Detail:    chunk.Metadata["detail"].(string),
                    Timestamp: time.Now(),
                })
            }
            out <- chunk
        }

        a.sendProgress(&ResearchProgress{
            Stage:     StageCompleted,
            Progress:  1.0,
            Timestamp: time.Now(),
        })
    }()

    return out, nil
}
```

## 5. Use Case 层改造

```go
// researchUseCase 改进版
type researchUseCase struct {
    orchestrator AgentOrchestrator
    config      *ResearchConfig
    progressMgr  ProgressManager
}

func (uc *researchUseCase) Execute(ctx context.Context, req *DeepResearchRequest) (*DeepResearchResponse, error) {
    startTime := time.Now()

    // 1. 创建研究 Agent
    researchAgent := uc.orchestrator.CreateResearchAgent(uc.config)

    // 2. 创建进度回调
    progressChan := make(chan *ResearchProgress, 50)
    go uc.monitorProgress(ctx, progressChan)

    // 3. 执行研究（带进度）
    agentWithProgress := &DeepResearchAgentWithProgress{
        agent:        researchAgent,
        progressChan: progressChan,
    }

    resp, err := agentWithProgress.Chat(ctx, req.Query)
    if err != nil {
        return &DeepResearchResponse{
            Success: false,
            Error:   err.Error(),
        }, nil
    }

    // 4. 构建响应
    return uc.buildResponse(resp, startTime), nil
}

func (uc *researchUseCase) ExecuteStreamWithProgress(
    ctx context.Context,
    req *DeepResearchRequest,
) (<-chan *ResearchProgressDTO, error) {
    progressChan := make(chan *ResearchProgressDTO, 50)

    go func() {
        defer close(progressChan)

        researchAgent := uc.orchestrator.CreateResearchAgent(uc.config)

        streamChan, err := researchAgent.Stream(ctx, req.Query)
        if err != nil {
            sendProgress(progressChan, &ResearchProgressDTO{
                Stage: StageFailed,
                Error: err.Error(),
            })
            return
        }

        for chunk := range streamChan {
            // 转换内部进度为 DTO
            if stage, ok := chunk.Metadata["stage"].(string); ok {
                sendProgress(progressChan, &ResearchProgressDTO{
                    Stage:    stage,
                    Progress: chunk.Metadata["progress"].(float64),
                    Detail:   chunk.Metadata["detail"].(string),
                })
            }
        }
    }()

    return progressChan, nil
}
```

## 6. 工具集成

Deep Research 需要以下工具的支持：

```go
// 必需工具
var requiredTools = []string{
    "web_search",      // 网络搜索
    "rag_query",       // RAG 检索
    "graph_query",     // 图谱查询
}

// 可选工具
var optionalTools = []string{
    "data_query",      // 数据查询
    "sql_query",       // SQL 查询
    "knowledge_base",  // 知识库查询
}
```

## 7. API 接口

```go
// Handler 改进
func (h *AgentHandler) DeepResearch(c *gin.Context) {
    var req DeepResearchRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        response.BadRequest(c, err.Error())
        return
    }

    // 判断是否流式
    if req.Options != nil && req.Options.Stream {
        h.handleDeepResearchStream(c, &req)
    } else {
        h.handleDeepResearchSync(c, &req)
    }
}

func (h *AgentHandler) handleDeepResearchStream(c *gin.Context, req *DeepResearchRequest) {
    // 设置 SSE 头
    sse.SetSSEHeaders(c.Writer)

    // 执行带进度的研究
    progressChan, err := h.researchUseCase.ExecuteStreamWithProgress(c.Request.Context(), req)
    if err != nil {
        sse.SendSSE(c.Writer, "error", err.Error())
        return
    }

    // 发送进度更新
    for progress := range progressChan {
        sse.SendSSE(c.Writer, "progress", progress)

        if progress.Stage == "completed" || progress.Stage == "failed" {
            break
        }
    }
}
```

## 8. 配置示例

```yaml
# config/deep_research.yaml
research:
  max_sub_questions: 5
  max_concurrency: 3
  timeout: 10m

  depth:
    level: 2  # 1=shallow, 2=standard, 3=deep
    max_sources_per_query: 10

  validation:
    enable_fact_check: true
    fact_check_sample: 5
    enable_cross_validation: true
    confidence_threshold: 0.7

  output:
    format: markdown
    include_sources: true
    include_metadata: true
    language: zh-CN

  tools:
    enabled:
      - web_search
      - rag_query
      - graph_query
    optional:
      - data_query
      - sql_query
```

## 9. 测试用例

```go
func TestDeepResearchFlow(t *testing.T) {
    // 创建 Mock 工具
    mockWebSearch := &MockTool{Name: "web_search"}
    mockRAG := &MockTool{Name: "rag_query"}

    // 创建研究 Agent
    agent := NewDeepResearchAgent(
        testModel,
        []tool.BaseTool{mockWebSearch, mockRAG},
        DefaultResearchConfig(),
    )

    // 执行研究
    resp, err := agent.Chat(context.Background(), "什么是 RAG？")
    require.NoError(t, err)

    // 验证响应
    assert.Contains(t, resp.Content, "执行摘要")
    assert.Contains(t, resp.Content, "关键发现")

    // 验证元数据
    assert.Equal(t, "completed", resp.Metadata["stage"])
    assert.Greater(t, len(resp.Metadata["sources"]), 0)
}
```

## 10. 迁移路径

1. **Phase 1**: 创建新的研究编排器包
   - `internal/application/usecases/agent/research/`
   - 实现 Planning、Research、Synthesis、FactCheck Agents

2. **Phase 2**: 改造 UseCase 层
   - 更新 `researchUseCase` 使用新的编排器
   - 保持 API 接口兼容

3. **Phase 3**: 增强功能
   - 添加更多验证策略
   - 支持自定义报告模板
   - 添加研究缓存机制

4. **Phase 4**: 性能优化
   - 并行度动态调整
   - 结果缓存复用
   - 流式响应优化
