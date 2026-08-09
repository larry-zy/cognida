# Cognida 项目重构规则 V3：实用主义架构

> 真正解决"简单功能需要跨很多地方"的问题

## 一、问题分析：为什么简化后仍有问题？

### 1.1 当前 3.5 层架构的问题

```
❌ 问题 1: Domain 和 Service 职责模糊
┌─────────────────────────────────────────────────────────────────────────────┐
│                                                                             │
│   Domain: 实体 + Repository 接口                                           │
│   Service: 业务逻辑                                                         │
│                                                                             │
│   疑问: 实体的业务行为放在哪里？                                            │
│   - 放在 Domain → Service 变成调用者，Domain 变"重"                         │
│   - 放在 Service → Domain 变成纯数据结构（贫血模型）                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘

❌ 问题 2: 简单功能仍需跨层
一个"获取 Agent 列表"的功能：
Handler → Service.ListAgents() → Repository.FindAll() → MySQL

需要跨越 4 个地方，只是为了：
- 验证请求
- 调用 repository
- 转换格式
- 返回响应

❌ 问题 3: 引入 Domain 层的价值不明确
如果 Domain 只是定义接口和结构体，为什么不直接放在 Service 包里？
```

### 1.2 根本原因

**Clean Architecture 的假设在 AI 系统中不成立：**

| 假设 | 传统系统 | AI 系统 |
|------|---------|---------|
| 核心逻辑在代码 | ✅ 复杂业务规则 | ❌ 逻辑在 LLM/算法 |
| 领域模型稳定 | ✅ 业务规则不变 | ❌ 数据结构频繁变化 |
| 需要多种实现 | ✅ 多数据库 | ❌ 每个 store 特定 |

---

## 二、实用主义架构：按模块组织

### 2.1 核心理念

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              实用主义架构原则                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 按业务模块组织，不按技术分层                                            │
│     ────────────────────────────────────────────────────────────────────  │
│     • agent 模块：Agent 相关的所有代码                                      │
│     • rag 模块：RAG 相关的所有代码                                          │
│     • chat 模块：Chat 相关的所有代码                                        │
│                                                                             │
│  2. 每个模块内部自包含                                                      │
│     ────────────────────────────────────────────────────────────────────  │
│     • 数据结构定义在模块内                                                  │
│     • 业务逻辑实现在模块内                                                  │
│     • 外部依赖通过接口隔离                                                  │
│                                                                             │
│  3. 只有两层：Handler + Module                                             │
│     ────────────────────────────────────────────────────────────────────  │
│     • Handler: HTTP 协议处理（薄层）                                       │
│     • Module: 完整的业务能力（厚层）                                        │
│                                                                             │
│  4. Store 是纯粹的访问层                                                    │
│     ────────────────────────────────────────────────────────────────────  │
│     • MySQL/Milvus/Redis/Neo4j/gRPC/MCP                                    │
│     • 只负责数据访问，不包含业务逻辑                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 架构对比

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            架构演进对比                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ 【Clean Architecture - 当前】                                               │
│                                                                             │
│   Handler → UseCase → Domain Entity ← Repository Implement                  │
│     │          │           ▲                    │                          │
│     │          │           │                    │                          │
│     │          └───────────┴────────────────────┘                            │
│     │        (4 层，一个功能跨越多处)                                       │
│                                                                             │
│ 【3.5 层架构 - 之前提议】                                                   │
│                                                                             │
│   Handler → Service → Domain Entity ← Repository                           │
│     │          │          ▲                │                                 │
│     │          │          │                │                                 │
│     │          └──────────┴────────────────┘                                  │
│     │        (3.5 层，仍然跨越多处)                                         │
│                                                                             │
│ 【实用主义架构 - 新方案】⭐                                                  │
│                                                                             │
│   Handler ──────────────────────────────────────────────────────────────▶  │
│     │                                                                     │  │
│     └──────────────────▶ Module (agent/rag/chat/...) ◀─────────────┘   │  │
│                           │                        ▲                       │  │
│                           │                        │                       │  │
│                           └──────────────┬─────────┘                       │  │
│                                          │                                 │  │
│                                    Store (mysql/milvus/...)               │  │
│                                                                             │
│   (2 层 + Store，功能集中在模块内)                                         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、目录结构：按模块组织

