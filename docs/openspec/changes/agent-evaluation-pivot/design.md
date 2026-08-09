## Context

评测模块的链路大部分已铺好，但存在几处断点：

- **枚举/类型齐全**：`internal/model/evaluation/enum.go` 已有 `qa/rag/agent/llm` 四种；`EvaluationTaskConfig`/`EvaluationTask` 已含 `AgentID`；handler 的 CreateTask 请求已解析 `agent_id`（`evaluation_handler.go:48,82`）。
- **执行器已实现但未注册**：`executor/agent.go`、`executor/rag.go` 已写好，但组合根 `cmd/wire/wire.go:648` 只 `registry.Register(NewQAExecutor(...))`。RAG/Agent 出队会 `executor not found`。
- **AgentExecutor 依赖未接线**：`AgentExecutor` 需要一个 `AgentService{ Chat(ctx, agentID, msg); GetAgent(ctx, agentID) }`（`executor/agent.go:26-31`），当前无实现注入。Go 侧已有 Agent 注册中心与 `RegistryAgentHandler`（`GET /api/v1/agents` 列表、`ChatExecuteByID` 执行），可作为适配来源。
- **前端缺 Agent 选择**：`CreateEvaluationDialog.vue` 只有模型下拉，无 Agent 选择器；`api/agent` 无 `listAgents`；提交请求不带 `agent_id`。
- **数据集导入弱**：只能手工上传 JSONL；`cognida-python/services/dataset/loader.py` 有 `load_hf_dataset` 但未接入评测数据集格式与灌库流程；无评测数据集 seed 脚本。

约束：单人 main 分支开发；业务表用 `cmd/migrate-db` 从 GORM model 同步（评测表在其列表内）；Python 只做无状态计算，Go 承担编排；评测产物存 test-output/。

## Goals / Non-Goals

**Goals:**
- 接通 Agent 执行链路：组合根注册 QA/RAG/Agent 三类执行器，修正 Worker 按 type 路由，使 `agent` 任务可端到端跑通。
- 前端"选择运行中的 Agent"：新增 `listAgents` API 封装、创建对话框 Agent 选择器、提交 `agent_id`、Agent 类型必填校验；页面/路由重命名为"Agent 测评"。
- 从 HF 导入数据集：Python 转换器（QA + Agent 两类映射）→ 两条落地路径（Go seed 脚本灌库 + 前端上传 JSONL）。
- 数据集样本支持 Agent 字段（期望工具、期望步骤），向后兼容。

**Non-Goals:**
- 不改 Python 指标算法本身（ROUGE/BLEU/LLM-judge/agent 指标已存在，仅保证输入字段齐备）。
- 不重写 quality（数据质量）模块——与本次无关。
- 不引入 ragas/deepeval；沿用自研指标。
- 不做 Agent 的可视化编排/训练，仅"选择已注册运行的 Agent 作为被测对象"。

## Decisions

### D1: AgentExecutor 的 AgentService 用适配器桥接注册中心
`AgentExecutor` 只依赖窄接口 `AgentService`。新增一个适配器，内部委托给 Agent 注册中心/编排器（与 `RegistryAgentHandler.ChatExecuteByID` 同源的 `GetInstance(agentID).Chat/Run`），实现 `Chat(ctx, agentID, msg)` 与 `GetAgent(ctx, agentID)`。
- **为何**：复用已注册的运行实例（default/rag_assistant/text2sql/data_agent...），与前端 `ListAgents` 展示的列表一致，避免另起一套 Agent 抽象。
- **备选**：直接在 executor 里依赖 orchestrator——被否，破坏 executor 的窄依赖与可测性。

### D2: 组合根注册三执行器 + 修正 type 路由
在 `cmd/wire/wire.go`（及 `wire_gen.go`）注册 `NewRAGExecutor(...)`、`NewAgentExecutor(agentServiceAdapter, timeout)`。Worker 查执行器的键与执行器 `Type()` 对齐：QA 执行器注册键 `qa`，并让 `qa` 的历史别名 `llm` 也能命中（在注册时登记别名或在查找处归一化到已注册键）。
- **为何**：现状 Worker 用 `config.Type` 原值查找而只有 `qa` 注册，`llm/rag/agent` 必失败。统一键与别名后各类型可路由。
- **权衡**：别名映射放在注册表侧（更内聚）而非散落调用点。

