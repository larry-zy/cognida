## Context

Cognida-Go 当前采用 4 层 Clean Architecture，存在以下问题：

**当前状态**：
- `internal/interface/http/handler/*` - HTTP 处理
- `internal/application/usecases/*` + `internal/application/services/*` - 应用层（重复冗余）
- `internal/domain/*` - 领域层（含接口定义）
- `internal/infrastructure/persistence/*` - 基础设施层

**架构违规**：
- Application 层依赖 Infrastructure 层（如 `AgentExecutableAdapter`）
- Repository 接口在 Domain 和 Application 层重复定义（48 个接口）
- 层间 DTO 转换代码占比 35%

**约束**：
- 外部 HTTP/gRPC API 必须保持兼容
- 数据库 Schema 无变更
- 不能中断服务运行（分阶段迁移）

## Goals / Non-Goals

**Goals:**
1. 简化为 3 层架构：Handler → Service → Repository → Model
2. 减少代码量 30%（约 3 万行）
3. 消除所有架构违规依赖
4. 统一 Repository 接口定义（48 → 15 个）
5. 保持外部 API 完全兼容

**Non-Goals:**
- 不改变外部 API 接口
- 不修改数据库 Schema
- 不更换外部依赖（MySQL、Milvus、Neo4j、Redis）
- 不改变业务逻辑

## Decisions

### D1: 目标目录结构

采用扁平化 4 层结构：

```
internal/
├── handler/          # HTTP 处理层
├── service/          # 业务逻辑层（核心实现）
├── repository/       # 数据访问层
└── model/            # 数据模型定义层
```

**理由**：
- 保留 4 个逻辑层但简化命名，更易理解
- Model 比 Domain 更准确表达"只含数据结构"的定位
- 业务逻辑集中在 Service，便于查找和维护

### D2: 合并 usecases 和 services

将 `application/usecases/*` 和 `application/services/*` 合并为 `service/*`。

**理由**：
- 两者本质都是业务逻辑，分开导致重复
- 合并后功能内聚，减少跨包查找
- 之前 `usecases/rag` 和 `services/rag` 存在重复的检索优化代码

### D3: 删除适配器模式

移除 `AgentExecutableAdapter` 等适配器，直接使用 Model 层定义的接口。

**理由**：
- 适配器是架构违规的"补丁"，不应存在
- Model 层应定义清晰的接口契约
- 减少一层不必要的类型转换

**方案**：
```go
// Before (错误)
type AgentExecutableAdapter struct {
    agent infraagent.Agent  // 依赖 Infrastructure
}

// After (正确)
type Service struct {
    executor model.AgentExecutor  // 使用 Model 接口
}
```

### D4: Repository 接口只在 Model 层定义

**理由**：
- 接口定义属于契约，应在 Model 层
- Repository 实现在 repository/ 包，实现 Model 接口
- 避免重复定义导致的不一致

**合并策略**：
- 扩展 Model 层接口，整合 Application 层的方法
- 删除 Application 层的重复接口
- Repository 实现扩展以支持新方法

### D5: Handler/Service 直接使用上层类型

**Before（4 层）**：
```go
// Handler → UseCase DTO → Domain → Infrastructure
handler → usecase.ChatRequestDTO → domain.ChatRequest → llm.ChatRequest
```

**After（3 层）**：
```go
// Handler → Service 类型 → Model 类型
handler → service.ChatRequest (embed model.ChatRequest) → model.ChatRequest
```

**理由**：
- 减少类型转换，Service 类型可直接嵌入 Model 类型
- Handler 使用 Service 类型，保持请求/响应的语义清晰
- Service 使用 Model 类型或扩展类型，无需额外 DTO

### D6: 分阶段迁移策略

采用 4 个 Sprint（8 周）渐进式迁移：

| Sprint | 目标 | 风险控制 |
|--------|------|----------|
| 1-2 | 合并重复模块 | 每个模块合并后运行测试 |
| 3-4 | 删除适配器、修复依赖 | 架构测试验证无违规导入 |
| 5-6 | 简化 DTO | 性能基准测试 |
| 7-8 | 评估优化 | 决定是否继续调整 |

**理由**：
- 降低风险，每个阶段可独立回滚
- 团队有适应时间
- 可根据效果调整后续计划

## Risks / Trade-offs

### R1: 引入新 Bug

**风险**：大量文件移动和导入更新可能引入错误

**缓解措施**：
- 每个 Phase 完成后运行完整测试套件
- 使用 `go vet` 和静态分析检查
- 分阶段上线，保留回滚能力（Git Tag）

### R2: 性能下降

**风险**：减少 DTO 转换可能影响性能

**缓解措施**：
- Phase 3 完成后进行性能基准测试
- 对比关键接口（Chat、RAG）的响应时间
- 如有问题，可在关键路径保留必要的 DTO

### R3: 团队适应成本

**风险**：新目录结构需要团队学习

**缓解措施**：
- 更新开发文档
- 提供迁移指南和常见问题
- Code Review 中确保新代码符合规范

