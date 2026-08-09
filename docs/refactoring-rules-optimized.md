# Cognida 项目重构规则优化版 (2026)

> 结合项目现状、联网搜索最佳实践和 AI 系统特性优化后的重构指南

## 一、架构决策：选择 3.5 层实用架构

### 1.1 架构对比分析

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        架构方案对比                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  方案 A: Clean Architecture (当前)                                  │    │
│  │  ─────────────────────────────────────────────────────────────────  │    │
│  │  ✅ 优点: 依赖方向清晰、理论完备                                    │    │
│  │  ❌ 缺点: 4层过度设计、35%代码做转换、48个Repository接口           │    │
│  │  📊 评分: 3/10 (过度设计)                                          │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  方案 B: Component-based (备选)                                     │    │
│  │  ─────────────────────────────────────────────────────────────────  │    │
│  │  ✅ 优点: 组件化、适合AI服务、灵活性高                              │    │
│  │  ❌ 缺点: 重构成本高、风险大、需要重新设计接口                      │    │
│  │  📊 评分: 6/10 (适合但成本高)                                      │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  方案 C: 3.5层实用架构 (推荐) ⭐                                     │    │
│  │  ─────────────────────────────────────────────────────────────────  │    │
│  │  ✅ 优点: 渐进迁移、保持现有结构优化、适合Go语言                    │    │
│  │  ✅ 保留: 接口隔离、依赖注入、可测试性                              │    │
│  │  📊 评分: 8/10 (平衡成本与收益)                                    │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 3.5 层实用架构定义

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          3.5 层实用架构                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  Layer 1: Handler (接口层)                                          │   │
│   │  ─────────────────────────────────────────────────────────────────  │   │
│   │  职责: HTTP/gRPC 协议处理、请求验证、响应封装                        │   │
│   │  导出: DTO、Request/Response 类型                                    │   │
│   │  可依赖: Service 层                                                  │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  Layer 2: Service (服务层)                                          │   │
│   │  ─────────────────────────────────────────────────────────────────  │   │
│   │  职责: 业务逻辑编排、Agent执行、RAG检索、LLM调用                    │   │
│   │  导出: 业务接口、领域事件                                           │   │
│   │  可依赖: Service(同级)、Domain、Repository                           │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  Layer 3: Domain (领域层)                                           │   │
│   │  ─────────────────────────────────────────────────────────────────  │   │
│   │  职责: 实体、值对象、领域接口定义                                    │   │
│   │  导出: 实体、Repository 接口、领域服务接口                          │   │
│   │  可依赖: 无                                                          │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                         │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  Layer 3.5: Repository (数据访问层)                                 │   │
│   │  ─────────────────────────────────────────────────────────────────  │   │
│   │  职责: 数据访问实现 (MySQL/Milvus/Redis/Neo4j/gRPC/MCP)            │   │
│   │  实现: Domain 层定义的接口                                          │   │
│   │  可依赖: Domain、外部依赖                                           │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.3 关键设计决策

| 决策点 | Clean Architecture | 3.5层架构 | 理由 |
|-------|-------------------|-----------|------|
| **层数** | 4层 (Interface/Application/Domain/Infra) | 3.5层 | Go语言简洁优先 |
| **接口定义** | Domain层定义所有接口 | Domain层定义Repository接口 | Service间直接调用 |
| **DTO处理** | 每层独立DTO | Handler层DTO，Service用Domain | 减少转换 |
| **业务逻辑** | Application层编排 | Service层编排 | 更直接 |
| **Repository数量** | 48个接口 | 合并为15-20个 | 聚合根原则 |

---

## 二、重构原则（结合行业最佳实践）

### 2.1 核心原则

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              重构核心原则                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 渐进式迁移 (Incremental Migration)                                      │
│     ────────────────────────────────────────────────────────────────────  │
│     • 不破坏现有功能，分阶段迁移                                           │
│     • 每个阶段可独立验证和回滚                                             │
│     • 优先处理高价值模块                                                   │
│                                                                             │
│  2. AI-First 设计 (AI-First Design)                                        │
│     ────────────────────────────────────────────────────────────────────  │
│     • 核心逻辑在模型/算法，代码负责编排                                     │
│     • 接口设计考虑 LLM Function Calling                                     │
│     • 支持 MCP 协议用于AI工具调用                                           │
│                                                                             │
│  3. Go语言惯用法 (Go Idioms)                                               │
│     ────────────────────────────────────────────────────────────────────  │
│     • 接口在使用方定义，不在实现方                                          │
│     • 简单优于复杂，实用优于纯粹                                            │
│     • 组合优于继承                                                         │
│                                                                             │
│  4. 可测试性优先 (Testability First)                                       │
│     ────────────────────────────────────────────────────────────────────  │
│     • 所有层可独立测试                                                     │
│     • 外部依赖可mock                                                       │
│     • 集成测试覆盖关键路径                                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 依赖规则