### D3: HF→评测数据集 转换在 Python 侧，灌库在 Go 侧
Python 新增转换脚本，复用 `services/dataset/loader.py` 的 `load_hf_dataset`，把源集映射为评测样本 JSON（QA: question/reference_answer；Agent: 追加 expected_tools/expected_steps），输出 JSONL 到项目内目录。Go 新增 `cmd/seed-eval-datasets`，读取该 JSONL（或直接调用 Python 产物）写入 `evaluation_datasets` + 样本表，幂等（按 dataset_id upsert）。
- **为何**：转换属"计算/数据处理"归 Python；灌库属业务写入归 Go，与现有 `cmd/seed-*` 一致。前端上传路径复用同一 JSONL 格式，一份产物两用。
- **备选**：Go 直接下载 HF——被否，HF/datasets 生态在 Python 更成熟。

### D4: 数据集样本 Agent 字段以可选列扩展
在样本 model（`DatasetRecord`/`evaluation_dataset_records`）追加可选字段 `expected_tools`(JSON)、`expected_steps`(JSON)，经 `cmd/migrate-db` 同步。QA/RAG 样本留空即可，向后兼容。执行/指标时 Agent 类型读取这些字段供 tool_selection/trajectory 打分。
- **为何**：Agent 基准数据需要期望工具/轨迹，QA 数据集模型不含；用可选 JSON 列最小侵入。

### D6: 捕获 Agent 轨迹（工具调用序列），而非只留最终答案
**这是让 Agent 测评"有意义"的关键**。业界共识（Confident AI / DeepEval / TRAJECT-Bench 2025）：Agent 会以与单次 LLM 调用完全不同的方式失败——选对工具但传错参数、规划正确却不按计划执行、完成任务却绕了冗余步骤。因此评测必须打分**整条轨迹**，而非仅最终答案。现状 `AgentExecutor` 只写 `GeneratedAnswer`，把 Python 侧已存在的 `tool_selection`/`trajectory_match`/`step_efficiency` 全部饿死。

改法：
- `framework.Response` 已含 `ToolCalls []*ToolCall`（每个有 `Name`）。扩展窄接口 `AgentService.Chat` 返回富结果 `AgentChatResult{ Answer, ToolsUsed []string, Trajectory []string, TotalSteps int }`（适配器从 `Response.ToolCalls` 按调用顺序抽取工具名）。
- `QAResult` 追加 `ToolsUsed`/`Trajectory`/`TotalSteps`；`QAPair` 追加 `ExpectedTools`/`ExpectedSteps`（由样本列填充）。
- Worker 组装 Python payload 时，Agent 类型带上 references(expected_tools/expected_steps) 与 outputs(tools_used/trajectory/total_steps)，命中 `compute_agent_metrics`。
- **为何**：复用已实现但未接线的 Python 指标，最小侵入即从"只看答案对不对"升级到"工具选得对不对、顺序对不对、绕没绕路"。

### D7: 指标口径对齐 benchmark 分层（task completion / tool / trajectory / efficiency）
参考 BFCL→τ-bench→τ²-bench 的分层与 DeepEval 的拆分，本项目 Agent 指标分四层，UI 维度名与之对齐：
- **任务完成 (task_completion)**：`answer_accuracy`（语义相似度阈值近似成功率）。注：Agentic 任务随机性大，业界从 Pass@1 转向 Pass@k/Pass^k；本项目先单轮 Pass@1，聚合结果标注"单次采样"以免误读。
- **工具选择 (tool_selection)**：precision/recall/f1（已实现，去重集合比对）。补一个**有序包含 (tool_order / trajectory inclusion)**——期望工具是否按序作为实际调用的子序列出现（BFCL/TRAJECT-Bench 强调顺序）。
- **轨迹匹配 (trajectory_match)**：exact_match + 语义相似度（已实现）；无 gold trace 的数据集回退到 LLM-judge trajectory-satisfy（复用 `llm_judge.py`）。
- **步骤效率 (step_efficiency)**：实际步数 vs 最优步数（已实现）；含 loop/冗余提示。
- **参数正确性 (argument_correctness)** 列为 Non-Goal 的近期扩展：需要样本标注期望参数，首版不做，仅在 spec 标注为后续。
- **为何**：让维度名与主流 benchmark 词汇一致，便于对照解读；避免只报一个端到端数字而无法定位失败发生在哪一层。

