# Go 层级架构清理与整合 - Tasks

> **目标**：将 Service 层从 13 个模块精简到 5 个核心业务模块

## 最终架构

```
service/
├── agent/        # Agent 编排、执行、工具、记忆、反思
├── chat/         # LLM Chat + 会话管理（重命名自 llm，合并 conversation）
├── knowledge/    # 知识库 + RAG + 图谱（合并 rag + graph）
├── evaluation/   # 评测 + 任务（合并 task）
└── account/      # 账户管理（合并 tenant + user）
```

---

## 0. 准备工作

- [ ] 0.1 创建备份分支 `git checkout -b backup/before-layer-cleanup`
- [ ] 0.2 分析当前依赖关系 `go mod graph | grep cognida-go/internal`
- [ ] 0.3 统计受影响的文件数量
- [ ] 0.4 备份当前 openspec/changes 目录

## 1. 清理旧 Change ✅

- [x] 1.1 归档 `openspec/changes/go-architecture-refactor` 到 `openspec/changes/archive/`
- [x] 1.2 归档 `openspec/changes/consolidate-service-packages` 到 `openspec/changes/archive/`
- [x] 1.3 删除 `cognida-go/cmd/wire/openspec/changes/clean-domain-layer` 目录
- [x] 1.4 验证 openspec 结构正确

---

## 2. Service 层整合 - llm → chat（重命名）

- [ ] 2.1 创建 `service/chat/` 目录
- [ ] 2.2 移动 `service/llm/*` → `service/chat/*`
- [ ] 2.3 更新包名 `package llm` → `package chat`
- [ ] 2.4 更新所有 `service/llm` 的导入到 `service/chat`
- [ ] 2.5 删除 `service/llm/` 目录
- [ ] 2.6 验证编译 `go build ./...`

## 3. Service 层整合 - conversation → chat

- [ ] 3.1 分析 `service/conversation/` 和 `service/chat/` 的重叠
- [ ] 3.2 将 `service/conversation/session.go` 内容合并到 `service/chat/session_service.go`
- [ ] 3.3 将 `service/conversation/message.go` 内容合并到 `service/chat/message_converter.go`
- [ ] 3.4 更新所有 `service/conversation` 的导入到 `service/chat`
- [ ] 3.5 删除 `service/conversation/` 目录
- [ ] 3.6 验证编译 `go build ./...`

## 4. Service 层整合 - memory → agent

- [ ] 4.1 创建 `service/agent/memory/` 目录（如不存在）
- [ ] 4.2 移动 `service/memory/long_term.go` → `service/agent/memory/long_term.go`
- [ ] 4.3 移动 `service/memory/manage.go` → `service/agent/memory/manage.go`
- [ ] 4.4 与 `service/agent/core/memory.go` 整合，消除重复
- [ ] 4.5 更新所有 `service/memory` 的导入到 `service/agent/memory`
- [ ] 4.6 删除 `service/memory/` 目录
- [ ] 4.7 验证编译 `go build ./...`

## 5. Service 层整合 - guardrail → agent/tools

- [ ] 5.1 分析 `service/guardrail/service.go` 的接口
- [ ] 5.2 移动 `service/guardrail/service.go` → `service/agent/tools/guardrail.go`
- [ ] 5.3 将防护逻辑封装为 AgentTool 接口实现
- [ ] 5.4 更新所有 `service/guardrail` 的导入到 `service/agent/tools`
- [ ] 5.5 删除 `service/guardrail/` 目录
- [ ] 5.6 验证编译 `go build ./...`

## 6. Service 层整合 - rag → knowledge

