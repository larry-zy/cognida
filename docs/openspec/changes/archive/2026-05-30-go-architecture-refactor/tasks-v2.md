# Cognida-Go 3-Layer 架构重构任务 v2

## 执行进度

| 阶段 | 状态 | 完成时间 |
|------|------|----------|
| Phase 1: 清理旧架构目录 | ✅ 完成 | 2026-05-31 |
| Phase A: 移动 Business Logic | ✅ 完成 | 2026-05-31 |
| Phase B: 重命名 Infrastructure 模块 | ✅ 完成 | 2026-05-31 |
| Phase C: 整合 Redis 实现 | ✅ 完成 | 2026-05-31 |
| Phase D: 验证和测试 | ✅ 完成 | 2026-05-31 |

### 已完成的工作 (Phase 1)

- ✅ 删除 `application/dto/`, `application/usecases/`, `application/services/`, `application/`
- ✅ 移动 `application/initializer/agent/` → `service/agent/initializer/`
- ✅ 删除 `domain/` 目录
- ✅ 修复构建错误

---

## 当前问题分析

### 职责错位 - Business Logic 误放在 Infrastructure

| 模块 | 当前位置 | 代码量 | 问题 |
|------|----------|--------|------|
| **agent** | `infrastructure/agent/` | ~7781 行 | Agent 框架、Builder、Middleware、Orchestrator → **业务逻辑** |
| **rag** | `infrastructure/rag/` | ~500 行 | Pipeline、Query Strengthening、HyDE → **业务逻辑** |
| **guardrail** | `infrastructure/guardrail/` | ~200 行 | Input/Output Filter、Jailbreak → **业务逻辑** |

### 职责重复 - Redis 客户端分散

```
❌ 当前混乱状态:
infrastructure/cache/redis/   # 通用 Redis 客户端 (13个文件)
repository/redis/             # Redis 持久化实现 (3个文件)
infrastructure/redis/         # Redis (evaluation 用途)
```

### 命名不清晰

| 当前 | 问题 | 建议 |
|------|------|------|
| `infrastructure/skill/` | 名称不够明确 | `infrastructure/mcp/` |
| `infrastructure/telemetry/` | 术语不够标准 | `infrastructure/observability/` |
| `infrastructure/mq/` | 命名可简化 | `infrastructure/queue/` |

---

## Phase A: 移动 Business Logic (P0)

### 目标
将误放在 Infrastructure 的业务逻辑移到 Service 层

### A.1 移动 `infrastructure/agent/` → `service/agent/framework/`

**当前状态**:
- `infrastructure/agent/` (15 个文件, ~7781 行)
  - Eino Agent 框架实现
  - Builder, Middleware, Orchestrator
  - Registry, MemoryRegistry
  - Collaboration, Reflection hooks

**操作**:
```bash
mkdir -p internal/service/agent/framework
mv internal/infrastructure/agent/* internal/service/agent/framework/
```

**结果结构**:
```
service/agent/
├── framework/          # ← 新增：Eino 框架实现
│   ├── eino_agent.go
│   ├── eino_builder.go
│   ├── eino_middleware.go
│   ├── orchestrator.go
│   ├── registry.go
│   ├── hooks/
│   ├── reflection/
│   └── tools/adapter.go
├── orchestration/       # 现有：高层编排
├── core/               # 现有：核心 Agent
├── builtin/            # 现有：内置 Agent
├── tools/              # 现有：工具实现
└── initializer/         # 现有：初始化
```

**导入路径更新**:
- `link/internal/infrastructure/agent` → `link/internal/service/agent/framework`

**影响文件** (约 40 个):
- `service/llm/*.go`
- `service/agent/*.go`
- `service/rag/*.go`
- `service/guardrail/*.go`
- `handler/*.go`
- `cmd/wire/*.go`

---

### A.2 移动 `infrastructure/rag/` → `service/rag/pipeline/`

**当前内容**:
- `pipeline.go` - Pipeline 实现
- `retriever.go` - 检索器
- `reranker.go` - 重排序
- `query_strengthener.go` - 查询增强
- `hyde_generator.go` - HyDE 生成
- `multi_hop.go` - 多跳检索
- `llm_adapter.go` - LLM 适配
- `graph_repository_adapter.go` - 图谱适配

**操作**:
```bash
mkdir -p internal/service/rag/pipeline
mv internal/infrastructure/rag/* internal/service/rag/pipeline/
```

**导入路径更新**:
- `link/internal/infrastructure/rag` → `link/internal/service/rag/pipeline`

---

### A.3 移动 `infrastructure/guardrail/` → `service/guardrail/filter/`

**当前内容**:
- `input_filter.go` - 输入过滤
- `output_filter.go` - 输出过滤
- `jailbreak_detector.go` - 越狱检测