### D5: 数据集选型（小而通用、下载稳定，含期望工具标注）
按用户确认的三类各选，均取小样本（各 50–200 条）保证下载与评测快：
- **通用 QA（无工具标注，跑 task_completion + 生成/LLM-judge）**：
  - 中文：`hfl/cmrc2018`（阅读理解 QA，question/answers/context）。
  - 英文：`rajpurkar/squad` 或 `mandarjoshi/trivia_qa`（rc.nocontext 子集）取小样本。
- **Agent / 工具调用（含期望函数调用 → 映射 expected_tools）**：
  - `Salesforce/xlam-function-calling-60k`（query + answers 为函数调用列表，工具名易抽取，许可宽松，体量小）——首选。
  - 备选 `glaiveai/glaive-function-calling-v2`（多轮含 function_call）；BFCL/τ-bench 结构复杂、含用户模拟，首版不直接入库，仅作对照参考。
- **贴合本项目场景（自造，含 expected_tools/expected_steps）**：
  - 电商：基于 `ecommerce_demo` 库自造问句 → 期望走 `text2sql`/`data_agent` 的 semantic_query 类工具（如"上月销量 Top5 商品" → expected_tools=[semantic_query]）。
  - 知识库：自造问句 → 期望走 `rag_assistant` 的检索工具（expected_tools=[knowledge_retrieve]）。
- 无工具标注的数据集在转换器中**明确标注 `supports_trajectory=false`**，只产出 task_completion + 生成类指标，不误算 tool_selection。

## Risks / Trade-offs

- **[HF 下载不稳定/需联网/被墙]** → seed 脚本支持本地缓存目录与离线 JSONL 回退；CI/演示用已转好的 JSONL 兜底（存项目内）。
- **[Agent 执行耗时长、可能触发工具副作用]** → 评测走只读/沙箱语义的 Agent（rag_assistant/chat_assistant 优先），Agent 执行器设超时（已有 timeout 参数），并发受 Worker 槽位限制。
- **[qa/llm 别名不一致导致查找失败]** → 在注册表集中处理别名并加单测覆盖 `qa/llm/rag/agent` 四种路由。
- **[Agent 指标输入字段缺失导致 tool_selection 计算不准]** → 转换器对无工具标注的数据集仅产出 answer_accuracy/llm_judge，明确标注该数据集不支持轨迹指标。
- **[前端命名改动影响既有入口/收藏]** → 仅改展示文案与路由 meta.title，保留 `/evaluation` 路径，避免破坏链接。

## Migration Plan

1. 加样本字段 → `cd cognida-go && go run ./cmd/migrate-db` 同步表结构。
2. 组合根注册执行器 + 适配器 → 重新 `wire`/编译，单测验证四类型路由。
3. 前端加 Agent 选择器与 API → 本地验证创建 agent 任务能出结果。
4. Python 转换器 + Go seed 脚本 → 跑一次灌入预选数据集。
5. 回滚：executor 注册与前端选择器均为增量，回退只需还原注册与隐藏选择器；新增样本列为可选，保留无害。

## Open Questions

- BFCL/ToolBench 具体子集与许可是否适合直接入库演示？（实现时确认 license，必要时改用自造 Agent 样本 + 通用 QA。）
- Agent 执行器的 `AgentService.Chat` 是否需要透传会话/工具上下文，还是单轮问答即可满足评测？（默认单轮，后续可扩展。）
