## 1. 新增 common 层

- [ ] 1.1 创建 `internal/common/` 目录
- [ ] 1.2 创建 `common/errors/` 包（从 model/errors 移动内容）
- [ ] 1.3 创建 `common/types/` 包（从 model/types 移动内容）
- [ ] 1.4 添加 `common/utils/` 包（如有公共工具函数）
- [ ] 1.5 更新所有引用 model/errors 和 model/types 的代码

## 2. 整合 llm 到 service 层

- [ ] 2.1 分析 `infrastructure/llm/` 和 `service/llm/` 的内容差异
- [ ] 2.2 将 `infrastructure/llm/` 的内容合并到 `service/llm/`
- [ ] 2.3 删除 `infrastructure/llm/` 目录
- [ ] 2.4 更新所有引用 infrastructure/llm 的代码
- [ ] 2.5 验证 LLM 功能正常

## 3. 整合 graph 到 service 层

- [ ] 3.1 分析 `infrastructure/graph/` 和 `service/graph/` 的内容差异
- [ ] 3.2 将 `infrastructure/graph/` 的内容合并到 `service/graph/`
- [ ] 3.3 删除 `infrastructure/graph/` 目录
- [ ] 3.4 更新所有引用 infrastructure/graph 的代码
- [ ] 3.5 验证图谱功能正常

## 4. 整合 cache 到 service 层

- [ ] 4.1 分析 `infrastructure/cache/` 和 `service/cache/` 的内容差异
- [ ] 4.2 将内容合并到 `service/cache/`
- [ ] 4.3 删除 `infrastructure/cache/` 目录
- [ ] 4.4 更新所有引用 infrastructure/cache 的代码

## 5. 移动 document 到 service 层

- [ ] 5.1 移动 `infrastructure/document/` → `service/document/`
- [ ] 5.2 删除 `infrastructure/document/` 目录
- [ ] 5.3 更新所有引用 infrastructure/document 的代码

## 6. 移动 search/retrieval 到 service 层

- [ ] 6.1 移动 `infrastructure/search/` → `service/retrieval/`
- [ ] 6.2 删除 `infrastructure/search/` 目录
- [ ] 6.3 更新所有引用 infrastructure/search 的代码

## 7. 移动 tool 到 service 层

- [ ] 7.1 移动 `infrastructure/tool/` → `service/tool/`
- [ ] 7.2 删除 `infrastructure/tool/` 目录
- [ ] 7.3 更新所有引用 infrastructure/tool 的代码

## 8. 重命名 agent 子包

- [ ] 8.1 重命名 `service/agent/builtin/` → `service/agent/presets/`
- [ ] 8.2 重命名 `service/agent/reflection/` → `service/agent/think/`
- [ ] 8.3 合并 `service/agent/collaboration/` → `service/agent/orchestration/`
- [ ] 8.4 更新所有引用被重命名包的代码

## 9. 精简 infrastructure 层

- [ ] 9.1 确认 infrastructure 只保留：auth, grpc, queue, redis, config, observability, crypto, mcp
- [ ] 9.2 添加 `infrastructure/README.md` 说明职责
- [ ] 9.3 清理 infrastructure 下不需要的文件

## 10. 精简 model 层

- [ ] 10.1 清理 model/errors（已移至 common）
- [ ] 10.2 清理 model/types（已移至 common）
- [ ] 10.3 整理 model/ 下只保留 entity 定义

## 11. 测试和验证

- [ ] 11.1 运行 `go build` 确保编译通过
- [ ] 11.2 运行测试套件确保功能正常
- [ ] 11.3 验证 gRPC 接口无变化
- [ ] 11.4 执行 `go mod tidy` 清理依赖

## 12. 文档更新

- [ ] 12.1 更新 `internal/README.md`
- [ ] 12.2 更新 `CLAUDE.md` 中的架构说明
- [ ] 12.3 添加 `internal/common/README.md`
- [ ] 12.4 添加迁移指南
