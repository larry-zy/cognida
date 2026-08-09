# Agent 使用指南

> **版本**: v1.0
> **更新日期**: 2026-05-04
> **功能**: Agent 创建、配置和使用完整指南

---

## 目录

- [概述](#概述)
- [快速开始](#快速开始)
- [创建方式](#创建方式)
- [配置详解](#配置详解)
- [工具配置](#工具配置)
- [多租户支持](#多租户支持)
- [高级用法](#高级用法)
- [最佳实践](#最佳实践)

---

## 概述

Agent 是基于 CloudWeGo Eino 框架的智能体，支持对话、工具调用、多 Agent 协作等功能。

### 核心能力

| 能力 | 说明 |
|------|------|
| **对话** | 单轮/多轮对话，流式/非流式输出 |
| **工具调用** | 绑定工具，自动调用外部能力 |
| **RAG 集成** | 结合知识库检索增强回答 |
| **多 Agent** | Agent 间协作（委托、询问、交接） |
| **Hooks** | 可插拔扩展（澄清、结论、反思） |

---

## 快速开始

### 最简示例

```go
import (
    "context"
    "github.com/cloudwego/eino/components/model"
    "link/internal/application/usecases/agent"
)

func main() {
    ctx := context.Background()
    chatModel := createChatModel()
    
    // 创建简单 Agent
    myAgent := agent.NewSimpleAgent(chatModel, "助手", "你是一个有帮助的助手")
    
    // 使用 Agent
    resp, _ := myAgent.Chat(ctx, "你好")
    fmt.Println(resp.Content)
}
```

### 带工具的 Agent

```go
import (
    "github.com/cloudwego/eino/components/tool"
    "link/internal/application/usecases/agent"
)

func main() {
    ctx := context.Background()
    toolModel := createToolCallingChatModel()
    
    // 定义工具
    tools := []tool.BaseTool{
        sqlQueryTool,
        webSearchTool,
        calculatorTool,
    }
    
    // 创建带工具的 Agent
    myAgent, _ := agent.NewToolAgent(
        toolModel,
        "数据查询 Agent",
        "你可以查询数据库、搜索网络、进行计算",
        tools...,
    )
    
    // 使用
    resp, _ := myAgent.Chat(ctx, "查询本月销售额")
    fmt.Println(resp.Content)
}
```

---

## 创建方式

### 方式一：Builder 模式（推荐）

```go
agent.New(chatModel).
    Name("我的 Agent").
    Prompt("系统提示词...").
    Tools(tool1, tool2).
    Before(beforeHook).
    After(afterHook).
    Build(ctx)
```

### 方式二：NewSimpleAgent

```go
// 无工具的简单 Agent
agent.NewSimpleAgent(chatModel, name, prompt)
```

### 方式三：NewToolAgent

```go
// 带工具的 Agent
agent.NewToolAgent(toolModel, name, prompt, tools...)
```

### 方式四：NewAgentFromConfig

```go
// 从配置创建
config := &agent.AgentConfig{
    MaxIterations: 10,
    HookConfig:    hookConfig,
}
agent.NewAgentFromConfig(chatModel, config)
```

### 方式五：从注册中心加载工具

```go
// 创建 Agent 并从注册中心加载工具
agent.NewAgentFromRegistry(toolModel, name, prompt, toolRegistry)
```

---

## 配置详解

### AgentConfig 结构

```go
type AgentConfig struct {
    Name          string              // Agent 名称
    Prompt        string              // 系统提示词
    MaxIterations int                 // 最大工具调用迭代次数
    Tools         []string            // 工具名称列表
    HookConfig    *HookConfig         // Hook 配置
    ReflectionConfig *reflection.ReflectionConfig  // 反思配置
}
```

### 配置文件示例

```yaml
# config/agents/data_analyst.yaml
name: "数据分析 Agent"
prompt: |
  你是专业的数据分析助手。
  
  能力：
  - 查询数据库
  - 分析数据趋势
  - 生成可视化建议
  
  规则：
  - 只查询，不修改数据
  - 返回结果要简洁清晰

max_iterations: 10

tools:
  - sql_query
  - data_query
  - calculator

hooks:
  clarification:
    enabled: true
    business_context: "数据分析"
  conclusion:
    enabled: true
    data_tools: ["sql_query", "data_query"]
  reflection:
    enabled: true
    max_iterations: 2
```

### 加载配置文件

```go
import "gopkg.in/yaml.v3"

func loadAgentConfig(path string) (*agent.AgentConfig, error) {
    data, _ := os.ReadFile(path)
    
    var cfg struct {
        Name       string                      `yaml:"name"`
        Prompt     string                      `yaml:"prompt"`
        MaxIter    int                         `yaml:"max_iterations"`
        Tools      []string                    `yaml:"tools"`
        Hooks      map[string]interface{}      `yaml:"hooks"`
    }
    yaml.Unmarshal(data, &cfg)
    
    // 转换为 AgentConfig
    config := &agent.AgentConfig{
        Prompt:        cfg.Prompt,
        MaxIterations: cfg.MaxIter,
        // ...
    }
    
    return config, nil
}
```

---

## 工具配置

### 工具定义

```go
import (
    "github.com/cloudwego/eino/components/tool"
    "github.com/cloudwego/eino/schema"
)

// 定义工具
func NewSQLQueryTool(db *sql.DB) tool.BaseTool {
    return tool.NewTool(
        &schema.ToolInfo{
            Name:        "sql_query",
            Description: "执行 SQL 查询",
            ParamsOneOf: schema.NewParamsOneOfByJSON(
                `{"type":"object","properties":{"query":{"type":"string"}}}`,
            ),
        },
        func(ctx context.Context, input string) (string, error) {
            var req struct {
                Query string `json:"query"`
            }
            json.Unmarshal([]byte(input), &req)
            
            // 执行查询
            rows, _ := db.QueryContext(ctx, req.Query)
            // ...
            return result, nil
        },
    )
}
```

### 工具注册中心

```go
import "link/internal/application/usecases/agent"

// 创建工具注册中心
registry := agent.NewToolRegistry()

// 注册工具
registry.Register("sql_query", sqlQueryTool)
registry.Register("web_search", webSearchTool)
registry.Register("calculator", calculatorTool)

// 列出所有工具
tools := registry.List()  // ["sql_query", "web_search", "calculator"]

// 获取工具
tool, _ := registry.Get("sql_query")

// 批量获取
tools, _ := registry.GetToolsByNames([]string{"sql_query", "web_search"})
```

### 自动工具选择

```go
// 让 LLM 自动选择工具
agent.New(chatModel).
    Name("全能 Agent").
    ToolsAutoSelect().
    WithRegistry(registry).
    Build(ctx)
```

### 指定工具

```go
// 手动指定工具
agent.New(chatModel).
    Name("查询 Agent").
    Tools(sqlQueryTool, dataQueryTool).
    Build(ctx)

// 或从注册中心加载
agent.New(chatModel).
    Name("查询 Agent").
    ToolsFromRegistry("sql_query", "data_query").
    WithRegistry(registry).
    Build(ctx)
```

---

## 多租户支持

### 租户隔离

```go
// 创建租户专用 Agent
func createTenantAgent(tenantID string, chatModel model.ChatModel) (Agent, error) {
    // 获取租户配置
    config, _ := getTenantConfig(tenantID)
    
    // 创建租户专用工具
    tools := createTenantTools(tenantID, config)
    
    // 创建 Agent
    return agent.New(chatModel).
        Name(fmt.Sprintf("%s-Agent", tenantID)).
        Prompt(config.SystemPrompt).
        Tools(tools...).
        Build(ctx)
}
```

### 租户配置

```go
type TenantAgentConfig struct {
    TenantID      string   `json:"tenant_id"`
    SystemPrompt  string   `json:"system_prompt"`
    AllowedTools  []string `json:"allowed_tools"`
    MaxIterations int      `json:"max_iterations"`
    EnableHooks   bool     `json:"enable_hooks"`
}
```

---

## 高级用法

### 中间件

```go
// 日志中间件
func LoggingMiddleware(logger Logger) Middleware {
    return func(next AgentFunc) AgentFunc {
        return func(ctx context.Context, message string) (*Response, error) {
            logger.Info("Agent request", "message", message)
            start := time.Now()
            
            resp, err := next(ctx, message)
            
            logger.Info("Agent response", 
                "duration", time.Since(start),
                "error", err)
            
            return resp, err
        }
    }
}

// 使用
agent.New(chatModel).
    Middleware(LoggingMiddleware(logger)).
    Build(ctx)
```

### 会话记忆

```go
import "link/internal/application/usecases/agent"

// 创建带记忆的 Agent
sessionID := "user-123-session"

myAgent, _ := agent.New(chatModel).
    Name("对话 Agent").
    WithSession(sessionID).
    WithMemory(memoryService).
    Build(ctx)

// 对话
resp1, _ := myAgent.Chat(ctx, "我叫张三")
resp2, _ := myAgent.Chat(ctx, "我叫什么名字？")  // 能记住
```

### 多 Agent 协作

```go
// 创建 Agent 注册中心
agentRegistry := agent.NewCollaborationRegistry()

// 注册 Agent
agentRegistry.Register("sales", salesAgent)
agentRegistry.Register("support", supportAgent)
agentRegistry.Register("billing", billingAgent)

// 创建主 Agent，启用协作
mainAgent, _ := agent.New(chatModel).
    Name("客服总机").
    WithCollaboration(
        agentRegistry,
        agent.EnableAllCollaboration(),  // 启用所有协作工具
    ).
    Build(ctx)

// 主 Agent 可以：
// - 委托任务给其他 Agent
// - 向其他 Agent 提问
// - 将控制权交接给其他 Agent
```

### RAG 集成

```go
import "link/internal/application/usecases/rag"

// 创建 RAG Agent
ragService := rag.NewRAGService(ctx, ragConfig)

myAgent, _ := agent.New(chatModel).
    Name("知识库问答 Agent").
    WithRAG(ragService).
    Build(ctx)
```

---

## 最佳实践

### 1. 提示词设计

```go
// ❌ 不好的提示词
prompt := "你是一个助手"

// ✅ 好的提示词
prompt := `你是专业的销售数据分析助手。

职责：
- 分析销售数据趋势
- 识别异常和机会
- 提供业务建议

能力：
- SQL 查询
- 数据计算
- 报告生成

规则：
- 只查询，不修改数据
- 结果要简洁，突出重点
- 不确定时主动询问
`
```

### 2. 错误处理

```go
resp, err := myAgent.Chat(ctx, message)

// 处理澄清需求
var cerr *hooks.ClarificationNeededError
if errors.As(err, &cerr) {
    return handleClarification(cerr)
}

// 处理工具错误
if errors.Is(err, agent.ErrToolCallFailed) {
    // 降级处理
    return fallbackResponse()
}

// 处理其他错误
if err != nil {
    log.Error("agent error", "error", err)
    return err
}
```

### 3. 流式响应

```go
// 流式 Chat
stream, err := myAgent.ChatStream(ctx, message)
if err != nil {
    return err
}

for chunk := range stream.Chan() {
    fmt.Print(chunk.Content)
}
```

### 4. 性能优化

```go
// 使用连接池
chatModel := NewPooledChatModel(10)

// 限制迭代次数
config.MaxIterations = 5

// 禁用不必要的 Hook
config.HookConfig.Reflection.Enabled = false

// 使用缓存
agent.New(chatModel).
    After(cachingHook).
    Build(ctx)
```

### 5. 监控指标

```go
// Prometheus 指标
agent_invocation_total       // 调用总数
agent_duration_seconds        // 处理耗时
agent_tool_calls_total        // 工具调用总数
agent_errors_total            // 错误总数
```

---

## 完整示例

### 企业级 Agent

```go
package main

import (
    "context"
    "log"
    
    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/components/tool"
    
    "link/internal/application/usecases/agent"
    agentreflection "link/internal/domain/agent/reflection"
    "link/internal/infrastructure/agent/hooks"
)

func createEnterpriseAgent() (agent.Agent, error) {
    ctx := context.Background()
    
    // 1. 创建模型
    chatModel := createChatModel()
    toolModel := createToolCallingChatModel()
    
    // 2. 创建工具
    tools := createTools()
    
    // 3. 创建 Hooks
    clarifier := hooks.NewIntentClarifier(chatModel).
        Enable().
        WithBusinessContext("企业服务").
        WithMaxRounds(2)
    
    conclusionGen := hooks.NewConclusionGenerator(chatModel).
        Enable().
        AddDataTools("sql_query", "api_call").
        WithTimeout(30 * time.Second)
    
    reflectionConfig := &agentreflection.ReflectionConfig{
        Enabled:       true,
        MaxIterations: 2,
        CriticType:    "llm",
        Dimensions: []agentreflection.DimensionConfig{
            {Name: "factual", Weight: 0.4, PassThreshold: 0.85},
            {Name: "completeness", Weight: 0.3, PassThreshold: 0.75},
            {Name: "clarity", Weight: 0.3, PassThreshold: 0.7},
        },
    }
    
    // 4. 创建 Agent
    enterpriseAgent, err := agent.New(chatModel).
        Name("企业服务 Agent").
        Prompt(createSystemPrompt()).
        Tools(tools...).
        WithMaxIterations(10).
        WithClarification(clarifier).
        WithConclusion(conclusionGen).
        WithReflection(chatModel, reflectionConfig, "enterprise-agent").
        Build(ctx)
    
    if err != nil {
        return nil, err
    }
    
    return enterpriseAgent, nil
}

func createTools() []tool.BaseTool {
    return []tool.BaseTool{
        createSQLQueryTool(),
        createWebSearchTool(),
        createCalculatorTool(),
        createEmailTool(),
        createReportTool(),
    }
}

func createSystemPrompt() string {
    return `你是企业服务智能助手，帮助员工处理日常工作任务。

核心能力：
1. 数据查询与分析
2. 信息检索
3. 计算与转换
4. 邮件与通知
5. 报告生成

工作原则：
- 准确优先：不确定时主动询问
- 效率优先：快速给出结果
- 安全优先：不执行修改操作
- 礼貌友好：保持专业态度

输出格式：
- 简洁明了
- 结构化展示
- 必要时给出建议
`
}

func main() {
    myAgent, err := createEnterpriseAgent()
    if err != nil {
        log.Fatal(err)
    }
    
    // 使用 Agent
    resp, err := myAgent.Chat(context.Background(), "查询本月销售数据并生成报告")
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println(resp.Content)
}
```

---

## 相关文档

- [Agent Hooks](agent-hooks.md) - Hook 系统指南
- [Reflection Hook](reflection-hook.md) - 反思 Hook 详解
- [Agent Framework](agent-framework.md) - 框架概述

---

**维护者**: Cognida Team
**最后更新**: 2026-05-04
