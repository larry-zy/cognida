## Why

Cognida 项目正在进行 Clean Architecture 重构，但当前处于半完成状态：新旧目录结构并存、大量文件暂存未提交、pkg 目录职责模糊。这导致代码难以导航、依赖关系混乱、存在运行未定义代码版本的风险。需要完成重构并清理遗留结构，使项目架构清晰、可维护。

## What Changes

### 后端结构调整

- **删除遗留目录**：移除 `internal/application/service/` 及其子目录（agent、rag），约 8 个文件
- **重构 pkg 目录**：
  - 将 `pkg/gorm.go`、`pkg/middleware/` 移至 `internal/infrastructure/`
  - 保留纯工具类（convert、crypto、errors、jwt、page、parser、response）
- **验证依赖注入**：更新 `cmd/wire/wire.go` 确保所有新层正确注册

### 前端规范化

- **重命名目录**：`web/src/component/` → `web/src/components/`
- **清理资源文件**：将 `web/src/pic/`（含 1.5MB 壁纸）移至 `web/public/assets/` 或删除

### Git 清理

- **提交重构变更**：分阶段提交当前暂存区的重构文件
- **更新 .gitignore**：添加 `tmp/`、`uploads/*`、`*.exe`、`nul` 等规则
- **删除孤立文件**：移除根目录下的 `nul` 空文件

### 文档更新

- 更新 `README.md` 中的目录结构，移除已删除路径的引用

## Capabilities

### New Capabilities

- `clean-architecture-structure`: 完整的 Clean Architecture 分层结构（domain、application、infrastructure、interface）
- `refactored-pkg-directory`: 职责清晰的 pkg 目录，仅包含通用工具函数
- `standardized-frontend-structure`: 符合 Vue 项目约定的前端目录命名

### Modified Capabilities

*无 - 本次变更仅涉及代码结构和组织，不改变 API 契约或外部行为*

## Impact

### 代码影响

- 需要更新 `internal/application/service/rag/agent_adapter.go` 中的 import 路径
- 需要更新 `cmd/wire/wire.go` 和 `cmd/wire/provider.go` 的依赖注入配置
- 前端组件 import 路径需要从 `@/component/` 更新为 `@/components/`

### 构建影响

- Wire 重新生成 `wire_gen.go`
- Go module 路径变更可能导致首次构建变慢

### 运行时影响

- **无 API 变更**：HTTP 路由和请求/响应格式保持不变
- 服务启动后功能应与重构前完全一致

### 风险

- 中等风险：存在大量未提交代码，需谨慎处理以避免丢失工作
- 建议：在开始前创建备份分支或提交当前工作
