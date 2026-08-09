# Go 层级架构清理与整合 - Spec

## 目标

清理 Go 项目的层级架构，将分散的架构整合任务合并为一个统一的 change，精简 service 层为 5 个核心业务服务。

## 最终架构

### Service 层（5 个）

```
service/
├── agent/        # Agent 编排、执行、工具、记忆、反思
├── chat/         # LLM Chat + 会话管理
├── knowledge/    # 知识库 + RAG + 图谱
├── evaluation/   # 评测 + 任务
└── account/      # 账户管理
```

### Model 层（7 个 + 3 个）

```
model/
├── agent/        # Agent 实体、接口
├── chat/         # Chat、Session、Message 实体
├── knowledge/    # 知识库、图谱实体
├── evaluation/   # 评测、任务实体
├── account/      # 租户、用户实体
├── audit/        # 审计日志（保留）
├── cache/        # 缓存实体（保留）
├── errors/       # 统一错误定义
└── common/       # 通用类型（page, id, model）
```

## 变更汇总

### Service 层变更

| 操作 | 源模块 | 目标模块 |
|------|--------|----------|
| 重命名 | llm | **chat** |
| 合并 | conversation | → chat |
| 合并 | memory | → agent |
| 合并 | guardrail | → agent/tools |
| 合并 | rag | → knowledge |
| 合并 | graph | → knowledge |
| 合并 | task | → evaluation |
| 合并 | tenant + user | → **account** |
| 移除 | cache | → infrastructure |

### Model 层变更

| 操作 | 源模块 | 目标模块 |
|------|--------|----------|
| 重命名 | llm | **chat** |
| 合并 | tenant + user | → **account** |
| 合并 | rag + graph | → **knowledge** |
| 合并 | task | → **evaluation** |
| 移动 | types/* | → 各领域模块 |
| 删除 | services/ | → 分散 |

## 模块职责

| 模块 | 职责 |
|------|------|
| **agent** | Agent 编排、执行、工具、记忆、反思 |
| **chat** | LLM Chat、Embedding、Rerank、会话管理 |
| **knowledge** | 知识库、文档处理、RAG、图谱 |
| **evaluation** | 评测、数据集、任务执行 |
| **account** | 租户、用户、认证、权限 |

## 验收标准

- [ ] Service 层只有 5 个模块
- [ ] Model 层对应结构清晰
- [ ] model/types 目录已删除（内容移到 common 和各领域模块）
- [ ] model/services 目录已删除
- [ ] `go build ./...` 通过
- [ ] `go test ./...` 通过
- [ ] 文档已更新
