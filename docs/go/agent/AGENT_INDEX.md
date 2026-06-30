# Agent 文档导航

> **最后更新**: 2026-05-04

---

## 快速导航

| 文档 | 内容 |
|------|------|
| **[使用指南](agent-usage.md)** | Agent 创建、配置和使用的完整指南 |
| **[Hooks 指南](agent-hooks.md)** | Hook 系统完整指南 |
| **[Reflection Hook](reflection-hook.md)** | 反思 Hook 详解 |
| **[框架概述](../agent-framework.md)** | Agent 框架架构概述 |
| **[API 参考](API.md)** | 接口 API 参考 |
| **[迁移指南](MIGRATION.md)** | 版本迁移说明 |

---

## 功能概览

### 核心 Agent 类型

| 类型 | 说明 | 文档 |
|------|------|------|
| **SimpleAgent** | 简单对话 Agent | [使用指南](agent-usage.md#快速开始) |
| **ToolAgent** | 带工具的 Agent | [使用指南](agent-usage.md#创建方式) |
| **RAGAgent** | RAG 增强 Agent | [框架概述](../agent-framework.md) |
| **MultiAgent** | 多 Agent 协作 | [框架概述](../agent-framework.md) |

### Hook 系统

| Hook | 时机 | 用途 | 文档 |
|------|------|------|------|
| **Clarification** | Before | 意图澄清 | [Hooks 指南](agent-hooks.md#clarification-hook) |
| **Conclusion** | After | 结论生成 | [Hooks 指南](agent-hooks.md#conclusion-hook) |
| **Reflection** | After | 输出改进 | [Reflection Hook](reflection-hook.md) |

### 配置方式

| 方式 | 适用场景 | 文档 |
|------|----------|------|
| **Builder** | 代码配置 | [使用指南](agent-usage.md#方式一builder-模式推荐) |
| **配置文件** | 外部配置 | [使用指南](agent-usage.md#配置详解) |
| **默认配置** | 快速开始 | [使用指南](agent-usage.md#方式三newagentfromconfig) |

---

## 快速开始

### 1 分钟创建 Agent

```go
import "link/internal/application/usecases/agent"

// 简单 Agent
myAgent := agent.NewSimpleAgent(chatModel, "助手", "你有帮助的助手")
resp, _ := myAgent.Chat(ctx, "你好")

// 带 Hooks
agent.New(chatModel).
    Name("智能 Agent").
    WithReflection(chatModel, reflectionConfig, agentID).
    Build(ctx)
```

### 配置文件

```yaml
# config/my_agent.yaml
name: "我的 Agent"
prompt: "系统提示词..."
hooks:
  reflection:
    enabled: true
    max_iterations: 3
```

```go
config := loadConfig("config/my_agent.yaml")
agent.NewAgentFromConfig(chatModel, config)
```

---

## 常见场景

### 场景一：数据分析

```go
agent.NewToolAgent(toolModel, "数据分析师", prompt,
    sqlQueryTool,
    dataQueryTool,
    calculatorTool,
)
```

### 场景二：客服机器人

```go
agent.New(chatModel).
    Name("客服助手").
    WithRAG(ragService).
    WithClarification(clarifier).
    WithReflection(chatModel, reflectionConfig, agentID).
    Build(ctx)
```

### 场景三：代码助手

```go
config := &reflection.ReflectionConfig{
    Enabled:       true,
    MaxIterations: 3,
    CriticType:    "llm",
    Dimensions: []DimensionConfig{
        {Name: "correctness", Weight: 0.5, PassThreshold: 0.9},
        {Name: "best_practices", Weight: 0.3, PassThreshold: 0.8},
        {Name: "security", Weight: 0.2, PassThreshold: 0.9},
    },
}

agent.New(chatModel).
    Name("代码助手").
    WithReflection(chatModel, config, "code-assistant").
    Build(ctx)
```

---

## 更多资源

### 示例代码

- `internal/application/usecases/agent/eino_builder_test.go` - Builder 测试
- `internal/application/usecases/agent/reflection_integration_test.go` - Reflection 集成测试
- `config/agent_hooks.example.yaml` - Hooks 配置示例

### 相关包

| 包路径 | 说明 |
|--------|------|
| `internal/domain/agent/reflection` | Reflection 领域模型 |
| `internal/infrastructure/agent/reflection` | Reflection 实现 |
| `internal/infrastructure/agent/hooks` | Hooks 实现 |
| `internal/application/usecases/agent` | Agent 用例 |

---

## 获取帮助

- 📖 查看具体文档获取详细信息
- 💡 查看 `config/*.example.yaml` 了解配置格式
- 🧪 运行 `go test ./internal/application/usecases/agent/...` 查看测试

---

**维护者**: Link Team
**最后更新**: 2026-05-04
