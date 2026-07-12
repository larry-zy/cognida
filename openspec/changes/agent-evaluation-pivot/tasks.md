## 1. 后端：接通 Agent/RAG 执行器

- [x] 1.1 新增 `AgentService` 适配器（桥接 Agent 注册中心 GetInstance+Chat），实现 `Chat(ctx, agentID, msg)` 与 `GetAgent(ctx, agentID)`
- [x] 1.2 在 `cmd/wire/wire.go` 与 `wire_gen.go` 注册 `NewRAGExecutor(...)` 与 `NewAgentExecutor(adapter, timeout)`，与 QA 一并注入 Worker（新增 retriever + agentRegistry 依赖）
- [x] 1.3 修正执行器注册/查找：统一注册键与 `Type()`，让 `qa` 的历史别名 `llm` 命中同一执行器（别名在注册表侧处理）
- [x] 1.4 修正 `worker.go` 按类型路由的逻辑，未知类型明确 FAILED 并给出可诊断错误（Get 报错含已注册类型清单）
- [x] 1.5 单元测试：`qa/llm/rag/agent` 四类型均能 `registry.Get` 到执行器；未知类型报错
- [x] 1.6 **（轨迹捕获，D6）** `AgentService.Chat` 返回富结果 `AgentChatResult{Answer, ToolsUsed, Trajectory, TotalSteps}`（从 `Response.ToolCalls` 按序抽工具名）；`AgentExecutor.Execute` 写入 `QAResult.ToolsUsed/Trajectory/TotalSteps`

## 2. 后端：任务创建校验 agent_id

- [x] 2.1 `evaluation_handler.go` CreateTask：类型为 `agent` 时校验 `agent_id` 非空且存在于注册中心，否则返回 400
- [x] 2.2 单元/接口测试：agent 类型缺 agent_id / 非法 agent_id 均返回 400（短路 service）

## 3. 后端：数据集样本 Agent 字段

- [x] 3.1 `internal/model/evaluation/entity.go` 样本模型 + `QAPair` 追加可选 `ExpectedTools`/`ExpectedSteps`（JSON）；`QAResult` 追加 `ToolsUsed`/`Trajectory`/`TotalSteps`
- [x] 3.2 `repository/mysql` 样本 model 加对应列与读写映射，并在加载 `QAPair` 时回填 expected_tools/expected_steps
- [x] 3.3 运行 `cd link-go && set -a && source .env && set +a && go run ./cmd/migrate-db` 同步表结构
- [x] 3.4 Worker 组装 Python payload：Agent 类型带 references(expected_tools/expected_steps) + outputs(tools_used/trajectory/total_steps)，命中 `compute_agent_metrics`（Go `ComputeItem` 透传 5 字段 + Python compute-metrics 端点分流 `_AGENT_GRADERS` → 平铺进 aggregate）

## 4. 前端：选择运行中的 Agent

- [x] 4.1 `src/api/agent/index.ts` 新增 `listAgents()` 封装（GET `/api/v1/agents`）
- [x] 4.2 `composables/useEvaluationList.ts` 的 `loadAll` 增加加载 agent 列表
- [x] 4.3 `CreateEvaluationDialog.vue` 新增"被测 Agent"选择器，类型为 `agent` 时显示并必填；提交请求带 `agent_id`
- [x] 4.4 类型为 `qa/rag` 时不要求选 Agent，行为不变

## 5. 前端：模块重命名为 Agent 测评

- [x] 5.1 `router/index.ts` 将 `/evaluation` 的 `meta.title` 改为"Agent 测评"（保留路径不变）
- [x] 5.2 页面标题/导航文案与 `EvaluationList.vue` 处的"大模型测评"改为"Agent 测评"（含 i18n zh/en）
- [x] 5.3 校验评测类型下拉仍可切 QA/RAG，默认突出 Agent（选项顺序 Agent 置顶 + 默认 type=agent）

## 6. 数据集：HF 转换器（Python）

- [x] 6.1 新增转换脚本 `scripts/convert_eval_datasets.py`，复用 `services/dataset/loader.py` 的 `load_hf_dataset`，QA 集映射 question/reference_answer
- [x] 6.2 Agent 基准映射：从 xLAM function-calling 的 answers 抽 expected_tools（有序）；缺工具标注时置 `supports_trajectory=false` 只产 QA 指标并计数告警
- [x] 6.3 输出为与前端上传兼容的 JSONL（每行一条样本 + manifest.json），存 `cmd/seed-eval-datasets/data/`
- [x] 6.4 选定预选数据集：中文 `hfl/cmrc2018`、英文 `rajpurkar/squad`、Agent `NobodyExistsOnTheInternet/xlam-function-calling-60k`（官方 Salesforce 集为 gated，用 schema 一致公开镜像）、场景自造电商/知识库各一，各 50–200 条限量导出（实测 80/80/80/5/5）
- [x] 6.5 **（指标增强，D7）** `metrics/agent.py` 新增有序包含 `tool_order`（期望工具按序作为实际调用子序列）；`compute_agent_metrics` 汇总 answer_accuracy/tool_selection/tool_order/trajectory/step_efficiency，compute-metrics 端点平铺进 aggregate

## 7. 数据集：seed 脚本一键灌库（Go）

- [x] 7.1 新增 `cmd/seed-eval-datasets`，读取转换 JSONL 写入 `evaluation_datasets` + 样本表
- [x] 7.2 幂等：按 dataset_id upsert，重复执行不产生重复数据集，标注评测类型（agent/qa）
- [x] 7.3 跑一次 seed，验证数据集与样本数量正确、可被评测任务选用

## 8. 联调与测试

- [x] 8.1 集成：创建一个 `agent` 类型任务（选 rag_assistant/chat_assistant），端到端出聚合指标
- [x] 8.2 集成：创建 `qa`、`rag` 任务回归，确认未被破坏
- [x] 8.3 前端手工验证：Agent 选择器、必填校验、页面命名（vue-tsc 通过 + 静态核对：选择器仅 agent 显示且必填、类型下拉 Agent 置顶默认 agent、路由与列表标题「Agent 测评」、listAgents 已接入 loadAll）
- [x] 8.4 触发 code-review skill，修复问题后提交（CodeReview 全绿；闭环复核发现并修复 Sequential 编排丢弃子 Agent 工具轨迹的真实缺陷——见 9.1）

## 9. 闭环复核修复（task 创建 → agent 执行 → 指标持久化）

- [x] 9.1 **（真实闭环缺陷）** `orchestration/sequential.go` 跨段累积 ToolCalls：Plan-Execute-Reflect（text2sql）等管道此前只把末段 Reflect 响应返回，Plan/Execute 段真实发生的 get_schema/sql_execute 轨迹被覆盖丢弃，导致评测轨迹类指标恒为 0。修复后按序合并全轨迹并加 nil 保护 + 回归单测 `TestSequential_AccumulatesToolCalls`
- [x] 9.2 端到端复核：text2sql Agent 评测真实捕获 `[get_schema×5]` / `[skill_invoke,get_schema,…,sql_execute]` 轨迹，Python 计算出非零 tool_recall=1.0/tool_precision=0.667/tool_f1=0.8/tool_order=0.5/traj_similarity=0.62/step_optimal_ratio=0.36，并持久化进 `evaluation_tasks.metrics`
