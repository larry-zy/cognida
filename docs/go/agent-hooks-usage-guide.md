# Agent Hooks 启用指南

本文档说明如何启用 MemoryWriteHook 和 AutoCompressHook。

---

## 概述

| Hook | 类型 | 说明 | 状态 |
|------|------|------|------|
| **MemoryWriteHook** | 内置 | 自动保存对话消息到 Memory Repository | ✅ 已实现 |
| **AutoCompressHook** | After | 自动压缩会话历史，控制 token 使用量 | ✅ 已实现 |

---

## 一、MemoryWriteHook（内置）

MemoryWriteHook 不是独立的 Hook，而是集成在 Agent 的 `chatWithMemory` 方法中。

### 启用方式

通过配置 `HookConfig.EnableMemoryWrite` 和注入 `MemoryService` 来启用：

```go
import (
    "link/internal/application/usecases/agent"
    "link/internal/infrastructure/adapter/memory"
)

// 1. 创建 MemoryRepository（数据库实现）
memoryRepo := mysql.NewMemoryRepository(db)

// 2. 创建 MemoryService
memoryService := memory.NewMemoryService(memoryRepo)

// 3. 创建 TokenCounter（用于 token 计数）
tokenCounter, _ := memory.NewTokenCounter("gpt-4")

// 4. 创建 Agent，启用记忆功能
agent, err := agent.New(chatModel).
    Name("My Agent").
    WithMemoryService(memoryService).  // 启用记忆功能
    WithContextBuilder(contextBuilder).
    Build(ctx)
```

### 配置 HookConfig

```go
// 在 AgentConfig 中启用 memory write
config := &domain.AgentConfig{
    // ... 其他配置
    HookConfig: &domain.HookConfig{
        EnableMemoryWrite: true,  // 启用消息保存
        EnableAutoCompress: true,  // 启用自动压缩
    },
    MemoryConfig: &domain.MemoryConfig{
        MaxTokens:     4000,           // 最大 token 数
        CompressThreshold: 0.8,       // 80% 时触发压缩
        CompressionStrategy: "summary", // 压缩策略
    },
}
```

### 工作原理

1. **消息保存**：每次对话后，自动调用 `memoryService.SaveMessage()` 保存消息
2. **历史加载**：下次对话时，自动调用 `memoryService.LoadHistoryWithLimit()` 加载历史
3. **摘要处理**：压缩后的摘要会自动插入到上下文开头

---

## 二、AutoCompressHook

AutoCompressHook 是一个独立的 After Hook，在每次响应后检查并压缩会话历史。

### 1. 创建 CompressionService

首先需要创建压缩服务：

```go
import (
    "link/internal/infrastructure/adapter/memory"
    "link/internal/infrastructure/agent/hooks"
)

// 1. 创建 TokenCounter
tokenCounter, _ := memory.NewTokenCounter("gpt-4")

// 2. 创建压缩配置
compressConfig := &memory.CompressionConfig{
    Strategy:          memory.CompressionStrategySummary, // 摘要压缩
    MaxTokens:         4000,                              // 最大 token 数
    CompressThreshold: 0.8,                               // 80% 触发压缩
    KeepRecentN:       5,                                 // 保留最近 5 条消息
    OffloadThreshold:  1000,                              // 超过 1000 tokens 卸载内容
}

// 3. 创建 LLMClient（用于生成摘要）
type SummaryLLM struct {
    client model.ChatModel
}

func (s *SummaryLLM) GenerateSummary(ctx context.Context, messages []*memory.Message) (string, error) {
    // 构建摘要提示
    prompt := buildSummaryPrompt(messages)
    resp, err := s.client.Chat(ctx, []*llm.Message{
        {Role: "system", Content: "请总结以下对话历史，保留关键信息。"},
        {Role: "user", Content: prompt},
    })
    if err != nil {
        return "", err
    }
    return resp.Content, nil
}

summaryLLM := &SummaryLLM{client: chatModel}

// 4. 创建 Storage（可选，用于大内容卸载）
storage := memory.NewFileSystemStorage("/data/offload")

// 5. 创建 CompressionService
compressionService := memory.NewCompressionService(
    memoryRepo,
    tokenCounter,
    compressConfig,
    summaryLLM,
    storage,
)
```

### 2. 创建 AutoCompressHook

```go
// 创建自动压缩 Hook
compressHook := hooks.NewAutoCompressHook(compressionService).
    Enable().                           // 启用 Hook
    WithThreshold(0.8).                  // 80% token 使用量时触发
    WithMaxTokens(4000).                 // 最大 4000 tokens
    WithAsyncMode(true).                 // 异步执行（不阻塞响应）
    WithCompressInterval(30 * time.Second) // 最小压缩间隔 30 秒
```

### 3. 集成到 Agent

```go
// 创建 Agent 并配置压缩 Hook
agent, err := agent.New(chatModel).
    Name("My Agent").
    WithMemoryService(memoryService).      // 需要先启用记忆
    WithAutoCompress(compressHook).        // 添加压缩 Hook
    Build(ctx)
```

### 工作流程