```
                    ┌─────────────┐
                    │   Handler   │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Service   │◄─────┐
                    └──────┬──────┘      │
                           │             Service间
                    ┌──────▼──────┐      │ 协作
                    │   Domain    │◄─────┘
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │ Repository  │
                    └─────────────┘

规则说明:
✅ Handler → Service: 通过接口调用
✅ Service → Service: 同层协作（如 Agent 调用 RAG）
✅ Service → Domain: 使用实体和接口
✅ Repository → Domain: 实现接口
❌ Domain → 任何: 无依赖
❌ Repository → Handler/Service: 禁止反向依赖
```

### 2.3 模块职责定义

| 层 | 职责 | 包含内容 | 禁止事项 |
|---|------|---------|---------|
| **Handler** | 协议处理 | HTTP/gRPC Handler、中间件、请求验证 | ❌ 业务逻辑、❌ 数据访问 |
| **Service** | 业务编排 | Agent执行、RAG检索、LLM调用、评测 | ❌ 协议细节、❌ SQL查询 |
| **Domain** | 领域定义 | 实体、值对象、Repository接口、领域事件 | ❌ 外部依赖、❌ 实现细节 |
| **Repository** | 数据访问 | MySQL/Milvus/Redis/Neo4j/gRPC/MCP实现 | ❌ 业务逻辑、❌ Handler依赖 |

---

## 三、目录结构映射

### 3.1 当前 → 目标映射

```
当前结构 (Clean Architecture)              目标结构 (3.5层)
─────────────────────────────────────    ────────────────────────────────────
internal/                                 internal/
├── interface/http/handler/           →   ├── handler/          # HTTP处理
│   ├── agent/                             │   ├── agent/
│   ├── chat/                              │   ├── chat/
│   └── middleware/                        │   └── middleware/
│                                         │
├── application/                        →   ├── service/          # 业务逻辑
│   ├── usecases/                          │   ├── agent/
│   │   ├── agent/     ──────────────────→ │   │   ├── agent.go
│   │   ├── rag/       ──────────────────→ │   │   ├── react.go
│   │   └── llm/       ──────────────────→ │   │   └── tools.go
│   └── services/                          │   ├── rag/
│       ├── rag/       ──────────────────→ │   │   ├── retriever.go
│       └── evaluation/ ──────────────────→ │   │   └── pipeline.go
│                                          │   ├── llm/
│                                          │   ├── kb/
│                                          │   ├── chat/
│                                          │   └── evaluation/
│                                         │
├── domain/                             →   ├── domain/           # 领域定义
│   ├── agent/         ─────────────────→ │   ├── agent/
│   ├── rag/           ─────────────────→ │   │   ├── entity.go
│   ├── knowledge/     ─────────────────→ │   │   └── repository.go  # 接口
│   └── types/         ─────────────────→ │   ├── rag/
│                                          │   ├── knowledge/
│                                          │   └── types/
│                                         │
└── infrastructure/                     →   ├── repository/       # 数据访问
    ├── persistence/                      │   ├── mysql/
    │   ├── mysql/      ─────────────────→ │   │   ├── agent_repo.go
    │   ├── milvus/     ─────────────────→ │   │   └── kb_repo.go
    │   ├── redis/      ─────────────────→ │   ├── milvus/
    │   └── neo4j/      ─────────────────→ │   ├── redis/
    ├── llm/           ─────────────────→ │   ├── neo4j/
    │   └── openai.go                     │   ├── llm/       # LLM客户端
    ├── agent/         ─────────────────→ │   ├── agent/      # Agent工具
    │   └── tools.go                     │   └── mcp/        # MCP客户端
    └── mcp/          ─────────────────→ │
        └── client.go                    │
```

### 3.2 新目录结构

