# Agent Framework - Design Document

## Context

### 设计理念

**这是一个"代码优先"的 Agent 框架，复用项目现有能力，不是配置驱动的系统。**

核心理念：
1. **复用现有能力** - Tool Registry、RAG、Memory 等已存在，直接集成
2. **提供便捷构建器** - 链式 API，易用不牺牲灵活性
3. **支持工具自动选择** - LLM 自动决定使用哪些工具，也可手动指定
4. **用户用代码组合** - 像搭积木一样自由组合

### 当前现状

项目中已存在以下能力：

| 能力 | 位置 | 说明 |
|------|------|------|
| Tool Registry | `internal/application/usecases/agent/tools/registry.go` | 工具注册中心，单例模式 |
| RAG Service | `internal/application/usecases/rag/` | 检索、增强、图谱查询 |
| Session/Memory | `internal/application/usecases/chat/` | 会话管理、消息历史 |
| 预置工具 | `tools/` 目录 | rag_query, web_search, graph_query, kb_select, kb_list |

### 设计目标

1. **复用优先** - 集成现有 Tool Registry、RAG、Session
2. **极简接口** - Agent 就两个方法：Chat 和 Stream
3. **链式构建** - Builder 模式，流畅好用
4. **自动/手动工具** - 支持自动工具选择（LLM 决定），也支持手动指定
5. **自由组合** - 编排就是组合 Agent
6. **类型安全** - 全部是 Go 代码

## Architecture

### 核心接口

```go
// Agent 接口 - 简单清晰
type Agent interface {
    Chat(ctx context.Context, message string) (*Response, error)
    Stream(ctx context.Context, message string) (<-chan *Chunk, error)
    Name() string
}

// Response 响应
type Response struct {
    Content   string
    ToolCalls []*ToolCall
    Metadata  map[string]interface{}
}

// Chunk 流式片段
type Chunk struct {
    Content  string
    Done     bool
    Metadata map[string]interface{}
}
```

### 分层架构

```
┌─────────────────────────────────────────────┐
│              用户代码层                      │
│  用代码自由组合 Agent 和编排                │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│            Agent 框架层                      │
├─────────────────────────────────────────────┤
│  agent/       - Agent 接口 + Builder         │
│  orchestration/ - 编排模式                   │
│  (复用现有 tools/, rag/, chat/)             │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│            现有能力层                        │
│  Tool Registry | RAG Service | Session       │
└─────────────────────────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│            eino ADK 层                       │
│  使用 eino 提供的底层能力                   │
└─────────────────────────────────────────────┘
```

### 集成现有能力

```go
// 复用现有 Tool Registry
import "link/internal/application/usecases/agent/tools"

// 复用现有 RAG Service
import "link/internal/application/usecases/rag"

// 复用现有 Session/Memory
import "link/internal/application/usecases/chat"
```

## 关键组件

### 1. Agent Builder

```go
// 方式一：自动工具选择 (LLM 决定)
agent := agent.New(model).
    Name("assistant").
    ToolsAutoSelect().        // 自动从注册中心选择工具
    Build(ctx)

// 方式二：手动指定工具
agent := agent.New(model).
    Name("assistant").
    Tools(
        tools.RAGQueryTool(),
        tools.WebSearchTool(),
    ).
    Build(ctx)

// 方式三：指定工具名称（从注册中心获取）
agent := agent.New(model).
    Name("assistant").
    ToolsFromRegistry("rag_query", "web_search").
    Build(ctx)

// 方式四：混合使用
agent := agent.New(model).
    Name("assistant").
    ToolsAutoSelect().
    Tools(                    // 额外添加工具
        tools.CustomTool(...),
    ).
    Build(ctx)
```

### 2. 集成 Memory/Session

```go
// 使用现有 Session 管理
agent := agent.New(model).
    Name("assistant").
    ToolsAutoSelect().
    WithSession(sessionID).   // 集成会话历史
    Build(ctx)

// 或者使用 Memory 接口
agent := agent.New(model).
    Name("assistant").
    ToolsAutoSelect().
    Memory(memoryStore).      // 自定义 Memory
    Build(ctx)
```

