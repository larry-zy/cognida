# Cognida-Go 架构文档

> 本文档记录 Cognida-Go 项目的 3-Layer Service Architecture 架构设计和实现状态

## 更新记录

| 日期 | 进度 |
|-----|------|
| 2025-03-04 | 创建统一的重构文档，整合之前的分散文档 |
| 2026-05-31 | 完成 3-Layer 架构重构，简化层次结构 |

---

## 一、架构目标

### 1.1 3-Layer 分层结构

```
┌─────────────────────────────────────────────────────────────────────┐
│  Handler Layer (接口层) - HTTP/gRPC 接口处理                         │
│  ├── HTTP Handlers                                                   │
│  ├── Routers                                                         │
│  ├── Middlewares                                                     │
│  └── Request DTOs (Gin 绑定)                                         │
└─────────────────────────────────────────────────────────────────────┘
                              ↓ 依赖
┌─────────────────────────────────────────────────────────────────────┐
│  Service Layer (服务层) - 业务逻辑编排                                │
│  ├── 业务服务                                                         │
│  ├── Agent 执行                                                      │
│  ├── Pipeline 流程                                                   │
│  └── 直接使用 Model 类型                                              │
└─────────────────────────────────────────────────────────────────────┘
                              ↓ 依赖
┌─────────────────────────────────────────────────────────────────────┐
│  Model Layer (模型层) - 领域实体和接口定义                            │
│  ├── Entities (实体)                                                 │
│  ├── Value Objects (值对象)                                          │
│  ├── Repository Interfaces (仓储接口定义)                           │
│  └── Domain Errors (领域错误)                                        │
└─────────────────────────────────────────────────────────────────────┘
                              ↑ 实现
┌─────────────────────────────────────────────────────────────────────┐
│  Repository Layer (仓储层) - 数据访问实现                            │
│  ├── MySQL Repository (GORM Models)                                   │
│  ├── Milvus Repository (向量存储)                                    │
│  ├── Neo4j Repository (图存储)                                        │
│  └── Redis Repository (缓存/队列)                                    │
└─────────────────────────────────────────────────────────────────────┘
```

### 1.2 依赖规则

| 规则 | 说明 |
|-----|------|
| **依赖倒置** | Model 层定义接口，Repository 层实现接口 |
| **单向依赖** | Handler → Service → Model ← Repository |
| **核心隔离** | Model 层不依赖任何外部框架 |
| **简化转换** | Handler/Service 直接使用 Model 类型，减少 DTO 转换 |

### 1.3 最终目标结构

```
internal/
├── handler/                           # Handler 层 - HTTP 接口处理
│   ├── agent/                        # Agent handlers
│   ├── chat/                         # Chat handlers
│   ├── knowledge/                    # Knowledge handlers
│   └── middleware/                   # HTTP 中间件
│
├── service/                           # Service 层 - 业务逻辑编排
│   ├── agent/                        # Agent 服务
│   │   ├── core/                     # 通用编排模式 (ReAct, Planner)
│   │   ├── builtin/                  # 内置 Agent
│   │   ├── custom/                   # 自定义 Agent
│   │   └── tools/                    # Agent 工具
│   ├── llm/                          # LLM 服务
│   ├── rag/                          # RAG 服务
│   ├── knowledge/                    # 知识库服务
│   └── evaluation/                   # 评测服务
│
├── repository/                        # Repository 层 - 数据访问实现
│   ├── mysql/                        # MySQL 实现
│   ├── milvus/                       # Milvus 实现
│   ├── neo4j/                        # Neo4j 实现
│   └── redis/                        # Redis 实现
│
└── model/                             # Model 层 - 领域实体和接口定义
    ├── agent/                        # Agent 实体和接口
    ├── llm/                          # LLM 实体和接口
    ├── rag/                          # RAG 实体和接口
    ├── knowledge/                    # 知识库实体和接口
    ├── evaluation/                   # 评测实体和接口
    └── types/                        # 通用类型
```

---

## 二、当前状态 (2026-05-31)

### 2.1 已完成的重构 ✅