### 3.1 完整目录结构

```
cognida-go/internal/
├── handler/                      # HTTP 处理层（薄层）
│   ├── agent.go                  # Agent HTTP 处理
│   ├── chat.go                   # Chat HTTP 处理
│   ├── kb.go                     # 知识库 HTTP 处理
│   └── middleware/               # 中间件
│       ├── auth.go
│       └── logger.go
│
├── agent/                        # Agent 模块（自包含）
│   ├── agent.go                  # Agent 核心逻辑
│   ├── react.go                  # ReAct 实现
│   ├── tools.go                  # 工具管理
│   ├── types.go                  # 类型定义（原 Domain 层内容）
│   ├── repository.go             # Repository 接口定义
│   └── memory.go                 # 对话记忆管理
│
├── rag/                          # RAG 模块（自包含）
│   ├── retriever.go              # 检索核心逻辑
│   ├── pipeline.go               # RAG 流程
│   ├── rerank.go                 # 重排序
│   ├── types.go                  # 类型定义
│   ├── repository.go             # 检索接口
│   └── hybrid.go                 # 混合检索策略
│
├── llm/                          # LLM 模块（自包含）
│   ├── chat.go                   # 聊天服务
│   ├── embedding.go              # 向量化
│   ├── stream.go                 # 流式处理
│   ├── types.go                  # 类型定义
│   └── harness.go                # LLM Harness
│
├── kb/                           # 知识库模块（自包含）
│   ├── kb.go                     # 知识库管理
│   ├── document.go               # 文档处理
│   ├── chunk.go                  # 分块处理
│   ├── types.go
│   └── repository.go
│
├── chat/                         # Chat 模块（自包含）
│   ├── service.go                # 聊天编排
│   ├── session.go                # 会话管理
│   └── types.go
│
├── evaluation/                   # 评测模块（自包含）
│   ├── service.go
│   ├── metrics.go
│   └── types.go
│
├── store/                        # 数据访问层（纯粹）
│   ├── mysql/                    # MySQL 访问
│   │   ├── agent_repo.go
│   │   ├── kb_repo.go
│   │   └── session_repo.go
│   ├── milvus/                   # 向量检索
│   │   ├── vector_repo.go
│   │   └── search.go
│   ├── redis/                    # 缓存
│   │   ├── cache.go
│   │   └── lock.go
│   ├── neo4j/                    # 图谱
│   │   └── graph_repo.go
│   ├── grpc/                     # gRPC 客户端
│   │   ├── doc_reader.go
│   │   └── ml_client.go
│   └── mcp/                      # MCP 客户端
│       └── client.go
│
└── pkg/                          # 公共库
    ├── errors/                   # 错误定义
    ├── types/                    # 通用类型
    └── utils/                    # 工具函数
```

### 3.2 模块自包含示例

```go
// ========== agent 模块 ==========

// agent/types.go - 类型定义（原 Domain 内容）
package agent

type Agent struct {
    ID          string
    Name        string
    Type        AgentType
    Config      *Config
    CreatedAt   time.Time
}

type AgentType string
const (
    AgentTypeReAct   AgentType = "react"
    AgentTypeRAG     AgentType = "rag"
)

// agent/repository.go - 接口定义
package agent

type Repository interface {
    Save(ctx context.Context, agent *Agent) error
    FindByID(ctx context.Context, id string) (*Agent, error)
    List(ctx context.Context) ([]*Agent, error)
}

// agent/agent.go - 核心逻辑
package agent

type Service struct {
    repo      Repository
    llm       LLMClient
    tools     []Tool
    memory    Memory
}

func (s *Service) Chat(ctx context.Context, agentID string, message string) (*Response, error) {
    agent, err := s.repo.FindByID(ctx, agentID)
    if err != nil {
        return nil, err
    }

    // ReAct 循环
    return s.executeReAct(ctx, agent, message)
}

// handler/agent.go - HTTP 处理（薄层）
package handler

type AgentHandler struct {
    agentService *agent.Service
}

func (h *AgentHandler) Chat(c *gin.Context) {
    var req ChatRequest
    c.ShouldBindJSON(&req)

    resp, err := h.agentService.Chat(c.Request.Context(), req.AgentID, req.Message)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, resp)
}
```

---

## 四、重构映射规则