- [ ] 6.1 分析 `service/rag/` 和 `service/knowledge/` 的结构
- [ ] 6.2 移动 `service/rag/retriever.go` → `service/knowledge/retriever.go`（合并现有）
- [ ] 6.3 移动 `service/rag/pipeline.go` → `service/knowledge/rag_pipeline.go`
- [ ] 6.4 移动 `service/rag/optimizer.go` → `service/knowledge/optimizer.go`
- [ ] 6.5 合并 `service/rag/service.go` 到 `service/knowledge/knowledge_base_service.go`
- [ ] 6.6 移动 `service/rag/pipeline/` → `service/knowledge/rag_pipeline/`
- [ ] 6.7 移动 `service/rag/types.go` → `service/knowledge/rag_types.go`
- [ ] 6.8 移动 `service/rag/graph.go` → `service/knowledge/rag_graph.go`
- [ ] 6.9 更新所有 `service/rag` 的导入到 `service/knowledge`
- [ ] 6.10 删除 `service/rag/` 目录
- [ ] 6.11 验证编译 `go build ./...`

## 7. Service 层整合 - graph → knowledge

- [ ] 7.1 分析 `service/graph/` 和 `service/knowledge/` 的关系
- [ ] 7.2 移动 `service/graph/graph.go` → `service/knowledge/graph_service.go`
- [ ] 7.3 移动 `service/graph/dto.go` → `service/knowledge/graph_dto.go`
- [ ] 7.4 移动 `service/graph/graph_test.go` → `service/knowledge/graph_test.go`
- [ ] 7.5 更新所有 `service/graph` 的导入到 `service/knowledge`
- [ ] 7.6 删除 `service/graph/` 目录
- [ ] 7.7 验证编译 `go build ./...`

## 8. Service 层整合 - task → evaluation

- [ ] 8.1 分析 `service/task/` 和 `service/evaluation/` 的关系
- [ ] 8.2 移动 `service/task/service.go` → `service/evaluation/task_service.go`
- [ ] 8.3 移动 `service/task/worker.go` → `service/evaluation/task_worker.go`
- [ ] 8.4 移动 `service/task/dataset_loader.go` → `service/evaluation/task_dataset.go`
- [ ] 8.5 移动 `service/task/types.go` → `service/evaluation/task_types.go`
- [ ] 8.6 移动 `service/task/executor/` → `service/evaluation/task_executor/`
- [ ] 8.7 更新所有 `service/task` 的导入到 `service/evaluation`
- [ ] 8.8 删除 `service/task/` 目录
- [ ] 8.9 验证编译 `go build ./...`

## 9. Service 层整合 - tenant + user → account

- [ ] 9.1 创建 `service/account/` 目录
- [ ] 9.2 分析 `service/tenant/` 和 `service/user/` 的接口
- [ ] 9.3 移动 `service/tenant/tenant.go` → `service/account/tenant.go`
- [ ] 9.4 移动 `service/tenant/interfaces.go` → `service/account/tenant_interfaces.go`
- [ ] 9.5 移动 `service/tenant/service.go` → `service/account/tenant_service.go`
- [ ] 9.6 移动 `service/tenant/dto.go` → `service/account/tenant_dto.go`
- [ ] 9.7 移动 `service/user/auth.go` → `service/account/auth.go`
- [ ] 9.8 移动 `service/user/profile.go` → `service/account/profile.go`
- [ ] 9.9 移动 `service/user/service.go` → `service/account/user_service.go`
- [ ] 9.10 移动 `service/user/interfaces.go` → `service/account/user_interfaces.go`
- [ ] 9.11 移动 `service/user/dto.go` → `service/account/user_dto.go`
- [ ] 9.12 创建 `service/account/service.go` 统一入口
- [ ] 9.13 更新包名和导入
- [ ] 9.14 更新所有 `service/tenant` 的导入到 `service/account`
- [ ] 9.15 更新所有 `service/user` 的导入到 `service/account`
- [ ] 9.16 删除 `service/tenant/` 目录
- [ ] 9.17 删除 `service/user/` 目录
- [ ] 9.18 验证编译 `go build ./...`

## 10. Service 层迁移 - cache → infrastructure

