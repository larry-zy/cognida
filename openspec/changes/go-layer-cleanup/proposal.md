# Go 层级架构清理与整合 - Proposal

## 背景与问题

当前项目存在多个架构重构相关的 OpenSpec change，导致：
1. 任务分散，难以统一管理
2. service 层职责不清，模块过多（13 个）
3. model 层包含非领域实体（如 services、types）
4. 基础设施功能混杂在业务服务中

### 当前问题

**Service 层（13 个模块）**：
- `cache` - 特性标志、语义缓存（基础设施功能）
- `conversation` - 会话管理（是 llm/chat 的一部分）
- `memory` - 记忆管理（是 agent 的一部分）
- `guardrail` - 防护服务（是 agent 工具）
- `task` - 任务管理（是 evaluation 的一部分）
- `rag` - 检索增强（应该合并到 knowledge）
- `graph` - 图谱服务（应该合并到 knowledge）
- `tenant`, `user` - 简单 CRUD（可合并为 account）
- `llm` - 名称不够准确（应该是 chat）

## 目标

### 核心原则

1. **Service 层精简为核心业务服务**
2. **基础设施功能移到 infrastructure 层**
3. **从属功能合并到主模块**
4. **Model 层只保留领域实体和接口定义**
5. **Service 层与 Model 层结构对应**

### 目标架构（5 个核心模块）

```
internal/
├── handler/          # HTTP handlers
├── service/          # 核心业务服务（精简到 5 个）
│   ├── agent/        # Agent 编排、执行、工具、记忆、反思
│   ├── chat/         # LLM Chat + 会话管理（重命名自 llm）
│   ├── knowledge/    # 知识库 + RAG + 图谱
│   ├── evaluation/   # 评测 + 任务
│   └── account/      # 账户管理（合并 tenant + user）
├── repository/       # 数据访问实现
├── model/            # 领域实体 + 接口定义
│   ├── agent/
│   ├── chat/         # 重命名自 llm
│   ├── knowledge/    # 合并 rag + graph
│   ├── evaluation/   # 合并 task
│   ├── account/      # 合并 tenant + user
│   ├── audit/        # 审计日志（保留）
│   ├── cache/        # 缓存实体（保留）
│   ├── errors/       # 统一错误定义
│   └── common/       # 通用类型（page, id, model，从 types 迁移）
└── infrastructure/    # 基础设施
    ├── cache/        # 缓存实现（从 service 迁移）
    └── ...
```

## 变更范围

### Service 层变更

| 操作 | 源模块 | 目标模块 | 说明 |
|------|--------|----------|------|
| 重命名 | service/llm | **service/chat** | 更准确反映核心功能 |
| 合并 | service/conversation | → service/chat | 会话是 Chat 的一部分 |
| 合并 | service/memory | → service/agent | 记忆是 Agent 能力 |
| 合并 | service/guardrail | → service/agent/tools | 防护是 Agent 工具 |
| 合并 | service/rag | → service/knowledge | RAG 是知识检索 |
| 合并 | service/graph | → service/knowledge | 图谱是知识表示 |
| 合并 | service/task | → service/evaluation | 任务是评测执行方式 |
| 合并 | service/tenant + service/user | → **service/account** | 账户统一管理 |
| 移除 | service/cache | → infrastructure/cache | 缓存是基础设施 |

**最终 Service 层（5 个）**：
1. **agent** - Agent 编排、执行、工具、记忆、反思
2. **chat** - LLM Chat、Embedding、Rerank、会话管理
3. **knowledge** - 知识库、RAG、图谱
4. **evaluation** - 评测、任务
5. **account** - 租户、用户

### Model 层变更

| 操作 | 源模块 | 目标模块 | 说明 |
|------|--------|----------|------|
| 重命名 | model/llm | **model/chat** | 对应 service 层 |
| 合并 | model/tenant + model/user | → **model/account** | 对应 service 层 |
| 合并 | model/rag + model/graph | → **model/knowledge** | 对应 service 层 |
| 合并 | model/task | → **model/evaluation** | 对应 service 层 |
| 移动 | model/types/* | → 各领域模块 + common | 领域类型归属，通用类型移到 common |
| 删除 | model/services/ | → 各领域模块 | 分散到对应模块 |
| 删除 | model/types/ | → | 清空后删除 |
| 清理 | model/types/ | → 只保留通用 | page, id, model |

### 需删除的 Change

以下 change 已被整合：
1. ~~`openspec/changes/go-architecture-refactor`~~ → 已归档
2. ~~`openspec/changes/consolidate-service-packages`~~ → 已归档
3. ~~`link-go/cmd/wire/openspec/changes/clean-domain-layer`~~ → 已删除

## 实施计划

### Phase 1: Service 层整合（10 步）
1. llm → chat（重命名）
2. conversation → chat（合并）
3. memory → agent（合并）
4. guardrail → agent/tools（合并）
5. rag → knowledge（合并）
6. graph → knowledge（合并）
7. task → evaluation（合并）
8. tenant + user → account（合并）
9. cache → infrastructure（移除）
10. 验证编译和测试

### Phase 2: Model 层清理（4 步）
1. types 重组（移动领域特定类型）
2. services 清理（删除目录）
3. 对应 service 层变更（重命名和合并）
4. 验证编译和测试

### Phase 3: 更新与验证（3 步）
1. 更新测试代码
2. Wire 依赖注入更新
3. 文档更新

### Phase 4: 最终验收（2 步）
1. 最终验证（5 个模块）
2. 提交与清理

## 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| 导入路径变更导致编译失败 | 分阶段更新，每次更新后验证编译 |
| 测试用例需要大量修改 | 先更新测试代码，再更新业务代码 |
| 职责划分不清导致返工 | 每个合并操作前先确认职责边界 |
| Wire 依赖注入配置复杂 | 保持接口不变，只更新实现路径 |

## 验收标准

1. ✅ Service 层只有 5 个模块：agent, chat, knowledge, evaluation, account
2. ✅ Model 层对应结构清晰
3. ✅ model/types 目录已删除（内容移到 common 和各领域模块）
4. ✅ model/services 目录已删除
5. ✅ `go build ./...` 编译通过
6. ✅ `go test ./...` 测试通过
7. ✅ 无旧架构路径残留
8. ✅ 文档已更新

## 模块职责说明

| 模块 | 职责 | 包含功能 |
|------|------|----------|
| **agent** | Agent 编排与执行 | 编排、内置 Agent、框架、工具、记忆、反思、协作 |
| **chat** | LLM 调用与会话 | Chat、Embedding、Rerank、Model、会话管理、消息转换 |
| **knowledge** | 知识管理 | 知识库、文档处理、RAG、检索、图谱 |
| **evaluation** | 评测与任务 | 数据集、评测执行、指标、任务队列 |
| **account** | 账户管理 | 租户、用户、认证、权限 |

## 参考资料

- 当前架构：`link-go/CLAUDE.md`
- Clean Architecture：`docs/CLEAN_ARCHITECTURE.md`
- 任务列表：`tasks.md`