```
cognida-go/internal/
├── handler/                      # HTTP/gRPC 处理层
│   ├── agent/
│   │   └── agent_handler.go
│   ├── chat/
│   │   └── chat_handler.go
│   ├── middleware/
│   │   ├── auth.go
│   │   └── logger.go
│   └── response/
│       └── common.go
│
├── service/                      # 业务逻辑层
│   ├── agent/
│   │   ├── agent.go             # Agent 服务
│   │   ├── react.go             # ReAct 逻辑
│   │   ├── tools.go             # 工具管理
│   │   └── types.go             # 请求/响应类型
│   ├── rag/
│   │   ├── retriever.go         # 检索服务
│   │   ├── pipeline.go          # RAG 流程
│   │   ├── rerank.go            # 重排序
│   │   └── hybrid.go            # 混合检索
│   ├── llm/
│   │   ├── chat.go              # 聊天服务
│   │   ├── embedding.go         # 向量化
│   │   └── stream.go            # 流式处理
│   ├── kb/
│   │   ├── kb.go                # 知识库服务
│   │   ├── document.go          # 文档处理
│   │   └── chunk.go             # 分块处理
│   ├── chat/
│   │   ├── service.go           # 聊天编排
│   │   ├── session.go           # 会话管理
│   │   └── memory.go            # 对话记忆
│   └── evaluation/
│       ├── service.go           # 评测服务
│       └── metrics.go           # 指标计算
│
├── domain/                       # 领域层（简化）
│   ├── agent/
│   │   ├── entity.go            # Agent 实体
│   │   ├── repository.go        # Repository 接口
│   │   └── types.go             # 领域类型
│   ├── rag/
│   │   ├── entity.go            # Document 实体
│   │   ├── repository.go        # 检索接口
│   │   └── types.go
│   ├── knowledge/
│   │   ├── entity.go
│   │   ├── repository.go
│   │   └── types.go
│   ├── llm/
│   │   ├── types.go             # LLM 类型定义
│   │   └── interface.go         # LLM 接口
│   └── types/
│       ├── common.go            # 通用类型
│       └── errors.go            # 错误定义
│
└── repository/                   # 数据访问层
    ├── mysql/
    │   ├── agent_repo.go        # Agent 存储
    │   ├── kb_repo.go           # 知识库存储
    │   └── session_repo.go      # 会话存储
    ├── milvus/
    │   ├── vector_repo.go       # 向量存储
    │   └── retriever.go         # 检索实现
    ├── redis/
    │   ├── cache.go             # 缓存
    │   └── lock.go              # 分布式锁
    ├── neo4j/
    │   └── graph_repo.go        # 图谱存储
    ├── llm/
    │   ├── openai.go            # OpenAI 客户端
    │   ├── anthropic.go         # Anthropic 客户端
    │   └── client.go            # 统一客户端
    └── mcp/
        └── client.go            # MCP 客户端
```

---

## 四、重构实施路径

### 4.1 分阶段执行计划

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          渐进式重构时间线                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   Sprint 1-2 (2周)                     Sprint 3-4 (2周)                      │
│   ┌─────────────┐                     ┌─────────────┐                      │
│   │  Phase 1    │                     │  Phase 2    │                      │
│   │  合并重复   │  ──────────────────▶│  删除适配器 │                      │
│   │  模块       │                     │  修复违规   │                      │
│   └─────────────┘                     └─────────────┘                      │
│          │                                   │                             │
│          ▼                                   ▼                             │
│   合并 usecases 和 services              修复 Application →               │
│   删除空目录                             Infrastructure 违规                │
│                                         删除重复 Repository 接口            │
│                                                                             │
│   Sprint 5-6 (2周)                     Sprint 7-8 (2周)                     │
│   ┌─────────────┐                     ┌─────────────┐                      │
│   │  Phase 3    │                     │  Phase 4    │                      │
│   │  简化转换   │  ──────────────────▶│  评估优化   │                      │
│   │  减少DTO   │                     │  文档更新   │                      │
│   └─────────────┘                     └─────────────┘                      │
│          │                                   │                             │
│          ▼                                   ▼                             │
│   Handler直接使用Service类型              性能测试、代码审查                │
│   Service直接使用Domain实体               决定是否继续调整                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Phase 1: 合并重复模块 (Sprint 1-2)

#### 目标
- 消除 `usecases` 和 `services` 的重复
- 统一为 `service` 层

#### 详细步骤

