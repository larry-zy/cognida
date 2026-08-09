## ADDED Requirements

### Requirement: 列出可作为被测对象的运行中 Agent

系统 SHALL 向评测创建界面提供"运行中的 Agent"列表，数据来源为 Go 侧 Agent 注册中心（`ListAgents`），至少包含每个 Agent 的 `id`、`name`、`type`、`status`。

#### Scenario: 前端加载 Agent 列表
- **WHEN** 用户打开创建测评对话框
- **THEN** 前端调用 Agent 列表接口获取注册中心中的 Agent
- **AND** 下拉控件展示每个 Agent 的名称与类型（如 default / rag_assistant / chat_assistant / text2sql / data_agent）

#### Scenario: Agent 列表为空
- **WHEN** 注册中心未注册任何可用 Agent
- **THEN** Agent 选择控件显示"暂无可用 Agent"占位
- **AND** 不阻塞 QA / RAG 类型的测评创建

### Requirement: 以 Agent 作为被测对象创建测评

当评测类型为 `agent` 时，系统 SHALL 要求选择一个被测 Agent，并在创建请求中携带 `agent_id`。

#### Scenario: 选择 Agent 提交测评
- **WHEN** 用户选择评测类型 `agent` 并选定某个 Agent 后提交
- **THEN** 创建请求体包含选中的 `agent_id`
- **AND** 任务被路由到 Agent 执行器，对每条样本调用该 Agent 生成答案

#### Scenario: Agent 类型缺少 agent_id
- **WHEN** 用户选择评测类型 `agent` 但未选择任何 Agent 即提交
- **THEN** 系统拒绝创建并提示"请选择被测 Agent"
- **AND** 不创建任务记录

### Requirement: 模块以 Agent 测评为主并保留三种类型

评测模块 SHALL 以 Agent 测评为主入口对外呈现（页面标题/路由为"Agent 测评"），同时 SHALL 保留 `qa`（大模型）与 `rag` 评测类型可选。

#### Scenario: 页面命名
- **WHEN** 用户进入评测模块
- **THEN** 页面标题与导航显示为"Agent 测评"
- **AND** 评测类型下拉仍可切换到 QA / RAG

#### Scenario: 非 Agent 类型不要求选 Agent
- **WHEN** 用户选择评测类型 `qa` 或 `rag`
- **THEN** Agent 选择控件不作为必填项
- **AND** 对应类型按原有被测对象（大模型 / 检索+大模型）执行
