# 3-Layer 架构迁移指南

> 本文档描述 Link-Go 项目从 4-Layer Clean Architecture 到 3-Layer Service Architecture 的迁移过程

## 概述

### 迁移原因

1. **简化层次**：4 层架构过于复杂，增加开发成本
2. **减少 DTO 转换**：Handler → Service → Domain 层层转换，增加维护负担
3. **提高开发效率**：减少抽象层级，更符合 Go 实用主义

### 架构对比

#### 旧架构 (4-Layer)

```
Interface → Application → Domain ← Infrastructure
```

- **Interface**：HTTP handlers，依赖 Application
- **Application**：Use cases + DTOs，依赖 Domain
- **Domain**：实体 + Repository 接口
- **Infrastructure**：外部依赖实现

#### 新架构 (3-Layer)

```
Handler → Service → Model ← Repository
```

- **Handler**：HTTP handlers，依赖 Service
- **Service**：业务逻辑 +编排，依赖 Model
- **Model**：实体 + Repository 接口
- **Repository**：数据访问实现

---

## 目录结构变化

### 旧结构

```
internal/
├── interface/http/
│   ├── handler/
│   └── middleware/
├── application/
│   ├── usecases/
│   └── services/
├── domain/
└── infrastructure/
    └── persistence/
        ├── mysql/
        ├── milvus/
        └── neo4j/
```

### 新结构

```
internal/
├── handler/                    # 原 interface/http/handler
│   └── middleware/             # 原 interface/http/middleware
├── service/                    # 原 application/usecases + services
├── model/                      # 原 domain
└── repository/                 # 原 infrastructure/persistence
    ├── mysql/
    ├── milvus/
    ├── neo4j/
    └── redis/
```

---

## 迁移步骤

### 步骤 1：更新导入路径

#### Handler 层

```go
// 旧
import "link/internal/interface/http/handler"

// 新
import "link/internal/handler"
```

#### Service 层

```go
// 旧
import "link/internal/application/usecases/agent"

// 新
import "link/internal/service/agent"
```

#### Model 层

```go
// 旧
import "link/internal/domain/agent"

// 新
import "link/internal/model/agent"
```

#### Repository 层

```go
// 旧
import "link/internal/infrastructure/persistence/mysql"

// 新
import "link/internal/repository/mysql"
```

### 步骤 2：删除适配器模式

旧架构使用适配器桥接不同接口，新架构直接使用 Model 接口：

```go
// 旧方式：使用适配器
kbRepoAdapter := app_kb.NewKnowledgeBaseRepositoryAdapter(c.KBRepo)
c.KBService = app_kb.NewKnowledgeBaseService(kbRepoAdapter)

// 新方式：直接注入
c.KBService = service.NewKnowledgeService(c.KBRepo)
```

### 步骤 3：简化 DTO 转换

Handler 和 Service 现在可以直接使用 Model 类型：

```go
// 旧方式：层层转换
Handler DTO → Service DTO → Domain Entity → GORM Model

// 新方式：最小转换
Handler DTO (Gin binding) → Model Entity → GORM Model
```

---

## 代码示例

### Handler 示例

```go
// handler/agent/agent.go
package agent

import (
    "github.com/gin-gonic/gin"

    "link/internal/service/agent"
)

type AgentHandler struct {
    agentService *agent.Service
}

func NewAgentHandler(svc *agent.Service) *AgentHandler {
    return &AgentHandler{agentService: svc}
}

type ExecuteRequest struct {
    AgentID string                 `json:"agent_id"`
    Input   map[string]interface{} `json:"input"`
}

func (h *AgentHandler) Execute(c *gin.Context) {
    var req ExecuteRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // 直接调用 Service
    result, err := h.agentService.Execute(c.Request.Context(), &agent.ExecuteRequest{
        AgentID: req.AgentID,
        Input:   req.Input,
    })
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, result)
}
```

### Service 示例

```go
// service/agent/agent.go
package agent

import (
    "context"
    "link/internal/model/agent"
)

type Service struct {
    agentRepo agent.Repository
    llm       LLMService
}

func (s *Service) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
    // 直接使用 Model 类型
    agentDef, err := s.agentRepo.GetByID(ctx, req.AgentID)
    if err != nil {
        return nil, err
    }

    // 业务逻辑
    return s.doExecute(ctx, agentDef, req)
}
```

### Repository 示例

```go
// repository/mysql/agent_repo.go
package mysql

import (
    "context"
    "link/internal/model/agent"
)

type AgentRepository struct {
    db *gorm.DB
}

func (r *AgentRepository) GetByID(ctx context.Context, id string) (*agent.Agent, error) {
    var gormModel GormAgent
    if err := r.db.WithContext(ctx).Where("id = ?", id).First(&gormModel).Error; err != nil {
        return nil, err
    }
    return gormModel.ToDomain(), nil
}
```

---

## 验证清单

迁移完成后，验证以下项目：

- [ ] `go build ./...` 编译通过
- [ ] `go vet ./...` 无警告
- [ ] 所有测试通过
- [ ] 无遗留的旧导入路径
- [ ] 依赖关系正确：handler → service → model ← repository

---

## 常见问题

### Q1: 为什么删除 Application 层？

A: Application 层的 Use Cases 和 Services 职责重叠，合并后减少冗余。

### Q2: 为什么不使用 DTO？

A: 现代架构更倾向于直接使用领域实体，减少不必要的类型转换。

### Q3: 如何处理 HTTP 请求绑定？

A: 在 Handler 层定义请求结构体用于 Gin 绑定，然后转换为 Model 类型传递给 Service。

### Q4: Repository 层如何处理多种存储？

A: 不同存储（MySQL、Milvus、Neo4j）在 repository/ 下有独立实现，都实现 Model 层定义的接口。

---

## 参考

- 主架构文档：`docs/go/CLEAN_ARCHITECTURE.md`
- CLAUDE.md：`link-go/CLAUDE.md`
