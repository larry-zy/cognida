# Reflection Hook 操作文档

> **版本**: v1.0
> **更新日期**: 2026-05-04
> **功能**: Agent 自我反思与输出质量改进

---

## 目录

- [概述](#概述)
- [架构设计](#架构设计)
- [快速开始](#快速开始)
- [配置详解](#配置详解)
- [集成方式](#集成方式)
- [Critic 类型](#critic-类型)
- [Memory 配置](#memory-配置)
- [监控与调试](#监控与调试)
- [最佳实践](#最佳实践)

---

## 概述

Reflection Hook 是一个可插拔的 Agent 扩展，通过 **Actor-Critic-Memory** 架构让 Agent 能够自我评估和改进输出质量。

### 核心能力

| 能力 | 说明 |
|------|------|
| **自我评估** | Critic 组件评估输出的质量和准确性 |
| **迭代改进** | Actor 根据 Critic 反馈重新生成更好的输出 |
| **经验学习** | Memory 组件存储历史经验，避免重复错误 |
| **可插拔设计** | 支持多种 Critic 实现（LLM/规则/自定义） |

### 适用场景

| 场景 | 推荐配置 |
|------|----------|
| 代码生成 | `critic_type: "llm"`, `max_iterations: 3` |
| 数据分析 | `dimensions: [factual, completeness]` |
| 简单问答 | `enabled: false` (关闭以降低延迟) |
| 创意写作 | `critic_type: "rule"`, `min_score: 0.6` |
| 生产环境 | `memory: enabled` (持续学习) |

---

## 架构设计

### Actor-Critic-Memory 模型

```
┌─────────────────────────────────────────────────────────────┐
│                    Reflection Hook                          │
│                                                             │
│  ┌──────────┐    ┌──────────┐    ┌──────────────────┐     │
│  │  Actor   │───→│  Critic  │←───│     Memory       │     │
│  │  (LLM)   │    │  (评估器) │    │  (历史经验库)     │     │
│  └──────────┘    └──────────┘    └──────────────────┘     │
│        ↑                                   │                │
│        │          改进反馈                  │                │
│        └───────────────────────────────────┘                │
└─────────────────────────────────────────────────────────────┘
```

### 工作流程

```go
// 核心迭代逻辑
for i := 0; i < maxIterations; i++ {
    // 1. 获取历史经验 (首次迭代)
    lessons := memory.Retrieve(agentID, task, 3)
    
    // 2. Critic 评估当前输出
    critique := critic.Evaluate(task, currentOutput)
    
    // 3. 判断是否需要改进
    if !critic.ShouldRefine(critique) {
        // 评估通过 → 保存成功经验 → 返回
        memory.Store(successRecord)
        return result
    }
    
    // 4. 构建改进提示词
    refinePrompt := buildRefinePrompt(task, currentOutput, critique, lessons)
    
    // 5. Actor 重新生成
    currentOutput = actor.Chat(refinePrompt)
}
```

---

## 快速开始

### 方式一：代码配置（推荐）

```go
import (
    "context"
    "github.com/cloudwego/eino/components/model"
    agentreflection "link/internal/domain/agent/reflection"
    "link/internal/application/usecases/agent"
)

func main() {
    ctx := context.Background()
    chatModel := createChatModel()
    
    // 配置 Reflection
    reflectionConfig := &agentreflection.ReflectionConfig{
        Enabled:       true,
        MaxIterations: 3,
        CriticType:    "llm",
        Dimensions: []agentreflection.DimensionConfig{
            {Name: "factual", Weight: 0.3, PassThreshold: 0.8},
            {Name: "logic", Weight: 0.25, PassThreshold: 0.75},
            {Name: "completeness", Weight: 0.25, PassThreshold: 0.7},
            {Name: "clarity", Weight: 0.2, PassThreshold: 0.7},
        },
    }
    
    // 创建 Agent
    myAgent, err := agent.New(chatModel).
        Name("智能分析 Agent").
        Prompt("你是专业的数据分析助手...").
        WithReflection(chatModel, reflectionConfig, "my-agent-001").
        Build(ctx)
    
    // 使用 Agent
    resp, _ := myAgent.Chat(ctx, "分析销售趋势")
    
    // 检查 Reflection 元数据
    if reflectionData, ok := resp.Metadata["reflection"].(map[string]interface{}); ok {
        fmt.Printf("迭代次数: %v\n", reflectionData["iterations"])
        fmt.Printf("最终评分: %v\n", reflectionData["final_score"])
    }
}
```

### 方式二：配置文件加载

```yaml
# config/agent_hooks.yaml
reflection:
  enabled: true
  max_iterations: 3
  critic_type: "llm"
  dimensions:
    - name: "factual"
      weight: 1.0
      pass_threshold: 0.8
    - name: "logic"
      weight: 1.0
      pass_threshold: 0.75
  memory:
    enabled: true
    ttl: 720h
    vector_dim: 1536
```

```go
// 加载配置
config, _ := loadAgentConfig("config/agent_hooks.yaml")
myAgent, _ := agent.NewAgentFromConfig(chatModel, config)
```

### 方式三：使用默认配置

```go
import "link/internal/domain/agent/reflection"

// 使用默认配置
config := reflection.DefaultReflectionConfig()
config.Enabled = true

myAgent, _ := agent.New(chatModel).
    WithReflection(chatModel, config, "my-agent").
    Build(ctx)
```

---

## 配置详解

### ReflectionConfig 结构

```go
type ReflectionConfig struct {
    Enabled       bool              // 是否启用
    MaxIterations int               // 最大迭代次数 (1-10)
    CriticType    string            // Critic 类型: "llm" 或 "rule"
    CriticModel   string            // Critic 使用的模型 (可选)
    Dimensions    []DimensionConfig // 评估维度配置
    Memory        *MemoryConfig     // Memory 配置
}
```

### 参数说明

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | false | 是否启用反思功能 |
| `max_iterations` | int | 3 | 最大迭代次数，防止无限循环 |
| `critic_type` | string | "llm" | Critic 类型：llm/rule |
| `critic_model` | string | "" | Critic 使用的模型（可选，默认使用主模型） |
| `dimensions` | []DimensionConfig | - | 评估维度配置 |

### DimensionConfig 结构

```go
type DimensionConfig struct {
    Name          string  // 维度名称
    Weight        float64 // 权重 (0-1)
    PassThreshold float64 // 及格阈值 (0-1)
    Description   string  // 维度描述（可选）
}
```

### 默认评估维度

```go
func DefaultDimensions() []DimensionConfig {
    return []DimensionConfig{
        {Name: "factual", Weight: 0.3, PassThreshold: 0.8},      // 事实准确性
        {Name: "logic", Weight: 0.25, PassThreshold: 0.75},      // 逻辑一致性
        {Name: "completeness", Weight: 0.25, PassThreshold: 0.7}, // 完整性
        {Name: "clarity", Weight: 0.2, PassThreshold: 0.7},      // 表达清晰度
    }
}
```

---

## 集成方式

### 方式一：Builder 链式调用

```go
agent.New(chatModel).
    Name("我的 Agent").
    Prompt("系统提示词...").
    WithReflection(chatModel, reflectionConfig, agentID).
    Build(ctx)
```

### 方式二：通过 AgentConfig

```go
config := &agent.AgentConfig{
    MaxIterations: 10,
    ReflectionConfig: &reflection.ReflectionConfig{
        Enabled: true,
        MaxIterations: 3,
        // ...
    },
}

agent.NewAgentFromConfig(chatModel, config)
```

### 方式三：工具调用 Agent

```go
toolModel := createToolCallingChatModel()

config := &agent.AgentConfig{
    ReflectionConfig: reflectionConfig,
}

agent.NewAgentFromConfigWithTools(toolModel, tools, config)
```

---

## Critic 类型

### LLMCritic（基于 LLM 评估）

使用 LLM 对输出进行多维度评估，适合需要高质量输出的场景。

```go
config := &reflection.ReflectionConfig{
    Enabled:    true,
    CriticType: "llm",  // 使用 LLM Critic
    Dimensions: []reflection.DimensionConfig{
        {Name: "factual", Weight: 0.3, PassThreshold: 0.8},
        {Name: "logic", Weight: 0.25, PassThreshold: 0.75},
    },
}
```

**评估提示词模板**：

```
请评估以下回答的质量：

任务：{task}

回答：
{output}

请按照以下维度评估（每项 0-1 分）：
- factual: 事实准确性
- logic: 逻辑一致性
- completeness: 完整性
- clarity: 表达清晰度

请以 JSON 格式返回评估结果：
{
    "overall_score": 总体评分 (0-1),
    "dimensions": {
        "factual": {"score": 分数, "feedback": "反馈"},
        ...
    },
    "issues": ["问题1", "问题2"],
    "suggestions": ["建议1", "建议2"],
    "should_refine": true/false
}
```

### RuleCritic（基于规则评估）

使用预定义规则进行评估，零延迟，适合简单场景。

```go
config := &reflection.ReflectionConfig{
    Enabled:    true,
    CriticType: "rule",  // 使用规则 Critic
}
```

**默认规则**：

```go
// 规则示例
func (c *RuleCritic) Evaluate(task, output string) *CritiqueResult {
    issues := []string{}
    
    // 规则1: 输出不能为空
    if len(output) == 0 {
        issues = append(issues, "输出为空")
    }
    
    // 规则2: 输出长度检查
    if len(output) < 50 {
        issues = append(issues, "输出过短")
    }
    
    // 规则3: 格式检查
    if !hasValidFormat(output) {
        issues = append(issues, "格式不正确")
    }
    
    // 计算评分
    score := 1.0 - float64(len(issues))*0.2
    
    return &CritiqueResult{
        OverallScore: score,
        Issues:       issues,
        ShouldRefine: len(issues) > 0,
    }
}
```

### 自定义 Critic

实现 `Critic` 接口：

```go
import "link/internal/domain/agent/reflection"

type MyCustomCritic struct {
    // 自定义字段
}

func (c *MyCustomCritic) Evaluate(ctx context.Context, task, output string) (*reflection.CritiqueResult, error) {
    // 自定义评估逻辑
    return &reflection.CritiqueResult{
        OverallScore: 0.85,
        Dimensions: map[string]reflection.DimensionScore{
            "custom": {Score: 0.85, Feedback: "良好"},
        },
        ShouldRefine: false,
    }, nil
}

func (c *MyCustomCritic) ShouldRefine(result *reflection.CritiqueResult) bool {
    return result.OverallScore < 0.8
}

// 使用自定义 Critic
hook := reflection.NewReflectionHook(actor, &MyCustomCritic{}, memory, config, agentID)
```

---

## Memory 配置

Memory 组件使用 Milvus 向量数据库存储和检索历史经验。

### MemoryConfig 结构

```go
type MemoryConfig struct {
    Enabled    bool          // 是否启用
    TTL        time.Duration // 保留时间
    MaxEntries int           // 最大条目数
    VectorDim  int           // 向量维度
}
```

### 创建 Memory 存储

```go
import (
    "github.com/cloudwego/eino/components/embedding"
    "link/internal/infrastructure/agent/reflection/memory"
)

// 1. 创建 Embedder
embedder := createEmbedder()

// 2. 创建 Memory 存储
reflectionMemory, err := memory.NewMilvusReflectionMemory(
    embedder,
    &reflection.MemoryConfig{
        Enabled:   true,
        TTL:       720 * time.Hour, // 30天
        VectorDim: 1536,
    },
)

// 3. 设置到 Hook
hook.SetMemory(reflectionMemory)
```

### Memory 工作原理

```
┌─────────────────────────────────────────────────────────┐
│                    Milvus Memory                        │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │  Store: 成功/失败经验                            │   │
│  │  - 向量化任务+教训                              │   │
│  │  - 存储到 Milvus                                │   │
│  │  - 设置 TTL 自动过期                             │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │  Retrieve: 相关历史经验                          │   │
│  │  - 向量化当前任务                               │   │
│  │  - 向量检索最相关的失败经验                     │   │
│  │  - 返回 Top-K 经验教训                          │   │
│  └─────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

### 经验记录结构

```go
type ReflectionRecord struct {
    ID        string                 // 记录 ID
    AgentID   string                 // Agent ID
    Task      string                 // 任务描述
    Attempt   string                 // 尝试的输出
    Critique  *CritiqueResult        // 批评结果
    Lesson    string                 // 学到的教训
    Success   bool                   // 是否最终成功
    Iterations int                   // 迭代次数
    CreatedAt time.Time              // 创建时间
}
```

---

## 监控与调试

### Prometheus 指标

```go
import "link/internal/infrastructure/agent/reflection"

// 可用指标
reflection_invocation_total       // 调用总数
reflection_iterations_bucket      // 迭代次数分布
reflection_duration_seconds       // 处理耗时
reflection_quality_score          // 质量分数
reflection_memory_hit_total       // 记忆命中次数
reflection_memory_miss_total      // 记忆未命中次数
reflection_error_total            // 错误总数
```

### 响应元数据

```go
type Response struct {
    Content  string                 // 最终输出（已改进）
    Metadata map[string]interface{} // 元数据
}

// Metadata["reflection"] 包含：
{
    "iterations": 2,           // 迭代次数
    "initial_score": 0.65,     // 初始评分
    "final_score": 0.85,       // 最终评分
    "used_memory": true,       // 是否使用了历史记忆
    "success": true,           // 是否评估通过
    "duration_ms": 2340        // 处理耗时（毫秒）
}
```

### 调试示例

```go
resp, err := myAgent.Chat(ctx, userMessage)

// 检查 Reflection 元数据
if reflectionData, ok := resp.Metadata["reflection"].(map[string]interface{}); ok {
    log.Printf("迭代次数: %v", reflectionData["iterations"])
    log.Printf("初始评分: %.2f → 最终评分: %.2f", 
        reflectionData["initial_score"], 
        reflectionData["final_score"])
    log.Printf("使用记忆: %v", reflectionData["used_memory"])
    log.Printf("处理耗时: %vms", reflectionData["duration_ms"])
}
```

---

## 最佳实践

### 场景配置建议

| 场景 | Reflection 配置 | 理由 |
|------|----------------|------|
| **简单问答** | `enabled: false` | 降低延迟和成本 |
| **代码生成** | `critic_type: "llm"`, `max_iterations: 3` | 需要严格质量检查 |
| **数据分析** | `dimensions: [factual, completeness]` | 关注事实和完整性 |
| **创意写作** | `critic_type: "rule"`, `min_score: 0.6` | 不需要过度评估 |
| **生产环境** | `memory: enabled` | 持续学习，避免重复错误 |

### 性能优化

```go
// 1. 限制最大迭代次数
config.MaxIterations = 2  // 降低延迟

// 2. 使用 RuleCritic 替代 LLMCritic
config.CriticType = "rule"  // 零延迟

// 3. 禁用 Memory（不需要学习时）
config.Memory.Enabled = false

// 4. 设置 Critic 超时
config.CriticTimeout = 5 * time.Second
```

### 成本控制

```go
// 1. 仅在关键场景启用
if isCriticalTask(task) {
    config.Enabled = true
} else {
    config.Enabled = false
}

// 2. 使用更小的 Critic 模型
config.CriticModel = "gpt-3.5-turbo"  // 替代主模型

// 3. 降低迭代次数
config.MaxIterations = 1  // 仅评估，不迭代
```

### 与其他 Hook 组合

```go
agent.New(chatModel).
    Name("全能 Agent").
    // 意图澄清 - 理解用户意图
    WithClarification(clarifier).
    // 反思 - 改进输出质量
    WithReflection(chatModel, reflectionConfig, agentID).
    // 结论生成 - 总结数据工具结果
    WithConclusion(conclusionGen).
    Build(ctx)
```

---

## 常见问题

### Q: Reflection Hook 会增加多少延迟？

A: 取决于配置：
- RuleCritic: <10ms
- LLMCritic (1次迭代): ~1-2s
- LLMCritic (3次迭代): ~3-6s

### Q: 如何禁用 Reflection？

A: 设置 `config.Enabled = false` 或直接不调用 `WithReflection()`。

### Q: Memory 会存储什么数据？

A: 仅存储任务描述、输出摘要、评估结果和提取的教训。不存储原始用户输入。

### Q: 可以使用自定义的向量数据库吗？

A: 可以，实现 `ReflectionMemory` 接口即可。

---

## 相关文档

- [Agent Framework](agent-framework.md) - Agent 框架概述
- [Agent Hooks Guide](agent-hooks.md) - Hook 系统完整指南
- [Agent Configuration](agent-config.md) - 配置参考

---

**维护者**: Link Team
**最后更新**: 2026-05-04