### 3. 集成 RAG

```go
// RAG 作为工具集成
agent := agent.New(model).
    Name("rag-assistant").
    Tools(
        tools.RAGQueryTool(),
        tools.GraphQueryTool(),
    ).
    Build(ctx)

// 或者使用内置 RAG 快捷方法
agent := agent.New(model).
    Name("rag-assistant").
    WithRAG(ragService).      // 集成 RAG 能力
    Build(ctx)
```

### 4. Orchestration Builder

```go
// 顺序执行
pipeline := orchestration.Sequential(agent1, agent2, agent3)

// 并行执行
parallel := orchestration.Parallel(agent1, agent2)

// 主从模式
supervisor := orchestration.Supervisor(coordinator, worker1, worker2)

// 条件分支
router := orchestration.Conditional(
    func(msg string) bool { return len(msg) < 100 },
    shortAgent,    // 短消息
    longAgent,     // 长消息
)

// 循环执行
loop := orchestration.Loop(
    bodyAgent,
    func(resp *Response) bool { return !resp.Done },
)
```

### 5. 自定义 Agent

```go
// 函数转 Agent
funcAgent := orchestration.Func(func(ctx context.Context, msg string) (*Response, error) {
    // 任意逻辑
    return &Response{Content: "结果"}, nil
})

// 结构体实现 Agent
type MyAgent struct {
    base        Agent
    cache       map[string]string
    ragService  *rag.RAGService
}

func (a *MyAgent) Chat(ctx context.Context, msg string) (*Response, error) {
    // 自定义逻辑
}
```

## 工具选择模式

### 自动工具选择 (推荐)

```go
// LLM 自动决定使用哪些工具
agent := agent.New(model).
    Name("smart-assistant").
    ToolsAutoSelect().  // 从注册中心暴露所有工具给 LLM
    Build(ctx)

// LLM 会根据请求自动选择：
// - 文档查询 -> rag_query
// - 网络搜索 -> web_search
// - 关系查询 -> graph_query
```

### 手动指定工具

```go
// 只暴露特定工具
agent := agent.New(model).
    Name("doc-searcher").
    Tools(
        tools.RAGQueryTool(),
    ).
    Build(ctx)
```

### 动态工具选择

```go
// 根据上下文动态选择工具
agent := agent.New(model).
    Name("dynamic-agent").
    ToolsSelector(func(ctx context.Context, msg string) []string {
        // 自定义选择逻辑
        if strings.Contains(msg, "搜索") {
            return []string{"rag_query", "web_search"}
        }
        return []string{"rag_query"}
    }).
    Build(ctx)
```

## 目录结构

```
internal/agent/
├── agent.go              # 核心 Agent 接口
├── builder.go            # Agent 构建器
├── context.go            # Agent 上下文
│
├── orchestration/
│   ├── sequential.go     # 顺序执行
│   ├── parallel.go       # 并行执行
│   ├── supervisor.go     # 主从模式
│   ├── conditional.go    # 条件分支
│   ├── loop.go          # 循环执行
│   └── func.go           # 函数转 Agent
│
└── integration/
    ├── tools.go          # 集成现有 Tool Registry
    ├── rag.go            # 集成现有 RAG Service
    └── memory.go         # 集成现有 Session/Memory

# 现有能力（复用，不重新实现）
internal/application/usecases/
├── agent/tools/          # Tool Registry + 预置工具
├── rag/                  # RAG Service
└── chat/                 # Session/Memory
```

## 使用示例

### 示例 1：智能助手（自动工具选择）

```go
import (
    "link/internal/agent"
    "link/internal/agent/orchestration"
    "link/internal/application/usecases/agent/tools"
)

// 使用现有 Tool Registry
registry := tools.GetDefaultRegistry()

// 创建智能助手 - 自动选择工具
assistant := agent.New(model).
    Name("assistant").
    Description("智能助手，可以使用各种工具").
    ToolsAutoSelect().  // LLM 自动决定
    Build(ctx)

// 用户问问题
resp, err := assistant.Chat(ctx, "最新 AI Agent 发展趋势")
// LLM 自动选择 web_search 工具

resp, err = assistant.Chat(ctx, "公司产品的技术架构")
// LLM 自动选择 rag_query 工具
```

