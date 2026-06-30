# Link-Go 架构迁移指南

> 从 Clean Architecture 迁移到 Component-based Architecture

## 目录

- [架构对比](#架构对比)
- [迁移策略](#迁移策略)
- [迁移步骤](#迁移步骤)
- [目录结构映射](#目录结构映射)
- [代码迁移示例](#代码迁移示例)
- [验证清单](#验证清单)

---

## 架构对比

### 当前架构：Clean Architecture

```
internal/
├── domain/                    # 领域层 - 核心业务规则
│   ├── agent/                # Agent 实体、Repository 接口
│   ├── rag/                  # RAG 实体、Repository 接口
│   ├── knowledge/            # 知识库实体、Repository 接口
│   ├── evaluation/           # 评测实体、Repository 接口
│   ├── llm/                  # LLM 实体
│   └── types/                # 通用类型、接口
│
├── application/              # 应用层 - 用例编排
│   ├── usecases/             # 用例实现
│   │   ├── agent/
│   │   ├── rag/
│   │   └── knowledge/
│   └── services/             # 应用服务
│
├── infrastructure/           # 基础设施层 - 外部依赖实现
│   ├── persistence/          # 数据持久化
│   │   ├── mysql/
│   │   ├── milvus/
│   │   └── redis/
│   ├── llm/                  # LLM 客户端
│   └── agent/                # Agent 工具
│
└── interface/                # 接口层 - HTTP handlers
    └── http/
        ├── handler/
        └── middleware/
```

**依赖方向**：`Interface → Application → Domain ← Infrastructure`

### 目标架构：Component-based Architecture

```
internal/
├── pkg/                      # 公共库（所有层可用）
│   ├── errors/               # 错误定义
│   ├── types/                # 通用类型
│   └── utils/                # 工具函数
│
├── components/               # 核心组件接口（Domain 层接口定义）
│   ├── llm/                  # LLM 组件接口
│   ├── retriever/            # 检索器接口
│   ├── agent/                # Agent 组件接口
│   ├── memory/               # Memory 组件接口
│   └── kb/                   # 知识库组件接口
│
├── service/                  # 业务逻辑实现
│   ├── agent/                # Agent 服务实现
│   ├── rag/                  # RAG 服务
│   ├── kb/                   # 知识库服务
│   ├── chat/                 # 聊天服务
│   └── llm/                  # LLM 服务
│
├── store/                    # 数据访问层
│   ├── mysql/                # MySQL 实现
│   ├── milvus/               # Milvus 实现
│   ├── redis/                # Redis 实现
│   ├── neo4j/                # Neo4j 实现
│   └── interface.go          # 存储接口定义
│
└── handler/                  # HTTP 处理层
    ├── agent/
    ├── chat/
    ├── kb/
    └── middleware/
```

**依赖方向**：`handler → service → components ← store`，`pkg` 被所有层使用

### 架构差异分析

| 维度 | Clean Architecture | Component-based |
|------|-------------------|-----------------|
| **分层粒度** | 4 层（domain/application/infrastructure/interface）| 5 层（pkg/components/service/store/handler） |
| **接口定义** | domain 层定义 Repository 接口 | components 层定义所有核心能力接口 |
| **业务逻辑** | application 层编排用例 | service 层直接实现业务逻辑 |
| **数据访问** | infrastructure 层实现 Repository | store 层封装所有存储操作 |
| **公共代码** | 散落在各层 | 集中在 pkg 层 |

---

## 迁移策略

### 渐进式迁移（推荐）

采用**双轨运行**策略，确保服务在迁移过程中保持可用。

```
阶段 0: 准备阶段
├── 建立新结构（不删除旧代码）
└── 迁移公共代码到 pkg/

阶段 1: 基础迁移
├── 迁移 components 接口（从 domain 提取）
└── 迁移 store 层（从 infrastructure/persistence）

阶段 2: 服务迁移
├── 选择一个模块试点（如 evaluation）
├── 逐步迁移其他模块
└── 删除旧代码

阶段 3: Handler 迁移
├── 迁移 HTTP handlers
└── 删除 interface 层

阶段 4: 清理
└── 删除旧的 domain/application/infrastructure 结构
```

### 模块迁移优先级

| 优先级 | 模块 | 理由 |
|--------|------|------|
| 1 | evaluation | 结构独立，影响范围小 |
| 2 | kb | 相对独立，可验证架构 |
| 3 | rag | 依赖 kb，可作为集成验证 |
| 4 | agent | 核心模块，依赖较多 |
| 5 | chat | 最高层，最后迁移 |

---

## 迁移步骤

### 阶段 0：准备阶段

#### 0.1 建立新目录结构

```bash
cd link-go/internal

# 创建新目录（不影响现有结构）
mkdir -p pkg/{errors,types,utils}
mkdir -p components/{llm,retriever,agent,memory,kb}
mkdir -p service_v2  # 使用 v2 后缀避免冲突
mkdir -p store
mkdir -p handler
```

#### 0.2 迁移公共代码到 pkg/

**目标**：将跨层使用的公共代码集中到 pkg/

```bash
# 迁移错误定义
mv domain/errors/* pkg/errors/

# 迁移通用类型
mv domain/types/*.go pkg/types/  # 排除 interfaces/ 目录

# 迁移工具函数
# 检查各层是否有重复的工具函数，合并到 pkg/utils/
```

**代码示例**：

```go
// pkg/errors/errors.go
package errors

var (
    // Agent errors
    ErrAgentNotFound    = errors.New("agent not found")
    ErrInvalidAgentConfig = errors.New("invalid agent config")

    // RAG errors
    ErrRetrievalFailed  = errors.New("retrieval failed")
    ErrInvalidQuery     = errors.New("invalid query")

    // Knowledge base errors
    ErrKBNotFound       = errors.New("knowledge base not found")
    ErrDocumentNotFound = errors.New("document not found")
)
```

### 阶段 1：基础迁移

#### 1.1 迁移 Components 接口

**原则**：从 domain 层提取核心能力接口，不包含实现

**映射关系**：

| 源路径 | 目标路径 | 说明 |
|--------|----------|------|
| `domain/llm/` | `components/llm/` | LLM 组件接口 |
| `domain/rag/retriever.go` | `components/retriever/` | 检索器接口 |
| `domain/agent/` | `components/agent/` | Agent 组件接口 |
| `domain/memory/` | `components/memory/` | Memory 组件接口 |
| `domain/knowledge/` | `components/kb/` | 知识库组件接口 |

**迁移示例**：

```go
// components/llm/interface.go
package llm

// ChatModel 聊天模型接口
type ChatModel interface {
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    Stream(ctx context.Context, req *ChatRequest) (<-chan *Chunk, error)
}

// EmbeddingModel 向量化模型接口
type EmbeddingModel interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    EmbedQuery(ctx context.Context, query string) ([]float32, error)
}
```

```go
// components/retriever/interface.go
package retriever

// Retriever 检索器接口
type Retriever interface {
    Retrieve(ctx context.Context, query *Query) (*Result, error)
    Stream(ctx context.Context, query *Query) (<-chan *Document, error)
}

// Query 检索查询
type Query struct {
    Text       string
    TopK       int
    Filter     map[string]string
}
```

#### 1.2 迁移 Store 层

**原则**：将 infrastructure/persistence 重构为 store

**映射关系**：

| 源路径 | 目标路径 |
|--------|----------|
| `infrastructure/persistence/mysql/` | `store/mysql/` |
| `infrastructure/persistence/milvus/` | `store/milvus/` |
| `infrastructure/persistence/redis/` | `store/redis/` |
| `infrastructure/persistence/neo4j/` | `store/neo4j/` |

**迁移步骤**：

```bash
# 直接移动目录
mv infrastructure/persistence/mysql store/
mv infrastructure/persistence/milvus store/
mv infrastructure/persistence/redis store/
mv infrastructure/persistence/neo4j store/
```

**接口定义**：

```go
// store/interface.go
package store

// Interfaces 定义所有存储接口
type Interfaces struct {
    Agent      AgentRepository
    Knowledge  KnowledgeRepository
    Document   DocumentRepository
    Vector     VectorRepository
    Graph      GraphRepository
    Session    SessionRepository
    Cache      CacheRepository
    Lock       LockRepository
}

// AgentRepository Agent 存储接口
type AgentRepository interface {
    Save(ctx context.Context, agent *Agent) error
    FindByID(ctx context.Context, id string) (*Agent, error)
    List(ctx context.Context, filter *Filter) ([]*Agent, error)
    Delete(ctx context.Context, id string) error
}

// ... 其他接口
```

### 阶段 2：服务迁移

#### 2.1 选择试点模块：Evaluation

**为什么选择 Evaluation**：
- 结构相对独立
- 依赖关系清晰
- 影响范围小
- 可快速验证架构

**迁移步骤**：

```bash
# 创建新服务目录
mkdir -p service_v2/evaluation

# 迁移代码
cp -r application/usecases/evaluation/* service_v2/evaluation/
cp -r domain/evaluation/* service_v2/evaluation/
```

**重构服务实现**：

```go
// service_v2/evaluation/service.go
package evaluation

import (
    "link/internal/components/llm"
    "link/internal/store"
)

// Service 评测服务
type Service struct {
    store     *store.Interfaces
    llm       llm.ChatModel
    executors map[string]Executor
}

func NewService(
    store *store.Interfaces,
    llm llm.ChatModel,
) *Service {
    return &Service{
        store:     store,
        llm:       llm,
        executors: make(map[string]Executor),
    }
}

// CreateEvaluation 创建评测任务
func (s *Service) CreateEvaluation(ctx context.Context, req *CreateRequest) (*Task, error) {
    // 1. 创建任务实体
    task := NewTask(req)

    // 2. 持久化
    if err := s.store.Evaluation.Save(ctx, task); err != nil {
        return nil, err
    }

    // 3. 推送到队列
    if err := s.store.Task.Enqueue(ctx, task); err != nil {
        return nil, err
    }

    return task, nil
}
```

#### 2.2 更新路由和 Handler

```go
// handler/evaluation/evaluation_handler.go
package evaluation

import (
    "link/internal/service/evaluation"
)

type Handler struct {
    service *evaluation.Service
}

func NewHandler(service *evaluation.Service) *Handler {
    return &Handler{service: service}
}

func (h *Handler) CreateEvaluation(c *gin.Context) {
    var req evaluation.CreateRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    task, err := h.service.CreateEvaluation(c.Request.Context(), &req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(201, task)
}
```

#### 2.3 逐步迁移其他模块

按优先级迁移：`kb` → `rag` → `agent` → `chat`

### 阶段 3：Handler 迁移

```bash
# 迁移 handlers
mv interface/http/handler/* handler/

# 迁移中间件
mv interface/http/middleware/* handler/middleware/

# 迁移响应类型
mv interface/http/response/* handler/response/
```

### 阶段 4：清理

```bash
# 确认所有功能正常后，删除旧结构
rm -rf internal/domain/
rm -rf internal/application/
rm -rf internal/infrastructure/
rm -rf internal/interface/

# 重命名 service_v2 为 service
mv internal/service_v2 internal/service
```

---

## 目录结构映射

### 完整映射表

| 当前路径 | 目标路径 | 说明 |
|----------|----------|------|
| `domain/errors/` | `pkg/errors/` | 错误定义 |
| `domain/types/*.go` | `pkg/types/` | 通用类型（非接口）|
| `domain/llm/` | `components/llm/` | LLM 组件接口 |
| `domain/rag/` | `components/retriever/`, `service/rag/` | 接口 + 服务 |
| `domain/agent/` | `components/agent/`, `service/agent/` | 接口 + 服务 |
| `domain/memory/` | `components/memory/`, `service/memory/` | 接口 + 服务 |
| `domain/knowledge/` | `components/kb/`, `service/kb/` | 接口 + 服务 |
| `application/usecases/*` | `service/*` | 业务逻辑 |
| `application/services/*` | `service/*` | 应用服务 |
| `infrastructure/persistence/mysql/` | `store/mysql/` | MySQL 存储 |
| `infrastructure/persistence/milvus/` | `store/milvus/` | 向量存储 |
| `infrastructure/persistence/redis/` | `store/redis/` | 缓存存储 |
| `infrastructure/persistence/neo4j/` | `store/neo4j/` | 图存储 |
| `infrastructure/llm/` | `service/llm/` | LLM 服务实现 |
| `infrastructure/agent/tools/` | `service/agent/tools/` | Agent 工具 |
| `interface/http/handler/` | `handler/` | HTTP 处理器 |
| `interface/http/middleware/` | `handler/middleware/` | 中间件 |

---

## 代码迁移示例

### 示例 1：迁移 Agent 模块

#### Before (Clean Architecture)

```go
// domain/agent/entity.go
package agent

type Agent struct {
    ID     string
    Name   string
    Type   AgentType
    Config *Config
}

func (a *Agent) Execute(ctx context.Context, input string) (*Result, error) {
    // 执行逻辑
}

// domain/agent/repository.go
package agent

type Repository interface {
    Save(ctx context.Context, agent *Agent) error
    FindByID(ctx context.Context, id string) (*Agent, error)
}

// application/usecases/agent/execute.go
package agent

type ExecuteUseCase struct {
    repo   Repository
    llm    LLMService
}

func (uc *ExecuteUseCase) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
    agent, err := uc.repo.FindByID(ctx, req.AgentID)
    if err != nil {
        return nil, err
    }
    return agent.Execute(ctx, req.Input)
}

// infrastructure/persistence/mysql/agent_repo.go
package mysql

type AgentRepository struct {
    db *gorm.DB
}

func (r *AgentRepository) Save(ctx context.Context, agent *agent.Agent) error {
    // MySQL 保存逻辑
}
```

#### After (Component-based)

```go
// components/agent/interface.go
package agent

// Agent 接口定义核心能力
type Agent interface {
    Chat(ctx context.Context, message string, opts ...Option) (*Response, error)
    Stream(ctx context.Context, message string, opts ...Option) (<-chan *Chunk, error)
}

// Option 配置选项
type Option func(*Options)

type Options struct {
    Tools    []Tool
    Memory   Memory
    TopK     int
}

// Response 响应
type Response struct {
    Content  string
    ToolCalls []*ToolCall
    Metadata map[string]interface{}
}

// components/agent/types.go
package agent

type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, input string) (string, error)
}

type Memory interface {
    Add(ctx context.Context, messages []*Message) error
    Get(ctx context.Context, limit int) ([]*Message, error)
}

// service/agent/react.go
package agent

import (
    "link/internal/components/llm"
    "link/internal/components/agent"
)

// ReActAgent ReAct 模式 Agent
type ReActAgent struct {
    model    llm.ChatModel
    tools    []agent.Tool
    maxIter  int
}

func NewReActAgent(model llm.ChatModel, tools []agent.Tool) *ReActAgent {
    return &ReActAgent{
        model:   model,
        tools:   tools,
        maxIter: 10,
    }
}

func (a *ReActAgent) Chat(ctx context.Context, message string, opts ...agent.Option) (*agent.Response, error) {
    options := &agent.Options{}
    for _, opt := range opts {
        opt(options)
    }

    // ReAct 循环逻辑
    for i := 0; i < a.maxIter; i++ {
        // 1. 思考
        thought, err := a.model.Chat(ctx, &llm.ChatRequest{
            Messages: []*llm.Message{{Role: "user", Content: message}},
        })
        if err != nil {
            return nil, err
        }

        // 2. 行动
        if a.needsTools(thought.Content) {
            result := a.executeTools(ctx, thought.Content)
            message += "\n" + result
        } else {
            return &agent.Response{Content: thought.Content}, nil
        }
    }

    return nil, ErrMaxIterationsExceeded
}

// store/mysql/agent_repo.go
package mysql

type AgentRepository struct {
    db *gorm.DB
}

type Agent struct {
    ID     string `gorm:"primaryKey"`
    Name   string
    Type   string
    Config string `gorm:"type:json"`
}

func (r *AgentRepository) Save(ctx context.Context, agent *service.AgentConfig) error {
    entity := toEntity(agent)
    return r.db.WithContext(ctx).Save(entity).Error
}

// handler/agent/agent_handler.go
package agent

import (
    "link/internal/service/agent"
)

type Handler struct {
    service *agent.Service
}

func (h *Handler) Chat(c *gin.Context) {
    var req ChatRequest
    c.ShouldBindJSON(&req)

    resp, err := h.service.Chat(c.Request.Context(), &req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, resp)
}
```

### 示例 2：迁移 RAG 模块

#### Before

```go
// domain/rag/entity.go
package rag

type Document struct {
    ID       string
    Content  string
    Metadata map[string]interface{}
}

// domain/rag/repository.go
package rag

type Retriever interface {
    Retrieve(ctx context.Context, query string, topK int) ([]*Document, error)
}

// application/usecases/rag/query.go
package rag

type QueryUseCase struct {
    retriever Retriever
    llm       LLMService
}
```

#### After

```go
// components/retriever/interface.go
package retriever

type Retriever interface {
    Retrieve(ctx context.Context, query *Query) (*Result, error)
}

type Query struct {
    Text       string
    TopK       int
    Filter     map[string]string
}

type Result struct {
    Documents []*Document
    Scores    []float32
}

// service/rag/hybrid.go
package rag

import (
    "link/internal/components/retriever"
)

type HybridRetriever struct {
    dense   retriever.Retriever  // 向量检索
    sparse  retriever.Retriever  // 关键词检索
    weights *Weights
}

func (r *HybridRetriever) Retrieve(ctx context.Context, query *retriever.Query) (*retriever.Result, error) {
    // 并行检索
    var wg sync.WaitGroup
    var denseResult, sparseResult *retriever.Result

    wg.Add(2)
    go func() {
        defer wg.Done()
        denseResult, _ = r.dense.Retrieve(ctx, query)
    }()
    go func() {
        defer wg.Done()
        sparseResult, _ = r.sparse.Retrieve(ctx, query)
    }()
    wg.Wait()

    // 融合结果
    return r.merge(denseResult, sparseResult), nil
}

// store/milvus/vector_repo.go
package milvus

type VectorRepository struct {
    client *milvus.Client
}

func (r *VectorRepository) Search(ctx context.Context, vector []float32, topK int) ([]*Document, error) {
    // Milvus 检索逻辑
}
```

---

## 验证清单

### 每个模块迁移后检查

- [ ] 所有单元测试通过
- [ ] 所有集成测试通过
- [ ] API 端点响应正常
- [ ] 性能指标无退化
- [ ] 依赖注入正确配置
- [ ] 错误处理完整

### 架构一致性检查

- [ ] handler → service → components 依赖正确
- [ ] store 实现 components/service 定义的接口
- [ ] pkg 被正确引用，无循环依赖
- [ ] 无 domain/application/infrastructure 残留引用

### 最终验证

- [ ] 所有 API 功能正常
- [ ] 数据迁移正确
- [ ] 日志完整
- [ ] 监控指标正常
- [ ] 文档更新

---

## 注意事项

### 1. 导入路径更新

迁移后需要更新所有导入路径：

```go
// 旧的导入
import "link/internal/domain/agent"

// 新的导入
import "link/internal/components/agent"
```

### 2. 依赖注入配置

更新 `internal/application/initializer/` 中的初始化代码：

```go
// 新的初始化方式
func InitializeService(cfg *config.Config) (*service.Interfaces, error) {
    // 初始化 store
    store := store.New(cfg.Database)

    // 初始化 components 实现
    llmService := llm.NewOpenAI(cfg.LLM)

    // 初始化 services
    agentService := agent.NewService(store, llmService)
    ragService := rag.NewService(store, llmService)

    return &service.Interfaces{
        Agent: agentService,
        RAG:   ragService,
    }, nil
}
```

### 3. 配置文件

确保配置文件路径正确：

```go
// config/config.go
type Config struct {
    Database DatabaseConfig
    LLM      LLMConfig
    Store    StoreConfig  // 新增
}
```

### 4. 测试迁移

测试文件需要同步迁移：

```bash
# 迁移测试
mv domain/agent/*_test.go components/agent/
mv application/usecases/agent/*_test.go service/agent/
```

---

## 回滚计划

如果迁移出现问题，可以按以下步骤回滚：

1. 恢复数据库快照
2. 切换回旧版本代码
3. 分析失败原因
4. 修复后重新迁移

建议在迁移前创建 Git 分支：

```bash
git checkout -b feature/architecture-migration
```

---

## 参考资料

- [Eino - 字节跳动组件化框架](https://github.com/cloudwego/eino)
- [Grafana 架构](https://github.com/grafana/grafana)
- [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [架构参考设计](./architecture-reference.md)