### 4.1 目录映射

```
当前 Clean Architecture              实用主义架构
─────────────────────────────        ───────────────────────────
internal/                            internal/
├── interface/http/handler/     →   ├── handler/          # HTTP 处理
│                                    ├── agent.go
│                                    ├── chat.go
│                                    └── middleware/
│
├── application/usecases/       →   ├── agent/            # Agent 模块
│   ├── agent/                     ├── agent.go
│   │   ├── execute.go              ├── react.go
│   │   └── research.go             ├── tools.go
│   └── llm/                        ├── types.go         # 原 Domain
│                                    └── repository.go
│
├── domain/                    →   (合并到各模块)
│   ├── agent/         ──────────▶   agent/types.go
│   │   ├── entity.go               agent/repository.go
│   │   └── repository.go
│   │                                rag/types.go
│   ├── rag/           ──────────▶   rag/repository.go
│   │   ├── entity.go               llm/types.go
│   │   └── repository.go           llm/interface.go
│   └── types/         ──────────▶   pkg/types/
│
└── infrastructure/            →   ├── store/
    ├── persistence/              ├── mysql/
    │   ├── mysql/      ───────▶     ├── agent_repo.go
    │   ├── milvus/     ───────▶     ├── kb_repo.go
    │   └── redis/      ───────▶   ├── milvus/
    │                                ├── redis/
    ├── llm/           ────────▶   ├── grpc/
    └── agent/         ────────▶   └── mcp/
```

### 4.2 代码迁移示例

#### Before (Clean Architecture)

```go
// ========== 1. Domain 层 ==========
// domain/agent/entity.go
type Agent struct {
    ID     string
    Name   string
    Type   AgentType
}

// domain/agent/repository.go
type Repository interface {
    Save(ctx context.Context, agent *Agent) error
    FindByID(ctx context.Context, id string) (*Agent, error)
}

// ========== 2. Application 层 ==========
// application/usecases/agent/execute.go
type ExecuteUseCase struct {
    repo   Repository      // Domain 接口
    llm    LLMService
}

type ExecuteRequestDTO struct {
    AgentID  string `json:"agent_id"`
    Message  string `json:"message"`
}

func (uc *ExecuteUseCase) Execute(ctx context.Context, req *ExecuteRequestDTO) (*ExecuteResponseDTO, error) {
    // 转换 DTO
    agent, err := uc.repo.FindByID(ctx, req.AgentID)
    // ...
}

// ========== 3. Infrastructure 层 ==========
// infrastructure/persistence/mysql/agent_repo.go
type AgentRepository struct {
    db *gorm.DB
}

func (r *AgentRepository) Save(ctx context.Context, agent *domain.Agent) error {
    // ...
}

// ========== 4. Interface 层 ==========
// interface/http/handler/agent_handler.go
func (h *AgentHandler) Execute(c *gin.Context) {
    var req ExecuteRequestDTO
    // ...
}
```

#### After (实用主义架构)

```go
// ========== agent 模块（自包含）==========

// agent/types.go
package agent

type Agent struct {
    ID     string
    Name   string
    Type   AgentType
}

type AgentType string

// agent/repository.go
package agent

type Repository interface {
    Save(ctx context.Context, agent *Agent) error
    FindByID(ctx context.Context, id string) (*Agent, error)
}

// agent/agent.go
package agent

type Service struct {
    repo   Repository
    llm    LLMClient
}

// ChatRequest 直接定义在模块内
type ChatRequest struct {
    AgentID  string `json:"agent_id"`
    Message  string `json:"message"`
}

func (s *Service) Chat(ctx context.Context, req *ChatRequest) (*Response, error) {
    agent, err := s.repo.FindByID(ctx, req.AgentID)
    if err != nil {
        return nil, err
    }
    // 业务逻辑
}

// ========== handler（薄层）==========

// handler/agent.go
package handler

type AgentHandler struct {
    agent *agent.Service  // 直接使用模块
}

func (h *AgentHandler) Chat(c *gin.Context) {
    var req agent.ChatRequest  // 直接使用模块类型
    c.ShouldBindJSON(&req)

    resp, err := h.agent.Chat(c.Request.Context(), &req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, resp)
}

// ========== store（数据访问）==========

// store/mysql/agent_repo.go
package mysql

type AgentRepository struct {
    db *gorm.DB
}

// 实现 agent.Repository 接口
func (r *AgentRepository) Save(ctx context.Context, agent *agent.Agent) error {
    entity := &AgentEntity{
        ID:   agent.ID,
        Name: agent.Name,
    }
    return r.db.WithContext(ctx).Save(entity).Error
}
```