```bash
# 1. 创建新 service 目录
mkdir -p internal/service/{agent,rag,llm,kb,chat,evaluation}

# 2. 合并 RAG 模块
# 将以下文件合并到 service/rag/
# - application/usecases/rag/*
# - application/services/rag/*

# 3. 合并 Agent 模块
# 将以下文件合并到 service/agent/
# - application/usecases/agent/*
# - application/usecases/llm/*agent相关*

# 4. 更新导入路径
find . -name "*.go" -exec sed -i 's|link/internal/application/usecases/|link/internal/service/|g' {} \;
find . -name "*.go" -exec sed -i 's|link/internal/application/services/|link/internal/service/|g' {} \;

# 5. 删除旧目录（确认无引用后）
rm -rf internal/application/usecases
rm -rf internal/application/services
```

#### 验证标准
- [ ] 无 `application/usecases` 引用
- [ ] 无 `application/services` 引用
- [ ] 所有测试通过
- [ ] API 响应正常

### 4.3 Phase 2: 删除适配器和修复违规 (Sprint 3-4)

#### 目标
- 修复 47 处 Application → Infrastructure 违规
- 删除 `AgentExecutableAdapter` 等适配器
- 统一 Repository 接口定义

#### 违规修复优先级

| 优先级 | 文件 | 违规次数 | 修复方法 |
|-------|------|---------|---------|
| P0 | `application/usecases/agent/` | 15 | 通过Domain接口注入 |
| P0 | `application/usecases/llm/` | 8 | 使用LLM服务接口 |
| P1 | `application/usecases/cache/` | 6 | 通过CacheRepository接口 |
| P1 | `application/initializer/` | 6 | 使用工厂模式 |
| P2 | `application/services/*` | 8 | 逐个重构 |
| P2 | `application/usecases/knowledge/` | 4 | 使用检索接口 |

#### 修复示例

**Before (违规):**
```go
// application/usecases/llm/agent_adapter.go
import (
    infraagent "link/internal/infrastructure/agent"  // ❌ 违规
)

type AgentExecutableAdapter struct {
    agent infraagent.Agent  // ❌ 直接依赖
}
```

**After (正确):**
```go
// service/agent/service.go
import (
    "link/internal/domain/agent"  // ✅ 使用Domain接口
)

type Service struct {
    executor agent.AgentExecutor  // ✅ 依赖接口
}
```

### 4.4 Phase 3: 简化DTO转换 (Sprint 5-6)

#### 目标
- Handler 直接使用 Service 类型
- Service 直接使用 Domain 实体
- 减少类型转换代码 50%

#### 实施步骤

```go
// ========== Before: 多层DTO转换 ==========

// 1. Handler DTO
type ChatRequestDTO struct {
    Messages []MessageDTO `json:"messages"`
}

// 2. UseCase DTO
type ChatUseCaseRequest struct {
    Messages []llm.Message
}

// 3. Domain 类型
type ChatRequest struct {
    Messages []llm.Message
}

// ========== After: 简化转换 ==========

// Handler 直接使用 Service 类型
func (h *Handler) Chat(c *gin.Context) {
    var req service.ChatRequest  // Service 定义的类型
    c.ShouldBindJSON(&req)

    resp, err := h.service.Chat(c.Request.Context(), &req)
    // 直接使用 Service 响应类型
}

// Service 可以嵌入或直接使用 Domain 类型
type ChatRequest struct {
    llm.ChatRequest              // 嵌入 Domain 类型
    SessionID string `json:"session_id"`  // 扩展字段
}
```

#### 验证标准
- [ ] Handler 直接使用 Service 类型
- [ ] Service 直接使用 Domain 实体
- [ ] DTO 转换代码减少 50%
- [ ] 代码行数减少 20%

### 4.5 Phase 4: 评估与优化 (Sprint 7-8)

#### 目标
- 性能测试
- 代码审查
- 文档更新
- 决定后续方向

#### 评估指标

| 指标 | 当前 | 目标 | 实际 |
|------|------|------|------|
| 代码行数 | 10万 | 7万 | ___ |
| 新功能开发时间 | 60分钟 | 25分钟 | ___ |
| Repository 接口数 | 48 | 15-20 | ___ |
| 架构违规 | 47 | 0 | ___ |
| 测试覆盖率 | __% | >70% | ___ |

---

## 五、AI 系统特殊规则

