# Agent Hooks 操作指南

> **版本**: v1.0
> **更新日期**: 2026-05-04
> **功能**: Agent 可扩展 Hook 系统完整指南

---

## 目录

- [概述](#概述)
- [Hook 类型](#hook-类型)
- [配置方式](#配置方式)
- [Conclusion Hook](#conclusion-hook)
- [Clarification Hook](#clarification-hook)
- [Reflection Hook](#reflection-hook)
- [自定义 Hook](#自定义-hook)
- [完整示例](#完整示例)

---

## 概述

Agent Hooks 是可插拔的扩展机制，允许在 Agent 的执行流程中注入自定义逻辑。

### Hook 执行时机

```
用户请求 → [Before Hooks] → Agent 执行 → [After Hooks] → 响应
           ↑                       ↑
    意图澄清                 结论生成/反思
```

### 可用 Hook

| Hook | 类型 | 时机 | 用途 |
|------|------|------|------|
| **Clarification** | Before | 处理请求前 | 意图澄清，模糊查询处理 |
| **Conclusion** | After | 生成响应后 | 数据工具结果总结 |
| **Reflection** | After | 生成响应后 | 输出质量评估与改进 |

---

## Hook 类型

### Before Hook

在 Agent 处理请求之前执行：

```go
type BeforeHook func(ctx context.Context, message string) (context.Context, string, error)
```

**用途**：
- 意图澄清
- 请求验证
- 查询重写
- 上下文注入

### After Hook

在 Agent 生成响应之后执行：

```go
type AfterHook func(ctx context.Context, resp *Response) error
```

**用途**：
- 结论生成
- 输出改进
- 格式转换
- 元数据增强

---

## 配置方式

### 方式一：代码配置

```go
import (
    "link/internal/application/usecases/agent"
    "link/internal/infrastructure/agent/hooks"
)

agent.New(chatModel).
    Name("我的 Agent").
    Before(clarifier.Hook()).
    After(conclusionGenerator.Hook()).
    After(reflectionHook).
    Build(ctx)
```

### 方式二：Builder 方法

```go
agent.New(chatModel).
    WithClarification(clarifier).
    WithConclusion(conclusionGen).
    WithReflection(chatModel, reflectionConfig, agentID).
    Build(ctx)
```

### 方式三：配置文件

```yaml
# config/agent_hooks.yaml
conclusion:
  enabled: true
  data_tools: ["sql_query", "data_query"]
  timeout: 30

clarification:
  enabled: true
  business_context: "销售数据分析"
  max_rounds: 2

reflection:
  enabled: true
  max_iterations: 3
  critic_type: "llm"
```

```go
config := loadAgentConfig("config/agent_hooks.yaml")
agent.NewAgentFromConfig(chatModel, config)
```

---

## Conclusion Hook

### 功能说明

结论生成 Hook 会在 Agent 响应后检测数据工具调用，并使用 LLM 分析结果生成结构化结论。

### 适用场景

- SQL 查询结果总结
- 数据分析报告生成
- API 返回数据解读
- 多数据源结果整合

### 配置方式

```go
import "link/internal/infrastructure/agent/hooks"

// 1. 创建结论生成器
conclusionGen := hooks.NewConclusionGenerator(llmClient).
    Enable().
    AddDataTools("sql_query", "data_query", "vector_search").
    WithTimeout(30 * time.Second)

// 2. 添加到 Agent
agent.New(chatModel).
    WithConclusion(conclusionGen).
    Build(ctx)
```

### 配置参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | false | 是否启用 |
| `data_tools` | []string | - | 数据工具名称列表 |
| `timeout` | time.Duration | 30s | LLM 分析超时时间 |

### 配置文件

```yaml
conclusion:
  enabled: true
  data_tools:
    - sql_query
    - data_query
    - vector_search
  timeout: 30
```

### 响应增强

启用后，响应中会包含结论：

```go
type Response struct {
    Content  string                 // 原始响应
    ToolCalls []*ToolCall           // 工具调用
    Metadata  map[string]interface{} // 元数据
}

// Metadata["conclusion"] 包含：
{
    "summary": "根据查询结果，本月销售额增长15%",
    "insights": ["华东地区增长最快", "产品A销量下滑"],
    "recommendations": ["建议加强产品A推广"]
}
```

---

## Clarification Hook

### 功能说明

意图澄清 Hook 会在处理查询前分析清晰度，需要澄清时返回 `ClarificationNeededError`。

### 适用场景

- 模糊查询处理
- 缺少必要参数
- 多义性问题
- 复杂需求确认

### 配置方式

```go
import "link/internal/infrastructure/agent/hooks"

// 1. 创建意图澄清器
clarifier := hooks.NewIntentClarifier(llmClient).
    Enable().
    WithBusinessContext("销售数据分析").
    WithMaxRounds(2)

// 2. 添加到 Agent
agent.New(chatModel).
    WithClarification(clarifier).
    Build(ctx)
```

### 配置参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | false | 是否启用 |
| `business_context` | string | "" | 业务上下文 |
| `max_rounds` | int | 2 | 最大澄清轮次 |

### 配置文件

```yaml
clarification:
  enabled: true
  business_context: "销售数据分析"
  max_rounds: 2
```

### 澄清处理

```go
resp, err := myAgent.Chat(ctx, userMessage)

// 检查是否需要澄清
var cerr *hooks.ClarificationNeededError
if errors.As(err, &cerr) {
    // 返回澄清响应给用户
    return &ClarificationResponse{
        NeedsClarification: true,
        Questions:          cerr.State.Questions,
        OriginalQuery:      cerr.State.OriginalQuery,
        Round:              cerr.State.Round,
    }
}
```

### 澄清状态

```go
type ClarificationState struct {
    OriginalQuery string        // 原始查询
    Questions     []Question    // 澄清问题列表
    Answers       map[string]string // 用户回答
    Round         int           // 当前轮次
}

type Question struct {
    ID          string   // 问题 ID
    Text        string   // 问题内容
    Options     []string // 可选答案（可选）
    Required    bool     // 是否必填
}
```

---

## Reflection Hook

详细文档：[Reflection Hook 操作文档](reflection-hook.md)

### 快速配置

```go
import agentreflection "link/internal/domain/agent/reflection"

config := &agentreflection.ReflectionConfig{
    Enabled:       true,
    MaxIterations: 3,
    CriticType:    "llm",
    Dimensions: []agentreflection.DimensionConfig{
        {Name: "factual", Weight: 0.3, PassThreshold: 0.8},
        {Name: "logic", Weight: 0.25, PassThreshold: 0.75},
    },
}

agent.New(chatModel).
    WithReflection(chatModel, config, "my-agent").
    Build(ctx)
```

---

## 自定义 Hook

### Before Hook 示例

```go
// 自定义 Before Hook - 请求日志
func loggingBeforeHook(logger Logger) BeforeHook {
    return func(ctx context.Context, message string) (context.Context, string, error) {
        logger.Info("收到请求", "message", message)
        
        // 添加追踪 ID
        traceID := generateTraceID()
        ctx = context.WithValue(ctx, "trace_id", traceID)
        
        return ctx, message, nil
    }
}

// 使用
agent.New(chatModel).
    Before(loggingBeforeHook(logger)).
    Build(ctx)
```

### After Hook 示例

```go
// 自定义 After Hook - 响应缓存
func cachingAfterHook(cache Cache) AfterHook {
    return func(ctx context.Context, resp *Response) error {
        if resp.Error == nil {
            key := generateCacheKey(ctx)
            cache.Set(key, resp.Content, 5*time.Minute)
        }
        return nil
    }
}

// 使用
agent.New(chatModel).
    After(cachingAfterHook(cache)).
    Build(ctx)
```

### 链式 Hook

```go
// Before Hooks 链
agent.New(chatModel).
    Before(authHook).
    Before(rateLimitHook).
    Before(loggingHook).
    Build(ctx)

// After Hooks 链
agent.New(chatModel).
    After(metricsHook).
    After(cachingHook).
    After(notificationHook).
    Build(ctx)
```

---

## 完整示例

### 示例一：数据分析 Agent

```go
package main

import (
    "context"
    "time"
    
    "github.com/cloudwego/eino/components/model"
    agentreflection "link/internal/domain/agent/reflection"
    "link/internal/application/usecases/agent"
    "link/internal/infrastructure/agent/hooks"
)

func main() {
    ctx := context.Background()
    chatModel := createChatModel()
    
    // 1. 意图澄清器
    clarifier := hooks.NewIntentClarifier(chatModel).
        Enable().
        WithBusinessContext("销售数据分析").
        WithMaxRounds(2)
    
    // 2. 结论生成器
    conclusionGen := hooks.NewConclusionGenerator(chatModel).
        Enable().
        AddDataTools("sql_query", "data_query").
        WithTimeout(30 * time.Second)
    
    // 3. 反思配置
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
    dataAgent, err := agent.New(chatModel).
        Name("销售数据分析 Agent").
        Prompt(`你是专业的销售数据分析助手。
        
        功能：
        - 分析销售趋势
        - 生成数据报告
        - 提供业务建议
        
        注意：
        - 数据来自内部系统
        - 只分析，不执行操作`).
        WithClarification(clarifier).
        WithConclusion(conclusionGen).
        WithReflection(chatModel, reflectionConfig, "sales-agent").
        Build(ctx)
    
    if err != nil {
        panic(err)
    }
    
    // 5. 使用 Agent
    resp, err := dataAgent.Chat(ctx, "分析上个月的销售情况")
    
    // 6. 处理澄清需求
    var cerr *hooks.ClarificationNeededError
    if errors.As(err, &cerr) {
        // 需要澄清
        return handleClarification(cerr)
    }
    
    // 7. 检查 Reflection 结果
    if reflectionData, ok := resp.Metadata["reflection"].(map[string]interface{}); ok {
        fmt.Printf("迭代: %v, 评分: %.2f\n", 
            reflectionData["iterations"],
            reflectionData["final_score"])
    }
    
    // 8. 获取结论
    if conclusion, ok := resp.Metadata["conclusion"].(map[string]interface{}); ok {
        fmt.Printf("结论: %v\n", conclusion["summary"])
    }
    
    fmt.Printf("回答: %s\n", resp.Content)
}
```

### 示例二：配置文件方式

```yaml
# config/sales_agent_hooks.yaml
hooks:
  # 意图澄清
  clarification:
    enabled: true
    business_context: "销售数据分析"
    max_rounds: 2
  
  # 结论生成
  conclusion:
    enabled: true
    data_tools:
      - sql_query
      - data_query
    timeout: 30
  
  # 反思
  reflection:
    enabled: true
    max_iterations: 2
    critic_type: "llm"
    dimensions:
      - name: "factual"
        weight: 0.4
        pass_threshold: 0.85
      - name: "completeness"
        weight: 0.3
        pass_threshold: 0.75
      - name: "clarity"
        weight: 0.3
        pass_threshold: 0.7
```

```go
// 加载配置并创建 Agent
config := loadHooksConfig("config/sales_agent_hooks.yaml")
agent.NewAgentFromConfig(chatModel, config)
```

### 示例三：自定义 Hook 组合

```go
// 自定义 Hook
func NewMetricsHook(metrics *Metrics) AfterHook {
    return func(ctx context.Context, resp *Response) error {
        metrics.RecordRequest(resp.Metadata["duration"])
        if resp.Error != nil {
            metrics.RecordError(resp.Error)
        }
        return nil
    }
}

func NewAuditLog(logger Logger) BeforeHook {
    return func(ctx context.Context, message string) (context.Context, string, error) {
        userID := getUserID(ctx)
        logger.Audit("agent_request", "user_id", userID, "message", message)
        return ctx, message, nil
    }
}

// 组合使用
agent.New(chatModel).
    Name("企业级 Agent").
    Before(NewAuditLog(logger)).
    WithClarification(clarifier).
    WithConclusion(conclusionGen).
    WithReflection(chatModel, reflectionConfig, agentID).
    After(NewMetricsHook(metrics)).
    Build(ctx)
```

---

## Hook 执行顺序

### Before Hooks 执行顺序

```
请求 → BeforeHook1 → BeforeHook2 → BeforeHook3 → Agent
      (日志)       (认证)        (限流)
```

### After Hooks 执行顺序

```
Agent → AfterHook1 → AfterHook2 → AfterHook3 → 响应
       (指标)      (缓存)        (通知)
```

### 错误处理

| Hook 类型 | 错误处理 |
|-----------|----------|
| Before Hook | 中断执行，返回错误 |
| After Hook | 记录错误，继续执行下一个 |

---

## 相关文档

- [Reflection Hook](reflection-hook.md) - 反思 Hook 详解
- [Agent Configuration](agent-config.md) - 配置参考
- [Agent Framework](agent-framework.md) - 框架概述

---

**维护者**: Link Team
**最后更新**: 2026-05-04