**对比**：
- Before: 4 个文件，跨越 4 层
- After: 3 个文件，逻辑集中在模块内

---

## 五、实用主义架构的关键规则

### 5.1 模块组织原则

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              模块组织原则                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 一个模块 = 一个业务能力                                                 │
│     ────────────────────────────────────────────────────────────────────  │
│     • agent 模块：Agent 的完整生命周期                                       │
│     • rag 模块：检索的完整流程                                               │
│     • chat 模块：对话的完整管理                                              │
│                                                                             │
│  2. 模块内部包含：                                                          │
│     ────────────────────────────────────────────────────────────────────  │
│     • types.go: 类型定义（原 Domain 内容）                                   │
│     • repository.go: 接口定义（如果需要存储）                                 │
│     • service.go: 核心服务                                                   │
│     • 其他业务文件：具体实现                                                  │
│                                                                             │
│  3. 模块间依赖：                                                             │
│     ────────────────────────────────────────────────────────────────────  │
│     • chat → agent: 聊天调用 Agent                                          │
│     • agent → rag: Agent 调用检索                                            │
│     • agent → llm: Agent 调用 LLM                                            │
│     • 所有模块 → store: 通过接口访问数据                                      │
│                                                                             │
│  4. Handler 不做业务决策                                                     │
│     ────────────────────────────────────────────────────────────────────  │
│     • 只负责协议转换（HTTP ↔ 内部类型）                                      │
│     • 只负责请求验证                                                         │
│     • 所有业务逻辑在模块内                                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 类型定义规则

```go
// ✅ 推荐：类型定义在模块内
// agent/types.go
package agent

type Agent struct {
    ID     string
    Name   string
    Type   AgentType
}

// ✅ Repository 接口也在模块内定义
// agent/repository.go
package agent

type Repository interface {
    Save(ctx context.Context, agent *Agent) error
    FindByID(ctx context.Context, id string) (*Agent, error)
}

// ❌ 避免：单独的 domain 层
// domain/agent/entity.go - 需要跨包引用
```

### 5.3 依赖注入规则

```go
// ✅ 推荐：在 main.go 中组装
func main() {
    // 初始化 Store
    mysqlStore := mysql.New(cfg.Database)
    milvusStore := milvus.New(cfg.Milvus)

    // 初始化模块
    agentRepo := store.NewAgentRepository(mysqlStore)
    llmClient := llm.NewClient(cfg.LLM)

    agentService := agent.NewService(agentRepo, llmClient)

    ragRepo := store.NewRAGRepository(milvusStore)
    ragService := rag.NewService(ragRepo, llmClient)

    chatService := chat.NewService(agentService, ragService)

    // 初始化 Handler
    agentHandler := handler.NewAgentHandler(agentService)
    chatHandler := handler.NewChatHandler(chatService)

    // 启动服务
    router := gin.Default()
    router.POST("/api/v1/agent/chat", agentHandler.Chat)
    router.POST("/api/v1/chat", chatHandler.Chat)
}
```

---

## 六、回答你的问题

### 6.1 Domain 和 Service 冲突吗？

**在实用主义架构中不冲突，因为：**

1. **取消了独立的 Domain 层**
   - 类型定义放在模块内（agent/types.go）
   - 接口定义放在模块内（agent/repository.go）
   - 业务逻辑也在模块内（agent/agent.go）

2. **模块自包含**
   - 一个模块包含所有相关代码
   - 不需要跨越多个包

### 6.2 简单功能还需要跨很多地方吗？

**在实用主义架构中大幅减少：**

```
【获取 Agent 列表】

Before (Clean Architecture):
Handler → UseCase → Repository(接口) → Repository(实现) → MySQL
  │        │          │                │              │
  └────────┴──────────┴────────────────┴──────────────┘
      需要跨越 5 个地方

After (实用主义架构):
Handler → agent.Service → store.AgentRepository → MySQL
  │          │                │                    │
  └──────────┴────────────────┴────────────────────┘
      需要跨越 4 个地方（但逻辑集中在模块内）

如果再简化（直接调用）:
Handler → agent.List() → MySQL
  │          │           │
  └──────────┴───────────┘
      只需 3 个地方
```