### 5.1 MCP + gRPC 混合模式

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         协议选择决策树                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  需要暴露 Python 能力？                                                     │
│         │                                                                   │
│         ▼                                                                   │
│  ┌─────────────────┐                                                       │
│  │ 是否已存在 gRPC？│                                                       │
│  └─────────────────┘                                                       │
│    │           │                                                           │
│   是           否                                                           │
│    │           │                                                           │
│    ▼           ▼                                                           │
│ ┌─────────┐  ┌─────────────────┐                                           │
│ │保持gRPC │  │ 是否 AI 工具？  │                                           │
│ │+可选MCP │  └─────────────────┘                                           │
│ └─────────┘    │           │                                               │
│               是           否                                                │
│                │           │                                                │
│                ▼           ▼                                                │
│            ┌─────────┐  ┌───────────────┐                                  │
│            │  MCP    │  │数据量>100MB?  │                                  │
│            │         │  └───────────────┘                                  │
│            └─────────┘    │           │                                      │
│                         是          否                                      │
│                          │           │                                      │
│                          ▼           ▼                                      │
│                       ┌───────┐  ┌─────────┐                               │
│                       │ gRPC  │  │  MCP   │                               │
│                       └───────┘  └─────────┘                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 Tool 统一注册

```go
// service/agent/tools/registry.go

type ToolType int
const (
    ToolTypeInternal ToolType = iota  // 内部 Go 工具
    ToolTypeGRPC                       // gRPC Python 工具
    ToolTypeMCP                        // MCP Python 工具
)

type Registry struct {
    internal   map[string]*Tool
    grpcTools  map[string]*Tool
    mcpTools   map[string]*Tool
}

// 统一工具接口
func (r *Registry) ToLLMFunctions() []LLMFunction {
    // 转换为 LLM Function Calling 格式
}
```

### 5.3 LLM Harness 设计

```go
// service/llm/harness.go

type Harness struct {
    model     ChatModel
    tools     []Tool
    memory    Memory
    hooks     []Hook
}

// 支持流式、工具调用、记忆
func (h *Harness) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
    // 1. 加载历史
    history := h.memory.Get(ctx, req.SessionID)

    // 2. 执行 hooks
    for _, hook := range h.hooks {
        history = hook.BeforeChat(ctx, history)
    }

    // 3. LLM 调用
    response := h.model.Chat(ctx, &ChatRequest{
        Messages: append(history, req.Message),
        Tools:    h.tools,
    })

    // 4. 工具调用处理
    for _, toolCall := range response.ToolCalls {
        result := h.executeTool(ctx, toolCall)
        // 将结果返回给 LLM
    }

    return response, nil
}
```

---

## 六、代码规范更新

### 6.1 包命名

```go
// ✅ 推荐：简洁明确的包名
package agent      // service/agent
package rag        // service/rag
package handler    // handler
package repository // repository/mysql

// ❌ 避免：过于具体或冗长
package agentService
package ragServiceImplementation
package httpHandlerV2
```

### 6.2 接口定义

```go
// ✅ 推荐：接口在使用方定义
// service/agent/service.go
type Service struct {
    agentRepo domain.AgentRepository  // 使用 Domain 接口
    llm       LLMService             // 同层服务
}

// ✅ Domain 层定义 Repository 接口
// domain/agent/repository.go
type AgentRepository interface {
    Save(ctx context.Context, agent *Agent) error
    FindByID(ctx context.Context, id string) (*Agent, error)
}

// ❌ 避免：在实现方定义接口
// repository/mysql/agent_repo.go
type AgentRepository interface { ... }  // 错误位置
```

### 6.3 错误处理

```go
// ✅ 推荐：定义包级错误
// domain/types/errors.go
var (
    ErrAgentNotFound    = errors.New("agent not found")
    ErrInvalidAgentConfig = errors.New("invalid agent config")
    ErrLLMTimeout       = errors.New("LLM request timeout")
)

// ✅ 错误包装
if err != nil {
    return fmt.Errorf("failed to execute agent %s: %w", agentID, err)
}

// ✅ 错误判断
if errors.Is(err, domain.ErrAgentNotFound) {
    // 处理未找到
}
```

### 6.4 Context 使用

```go
// ✅ 第一参数必须是 context.Context
func (s *Service) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

// ✅ 传递 context 到所有调用
func (s *Service) ExecuteAgent(ctx context.Context, agentID string) error {
    agent, err := s.agentRepo.FindByID(ctx, agentID)
    if err != nil {
        return err
    }
    return s.llm.Chat(ctx, agent.Prompt)
}

// ✅ 超时控制
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
```