- [ ] 10.1 创建 `infrastructure/cache/` 目录（如不存在）
- [ ] 10.2 移动 `service/cache/feature_flag.go` → `infrastructure/cache/feature_flag.go`
- [ ] 10.3 移动 `service/cache/semantic_cache.go` → `infrastructure/cache/semantic_cache.go`
- [ ] 10.4 移动 `service/cache/management.go` → `infrastructure/cache/management.go`
- [ ] 10.5 移动 `service/cache/metrics.go` → `infrastructure/cache/metrics.go`
- [ ] 10.6 在 `model/cache/` 定义接口（如需要）
- [ ] 10.7 更新 service 层依赖为接口调用
- [ ] 10.8 更新所有 `service/cache` 的导入到 `infrastructure/cache`
- [ ] 10.9 删除 `service/cache/` 目录
- [ ] 10.10 验证编译 `go build ./...`

## 11. Service 层最终验证

- [ ] 11.1 确认 service 层只有 5 个模块：agent, chat, knowledge, evaluation, account
- [ ] 11.2 验证无空目录 `ls -la internal/service/`
- [ ] 11.3 运行 `go build ./...`
- [ ] 11.4 运行 `go test ./...`

---

## 12. Model 层清理 - types 重组

**目标**：将 model/types/ 中所有文件移动到对应目录，然后删除 types 目录

- [ ] 12.1 创建 `model/common/` 目录存放通用类型
- [ ] 12.2 移动 `model/types/page.go` → `model/common/page.go`（Req, Resp, Info 分页类型）
- [ ] 12.3 移动 `model/types/id_generator.go` → `model/common/id_generator.go`（ID 生成器）
- [ ] 12.4 移动 `model/types/model.go` → `model/common/model.go`（ModelSource 类型）

- [ ] 12.5 移动 `model/types/message.go` → `model/chat/message_entity.go`
  - 更新包名为 `package chat`
  - 包含：MessageEntity, MessageFeedback
- [ ] 12.6 移动 `model/types/session.go` → `model/chat/session_entity.go`
  - 更新包名为 `package chat`
  - 包含：SessionEntity
- [ ] 12.7 移动 `model/types/embedding.go` → `model/chat/embedding_types.go`
  - 更新包名为 `package chat`
  - 包含：SourceType, MatchType, IndexInfo

- [ ] 12.8 移动 `model/types/retriever.go` → `model/knowledge/retriever_types.go`
  - 更新包名为 `package knowledge`
  - 包含：RetrieverType, RetrieveParams, IndexWithScore, RetrieveResult

- [ ] 12.9 移动 `model/types/tenant.go` → `model/account/tenant_entity.go`
  - 更新包名为 `package account`
  - 包含：Tenant, TenantUser
- [ ] 12.10 移动 `model/types/user.go` → `model/account/user_entity.go`
  - 更新包名为 `package account`
  - 包含：User, UserInfo, RefreshTokenEntity, UserPreference, APIKey

- [ ] 12.11 验证 `model/types/` 目录已清空
- [ ] 12.12 删除 `model/types/` 目录

- [ ] 12.13 更新所有导入路径：
  - `link/internal/model/types` → `link/internal/model/common`（通用类型）
  - `link/internal/model/types` → `link/internal/model/chat`（消息/会话类型）
  - `link/internal/model/types` → `link/internal/model/knowledge`（检索类型）
  - `link/internal/model/types` → `link/internal/model/account`（租户/用户类型）

- [ ] 12.14 验证编译 `go build ./...`

## 13. Model 层清理 - services 清理

- [ ] 13.1 移动 `model/services/similarity.go` → `model/knowledge/similarity.go`
- [ ] 13.2 确认 `model/services/` 为空
- [ ] 13.3 删除 `model/services/` 目录
- [ ] 13.4 更新受影响的导入
- [ ] 13.5 验证编译 `go build ./...`

## 14. Model 层整合 - 对应 service 层变更