**操作**:
```bash
mkdir -p internal/service/guardrail/filter
mv internal/infrastructure/guardrail/* internal/service/guardrail/filter/
```

**导入路径更新**:
- `link/internal/infrastructure/guardrail` → `link/internal/service/guardrail/filter`

---

### A.4 更新导入路径

**批量替换命令**:
```bash
# Agent framework
find internal -name "*.go" -exec sed -i 's|link/internal/infrastructure/agent|link/internal/service/agent/framework|g' {} +

# RAG pipeline
find internal -name "*.go" -exec sed -i 's|link/internal/infrastructure/rag|link/internal/service/rag/pipeline|g' {} +

# Guardrail filter
find internal -name "*.go" -exec sed -i 's|link/internal/infrastructure/guardrail|link/internal/service/guardrail/filter|g' {} +
```

---

### A.5 验证构建

```bash
cd internal
go build ./...
go vet ./...
```

---

## Phase B: 重命名 Infrastructure 模块 (P1)

### B.1 ~~skill → mcp~~ (保持 skill)

**决策**: 保持 `skill` 目录名不变。
- `skill` 是独立的 Skill 功能模块，不仅仅是 MCP 客户端
- 目录名应反映业务语义而非协议名称

### B.2 telemetry → observability

```bash
mv internal/infrastructure/telemetry internal/infrastructure/observability
```

**导入路径更新**:
- `link/internal/infrastructure/telemetry` → `link/internal/infrastructure/observability`

### B.3 mq → queue

```bash
mv internal/infrastructure/mq internal/infrastructure/queue
```

**导入路径更新**:
- `link/internal/infrastructure/mq` → `link/internal/infrastructure/queue`

---

## Phase C: 整合 Redis 实现 (P1)

### C.1 审查当前 Redis 用途

| 位置 | 用途 | 文件数 |
|------|------|--------|
| `infrastructure/cache/redis/` | 通用 Redis 客户端 | 13 |
| `repository/redis/` | 持久化实现 | 3 |
| `infrastructure/redis/` | Evaluation Redis | ? |

### C.2 决定整合方案

**✅ 已选择：选项 2 - 保持分离，明确职责**

**理由**：
- `infrastructure/cache/redis/` - 通用 Redis 客户端，提供基础操作
- `repository/redis/` - 数据层缓存存储实现
- `infrastructure/redis/evaluation/` - Evaluation 系统专用（进度、队列）

三者职责不同，保持分离更清晰。

### C.3 执行整合 (根据决策)

---

## Phase D: 验证和测试 (P0)

### D.1 构建验证

```bash
go build ./...
go vet ./...
```

### D.2 运行测试

```bash
go test ./...
```

### D.3 功能验证

- 启动服务验证
- API 测试
- Agent 功能测试

---

## 最终目标架构

```
internal/
├── handler/              # HTTP 处理器
├── service/              # 业务逻辑
│   ├── agent/
│   │   ├── framework/   # ← Agent 框架实现
│   │   ├── tools/       # Agent 工具
│   │   ├── orchestration/ # 高层编排
│   │   ├── core/        # 核心 Agent
│   │   ├── builtin/     # 内置 Agent
│   │   └── initializer/
│   ├── rag/
│   │   └── pipeline/    # ← RAG Pipeline
│   ├── guardrail/
│   │   └── filter/      # ← 过滤器实现
│   ├── cache/
│   ├── conversation/
│   ├── evaluation/
│   ├── graph/
│   ├── knowledge/
│   ├── llm/
│   ├── memory/
│   ├── task/
│   ├── tenant/
│   └── user/
├── repository/           # 数据访问
│   ├── mysql/
│   ├── milvus/
│   ├── neo4j/
│   └── redis/
├── model/                # 领域模型
└── infrastructure/       # 外部依赖适配器（纯基础设施）
    ├── auth/            # ✅ 认证基础设施
    ├── cache/           # ✅ 缓存基础设施
    ├── config/          # ✅ 配置管理
    ├── crypto/          # ✅ 加密工具
    ├── document/        # ✅ 文档解析
    ├── grpc/            # ✅ gRPC 客户端
    ├── llm/             # ✅ LLM 客户端
    ├── skill/           # ✅ Skill MCP 客户端
    ├── observability/   # ← 重命名：telemetry → observability
    ├── queue/           # ← 重命名：mq → queue
    ├── search/          # ✅ 搜索基础设施
    ├── tool/            # ✅ 工具适配器
    └── graph/           # ✅ 图谱基础设施
```

---

## 优先级

| 阶段 | 优先级 | 说明 |
|-----|-------|------|
| Phase A | P0 | 移动 Business Logic，解决职责错位 |
| Phase B | P1 | 重命名，提高可读性 |
| Phase C | P1 | 整合 Redis，消除重复 |
| Phase D | P0 | 验证和测试 |
