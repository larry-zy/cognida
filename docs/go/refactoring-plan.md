# Link-Go 渐进式简化重构方案

> 从 4 层 Clean Architecture 简化为 3 层实用架构

## 目录

- [重构目标](#重构目标)
- [目标架构](#目标架构)
- [分阶段执行计划](#分阶段执行计划)
- [详细实施步骤](#详细实施步骤)
- [代码迁移示例](#代码迁移示例)
- [风险控制](#风险控制)
- [验证标准](#验证标准)

---

## 重构目标

### 主要目标

| 目标 | 当前状态 | 目标状态 | 改善 |
|------|----------|----------|------|
| 代码行数 | ~10 万行 | ~7 万行 | -30% |
| 层间转换代码 | ~35% | ~15% | -57% |
| 新功能开发时间 | ~60 分钟 | ~25 分钟 | -58% |
| Repository 接口 | 48 个 | ~15 个 | -69% |
| 层数 | 4 层 | 3 层 | -25% |

### 非目标

- ❌ 不改变核心业务逻辑
- ❌ 不改变外部 API 接口
- ❌ 不更换数据库或中间件
- ❌ 不采用全新的架构模式

---

## 目标架构

### 简化后的 3 层架构

```
┌─────────────────────────────────────────────────────────────┐
│                         Handler 层                          │
│  HTTP/gRPC 协议处理、请求验证、响应封装                      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                         Service 层                          │
│  业务逻辑编排、Agent 执行、RAG 检索、LLM 调用               │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      Repository 层                          │
│  数据访问：MySQL、Milvus、Redis、Neo4j                      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                         Model 层                           │
│  数据模型定义（实体、值对象、请求响应类型）                   │
└─────────────────────────────────────────────────────────────┘
```

### 目录结构对比

#### 当前结构

```
internal/
├── interface/http/           # HTTP 处理
│   ├── handler/
│   └── middleware/
├── application/              # 应用层（臃肿）
│   ├── usecases/             # 用例
│   └── services/             # 服务（与 usecases 重复）
├── model/                    # 模型层（数据结构）
│   ├── agent/
│   ├── rag/
│   └── knowledge/
└── infrastructure/           # 基础设施
    ├── persistence/
    ├── llm/
    └── agent/
```

#### 目标结构

```
internal/
├── handler/                  # HTTP 处理层
│   ├── agent.go              # Agent HTTP 处理
│   ├── chat.go               # Chat HTTP 处理
│   ├── knowledge.go                 # Knowledge HTTP 处理
│   ├── rag.go                # RAG HTTP 处理
│   └── middleware/           # 中间件
│
├── service/                  # 业务逻辑层（含核心实现）
│   ├── agent/
│   │   ├── agent.go          # Agent 服务
│   │   ├── react.go          # ReAct 逻辑
│   │   └── tools.go          # 工具管理
│   ├── rag/
│   │   ├── retriever.go      # 检索服务
│   │   ├── pipeline.go       # RAG 流程
│   │   └── rerank.go         # 重排
│   ├── llm/
│   │   ├── chat.go           # 聊天服务
│   │   └── embedding.go      # 向量化
│   ├── knowledge/
│   │   ├── knowledge.go             # Knowledge 服务
│   │   └── document.go       # 文档处理
│   ├── chat/
│   │   ├── service.go        # 聊天编排
│   │   └── session.go        # 会话管理
│   └── evaluation/
│       └── service.go        # 评测服务
│
├── repository/               # 数据访问层
│   ├── mysql/
│   │   ├── agent_repo.go
│   │   ├── knowledge_repo.go
│   │   └── session_repo.go
│   ├── milvus/
│   │   └── vector_repo.go
│   ├── redis/
│   │   ├── cache.go
│   │   └── lock.go
│   └── neo4j/
│       └── graph_repo.go
│
└── model/                    # 数据模型定义层
    ├── agent/
    │   └── entity.go         # Agent 实体
    ├── rag/
    │   └── entity.go         # RAG 实体
    ├── knowledge/
    │   └── entity.go         # Knowledge 实体
    ├── chat/
    │   └── entity.go         # 聊天实体
    └── types/
        └── common.go         # 通用类型
```

---

## 分阶段执行计划

### 时间线概览

```
Sprint 1-2 (2周)    Sprint 3-4 (2周)    Sprint 5-6 (2周)    Sprint 7-8 (2周)
    │                   │                   │                   │
    ▼                   ▼                   ▼                   ▼
┌─────────┐        ┌─────────┐        ┌─────────┐        ┌─────────┐
│ Phase 1 │        │ Phase 2 │        │ Phase 3 │        │ Phase 4 │
│ 合并重复 │   →   │ 删除适配器 │   →   │ 简化转换 │   →   │ 评估优化 │
└─────────┘        └─────────┘        └─────────┘        └─────────┘
    │                   │                   │                   │
    ▼                   ▼                   ▼                   ▼
 合并 usecases        删除 Adapter       减少 DTO            决定是否
   和 services          模式               转换               继续调整
```

### 阶段详情

#### Phase 1: 合并重复模块（Sprint 1-2）

**目标**：消除 usecases 和 services 的重复

| 任务 | 优先级 | 预估时间 |
|------|--------|----------|
| 1.1 合并 RAG 模块 | 高 | 2 天 |
| 1.2 合并 Agent 模块 | 高 | 3 天 |
| 1.3 合并 LLM 模块 | 中 | 2 天 |
| 1.4 合并 Knowledge 模块 | 中 | 2 天 |
| 1.5 删除空目录 | 低 | 1 天 |

#### Phase 2: 删除适配器和违规依赖（Sprint 3-4）

**目标**：修复架构违规，删除适配器

| 任务 | 优先级 | 预估时间 |
|------|--------|----------|
| 2.1 删除 AgentExecutableAdapter | 高 | 2 天 |
| 2.2 修复 Service → Repository 依赖违规 | 高 | 4 天 |
| 2.3 删除重复的 Repository 接口 | 高 | 2 天 |
| 2.4 统一错误处理 | 中 | 1 天 |

#### Phase 3: 简化 DTO 转换（Sprint 5-6）

**目标**：减少层间类型转换

| 任务 | 优先级 | 预估时间 |
|------|--------|----------|
| 3.1 Handler 直接使用 Service 类型 | 高 | 3 天 |
| 3.2 Service 直接使用 Model 实体 | 高 | 3 天 |
| 3.3 删除冗余 DTO | 中 | 2 天 |
| 3.4 统一请求/响应类型 | 中 | 2 天 |

#### Phase 4: 评估与优化（Sprint 7-8）

**目标**：评估重构效果，决定后续方向

| 任务 | 优先级 | 预估时间 |
|------|--------|----------|
| 4.1 性能测试 | 高 | 2 天 |
| 4.2 代码审查 | 高 | 2 天 |
| 4.3 文档更新 | 中 | 2 天 |
| 4.4 决定是否继续调整 | 高 | 1 天 |

---

## 详细实施步骤

### Phase 1.1: 合并 RAG 模块

**当前状态**：

```
application/
├── usecases/rag/
│   ├── retrieve.go         # 检索用例
│   ├── query.go            # 查询用例
│   ├── chat.go             # 聊天用例
│   └── service.go          # RAG 服务
└── services/rag/
    └── retrieval_optimizer.go  # 检索优化服务（重复！）
```

**目标状态**：

```
service/rag/
├── retriever.go            # 统一的检索服务
├── pipeline.go             # RAG 流程
├── rerank.go               # 重排服务
└── optimizer.go            # 检索优化
```

**迁移步骤**：

```bash
# 1. 创建新目录
mkdir -p internal/service/rag

# 2. 合并检索功能
# 将 usecases/rag/retrieve.go 和 services/rag/retrieval_optimizer.go 合并
# 到 service/rag/retriever.go

# 3. 更新导入路径
# find . -name "*.go" -exec sed -i 's|link/internal/application/usecases/rag|link/internal/service/rag|g' {} \;
# find . -name "*.go" -exec sed -i 's|link/internal/application/services/rag|link/internal/service/rag|g' {} \;

# 4. 删除旧目录（确认无引用后）
rm -rf internal/application/usecases/rag
rm -rf internal/application/services/rag
```

**代码示例**：

```go
// service/rag/retriever.go (合并后)

package rag

import (
    "context"
    "link/internal/model/rag"
    "link/internal/repository/milvus"
)

// Retriever 统一的检索服务
type Retriever struct {
    vectorRepo  *milvus.VectorRepository
    reranker    *Reranker
    llm         LLMClient
}

// NewRetriever 创建检索服务
func NewRetriever(
    vectorRepo *milvus.VectorRepository,
    reranker *Reranker,
    llm LLMClient,
) *Retriever {
    return &Retriever{
        vectorRepo: vectorRepo,
        reranker:   reranker,
        llm:        llm,
    }
}

// Retrieve 统一检索接口（合并原来的多个用例）
func (r *Retriever) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResponse, error) {
    // 1. 基础检索
    docs, err := r.vectorRepo.Search(ctx, req.Query, req.TopK)
    if err != nil {
        return nil, err
    }

    // 2. 可选：重排
    if req.EnableRerank {
        docs, err = r.reranker.Rerank(ctx, req.Query, docs)
        if err != nil {
            return nil, err
        }
    }

    // 3. 可选：查询优化
    if req.EnableOptimization {
        return r.optimizedRetrieve(ctx, req, docs)
    }

    return &RetrieveResponse{Documents: docs}, nil
}
```

### Phase 2.1: 删除 AgentExecutableAdapter

**当前状态**：

```go
// application/usecases/llm/agent_adapter.go

type AgentExecutableAdapter struct {
    agent infraagent.Agent  // 依赖 Infrastructure
}

func (a *AgentExecutableAdapter) Chat(...) (*agent.ChatResponse, error) {
    chatResp, err := a.agent.Chat(ctx, message)
    // 大量转换代码...
}
```

**目标状态**：删除适配器，直接使用正确的接口

```go
// service/agent/agent.go

package agent

import (
    "context"
    "link/internal/model/agent"
    "link/internal/repository/llm"
)

// Service Agent 服务
type Service struct {
    executor agent.AgentExecutor  // 使用 Model 接口
    llm      *llm.LLMClient
}

// NewService 创建服务
func NewService(executor agent.AgentExecutor, llm *llm.LLMClient) *Service {
    return &Service{
        executor: executor,
        llm:      llm,
    }
}

// Chat 执行聊天（直接调用 Model 接口）
func (s *Service) Chat(ctx context.Context, agentID, message string) (*ChatResponse, error) {
    // 直接调用 Model 接口，无需转换
    resp, err := s.executor.Chat(ctx, agentID, message)
    if err != nil {
        return nil, err
    }

    // Model 返回的类型已经符合需求，无需转换
    return &ChatResponse{
        Content:  resp.Content,
        ToolCalls: resp.ToolCalls,
    }, nil
}
```

**迁移步骤**：

```bash
# 1. 确保 Model 层有完整的 AgentExecutor 接口
# model/agent/executor.go

# 2. 更新 Service 使用 Model 接口
# 修改所有使用 AgentExecutableAdapter 的地方

# 3. 删除适配器文件
rm internal/application/usecases/llm/agent_adapter.go
```

### Phase 2.3: 删除重复的 Repository 接口

**当前状态**：

```go
// model/knowledge/repository.go
type ChunkRepository interface {
    Save(ctx context.Context, chunk *Chunk) error
    FindByID(ctx context.Context, id string) (*Chunk, error)
}

// service/graph/graph.go (重复！)
type ChunkRepository interface {
    GetChunk(ctx context.Context, chunkID string) (*GraphChunk, error)
    GetChunks(ctx context.Context, chunkIDs []string) ([]*GraphChunk, error)
}
```

**目标状态**：只在 Model 层定义接口

```go
// model/knowledge/repository.go (扩展)

type ChunkRepository interface {
    // 基础 CRUD
    Save(ctx context.Context, chunk *Chunk) error
    FindByID(ctx context.Context, id string) (*Chunk, error)

    // 批量查询（添加）
    FindByIDs(ctx context.Context, ids []string) ([]*Chunk, error)

    // 图谱相关（添加）
    FindForGraph(ctx context.Context, chunkID string) (*GraphChunk, error)
}
```

**迁移步骤**：

```bash
# 1. 扩展 Model 层接口，添加需要的方法

# 2. 更新 Infrastructure 实现以支持新方法

# 3. 更新 Service 使用 Model 接口

# 4. 删除 Service 层的重复接口定义
```

### Phase 3.1: Handler 直接使用 Service 类型

**当前状态**：

```go
// interface/http/handler/agent_handler.go

type AgentHandler struct {
    executeUseCase  agentuc.ExecuteUseCase     // UseCase 接口
    researchUseCase agentuc.ResearchUseCase    // UseCase 接口
}

func (h *AgentHandler) Chat(c *gin.Context) {
    var req agentuc.AgenticRAGRequest  // UseCase DTO
    c.ShouldBindJSON(&req)

    resp, err := h.executeUseCase.Execute(c.Request.Context(), &req)
    // 使用 UseCase DTO
}
```

**目标状态**：

```go
// handler/agent.go

package handler

import (
    "link/internal/service/agent"
)

type Handler struct {
    agentService *agent.Service  // 直接使用 Service
}

func (h *Handler) Chat(c *gin.Context) {
    var req agent.ChatRequest  // Service 定义的请求类型
    c.ShouldBindJSON(&req)

    resp, err := h.agentService.Chat(c.Request.Context(), &req)
    // 使用 Service 响应类型
}
```

**迁移步骤**：

```bash
# 1. 将 DTO 定义移到 Service 包

# 2. Handler 直接使用 Service 类型

# 3. 删除 UseCase 层的 DTO 文件
```

---

## 代码迁移示例

### 示例 1：聊天功能

#### Before（4 层架构）

```go
// ========== 1. Interface Layer ==========
// interface/http/handler/chat_handler.go

type ChatHandler struct {
    chatUseCase *llmuc.ChatUseCase
}

func (h *ChatHandler) Chat(c *gin.Context) {
    var req llmuc.ChatRequestDTO
    c.ShouldBindJSON(&req)

    resp, err := h.chatUseCase.Chat(c.Request.Context(), &req)
    c.JSON(200, resp)
}

// ========== 2. UseCase Layer ==========
// application/usecases/llm/chat_usecase.go

type ChatUseCase struct {
    llmClient llm.LLMClient
    modelRepo llm.ModelRepository
}

type ChatRequestDTO struct {
    Messages []MessageDTO `json:"messages"`
    Stream   bool         `json:"stream"`
}

type ChatResponseDTO struct {
    Content  string `json:"content"`
    Metadata map[string]interface{} `json:"metadata"`
}

func (uc *ChatUseCase) Chat(ctx context.Context, req *ChatRequestDTO) (*ChatResponseDTO, error) {
    modelReq := ToModelChatRequest(req)  // 转换
    modelResp, err := uc.llmClient.Chat(ctx, modelReq)
    return FromModelChatResponse(modelResp), nil  // 转换
}

// ========== 3. Model Layer ==========
// model/llm/types.go

type ChatRequest struct {
    Messages []Message
}

type ChatResponse struct {
    Content  string
    Metadata Metadata
}

// ========== 4. Repository Layer ==========
// repository/llm/client.go

func (c *Client) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
    // OpenAI API 调用
}
```

#### After（3 层架构）

```go
// ========== 1. Handler Layer ==========
// handler/chat.go

package handler

import (
    "link/internal/service/llm"
)

type ChatHandler struct {
    chatService *llm.Service
}

func (h *ChatHandler) Chat(c *gin.Context) {
    var req llm.ChatRequest  // Service 定义的类型
    c.ShouldBindJSON(&req)

    resp, err := h.chatService.Chat(c.Request.Context(), &req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, resp)
}

// ========== 2. Service Layer ==========
// service/llm/chat.go

package llm

import (
    "context"
    "link/internal/model/llm"
    "link/internal/repository/llm"
)

type Service struct {
    client *llm.Client
    repo   *llm.ModelRepository
}

// ChatRequest 直接使用 Model 类型或扩展
type ChatRequest struct {
    llm.ChatRequest
    Stream bool `json:"stream"`
}

// ChatResponse 直接使用 Model 类型或扩展
type ChatResponse struct {
    llm.ChatResponse
}

func (s *Service) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
    // 直接调用，无需转换
    modelReq := &req.ChatRequest
    modelResp, err := s.client.Chat(ctx, modelReq)
    if err != nil {
        return nil, err
    }

    return &ChatResponse{ChatResponse: *modelResp}, nil
}

// ========== 3. Repository Layer ==========
// repository/llm/client.go

package llm

import (
    "context"
    "link/internal/model/llm"
)

// Client 实现 Model 接口
type Client struct {
    openai *OpenAIClient
    anthropic *AnthropicClient
}

func (c *Client) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
    // 根据模型选择客户端
    if req.Model == "gpt-4" {
        return c.openai.Chat(ctx, req)
    }
    return c.anthropic.Chat(ctx, req)
}
```

**对比**：
- Before: 4 个文件，~150 行代码，多次类型转换
- After: 3 个文件，~80 行代码，最少转换

### 示例 2：RAG 检索

#### Before

```go
// application/usecases/rag/retrieve.go

type RetrieveUseCase struct {
    retrieever model.Retriever
}

type RetrieveRequestDTO struct {
    Query string `json:"query"`
    TopK  int    `json:"top_k"`
}

type RetrieveResponseDTO struct {
    Documents []*DocumentDTO `json:"documents"`
}

func (uc *RetrieveUseCase) Retrieve(ctx context.Context, req *RetrieveRequestDTO) (*RetrieveResponseDTO, error) {
    // 转换
    modelReq := &model.RetrieveRequest{
        Query: req.Query,
        TopK:  req.TopK,
    }

    // 调用
    modelResp, err := uc.retriever.Retrieve(ctx, modelReq)
    if err != nil {
        return nil, err
    }

    // 转换
    return &RetrieveResponseDTO{
        Documents: ToDocumentDTOs(modelResp.Documents),
    }, nil
}
```

#### After

```go
// service/rag/retriever.go

package rag

import (
    "context"
    "link/internal/model/rag"
    "link/internal/repository/milvus"
)

type Retriever struct {
    vectorRepo *milvus.VectorRepository
    reranker   *Reranker
}

// RetrieveRequest 可以直接使用 Model 类型
type RetrieveRequest = rag.RetrieveRequest

// RetrieveResponse 可以直接使用 Model 类型或扩展
type RetrieveResponse struct {
    rag.RetrieveResponse
    Reranked bool `json:"reranked,omitempty"`
}

func (r *Retriever) Retrieve(ctx context.Context, req *RetrieveRequest) (*RetrieveResponse, error) {
    // 直接调用 Repository
    docs, scores, err := r.vectorRepo.Search(ctx, req.Query, req.TopK)
    if err != nil {
        return nil, err
    }

    // 可选重排
    if req.EnableRerank {
        docs, scores, err = r.reranker.Rerank(ctx, req.Query, docs, scores)
        if err != nil {
            return nil, err
        }
    }

    // 直接返回，无需转换
    return &RetrieveResponse{
        RetrieveResponse: rag.RetrieveResponse{
            Documents: docs,
            Scores:    scores,
        },
        Reranked: req.EnableRerank,
    }, nil
}
```

---

## 风险控制

### 风险矩阵

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| 引入新 Bug | 中 | 高 | 完善测试，分阶段上线 |
| API 兼容性 | 低 | 高 | 保持外部 API 不变 |
| 性能下降 | 低 | 中 | 性能基准测试 |
| 团队适应 | 中 | 低 | 文档和培训 |

### 回滚计划

```bash
# 每个阶段完成后打 Tag
git tag -a phase1-complete -m "Phase 1 完成"

# 如果出现问题，快速回滚
git checkout phase1-complete

# 或使用 Git revert
git revert <commit-hash>
```

### 测试策略

```bash
# 1. 单元测试（必须）
go test ./service/... ./repository/... ./handler/...

# 2. 集成测试（必须）
go test ./test/integration/...

# 3. 性能测试（推荐）
go test -bench=. ./...

# 4. 架构测试（新增）
# test/architecture_test.go
func TestNoApplicationImport(t *testing.T) {
    // 禁止导入旧的 application/usecases 包
}
```

---

## 验证标准

### Phase 1 完成标准

- [ ] 所有 RAG 相关代码在 `service/rag/`
- [ ] 无 `application/usecases/rag` 引用
- [ ] 无 `application/services/rag` 引用
- [ ] 所有测试通过

### Phase 2 完成标准

- [ ] 无 `AgentExecutableAdapter` 等适配器
- [ ] 无 Service → Repository 依赖违规
- [ ] Repository 接口只在 Model 层定义
- [ ] 架构测试通过

### Phase 3 完成标准

- [ ] Handler 直接使用 Service 类型
- [ ] Service 直接使用 Model 实体
- [ ] DTO 转换代码减少 50%
- [ ] 代码行数减少 20%

### Phase 4 完成标准

- [ ] 性能无明显下降
- [ ] 代码审查通过
- [ ] 文档更新完成
- [ ] 团队培训完成

---

## 附录

### A. 文件移动清单

```
# Phase 1: 合并模块
application/usecases/agent/*      → service/agent/
application/usecases/rag/*        → service/rag/
application/usecases/llm/*        → service/llm/
application/usecases/knowledge/*  → service/knowledge/
application/usecases/chat/*       → service/chat/
application/services/rag/*        → service/rag/ (合并)
application/services/evaluation/* → service/evaluation/
application/services/graph/*      → service/graph/

# Phase 2: 删除适配器
application/usecases/llm/agent_adapter.go                  → DELETE
application/usecases/llm/agent_adapter_test.go             → DELETE

# Phase 3: 简化结构
interface/http/handler/*        → handler/ (保持)
application/usecases/*          → service/ (已移动)
infrastructure/persistence/*    → repository/ (重命名)
```

### B. 导入路径替换

```bash
# 替换脚本
#!/bin/bash

# application/usecases → service
find . -name "*.go" -exec sed -i 's|link/internal/application/usecases/agent|link/internal/service/agent|g' {} \;
find . -name "*.go" -exec sed -i 's|link/internal/application/usecases/rag|link/internal/service/rag|g' {} \;
find . -name "*.go" -exec sed -i 's|link/internal/application/usecases/llm|link/internal/service/llm|g' {} \;
find . -name "*.go" -exec sed -i 's|link/internal/application/usecases/knowledge|link/internal/service/knowledge|g' {} \;

# application/services → service
find . -name "*.go" -exec sed -i 's|link/internal/application/services/rag|link/internal/service/rag|g' {} \;
find . -name "*.go" -exec sed -i 's|link/internal/application/services/evaluation|link/internal/service/evaluation|g' {} \;

# infrastructure/persistence → repository
find . -name "*.go" -exec sed -i 's|link/internal/infrastructure/persistence/mysql|link/internal/repository/mysql|g' {} \;
find . -name "*.go" -exec sed -i 's|link/internal/infrastructure/persistence/milvus|link/internal/repository/milvus|g' {} \;
find . -name "*.go" -exec sed -i 's|link/internal/infrastructure/persistence/redis|link/internal/repository/redis|g' {} \;
```

### C. 检查清单

每个 Phase 完成后检查：

```bash
# 1. 检查是否有旧的导入引用
grep -r "application/usecases" internal/ --include="*.go" || echo "✓ 无旧引用"
grep -r "application/services" internal/ --include="*.go" || echo "✓ 无旧引用"

# 2. 检查是否有循环依赖
go mod graph | grep "link/internal" | grep "link/internal" || echo "✓ 无循环依赖"

# 3. 运行所有测试
go test ./... -v || echo "✗ 有测试失败"

# 4. 编译检查
go build ./... || echo "✗ 编译失败"
```