- [ ] 14.1 重命名 `model/llm/` → `model/chat/`（对应 service）
- [ ] 14.2 重命名 `model/tenant/` + `model/user/` → `model/account/`
- [ ] 14.3 合并 `model/conversation/` → `model/chat/`
- [ ] 14.4 合并 `model/memory/` → `model/agent/`
- [ ] 14.5 合并 `model/guardrail/` → `model/agent/`
- [ ] 14.6 合并 `model/graph/` → `model/knowledge/`
- [ ] 14.7 合并 `model/rag/` → `model/knowledge/`
- [ ] 14.8 合并 `model/task/` → `model/evaluation/`
- [ ] 14.9 保留 `model/audit/` 和 `model/cache/`（在使用中）
- [ ] 14.10 更新所有受影响的导入
- [ ] 14.11 验证编译 `go build ./...`

## 15. Model 层最终验证

- [ ] 15.1 确认 model 层结构清晰
- [ ] 15.2 验证 model 层包含：agent, chat, knowledge, evaluation, account, audit, cache, common
- [ ] 15.3 验证 `model/types/` 目录已删除
- [ ] 15.4 验证 `model/services/` 目录已删除
- [ ] 15.5 运行 `go build ./...`
- [ ] 15.6 运行 `go test ./...`

---

## 16. 更新测试代码

- [ ] 16.1 更新 handler 层测试
- [ ] 16.2 更新 service 层测试
- [ ] 16.3 更新 repository 层测试
- [ ] 16.4 运行完整测试套件 `go test ./... -v`

## 17. Wire 依赖注入更新

- [ ] 17.1 检查 Wire 配置文件
- [ ] 17.2 更新 provider 函数（如有变更）
- [ ] 17.3 重新生成 Wire 代码 `cd cmd/wire && wire`
- [ ] 17.4 验证编译

## 18. 文档更新

- [ ] 18.1 更新 `cognida-go/CLAUDE.md` 架构说明
- [ ] 18.2 更新 `docs/CLEAN_ARCHITECTURE.md`
- [ ] 18.3 更新其他受影响的文档
- [ ] 18.4 创建迁移说明文档

## 19. 最终验证

- [ ] 19.1 运行 `go build ./...`
- [ ] 19.2 运行 `go test ./...`
- [ ] 19.3 运行 `go vet ./...`
- [ ] 19.4 验证 service 层只有 5 个模块：agent, chat, knowledge, evaluation, account
- [ ] 19.5 验证 model 层对应结构
- [ ] 19.6 验证无旧架构残留导入

## 20. 提交与清理

- [ ] 20.1 检查变更 `git status`
- [ ] 20.2 检查 diff `git diff`
- [ ] 20.3 提交变更 `git commit -m "refactor: cleanup go layer - consolidate to 5 core services"`
- [ ] 20.4 创建标签 `git tag -a go-layer-cleanup-complete`
- [ ] 20.5 更新 openspec 记录

## 21. 回滚准备（可选）

- [ ] 21.1 验证备份分支存在
- [ ] 21.2 记录回滚步骤
- [ ] 21.3 测试回滚流程（在测试环境）

---

## 变更汇总

| Service 层 | 操作 | 结果 |
|-----------|------|------|
| llm | 重命名 | → chat |
| conversation | 合并 | → chat |
| memory | 合并 | → agent |
| guardrail | 合并 | → agent/tools |
| rag | 合并 | → knowledge |
| graph | 合并 | → knowledge |
| task | 合并 | → evaluation |
| tenant | 合并 | → account |
| user | 合并 | → account |
| cache | 移除 | → infrastructure |

**最终：agent, chat, knowledge, evaluation, account（5 个）**

| Model 层 | 操作 | 结果 |
|----------|------|------|
| llm | 重命名 | → chat |
| tenant + user | 合并 | → account |
| graph + rag | 合并 | → knowledge |
| task | 合并 | → evaluation |
| services | 删除 | → 分散到各模块 |
| types | 删除 | → common（通用） + 各领域模块 |