---

## 七、测试策略

### 7.1 测试金字塔

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              测试金字塔                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│                     ┌─────────────┐                                        │
│                    │    E2E      │  10%  (端到端测试)                       │
│                   │   Tests      │       - 完整流程                        │
│                  └─────────────┘                                           │
│               ┌───────────────────┐                                        │
│              │  Integration       │  30%  (集成测试)                        │
│             │    Tests           │       - Service + Repository            │
│            └───────────────────┘                                            │
│         ┌──────────────────────────┐                                       │
│        │      Unit Tests           │  60%  (单元测试)                       │
│       │      (Service/Domain)      │       - 纯函数测试                     │
│      └──────────────────────────┘                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 架构测试

```go
// test/architecture_test.go

func TestNoRepositoryInHandler(t *testing.T) {
    // 禁止 Handler 直接依赖 Repository
    importMap := buildImportMap("internal/handler/...")
    for pkg, imports := range importMap {
        for _, imp := range imports {
            if strings.Contains(imp, "internal/repository") {
                t.Errorf("%s imports repository package: %s", pkg, imp)
            }
        }
    }
}

func TestNoDirectInfraInService(t *testing.T) {
    // Service 只能通过 Domain 接口访问 Repository
    // 允许 import domain/xxx/repository
    // 禁止 import repository/mysql
}
```

### 7.3 Mock 策略

```go
// 使用接口进行 mock
type MockAgentRepository struct {
    mock.Mock
}

func (m *MockAgentRepository) FindByID(ctx context.Context, id string) (*domain.Agent, error) {
    args := m.Called(ctx, id)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*domain.Agent), args.Error(1)
}

// 使用 mock
func TestServiceChat(t *testing.T) {
    mockRepo := new(MockAgentRepository)
    mockRepo.On("FindByID", mock.Anything, "agent-1").Return(&domain.Agent{
        ID:   "agent-1",
        Name: "Test Agent",
    }, nil)

    service := agent.NewService(mockRepo, nil)
    // ...
}
```

---

## 八、回滚与风险管理

### 8.1 风险矩阵

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| 引入新 Bug | 中 | 高 | 完善测试、分阶段上线 |
| API 兼容性 | 低 | 高 | 保持外部 API 不变 |
| 性能下降 | 低 | 中 | 性能基准测试 |
| 团队适应 | 中 | 低 | 文档和培训 |

### 8.2 回滚计划

```bash
# 每个 Sprint 完成后打 Tag
git tag -a phase1-complete -m "Phase 1 完成"

# 如果出现问题，快速回滚
git checkout phase1-complete

# 或使用 revert
git revert <commit-hash>
```

### 8.3 验证清单

每个 Phase 完成后检查：

```bash
# 1. 编译检查
go build ./...

# 2. 单元测试
go test ./service/... ./domain/... ./repository/...

# 3. 集成测试
go test ./test/integration/...

# 4. 架构检查
go test ./test/architecture/...

# 5. 检查旧引用
grep -r "application/usecases" internal/ || echo "✓ 无旧引用"
```

---

## 九、总结

### 9.1 架构选择理由

| 选项 | Clean Arch | Component-based | 3.5层实用 |
|------|-----------|----------------|----------|
| 实施成本 | 低（现有） | 高 | 中 |
| 风险 | 低 | 高 | 低 |
| 收益 | 小 | 大 | 中大 |
| **推荐** | ❌ | 备选 | ✅ |

### 9.2 预期收益

| 指标 | 当前 | 目标 | 改善 |
|------|------|------|------|
| 代码行数 | 10万 | 7万 | -30% |
| 新功能开发时间 | 60分钟 | 25分钟 | -58% |
| Repository 接口 | 48个 | 15-20个 | -60% |
| DTO 转换代码 | 35% | 15% | -57% |
| 架构违规 | 47处 | 0 | -100% |

### 9.3 行动计划

```
Sprint 1-2: 合并 usecases 和 services
Sprint 3-4: 修复架构违规、删除适配器
Sprint 5-6: 简化 DTO 转换
Sprint 7-8: 评估、优化、文档更新
```

---

**文档版本**: v2.0
**更新时间**: 2026-05-30
**参考资料**:
- Cognida 项目重构文档
- 2026 年 Go + AI 架构最佳实践
- Clean Architecture vs Go Idioms 行业讨论