### R4: 循环依赖引入

**风险**：合并模块可能意外引入循环依赖

**缓解措施**：
- 添加架构测试 `go mod graph | grep "link/internal"`
- 使用 `import-cycle` 检测工具

## Migration Plan

### 阶段 1: 合并重复模块（Sprint 1-2）

```bash
# 1. RAG 模块
mkdir -p internal/service/rag
# 合并 usecases/rag/* 和 services/rag/* 到 service/rag/
find . -name "*.go" -exec sed -i 's|link/internal/application/usecases/rag|link/internal/service/rag|g' {} \;
find . -name "*.go" -exec sed -i 's|link/internal/application/services/rag|link/internal/service/rag|g' {} \;
rm -rf internal/application/usecases/rag internal/application/services/rag

# 2. Agent、LLM、KB 模块（类似操作）
# 3. 运行测试验证
go test ./...

# 4. 打 Tag
git tag -a phase1-complete -m "Phase 1 完成"
```

### 阶段 2: 删除适配器（Sprint 3-4）

```bash
# 1. 确保 Model 层有完整接口
# internal/model/agent/executor.go 定义 AgentExecutor

# 2. 更新 Service 使用 Model 接口
# 修改 service/agent/agent.go

# 3. 删除适配器
rm internal/application/usecases/llm/agent_adapter.go

# 4. 架构测试
go test ./test/architecture/...
```

### 阶段 3: 简化 DTO（Sprint 5-6）

```bash
# 1. Handler 直接使用 Service 类型
# 2. Service 类型嵌入 Model 类型
# 3. 删除冗余 DTO 文件

# 4. 性能测试
go test -bench=. ./...
```

### 阶段 4: 评估优化（Sprint 7-8）

- 代码审查
- 性能评估
- 文档更新
- 决定后续方向

### 回滚策略

```bash
# 每个阶段完成后打 Tag
git tag -a phase<N>-complete -m "Phase N 完成"

# 如需回滚
git checkout phase<N>-complete

# 或使用 revert
git revert <commit-range>
```

## Open Questions

1. **Q**: 是否需要保留 `infrastructure/` 目录存放非持久化基础设施（如 LLM 客户端）？
   - **A**: 当前 LLM 客户端已作为 Repository 层的一部分，无需保留 infrastructure 目录

2. **Q**: Model 层是否允许有接口定义？
   - **A**: 允许，但仅定义 Repository 接口等契约，不含业务逻辑接口

3. **Q**: Service 层内部是否需要子包划分？
   - **A**: 按功能领域划分（service/agent、service/rag、service/llm 等），保持内聚

4. **Q**: Agent 服务内部如何组织通用框架和业务实现？
   - **A**: 采用分层结构，`core/` 存放通用编排引擎，`builtin/` 存放业务 Agent 实现

### D7: Agent 服务内部结构

Agent 服务采用分层结构，分离通用框架和业务实现：

```
internal/service/agent/
├── agent.go              # Agent 核心服务（生命周期管理）
├── registry.go           # Agent 注册中心
├── factory.go            # Agent 工厂
├── runtime.go            # Agent 运行时管理
│
├── core/                 # 核心编排引擎（通用框架）
│   ├── react.go          # ReAct 编排逻辑
│   ├── planner.go        # 任务规划
│   ├── executor.go       # Agent 执行器
│   ├── tools.go          # 工具调用管理
│   ├── memory.go         # Agent 记忆管理
│   └── types.go          # 核心类型定义
│
├── builtin/              # 内置业务 Agent（具体实现）
│   ├── text2sql/         # Text2SQL Agent
│   ├── data_analysis/    # 数据分析 Agent
│   ├── code_review/      # 代码审查 Agent
│   ├── document_analysis/# 文档分析 Agent
│   ├── knowledge_qa/     # 知识问答 Agent
│   ├── workflow/         # 工作流 Agent
│   └── research/         # Deep Research Agent
│
├── custom/               # 自定义 Agent 支持
│   ├── loader.go         # 动态加载器
│   ├── validator.go      # 配置验证
│   └── sandbox.go        # 沙箱执行
│
└── types.go              # Agent Service 类型定义
```

**理由**：
- **core/** 存放通用编排引擎，避免与业务逻辑耦合
- **builtin/** 存放具体业务 Agent，便于扩展和查找
- **custom/** 支持用户自定义 Agent，提升灵活性
- 新增业务 Agent 只需在 builtin/ 下创建新目录，不影响核心框架

**业务 Agent 实现模式**：
```go
// service/agent/builtin/text2sql/agent.go
package text2sql

import "link/internal/service/agent/core"

// Agent Text2SQL Agent 实现
type Agent struct {
    *core.BaseAgent              // 嵌入基础 Agent
    promptMgr   *PromptManager
    sqlExecutor *SQLExecutor
    validator   *SQLValidator
}

// Execute 实现 Text2SQL 特定逻辑
func (a *Agent) Execute(ctx context.Context, input string) (string, error) {
    // 业务逻辑...
}
```
