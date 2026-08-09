## MODIFIED Requirements

### Requirement: Create evaluation task
The system SHALL allow users to create evaluation tasks through HTTP API. 被测对象由评测类型决定：`qa` 为大模型、`rag` 为检索+大模型、`agent` 为选定的 Agent。当类型为 `agent` 时，请求 MUST 携带有效的 `agent_id`。

#### Scenario: Successful task creation
- **WHEN** user POSTs to `/api/v1/evaluation/tasks` with valid dataset_id and evaluation_type
- **THEN** system creates a task record in MySQL
- **AND** system pushes task_id to Redis queue `eval:queue`
- **AND** system returns task_id and PENDING status

#### Scenario: Invalid dataset
- **WHEN** user POSTs with non-existent dataset_id
- **THEN** system returns 404 error
- **AND** no task is created

#### Scenario: Agent 类型携带 agent_id
- **WHEN** user POSTs with evaluation_type `agent` 和一个存在于注册中心的 `agent_id`
- **THEN** system 创建任务并在配置中持久化 `agent_id`
- **AND** 任务出队后被路由到 Agent 执行器

#### Scenario: Agent 类型缺失或非法 agent_id
- **WHEN** user POSTs with evaluation_type `agent` 但缺少 `agent_id` 或其不存在于注册中心
- **THEN** system 返回校验错误（400）
- **AND** no task is created
