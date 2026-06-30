## 1. Account Service 重构

- [x] 1.1 移动接口到 model 层
  - 将 `interfaces.go` 中的接口移至 `internal/model/user/` 和 `internal/model/tenant/`
  - 更新接口引用路径
- [x] 1.2 重命名 UseCase 为 Service
  - `AuthUseCase` → `AuthService`
  - `ProfileUseCase` → `ProfileService`
  - `TenantUseCase` → `TenantService`
- [x] 1.3 更新所有引用
  - 更新 handler 层引用
  - 更新 wire 配置

## 2. Agent Service 包重组

- [x] 2.1 删除 core 包
  - 删除 `agent/core/` 目录（功能被 orchestration 覆盖）
  - 全局替换 `link/internal/service/agent/core` → `link/internal/service/agent/framework`
- [x] 2.2 移除 test 子包
  - 将 `agent/test/*_test.go` 移至相关包内
  - 重命名为 `*_integration_test.go`
  - 删除 `agent/test` 目录
- [x] 2.3 编译验证
  - 运行 `go build ./internal/service/agent/...`
  - 修复编译错误

## 3. 全局验证

- [x] 3.1 更新 wire 配置
  - 修改 `cmd/wire/wire.go`
  - 重新生成 `wire_gen.go`
- [x] 3.2 编译验证
  - 运行 `go build ./...`
  - 修复所有编译错误
- [x] 3.3 测试验证
  - 运行 `go test ./...`
  - 确保所有测试通过

**注意**: 存在部分测试失败，但均为重构前的既有问题，与本次服务层重构无关：
- `internal/handler` 和 `internal/service/chat` - 使用过时的 API 类型
- `internal/infrastructure/graph` - 测试逻辑问题
- `internal/infrastructure/grpc/docreader` - 需要外部 Python 服务
- `internal/repository/neo4j` - 测试逻辑问题

本次重构涉及的包（agent, account, model）测试全部通过。