### 示例 2：带会话历史的助手

```go
import "link/internal/application/usecases/chat"

// 获取现有 Session
session, _ := sessionService.GetSessionByID(ctx, sessionID)

// 创建带记忆的 Agent
assistant := agent.New(model).
    Name("assistant").
    ToolsAutoSelect().
    WithSession(sessionID).  // 集成会话历史
    Build(ctx)

// 对话自动记录历史
resp1, _ := assistant.Chat(ctx, "我叫什么名字？")
resp2, _ := assistant.Chat(ctx, "我刚才问了什么？")
```

### 示例 3：数据搜集流水线

```go
// 使用现有工具创建专业 Agent
urlFinder := agent.New(model).
    Name("url-finder").
    Tools(tools.WebSearchTool()).
    Build(ctx)

extractor := agent.New(model).
    Name("extractor").
    Tools(tools.WebScraperTool()).
    Build(ctx)

cleaner := agent.New(model).
    Name("cleaner").
    Build(ctx)

// 组装流水线
pipeline := orchestration.Sequential(
    urlFinder,
    extractor,
    cleaner,
)

resp, err := pipeline.Chat(ctx, "搜集 AI Agent 资料")
```

### 示例 4：自定义业务逻辑

```go
// 添加缓存逻辑
type CachedAgent struct {
    base  Agent
    cache map[string]string
}

func (a *CachedAgent) Chat(ctx context.Context, msg string) (*Response, error) {
    if cached, ok := a.cache[msg]; ok {
        return &Response{Content: cached}, nil
    }

    resp, err := a.base.Chat(ctx, msg)
    if err == nil {
        a.cache[msg] = resp.Content
    }
    return resp, err
}

// 包装现有 Agent
cached := &CachedAgent{
    base:  assistant,
    cache: make(map[string]string),
}

// 和普通 Agent 一样使用
pipeline := orchestration.Sequential(cached, cleaner)
```

### 示例 5：中间件

```go
type loggingMiddleware struct{}

func (m *loggingMiddleware) Before(ctx context.Context, msg string) (context.Context, string, error) {
    log.Printf("[LOG] Request: %s", msg)
    return ctx, msg
}

func (m *loggingMiddleware) After(ctx context.Context, resp *Response) error {
    log.Printf("[LOG] Response: %d chars", len(resp.Content))
    return nil
}

agent := agent.New(model).
    ToolsAutoSelect().
    Middleware(&loggingMiddleware{}).
    Build(ctx)
```

## Migration Path

### 阶段 1：核心框架
- 实现 Agent 接口和 Builder
- 集成现有 Tool Registry
- 实现编排模式

### 阶段 2：集成层
- 集成现有 RAG Service
- 集成现有 Session/Memory
- 实现自动工具选择

### 阶段 3：示例和文档
- 提供完整示例
- 编写使用文档

### 阶段 4：可选迁移
- 用户可以选择性地迁移现有代码
- 提供迁移指南

## Risks

| Risk | Mitigation |
|------|------------|
| 用户不熟悉新 API | 提供详细示例和文档 |
| 与现有代码冲突 | 新框架独立，不影响现有代码 |
| 性能开销 | 最小化抽象层开销 |
| 工具自动选择不准确 | 支持回退到手动指定 |

## Open Questions

1. **自动工具选择的实现方式？**
   - 选项 A: 传递所有工具信息给 LLM (Function Calling)
   - 选项 B: 根据请求内容预筛选工具
   - **决策**: 选项 A (eino 已支持 Function Calling)

2. **是否需要 Agent 注册表？**
   - **决策**: 不需要，代码直接引用即可

3. **是否支持热重载？**
   - **决策**: 不需要，代码修改后重新编译部署

4. **是否需要 YAML 配置？**
   - **决策**: 只配置基础设施，业务逻辑全用代码