| 阶段 | 任务 | 说明 |
|-----|------|------|
| **Phase 1** | 模块合并 | ✅ 合并 usecases 和 services 到 service 层 |
| **Phase 2** | 删除适配器 | ✅ 移除 AgentExecutableAdapter 等适配器 |
| **Phase 3** | 简化 DTO | ✅ 删除冗余 DTO 转换，直接使用 Model 类型 |
| **Phase 5** | 目录重命名 | ✅ interface→handler, infrastructure/persistence→repository, domain→model |

### 2.2 当前目录结构

```
internal/
├── handler/                          # ✅ Handler 层 (原 interface/http/handler)
│   ├── agent/                        # ✅
│   ├── chat/                         # ✅
│   ├── knowledge/                    # ✅
│   └── middleware/                   # ✅ (原 interface/http/middleware)
│
├── service/                          # ✅ Service 层 (原 usecases + services)
│   ├── agent/                        # ✅ (含 core/, builtin/, tools/)
│   ├── llm/                          # ✅
│   ├── rag/                          # ✅
│   ├── knowledge/                    # ✅
│   └── evaluation/                   # ✅
│
├── repository/                       # ✅ Repository 层 (原 infrastructure/persistence)
│   ├── mysql/                        # ✅
│   ├── milvus/                       # ✅
│   ├── neo4j/                        # ✅
│   └── redis/                        # ✅
│
└── model/                             # ✅ Model 层 (原 domain)
    ├── agent/                        # ✅
    ├── llm/                          # ✅
    ├── rag/                          # ✅
    ├── knowledge/                    # ✅
    ├── evaluation/                   # ✅
    └── types/                        # ✅
```

### 2.3 架构优势

| 优势 | 说明 |
|-----|------|
| **简化层次** | 4 层 → 3 层，减少抽象层级 |
| **减少 DTO 转换** | Handler 直接使用 Service 类型，Service 直接使用 Model 类型 |
| **清晰职责** | 每层职责明确，易于理解和维护 |
| **依赖方向一致** | 所有实现依赖 Model，Model 无依赖 |

---

## 三、架构模式

### 3.1 Handler 层模式

```go
// handler/agent.go
type AgentHandler struct {
    agentService *service.AgentService
}

// Execute 执行 Agent
func (h *AgentHandler) Execute(c *gin.Context) {
    var req ExecuteAgentRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // 直接调用 Service，使用 Service 类型
    result, err := h.agentService.Execute(c.Request.Context(), &service.ExecuteRequest{
        AgentID: req.AgentID,
        Input:   req.Input,
        Context: req.Context,
    })
    // ...
}
```

### 3.2 Service 层模式

```go
// service/agent/agent.go
type AgentService struct {
    agentRepo      model.AgentRepository
    llmService     *service.LLMService
    toolRegistry   *service.ToolRegistry
}

// Execute 执行 Agent
func (s *AgentService) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
    // 直接使用 Model 类型
    agentDef, err := s.agentRepo.GetByID(ctx, req.AgentID)
    if err != nil {
        return nil, err
    }

    // 业务逻辑
    return s.executeAgent(ctx, agentDef, req)
}
```

### 3.3 Repository 层模式

```go
// repository/mysql/agent_repo.go
type AgentRepository struct {
    db *gorm.DB
}

// GetByID 获取 Agent 定义
func (r *AgentRepository) GetByID(ctx context.Context, id string) (*model.Agent, error) {
    var gormModel GormAgent
    if err := r.db.WithContext(ctx).Where("id = ?", id).First(&gormModel).Error; err != nil {
        return nil, err
    }

    // 转换 GORM Model → Domain Model
    return gormModel.ToDomain(), nil
}
```

---

## 四、验证检查清单

重构完成后，检查以下项目：

- [x] 代码可以编译通过 (`go build ./...`)
- [x] 所有导入路径已更新
- [x] 依赖方向正确 (handler → service → model ← repository)
- [x] Model 层无外部框架依赖
- [x] 删除了适配器文件
- [ ] 单元测试通过 (Phase 6)
- [ ] 集成测试通过 (Phase 6)

---

## 五、预期收益

1. **可维护性** - 清晰的分层和职责划分
2. **可测试性** - 依赖注入和接口抽象，Model 层可独立测试
3. **可扩展性** - 符合开闭原则，新增功能不影响现有代码
4. **可读性** - 符合 Go 最佳实践，代码结构清晰
5. **性能** - 减少不必要的 DTO 转换，提高运行效率
