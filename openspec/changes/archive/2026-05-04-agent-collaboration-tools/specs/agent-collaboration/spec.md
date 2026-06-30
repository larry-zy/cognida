# Agent Collaboration Tools Capability Specification

本规格定义 **LLM 可调用的协作工具**能力，与现有的框架驱动协作（TaskDecomposer + Dispatcher）互补。

## ADDED Requirements

### Requirement: LLM 可调用委派工具

系统 SHALL 提供 `delegate_to_agent` 工具，允许 LLM 将任务委派给注册的其他 Agent。

#### Scenario: 成功委派任务
- **WHEN** LLM 调用 `delegate_to_agent` 并指定目标 Agent 名称和任务描述
- **THEN** 系统 SHALL 将任务转发给目标 Agent
- **AND** 系统 SHALL 返回目标 Agent 的响应内容
- **AND** 系统 SHALL 在 context 中记录委派链路用于循环检测

#### Scenario: 目标 Agent 不存在
- **WHEN** LLM 调用 `delegate_to_agent` 但目标 Agent 未注册
- **THEN** 系统 SHALL 返回错误信息，说明可用的 Agent 列表及其描述

#### Scenario: 委派任务超时
- **WHEN** 目标 Agent 在超时时间内未返回响应
- **THEN** 系统 SHALL 终止等待并返回超时错误

#### Scenario: 检测协作循环
- **WHEN** 委派链路中已存在目标 Agent（如 A → B → A）
- **THEN** 系统 SHALL 检测到循环并拒绝委派
- **AND** 系统 SHALL 返回循环检测错误，包含完整链路信息

### Requirement: LLM 可调用咨询工具

系统 SHALL 提供 `ask_agent` 工具，允许 LLM 向其他 Agent 咨询问题，但不转移对话控制权。

#### Scenario: 成功咨询其他 Agent
- **WHEN** LLM 调用 `ask_agent` 并向目标 Agent 提问
- **THEN** 系统 SHALL 获取目标 Agent 的回答
- **AND** 系统 SHALL 返回回答内容，控制权仍保留在原 Agent

#### Scenario: 咨询用于信息验证
- **WHEN** LLM 需要验证某些信息的准确性
- **THEN** LLM SHALL 可调用 `ask_agent` 向专业 Agent 咨询
- **AND** LLM SHALL 根据咨询结果决定后续行动

#### Scenario: 咨询目标不存在
- **WHEN** LLM 调用 `ask_agent` 但目标 Agent 未注册
- **THEN** 系统 SHALL 返回错误信息，说明可用的 Agent 列表

### Requirement: LLM 可调用转移工具

系统 SHALL 提供 `handoff_to` 工具，允许 LLM 将对话控制权转移给其他 Agent。

#### Scenario: 成功转移对话控制权
- **WHEN** LLM 判断任务更适合其他 Agent 处理
- **THEN** LLM SHALL 可调用 `handoff_to` 转移控制权
- **AND** 目标 Agent SHALL 接管后续对话

#### Scenario: 转移时携带上下文
- **WHEN** LLM 调用 `handoff_to` 转移对话
- **THEN** 系统 SHALL 将当前对话上下文传递给目标 Agent
- **AND** 目标 Agent SHALL 基于上下文继续处理

#### Scenario: 转移目标不存在
- **WHEN** LLM 调用 `handoff_to` 但目标 Agent 未注册
- **THEN** 系统 SHALL 返回错误信息，说明可用的 Agent 列表
- **AND** 原 Agent SHALL 继续处理，不会丢失控制权

### Requirement: Agent 注册表扩展

系统 SHALL 扩展现有 AgentRegistry，提供 Tool 友好的方法。

#### Scenario: 按名称查找 Agent
- **WHEN** 协作工具需要通过名称查找 Agent
- **THEN** 系统 SHALL 提供 `GetByName(name string) (Agent, error)` 方法
- **AND** 当 Agent 不存在时返回明确的错误信息

#### Scenario: 获取 Agent 元数据
- **WHEN** 协作工具需要生成工具描述或错误信息
- **THEN** 系统 SHALL 提供获取 Agent 描述和能力的方法
- **AND** 返回的信息 SHALL 包含 name, description, capabilities

#### Scenario: 列出所有可用 Agent
- **WHEN** 需要向用户展示可用的 Agent 选项
- **THEN** 系统 SHALL 提供 `ListWithDescriptions()` 方法
- **AND** 返回列表 SHALL 包含每个 Agent 的简要描述

#### Scenario: 注册表线程安全
- **WHEN** 多个 goroutine 并发访问注册表
- **THEN** 系统 SHALL 保证读写操作线程安全
- **AND** 现有的 `sync.RWMutex` 保护 SHALL 继续有效

### Requirement: Builder 支持协作配置

系统 SHALL 扩展 Builder，支持便捷配置协作能力。

#### Scenario: 启用所有协作工具
- **WHEN** 使用 `WithCollaboration()` 方法创建 Agent
- **THEN** 系统 SHALL 自动将所有协作工具添加到 Agent 工具列表
- **AND** LLM SHALL 可自主调用这些工具

#### Scenario: 选择性启用协作工具
- **WHEN** 使用 `WithCollaboration()` 并传入选项参数
- **THEN** 系统 SHALL 根据选项只添加指定的协作工具
- **AND** 未启用的工具对 LLM 不可见

#### Scenario: 向后兼容
- **WHEN** 现有 Agent 代码不调用 `WithCollaboration()`
- **THEN** 系统 SHALL 保持原有行为
- **AND** 现有测试 SHALL 继续通过

### Requirement: 协作工具参数验证

系统 SHALL 验证协作工具的输入参数。

#### Scenario: 缺少必需参数
- **WHEN** LLM 调用协作工具但缺少必需参数（如 agent_name）
- **THEN** 系统 SHALL 返回明确的参数缺失错误

#### Scenario: 参数格式错误
- **WHEN** LLM 传入的参数格式不符合要求
- **THEN** 系统 SHALL 返回格式错误提示
- **AND** 系统 SHALL 说明正确的参数格式

### Requirement: 协作循环检测

系统 SHALL 检测并防止协作循环。

#### Scenario: 检测直接循环
- **WHEN** Agent A 委派给 Agent B，Agent B 又委派回 Agent A
- **THEN** 系统 SHALL 检测到循环并拒绝第二次委派
- **AND** 系统 SHALL 返回循环检测错误，包含完整链路

#### Scenario: 检测间接循环
- **WHEN** 协作链路形成闭环（A → B → C → A）
- **THEN** 系统 SHALL 检测到循环并中断委派
- **AND** 系统 SHALL 返回相关错误信息

### Requirement: 与现有协作能力共存

系统 SHALL 保持与现有 orchestration 和 collaboration 包的兼容性。

#### Scenario: 现有编排模式不受影响
- **WHEN** 现有代码使用 Sequential、Parallel、Supervisor 等编排模式
- **THEN** 这些模式 SHALL 继续正常工作
- **AND** 不需要修改任何现有代码

#### Scenario: 现有框架协作不受影响
- **WHEN** 现有代码使用 TaskDecomposer、TaskDispatcher、ResultAggregator
- **THEN** 这些组件 SHALL 继续正常工作
- **AND** 不需要修改任何现有代码

#### Scenario: 协作模式互补
- **WHEN** 用户需要选择协作模式
- **THEN** 系统 SHALL 提供清晰的文档说明三种模式的区别
- **AND** 文档 SHALL 包含选择原则和示例