### 6.3 是否可以进一步简化？

**可以，根据复杂度选择：**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           简化程度选择                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  【Level 1: 完整分层】（适合复杂场景）                                       │
│                                                                             │
│  Handler → Module.Service → Module.Repository → Store                        │
│                                                                             │
│  使用场景:                                                                  │
│  - 需要多种存储实现（MySQL + PostgreSQL）                                   │
│  - 复杂的业务逻辑                                                           │
│  - 需要完整测试覆盖                                                          │
│                                                                             │
│  【Level 2: 简化分层】（适合大多数场景）⭐ 推荐                              │
│                                                                             │
│  Handler → Module.Service → Store                                           │
│                                                                             │
│  使用场景:                                                                  │
│  - 单一存储实现                                                             │
│  - 中等复杂度业务                                                           │
│  - 需要一定抽象                                                             │
│                                                                             │
│  【Level 3: 直接调用】（适合简单场景）                                      │
│                                                                             │
│  Handler → Module                                                           │
│                                                                             │
│  使用场景:                                                                  │
│  - 纯计算，无持久化                                                         │
│  - 简单 CRUD                                                                │
│  - 快速原型                                                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 七、重构实施路径

### 7.1 渐进式迁移

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          渐进式迁移路径                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  阶段 1: 创建模块结构（不删除旧代码）                                        │
│  ────────────────────────────────────────────────────────────────────────  │
│  1. 创建 internal/agent/ 目录                                               │
│  2. 迁移 domain/agent/entity.go → agent/types.go                           │
│  3. 迁移 domain/agent/repository.go → agent/repository.go                  │
│  4. 合并 application/usecases/agent/* → agent/                             │
│                                                                             │
│  阶段 2: 迁移其他模块                                                       │
│  ────────────────────────────────────────────────────────────────────────  │
│  1. 创建 rag/、llm/、kb/ 等模块                                             │
│  2. 按相同模式迁移                                                           │
│                                                                             │
│  阶段 3: 重命名 Infrastructure                                              │
│  ────────────────────────────────────────────────────────────────────────  │
│  infrastructure/persistence/ → store/                                       │
│                                                                             │
│  阶段 4: 更新 Handler                                                       │
│  ────────────────────────────────────────────────────────────────────────  │
│  interface/http/handler/* → handler/*                                       │
│                                                                             │
│  阶段 5: 清理                                                               │
│  ────────────────────────────────────────────────────────────────────────  │
│  删除旧的 domain/、application/、infrastructure/、interface/ 目录              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 Sprint 计划

```
Sprint 1-2: 创建 agent 模块（试点）
Sprint 3-4: 迁移 rag、llm、kb 模块
Sprint 5-6: 重命名 store，更新 handler
Sprint 7-8: 清理旧代码，文档更新
```

---

## 八、总结

### 8.1 核心变化

| 维度 | Clean Architecture | 3.5 层 | 实用主义架构 |
|------|-------------------|--------|-------------|
| 层数 | 4 层 | 3.5 层 | 2 层 + Store |
| 组织方式 | 按技术分层 | 按技术分层 | **按业务模块** |
| Domain 独立 | ✅ | ✅ | ❌（合并到模块）|
| 简单功能跨度 | 4-5 处 | 3-4 处 | **2-3 处** |
| 适用场景 | 大型复杂系统 | 中型系统 | **AI/快速迭代** |

### 8.2 关键优势

1. **功能集中**：一个功能的代码集中在一个模块内
2. **减少跳跃**：不需要跨越多个包
3. **符合直觉**：新成员容易找到代码
4. **独立演进**：每个模块可以独立开发和测试

### 8.3 实施建议

1. **先试点**：选择 agent 模块作为试点
2. **渐进迁移**：不删除旧代码，逐步迁移
3. **验证价值**：确认确实简化后再全面推广
4. **保持灵活**：根据实际情况调整分层程度

---

**文档版本**: v3.0
**更新时间**: 2026-05-30
**核心改进**: 按业务模块组织，而非按技术分层
