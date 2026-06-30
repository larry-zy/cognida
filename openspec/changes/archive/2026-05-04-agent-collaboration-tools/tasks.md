## 1. AgentRegistry 扩展

> 现有 `collaboration/task.go` 已有基础实现，需要扩展 Tool 友好的方法

- [x] 1.1 添加 `GetByName(name string) (Agent, error)` 方法
- [x] 1.2 添加 `GetDescription(name string) (string, error)` 方法
- [x] 1.3 添加 `GetCapabilities(name string) ([]AgentCapability, error)` 方法
- [x] 1.4 添加 `ListWithDescriptions() []AgentInfo` 方法
- [x] 1.5 定义 `AgentInfo` 结构体（包含 name, description, capabilities）
- [x] 1.6 添加单元测试验证新方法

## 2. 委派工具 (DelegateTool)

- [x] 2.1 创建 `collab_tools.go` 文件
- [x] 2.2 实现 `DelegateTool` 结构体
- [x] 2.3 实现 `Info()` 方法，返回工具描述和参数定义
- [x] 2.4 实现 `InvokableRun()` 方法，执行委派逻辑
- [x] 2.5 添加参数验证（agent_name 必填，task 必填）
- [x] 2.6 处理目标 Agent 不存在的错误情况
- [x] 2.7 实现基于 context 的循环检测
- [x] 2.8 添加超时处理逻辑
- [x] 2.9 编写单元测试

## 3. 咨询工具 (AskTool)

- [x] 3.1 实现 `AskTool` 结构体
- [x] 3.2 实现 `Info()` 方法，返回工具描述和参数定义
- [x] 3.3 实现 `InvokableRun()` 方法，执行咨询逻辑
- [x] 3.4 添加参数验证（agent_name 必填，question 必填）
- [x] 3.5 处理目标 Agent 不存在的错误情况
- [x] 3.6 编写单元测试

## 4. 转移工具 (HandoffTool)

- [x] 4.1 实现 `HandoffTool` 结构体
- [x] 4.2 实现 `Info()` 方法，返回工具描述和参数定义
- [x] 4.3 实现 `InvokableRun()` 方法，执行转移逻辑
- [x] 4.4 添加参数验证（agent_name 必填，context 可选）
- [x] 4.5 处理目标 Agent 不存在的错误情况
- [x] 4.6 实现上下文传递机制
- [x] 4.7 定义上下文传递格式规范
- [x] 4.8 编写单元测试

## 5. Builder 扩展

- [x] 5.1 在 `eino_builder.go` 中添加 `WithCollaboration()` 方法
- [x] 5.2 实现 `CollaborationOption` 选项模式
- [x] 5.3 添加 `CollaborationConfig` 配置结构
- [x] 5.4 实现 `EnableDelegate()`、`EnableAsk()`、`EnableHandoff()` 选项函数
- [x] 5.5 实现 `EnableAllCollaboration()` 便捷函数
- [x] 5.6 实现工具自动注入逻辑
- [x] 5.7 编写单元测试（全部启用、部分启用、向后兼容）

## 6. 错误处理

- [x] 6.1 创建 `collab_errors.go`
- [x] 6.2 定义 `ErrAgentNotFound` 错误常量
- [x] 6.3 定义 `ErrAgentAlreadyRegistered` 错误常量
- [x] 6.4 实现 `CollabLoopError` 循环检测错误类型
- [x] 6.5 添加统一的错误信息格式，返回可用 Agent 列表
- [x] 6.6 编写错误处理单元测试

## 7. 集成测试

- [ ] 7.1 创建多 Agent 协作的端到端测试
- [ ] 7.2 测试 LLM 自主决策是否正确调用协作工具
- [ ] 7.3 测试协作场景下的错误恢复
- [ ] 7.4 测试循环检测是否正确触发
- [ ] 7.5 测试 Handoff 上下文传递
- [ ] 7.6 测试并发协作场景

## 8. 文档更新

- [ ] 8.1 更新 `docs/agent/API.md`，添加协作工具说明
- [ ] 8.2 创建 `docs/agent/collaboration-tools.md` 详细文档
- [ ] 8.3 添加协作工具使用示例
- [ ] 8.4 更新 README.md，说明新增能力
- [ ] 8.5 添加三种协作模式对比说明（L1/L2/L3）

## 9. 代码审查和优化

- [x] 9.1 代码审查，确保符合项目规范
- [ ] 9.2 性能测试和优化
- [x] 9.3 添加必要的注释和文档
- [ ] 9.4 最终验收测试

---

## 已完成 ✓

> 以下能力已由现有实现提供，无需重复实现：

- [x] AgentRegistry 基础实现（`collaboration/task.go`）
- [x] 线程安全保护（sync.RWMutex）
- [x] Agent 注册和查找基础方法
- [x] 任务分解能力（TaskDecomposer）
- [x] 任务分发能力（TaskDispatcher）
- [x] 结果聚合能力（ResultAggregator）
- [x] Orchestration 编排模式（Sequential、Parallel、Supervisor 等）
