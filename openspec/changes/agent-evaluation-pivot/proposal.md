## Why

当前"大模型测评"模块的被测对象是裸大模型（QAExecutor 直接调 LLM 生成答案），而产品重心已转向 Agent。虽然后端早已定义 `qa/rag/agent` 三种评测类型、`executor/agent.go` 也已实现，但 Worker 只注册了 QAExecutor、前端没有 Agent 选择器、创建请求不带 `agent_id`——导致"评测运行中的 Agent"这条链路断裂，实际只有 QA 能跑。同时评测数据集只能手工上传，缺少从公开基准（HuggingFace）快速灌入的能力。

## What Changes

- **模块转向 Agent 测评（保留三种并存）**：以"选择运行中的 Agent"为主被测对象，QA/RAG 作为并存的评测类型继续可用。页面标题与路由从"大模型测评"重命名为"Agent 测评"。
- **接通 Agent 执行链路**：在 Worker 组合根注册 `AgentExecutor`（及 RAG），并修正 Worker 用 `Normalize` 前的原始 `type` 查执行器的问题，使 `agent`/`rag`/`qa`(别名 `llm`) 都能被正确路由。
- **前端 Agent 选择器**：创建测评对话框新增"被测 Agent"选择控件，调用 Go 侧已有的 `ListAgents` 接口拉取运行中的 Agent（default / rag_assistant / chat_assistant / text2sql / data_agent 等），提交时带上 `agent_id`；选 Agent 类型时校验必填。
- **数据集从 HuggingFace 导入**：新增将 HF 数据集（Agent/工具调用基准 + 通用 QA + 贴合电商/知识库场景）转换为本项目评测数据集格式的能力，支持两种落地方式：
  - **seed 脚本一键导入**：仿现有 `cmd/seed-*`，Python 下载/转换 → 写入 DB（`evaluation_datasets`/`evaluation_dataset_records`），幂等可重复执行。
  - **前端上传**：转换产物 JSONL 留在项目内，可经现有数据集管理页上传通道导入。
- **Agent 数据集字段扩展**：数据集样本支持 Agent 评测所需的期望工具调用 / 期望步骤等字段，用于 tool_selection / trajectory 打分。

## Capabilities

### New Capabilities
- `agent-evaluation-target`: 选择运行中的 Agent 作为被测对象的契约——前端 Agent 选择器、`ListAgents` 消费、创建请求携带并校验 `agent_id`、Worker 将任务路由到 AgentExecutor。
- `evaluation-dataset-import`: 从 HuggingFace/外部来源导入评测数据集的能力——数据集选型、字段映射（QA 与 Agent 两类）、seed 脚本一键灌库与前端上传两条落地路径。

### Modified Capabilities
- `evaluation-executor`: 在组合根注册 Agent（及 RAG）执行器；修正基于原始 `type` 的执行器查找，使非 QA 类型可被正确路由与执行。
- `evaluation-tasks`: 任务创建接受并校验 `agent_id`（Agent 类型必填）；被测对象由"模型"扩展为"Agent/模型/RAG"。

## Impact

- **link-go**：`cmd/wire`（组合根注册 executor）、`internal/service/evaluation/worker.go`（type 路由）、`executor/agent.go`、`internal/handler/evaluation_handler.go`（校验 `agent_id`）、`internal/model/evaluation/entity.go`（数据集样本 Agent 字段）、`repository/mysql`（迁移新字段，走 `cmd/migrate-db`）。
- **link-web**：`views/evaluation/components/CreateEvaluationDialog.vue`、`composables/useEvaluationList.ts`、`api/agent`（新增 ListAgents 封装）、`router`（重命名）、`evaluation-config.ts`。
- **link-python**：新增 HF→评测数据集转换脚本（复用 `services/dataset/loader.py`）、Agent 指标输入所需字段。
- **数据/脚本**：新增 `cmd/seed-eval-datasets`（Go）+ Python 转换器；转换产物 JSONL 存 test-output/ 或项目内 datasets 目录。
- **无破坏性变更**：QA/RAG 现有行为保留；新增字段与接口向后兼容。