```
用户消息 → Before Hooks → Agent 处理 → After Hooks → 响应
                                              ↓
                                        AutoCompressHook
                                              ↓
                                    检查 token 使用量
                                              ↓
                                    超过阈值？
                                   /           \
                                 是             否
                                 ↓               ↓
                            执行压缩          跳过
                                 ↓
                            生成摘要 / 滑动窗口
                                 ↓
                            保存摘要到数据库
```

---

## 三、完整示例

```go
package main

import (
    "context"
    "time"

    "github.com/cloudwego/eino/components/model"
    "link/internal/application/usecases/agent"
    "link/internal/domain/memory"
    "link/internal/infrastructure/adapter/memory"
    "link/internal/infrastructure/agent/hooks"
    mysqlrepo "link/internal/infrastructure/persistence/mysql"
)

func main() {
    ctx := context.Background()

    // 1. 初始化依赖
    db := initDB()                          // 数据库连接
    chatModel := initChatModel()            // LLM 客户端
    memoryRepo := mysqlrepo.NewMemoryRepository(db)
    tokenCounter, _ := memory.NewTokenCounter("gpt-4")

    // 2. 创建 MemoryService
    memoryService := memory.NewMemoryService(memoryRepo)

    // 3. 创建 CompressionService
    compressConfig := &memory.CompressionConfig{
        Strategy:          memory.CompressionStrategySummary,
        MaxTokens:         4000,
        CompressThreshold: 0.8,
        KeepRecentN:       5,
    }

    summaryLLM := &SummaryLLM{client: chatModel}
    storage := memory.NewFileSystemStorage("/data/offload")

    compressionService := memory.NewCompressionService(
        memoryRepo,
        tokenCounter,
        compressConfig,
        summaryLLM,
        storage,
    )

    // 4. 创建压缩 Hook
    compressHook := hooks.NewAutoCompressHook(compressionService).
        Enable().
        WithThreshold(0.8).
        WithMaxTokens(4000).
        WithAsyncMode(true).
        WithCompressInterval(30 * time.Second)

    // 5. 创建 Agent
    agent, err := agent.New(chatModel).
        Name("数据分析师").
        Prompt("你是一个专业的数据分析师，帮助用户分析数据。").
        WithMemoryService(memoryService).   // 启用记忆（包含 MemoryWriteHook）
        WithAutoCompress(compressHook).      // 启用自动压缩
        Build(ctx)

    if err != nil {
        panic(err)
    }

    // 6. 使用 Agent
    sessionID := "session-123"
    response, err := agent.Chat(ctx, "帮我分析最近的销售数据", agent.WithSession(sessionID))
    if err != nil {
        panic(err)
    }

    println(response.Content)

    // 继续对话，历史会自动保存和压缩
    response2, _ := agent.Chat(ctx, "有什么异常趋势吗？", agent.WithSession(sessionID))
    println(response2.Content)
}

// SummaryLLM 实现 memory.LLMClient 接口
type SummaryLLM struct {
    client model.ChatModel
}

func (s *SummaryLLM) GenerateSummary(ctx context.Context, messages []*memory.Message) (string, error) {
    // 构建历史消息文本
    var history strings.Builder
    for _, msg := range messages {
        history.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
    }

    // 调用 LLM 生成摘要
    prompt := fmt.Sprintf("请总结以下对话历史，保留关键信息：\n\n%s", history.String())
    resp, err := s.client.Chat(ctx, []*llm.Message{
        {Role: "system", Content: "你是一个专业的对话总结助手。"},
        {Role: "user", Content: prompt},
    })
    if err != nil {
        return "", err
    }

    return resp.Content, nil
}
```

---

## 四、压缩策略说明

### Summary（摘要压缩）

```go
CompressionStrategySummary
```

- 使用 LLM 生成对话摘要
- 适合：需要保留对话语义的场景
- 优点：信息保留完整，支持后续检索
- 缺点：需要 LLM 调用，有成本

### Sliding（滑动窗口）

```go
CompressionStrategySliding
```

- 直接删除旧消息，保留最近 N 条
- 适合：不需要历史信息的场景
- 优点：快速，无成本
- 缺点：丢失历史信息

### Hybrid（混合模式）

```go
CompressionStrategyHybrid
```

- 先尝试摘要压缩，失败时降级到滑动窗口
- 兼顾效果和可靠性

---

## 五、配置参考

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `EnableMemoryWrite` | bool | false | 是否启用消息保存 |
| `EnableAutoCompress` | bool | false | 是否启用自动压缩 |
| `MaxTokens` | int | 4000 | 最大 token 数 |
| `CompressThreshold` | float64 | 0.8 | 触发压缩的阈值（0-1） |
| `CompressionStrategy` | string | "summary" | 压缩策略 |
| `KeepRecentN` | int | 5 | 保留最近消息数 |
| `OffloadThreshold` | int | 1000 | 大内容卸载阈值 |

---

## 六、注意事项

1. **MemoryService 依赖**：AutoCompressHook 需要 MemoryService 先启用
2. **SessionID 传递**：确保 sessionID 正确传递到 context
3. **异步模式**：建议使用异步模式，避免阻塞响应
4. **压缩间隔**：设置合理的压缩间隔，避免频繁压缩
5. **LLM 成本**：摘要策略需要调用 LLM，会有额外成本
