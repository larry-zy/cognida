# Agent Reflection - 使用指南

## 概述

Agent Reflection 是一个自反思和自我改进机制，通过 Actor-Critic-Memory 模式让 Agent 能够评估自身输出质量并进行迭代改进。

## 核心概念

### Actor-Critic 循环

- **Actor**: 生成初始响应的 LLM
- **Critic**: 评估响应质量的评估器（LLM 或规则）
- **Memory**: 存储和检索历史经验的向量数据库

### 工作流程

```
用户请求 → Actor 生成初始响应 → Critic 评估
                                    ↓
                               需要改进？
                              ↙        ↘
                            是          否
                            ↓           ↓
              检索历史经验    返回最终响应
                     ↓
              Actor 重新生成 → Critic 评估
                     ↓
               (最多 N 次迭代)
```

## 快速开始

### 1. 基本使用

```go
import (
    "link/internal/application/usecases/agent"
    "link/internal/domain/agent/reflection"
)

// 创建配置
config := &reflection.ReflectionConfig{
    Enabled:       true,
    MaxIterations: 3,
    CriticType:    "rule", // 或 "llm"
}

// 创建 Agent
agent, err := agent.New(chatModel).
    Name("Reflection Agent").
    Prompt("You are a helpful assistant").
    WithReflection(chatModel, embedder, config, agentID).
    Build(ctx)
```

### 2. 配置 Critic

#### LLM Critic (高质量，有成本)

```go
config := &reflection.ReflectionConfig{
    Enabled:       true,
    MaxIterations: 3,
    CriticType:    "llm",
    Dimensions: []reflection.DimensionConfig{
        {Name: "factual", Weight: 1.0, Description: "事实准确性"},
        {Name: "logic", Weight: 1.0, Description: "逻辑一致性"},
        {Name: "completeness", Weight: 0.8, Description: "完整性"},
    },
}
```

#### Rule Critic (零成本，快速)

```go
config := &reflection.ReflectionConfig{
    Enabled:       true,
    MaxIterations: 2,
    CriticType:    "rule", // 基于规则评估
}
```

### 3. 启用经验记忆

```go
config := &reflection.ReflectionConfig{
    Enabled:    true,
    CriticType: "llm",
    Memory: &reflection.MemoryConfig{
        Enabled:    true,
        TTL:        720 * time.Hour, // 30天
        VectorDim:  1536,             // 与 embedding 模型一致
    },
}
```

## 配置选项

### ReflectionConfig

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Enabled` | bool | false | 是否启用反思 |
| `MaxIterations` | int | 3 | 最大迭代次数 |
| `CriticType` | string | "llm" | 评估器类型：llm/rule |
| `Dimensions` | []DimensionConfig | 默认4维度 | 评估维度配置 |
| `Memory` | *MemoryConfig | nil | 记忆配置 |

### DimensionConfig

| 参数 | 类型 | 说明 |
|------|------|------|
| `Name` | string | 维度名称 |
| `Weight` | float64 | 权重 (0-1) |
| `PassThreshold` | float64 | 及格阈值 (0-1) |
| `Description` | string | 维度描述 |

### MemoryConfig

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `Enabled` | bool | true | 是否启用记忆 |
| `TTL` | Duration | 720h | 记忆保留时间 |
| `MaxEntries` | int | 10000 | 最大条目数 |
| `VectorDim` | int | 1536 | 向量维度 |

## YAML 配置

```yaml
# config/agent_hooks.example.yaml
reflection:
  enabled: false  # 默认关闭，按需启用
  critic_type: "llm"
  max_iterations: 3
  dimensions:
    - name: "factual"
      weight: 1.0
      description: "事实准确性"
    - name: "logic"
      weight: 1.0
      description: "逻辑一致性"
  memory:
    enabled: true
    ttl: 720h
    vector_dim: 1536
```

## 监控指标

Reflection 提供以下 Prometheus 指标：

- `agent_reflection_invocation_total`: 反思调用总数
- `agent_reflection_iterations`: 迭代次数分布
- `agent_reflection_duration_seconds`: 处理耗时
- `agent_reflection_errors_total`: 错误总数
- `agent_reflection_quality_score_before/after`: 反思前后质量分数

## 最佳实践

1. **生产环境建议**: 从 `critic_type: "rule"` 开始，零成本验证效果
2. **关键任务启用**: 对准确性要求高的场景启用 LLM Critic
3. **控制迭代次数**: 建议 `max_iterations <= 3`，避免延迟过高
4. **定期清理记忆**: 使用 `CleanupScheduler` 清理过期记录

## 故障排查

### 反思未生效

- 检查 `enabled: true`
- 确认 embedder 配置正确
- 查看日志中的反思元数据

### 记忆未命中

- 确认 Milvus 连接正常
- 检查 vector_dim 与 embedding 模型一致
- 验证 collection 创建成功

### 性能问题

- 降低 `max_iterations`
- 使用 `critic_type: "rule"`
- 调整 `memory.enabled: false`

## 自定义 Critic

如需自定义评估逻辑，实现 `Critic` 接口：

```go
type CustomCritic struct{}

func (c *CustomCritic) Evaluate(ctx context.Context, task, output string) (*reflection.CritiqueResult, error) {
    // 自定义评估逻辑
    return result, nil
}

func (c *CustomCritic) ShouldRefine(result *reflection.CritiqueResult) bool {
    return result.OverallScore < 0.8
}
```
