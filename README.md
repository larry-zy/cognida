# Cognida

<div align="center">

**企业级 AI-Native 数据全链路平台**

以 Agent 为内生执行内核，覆盖「数据 + 知识」从接入、治理、沉淀、分析到评测、进化的全生命周期——受治理、可评测、可安全执行

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Python Version](https://img.shields.io/badge/Python-3.11+-3776AB?style=flat&logo=python)](https://www.python.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

</div>

---

## 界面预览

> **DaDa 数据智能体 · 结果画布**：左侧「思考过程」实时展开 ReAct 推理步骤，右侧 A2UI 生成式 UI 随流下发 UISpec，将图表 / 明细表 / 趋势结论实时绘制到结果画布。

![DaDa 数据智能体 · 结果画布](assets/images/data-agent-canvas.png)

---

## 项目简介

**Cognida** 是一个面向企业的 **AI-Native 数据全链路平台**——**不是"又一个能问数据的 Agent"**，而是把 Agent 作为**内生执行内核**、覆盖「数据 + 知识」从接入到进化整条链路的数据平台。采用 Go + Python 异构多服务架构。

### 名字的意义

**Cognida = Cognition（认知）+ Data（数据）**，词根取自拉丁语 *cognoscere*「去认识、去理解」、*cognitio*「认知 / 知识」。

- **Cogni-** 是"认知"的词根——不满足于"存起来、查得到"，而追求**理解、推理、沉淀、进化**；
- **-da** 既收束于 **Data（数据）**，也让整个词读起来像一个专有的名字，而非一串功能缩写。

一句话：Cognida 想做的不是"能被查询的数据"，而是**具备认知能力的数据与知识**——让数据自己会理解、会解释、会积累经验、会越用越好。这正对应了页尾那句 slogan：**Build Intelligence into Data & Knowledge**。

它以**一个企业级硬化的 Agent 内核**为引擎——口径受治理、操作可管控、过程可审计、质量可评测——但对外交付的是一个**平台**，在整条数据链路上统一承载：

- **结构化数据**的接入、查询、分析与安全操作
- **非结构化知识**的收集、沉淀、问答与溯源

目标不是做一个"会问答的 Agent"，而是让 Agent **内生驱动**数据与知识从「**进来**」到「**被用起来**」再到「**被评估改进**」的**全链路**——这就是 "AI-Native"：智能不是套在数据平台外面的问答壳，而是长在每个环节里。

### 系统定位

> Cognida 不是「又一个能问数据的 bot / Data Agent」。**BI / ChatBI** 只会查结构化数据、出图；**Data Agent（问数 / RAG 应用）** 只是个会查数据、答文档的"会调工具的对话体"——两者都停在"能不能答"这一层的**应用**。Cognida 定位在更底层：一个 **AI-Native 的数据全链路平台**——Agent 只是它的**执行内核**，产品是覆盖数据/知识**从收集、治理、沉淀、分析到评测、进化**整条链路、且"答之后还能被验证"的平台底座。

一句话记忆点：

> **别人做的是「能问数据的助手 / Agent 应用」；Cognida 做的是「以 Agent 为内核、把数据与知识全链路做成 AI-Native 的企业级数据平台——敢托付、可评测、能进化」。**

### 核心差异

| | BI / ChatBI | Data Agent（问数 / RAG 应用） | 数据中台 | **Cognida** |
|---|--------|-----------|---------|----------|
| **形态** | 应用 | 应用 | 平台（非 AI-Native） | **平台 + AI-Native 内核** |
| 结构化数据 | ✅ | ✅ | ✅(底座) | ✅ |
| 非结构化知识 | ✗ | ✅ | 部分 | ✅ |
| 口径受治理(可信 SQL) | 部分 | ✗ | 部分 | ✅ |
| 可安全执行(读/写/ETL 分级) | ✗ | ✗ | ✗ | 🟡 底层工具已有，默认分析只读 |
| 可评测(轨迹级度量) | ✗ | ✗ | ✗ | ✅ |
| 全过程可审计 | 部分 | ✗ | 部分 | ✅ |
| AI-Native 全链路（收集→进化，Agent 内生驱动） | ✗ | ✗ | ✗ | 🟡 建设中 |

> 说明：**BI / Data Agent 都是"应用"**（停在能不能答），**数据中台是"平台但非 AI-Native"**（智能靠外挂）。Cognida 把两者合一——**平台形态 + AI-Native 内核**；其内置的 Data Agent（DaDa）只是这个平台的**一个执行面**，而非产品本身。

**护城河 = 三者的交集：企业级硬化的 Agent 内核 × 数据/知识双栖 × 全生命周期覆盖。** 单看每一格都是商品，叠加起来才是"组织敢长期托付、还能持续进化"的 **AI-Native 数据全链路平台**。

---

## 能力全景（数据/知识 × 全生命周期）

状态核对日期：**2026-10-15**。✅ 已有实现（须结合边界与验收范围） · 🟡 有基础，闭环待补 · 🚧 计划中 · ⏸ 后续阶段。本文区分代码实现与实际验收，不能把接口存在、流程执行成功或模型自评当作业务质量达标。

当前工作范围为 **第 1 组：业务闭环 + 第 2 组：能力补全**，详见[调研与实施设计](docs/openspec/changes/complete-governed-data-loop/design.md)及[实施任务](docs/openspec/changes/complete-governed-data-loop/tasks.md)。首批已实现统一任务持久化、草稿／计划预检 API 和 Sandbox 契约校验，已通过隔离 MySQL、HTTP 与独立 Review；详见[接口与验收边界](services/cognida-go/internal/service/taskcontrol/README.md)。预检固定返回 `execution_enabled: false`，Supervisor、实际 Runner、运行时调度与统一任务页面仍需实现。Agentic RL、模型训练／微调和独立生产化工程延后。

| 生命周期 → | ①收集/接入 | ②清洗/达标 | ③沉淀/资产化 | ④分析/决策 | ⑤评测/改进 |
| --- | --- | --- | --- | --- | --- |
| **结构化数据** | MySQL/PostgreSQL ✅<br>表/结果/评测来源导入 ✅<br>通用 API 与数仓适配 🚧 | 质量检查与预置清洗 ✅<br>诊断与差异预览 🟡<br>审核标注闭环 🟡 | 语义层、版本资产 ✅<br>字段特征/版本/时点读取 ✅<br>自动变换与物化 🚧 | Data Analysis / Text2SQL / A2UI ✅<br>专门受控写入/ETL入口 🟡<br>数仓分析 🚧 | QA/RAG/Agent/SQL评测与比较 ✅<br>真实基线漂移监控 🟡<br>评测返工与发布闭环 🚧 |
| **非结构化知识** | 文档上传解析 ✅<br>网页采集/调度/原始快照 ✅<br>逐格式和增量覆盖 🟡 | 质量/PII/去重/清洗 ✅<br>规则打标与人工审核 ✅<br>语义标注与审核结果应用 🟡 | 分块/向量化/知识图谱 ✅<br>私人长期记忆 ✅<br>审核样本→版本数据集 🚧 | Agentic RAG与优化器工具 ✅<br>DeepResearch规划/引用校验 ✅<br>主张支持与来源冲突核查 🚧 | 检索/生成/RAG评测 ✅<br>固定样本、返工、复测与撤回 🚧 |
| **贯穿层** | 来源与凭据授权 | 领域专家与审批 | 不可变引用、审计、租户隔离 | ReAct/协作/持久化流水线 ✅ | 统一任务草稿／预检 🟡 · 跨域 Supervisor 🚧 · Sandbox 契约 🟡 |

现有 Collection 还保留直接入库路径；新计划将其新建／重跑迁移至统一候选与发布审批。下述审批验证针对生命周期流水线，不能推断所有旧入口都已有相同门禁。

已有[真实页面验收](apps/cognida-web/e2e/ACCEPTANCE-2026-10-15-UI.md)覆盖文本、表格、网页、数据库表、SQL 结果、采集产物、知识文档、评测结果八类来源。文本类验证知识发布及真实向量命中；表格类验证版本资产与分析报告。该验收不代表全部文件格式、全部数仓、标签准确性、研究事实正确性或生产长期运行已经达标。

### 已实现能力速览 ✅

- **Agent 内核**：ReAct 编排 · 上下文三级压缩 · Multi-Agent 协作 · 反思 / Hooks · SSE 流式
- **Data Agent（DaDa）**：语义选表 · 指标语义层 · Text2SQL · 自我修复 · A2UI 生成式 UI
- **知识侧**：混合检索（RRF + 重排）· 知识图谱多跳 · Agentic RAG 与重写/HyDE/多跳工具 · 跨轮消息与私人长期记忆 · DeepResearch 规划、检索与引用校验
- **质量 / 评测**：数据质量中心 · QA / RAG / Agent / Text2SQL 四类评测（含轨迹级）
- **底座**：多租户 RBAC · request_id + 审计 + Loki · LLM 统一弹性 · Skill / MCP · Docker 一键部署
- **自进化**：会话经验蒸馏 → SKILL / 图谱 / 经验库（已接线，默认关）

> 尚未完成的跨域协调、审核产物消费、数据集版本、评测返工，以及数仓、自动特征、漂移、深研和受控操作扩展，统一列在文末的 **[规划中 🚧](#规划中-)** 与 **[路线图](#路线图)**。

---

## 已实现功能 ✅

### Agent 内核

下图是 **Data Agent（DaDa）的全流程**——单一 ReAct 内核（推理 → 行动 → 观察）的目标能力视图。当前默认入口执行取数、自我修复、分析和渲染；图中的写入操作需走独立授权，不能由示意图推断已开放：

![Data Agent（DaDa）全流程](assets/images/agent-flow.png)

- **ReAct 编排**：推理-行动循环，统一 Eino Tool 框架
- **上下文工程**：三级压缩（① 单条 >32k 就地压缩 · ② 全对话累计 ≥128k 折叠回 ~64k，按整轮折叠保 tool 配对，A 级观测遮蔽 + B 级摘要 · ③ 陈旧推理驱逐 16k），配套 `bpecount` 真实 BPE 计数（内嵌 DeepSeek-V3 tokenizer，`//go:embed`）而非字符估算
- **DeepResearch**：已实现有界规划、并发授权检索、综合报告和引用 ID／原文摘录校验，支持取消与部分失败说明；主张的语义支持、来源独立性与矛盾核查待补齐，引用有效不等于结论真实
- **Multi-Agent 协作**：任务分解、智能分发、结果聚合；委托含「上下文防火墙」+ 委托轨迹穿透（供评测/审计）
- **编排原语与领域执行**：Sequential / Parallel / Loop / Conditional / Supervisor 原语；七类数据专家与持久化流水线已具备审批、取消、重试和恢复。跨采集、标注、评测及研究的目标级 Supervisor 仍在规划，不能与现有原语或固定模板混为一谈
- **反思 / 评审（critic）**：可插拔 LLM / 规则评审子系统（Actor-Critic-Memory），按 Agent 配置启用
- **Agent Hooks**：数据结论生成、意图澄清、反思、自我修正护栏、guardrail、工具策略（按预设装配）
- **流式响应**：SSE 实时输出，结论/图表边推理边下发
- **Agent 自进化（经验沉淀，默认关）**：后台异步、不阻塞对话——**会话经验蒸馏**：空闲会话经 LLM 蒸馏为 SKILL.md + 知识图谱 + 经验库，图谱召回在首答注入历史经验；写侧 `EXPERIENCE_DISTILL_ENABLED`、读侧 `EXPERIENCE_RECALL_ENABLED`、SKILL 落地 `EXPERIENCE_SKILL_SINK_ENABLED` 各自独立（均默认关），非 LLM 客观失败门 `EXPERIENCE_PREGATE_ENABLED` 默认开

### 结构化数据：查询 · 分析 · 安全操作

- **Data Agent（DaDa）**：单一 ReAct 内核驱动取数、分析与渲染，意图路由 → playbook，分层子代理委派（SchemaExplorer / SQLAuthor / Analysis / Viz + Insight / Report）。当前默认分析入口不注册 Operation；底层操作工具与后续专门操作入口分开授权
- **Text2SQL**：**运行在 Data Agent ReAct 内核内**（`get_schema` + `sql_execute` + `semantic_query` 工具循环 + 下述自我修复），而非独立的 Plan-Execute-Reflect 智能体；Schema 感知、多轮对话、结果解释
- **指标语义层（NL2Semantics）**：在 LLM 与数仓之间引入受治理的指标语义层，把「裸写 SQL」升级为「按中心化口径生成 SQL」，附覆盖率统计（covered / cache_hit / fallback）
- **多数据源接入**：MySQL / PostgreSQL，凭据 AES-256-GCM 加密、连接池、健康检查、`information_schema` 内省，外部源视为**只读外部资源**
- **A2UI 生成式 UI**：`render_ui` 随流下发 UISpec，图表/表格/结论实时绘制到结果画布；Result Store 中间结果落库、会话重开恢复画布
- **安全执行基础**：已有 `read / write / etl` scope、写操作确认卡、令牌时效、幂等和审计工具。默认分析和外部数据源保持只读，查询限制结果规模并设置超时；专门写入／ETL 任务入口、审批绑定与提交恢复仍需补齐。现有工具门控不是代码 Sandbox
- **自我修复（业务层闭环）**：SQL/取数报错时不止于重试——① **结构化错误分级**（`syntax / unknown_column / unknown_table / permission / timeout / transient`，按 MySQL 错误码，`sql_error.go` `classifySQLError`）；② **schema 线索回注**（列不存在回传该表可用列、表不存在回传近似候选表，经 `information_schema` 现取，`buildSchemaHint`；并对枚举/取值列回注真实取值 `columnFactsHint`）；③ **重复失败护栏**（按 `tool:error_kind` 计数，触顶提前收尾并转部分结论，`self_repair_guard.go`，防无限失败循环）；④ **动态重规划**（同一失败签名达阈值注入再规划提示，引导换路径而非原样重试）

### 非结构化知识：检索 · 图谱 · 记忆

- **统一检索能力（`RetrievalCapability`）**：Agent 工具与 REST 接口同源——Milvus 语义向量 + BM25 全文双路召回，RRF 融合、可插拔重排精排、按知识库有界并发扇出（上限 8）；`kb_fetch_chunks` 支持定位原文片段。HyDE / 查询重写 / 多跳已提供**REST 与 Agent 内可组合工具**，复用授权检索能力
- **知识库范围强制**：范围经会话入口选定并由 ctx 强制透传，工具层求交拒绝越权
- **知识图谱**：文档入库时 LLM 并发抽取实体关系三元组，Neo4j 存储；图谱检索支撑关系/关联/溯源类问答；图谱提取=库级开关、检索=提问级开关；一键补建、图谱管理与可视化
- **知识库管理**：PDF / Word / Excel / Markdown / TXT，智能分块、向量化、OCR（委派 Python 文档服务，默认 `ExtractImages:false`）、按 file hash 去重预检
- **跨轮与长期记忆**：会话内消息回放；另有 MySQL 持久化的本人事实／偏好、管理页面、增删改查工具与每轮召回，强制 tenant/user 隔离。当前为显式保存，同模型向量召回在最多 500 条近期候选内排序，失败可词法回退；没有后台自动提取、共享记忆或全库 ANN。默认聊天、RAG 与数据分析已接入，专门研究流程尚未接入私人召回，详见[记忆边界](services/cognida-go/internal/service/memory/README.md)

### 质量 · 评测（可信与度量）

- **数据质量中心**：完整性 / 一致性 / 准确性 / 有效性 / 唯一性 / 时效性打分（维度由 Python 动态返回）；非结构化质量（可读性/信息密度/语言质量 + PII 检测）；数据清洗（去噪/去重/格式转换）。Go 端委派 Python 计算，既有质量入口走 gRPC，生命周期计算走受认证 HTTP。Python 漂移检测已有框架，但随机重建基线的统计方法需要修正，真实基线、调度与 Go/Web 监控闭环尚未完成
- **评测子系统**：QA / RAG / Agent / **Text2SQL(SQL)** 四类评测，**Go 编排/执行/存储，Python 计算打分**，grader 注册表为单一事实源
  - 检索指标：Recall@K、Precision@K、MRR、NDCG、MAP
  - 生成指标：BLEU、ROUGE、语义相似度
  - RAG 专项：faithfulness、context_relevance、noise_ratio
  - **Agent 轨迹级**：答案准确性、工具选择、工具顺序、轨迹匹配、步骤效率
  - **Text2SQL/SQL**：`sql_exact_match`、`sql_component_match`、`sql_execution_accuracy`（Go 只读执行金标准 + 生成两条 SQL，Python 无序比对结果集；SQL 评测自动剔除 ROUGE/BLEU 生成类评分器）
  - 规则类：exact / contains / regex / numeric
  - LLM-as-a-Judge：多维裁判（事实正确性、内容安全性等）
  - 独立 FastAPI 评测服务（默认 :18888），前端指标可视化配置与结果明细一体化

生命周期的 `data-evaluation` 阶段当前检查**产物完整性与证据关联**，不能替代上述基于标注样本的模型质量评测。固定数据集版本、质量门槛和返工发布将单独接入。

### 平台底座

- **多租户**：数据与权限隔离、RBAC（`model/user/rbac`）、用户管理
- **可观测性**：request_id 全链路透传、结构化审计（audit_logs）、Loki 日志采集，同 rid 关联
- **LLM 统一弹性**：跨目标有序降级链 + 同目标指数退避重试（全抖动）+ per-target 三态熔断，透明装饰、成功路径零改动；eino 化 `LLMClient`（`eino_client.go`）承接 OpenAI 兼容 Provider，DeepSeek 原生 `<｜｜DSML｜｜tool_calls>` 由 `chat/dsml.go` 解析回结构化工具调用
- **Skill 系统**：Agent 本地精选 skill/playbook 注册表，`skill_invoke` / `skill_list` + SKILL.md 目录（渐进式披露）
- **Python 数据分析工具（MCP）**：经 MCP 通道调用 Python `analytics`（趋势/归因/相关/异常等），缓存 + 指数退避重连 + 健康检查

---

## 技术架构

平台以 Go 主后端为承载、Agent 内核为引擎，Python 负责重计算，四类存储各司其职——Agent 是**长在平台里的执行内核**，而非独立部署的一个 bot。

![Cognida 技术架构](assets/images/arch-hero.png)

### 目标架构：业务闭环与能力补全

下图为 Cognida 的**目标架构**，采用六层主体，以及贯穿各层的安全治理、评测与自进化体系。内容以项目代码与[本期设计](docs/openspec/changes/complete-governed-data-loop/design.md)为基础，展示数据接入、治理、数仓与语义、知识与记忆、分析研究、特征及数据集之间的关系；参考图仅用于视觉样式。图中不标注实施进度，实际支持与验收状态以本文「能力全景」及「规划中」为准。

![Cognida 数据与知识平台目标架构：技术栈、中间件、数仓与自进化](assets/images/cognida-target-architecture-v2.png)

左侧为内容来源及数据库／数仓引擎，与底部的平台存储分开；加工写入须限定在授权的受管派生空间。自进化通过反馈归因、经验提炼、固定基线评测、审核发布与监测回退改进数据和执行策略；RL、模型训练及部署不在本图范围。

图中实线表示调用与数据流，虚线表示反馈与迭代；分层表达职责，不表示所有请求必须依次经过每层。品牌图标为图内重绘。[架构原图](assets/images/cognida-target-architecture-v2.png) · [生成提示词](assets/images/cognida-target-architecture-v2.prompt.txt)。

**依赖方向**：`handler → service → model ← repository`

### 存储约定

| 存储 | 用途 |
|------|------|
| MySQL | 元数据、配置、任务状态、语义模型、评测结果、审计日志 |
| Milvus | 知识库向量、检索特征 |
| Neo4j | 知识图谱、实体关系、经验图谱、血缘 |
| Redis | 缓存、任务队列、进度、Result Store、分布式锁 |

---

## 服务通信

### Go → Python

| 方式 | 端口 | Python 服务 | 状态 | 用途 |
|------|------|------------|------|------|
| gRPC | 50051 | Document Service | ✅ | 文档解析、OCR、分块、URL 抓取 |
| gRPC | 50051 | Quality Service | ✅ | 质量检查、数据清洗 |
| HTTP | 18888 | Evaluation Service | ✅ | 评测计算（compute-metrics，无状态） |
| HTTP | 8000 | 基础 HTTP 服务 | ✅ | Python `main.py` 基础接口 |
| MCP  | 3000（本地 3100） | Analytics 工具通道 | ✅ | Agent 经 MCP 调用 Python 数据分析能力 |
| HTTP | 18888 | Lifecycle Compute | ✅ | 质检、清洗、增强、分析及已有图像计算；Go 管理任务/资产/审批 |

采集、标注与特征元数据已由现有 Go 模块及 Python 计算接口承载，不以规划中的 50054/50055 服务已运行作为前提。自动特征物化、语义标注和 Sandbox 仍按下文计划实施。

- **Proto 单一数据源**：仓库根 `proto/*.proto`（buf 管理，`buf.yaml` / `buf.gen.yaml` 在根目录），Go / Python 各自生成绑定
- **MCP**：JSON-RPC 2.0 over HTTP/stdio；Go 端 `internal/infrastructure/mcp`（客户端），Python 端 `services/cognida-python/mcp_service`（服务端，默认端口 3000，本地占用改用 3100）

---

## 应用场景

### 能力 → 场景对照

| 场景 | 说明 | Cognida 能力 |
|------|------|----------|
| **可信智能分析** | 自然语言查数，口径受治理、结果可审计、直接出图 | Data Agent · 语义层 · A2UI ✅ |
| **深度研究分析** | 规划、授权检索、引用与报告；语义核查待增强 | DeepResearch 基础流程 ✅ |
| **知识问答与溯源** | 企业知识沉淀、检索、关系溯源 | Agentic RAG · 知识图谱 ✅ |
| **数据驱动决策** | 基于分析输出可落地行动建议 | Agent 决策 · 结论生成 ✅ |
| **自动数据沉淀** | 已有采集与审批发布；审核样本、特征和反馈仍需贯通 | 已有主链路 ✅ · 完整闭环 🚧 |

### 能落地的具体场景

Cognida 的价值在"数据 + 知识 + 受治理执行 + 可评测"叠加时才显现。下面是几个可以真实跑起来的落地样例——绝大多数依赖的能力今天已 ✅，标 🚧 的部分是当前的洞（也正是欢迎参与的地方）。

- **📊 经营分析 / 智能 BI 替身**
  业务负责人用大白话问「上月各区域毛利环比、掉得最狠的三个 SKU 是什么」，Agent 走**指标语义层**按中心化口径生成 SQL、只读执行、A2UI 直接画出图表与结论，全过程留 `request_id` 审计。相比传统 ChatBI，多了**口径治理**（不会各人各口径）和**结果可审计**（能追责、能复盘）。

- **📚 企业知识助手 / 制度问答与溯源**
  把制度、SOP、合同、产品手册灌进知识库，员工问「XX 报销标准是多少、依据哪条」，Agentic RAG 混合检索 + 重排给出答案并**定位到原文片段**；涉及"谁签了这份合同、关联哪个项目"这类关系型问题，走**知识图谱多跳**溯源。知识库范围按会话强制隔离，越权访问在工具层被拒。

- **🔍 尽调 / 竞品 / 市场深研报告**
  给一个宽泛课题（如"某赛道近一年格局变化"），DeepResearch 规划并检索授权来源，生成带引用的报告。当前校验来源 ID 与原文摘录，语义支持、同源转载和冲突核查仍待增强；研究结论需结合证据复核。

- **⚠️ 风险 / 异常运营洞察**
  「哪些客户有流失风险、为什么、怎么挽回」——Agent 经 MCP 调 Python analytics 做趋势/归因/异常检测，再由结论 Hook 生成可落地的行动建议，而不止于甩一张表。

- **🛡️ 受管控的数据操作（读 / 写 / ETL 分级）**
  已有分级、确认卡和底层操作工具；默认分析及外部源仍只读。当前计划新增专门操作入口，对受管派生对象展示计划、影响范围、审批与执行回执，补齐 ETL 先生成后切换及失败核对；不会把聊天入口整体升级为写权限。

- **🧪 Agent / RAG / Text2SQL 质量评测平台**
  团队自研了 AI 问数或 RAG 应用，需要客观回归："这次改 prompt 到底变好还是变坏了？" 用内置评测子系统跑**轨迹级 Agent 评测 / RAG faithfulness / Text2SQL 执行准确率**，把"感觉变好了"变成可度量的分数与明细。这一块可以**独立于主业务单独使用**。

- **🧩 数据 + 知识联合问答（双栖场景）**
  真正区别于"只会查库的 bot"或"只会答文档的 RAG"的场景：一个问题同时需要**结构化事实**（订单量、金额）和**非结构化背景**（合同条款、政策说明），Cognida 在同一个 Agent 内核里把两栖数据一起用起来。

- **🔄 自主进化的数据工位（愿景 🚧）**
  终态目标：Agent 自动收集 → 达标 → 沉淀 → 分析 → 评测 → **把评测结论反哺收集/沉淀策略**，让整条流水线自己越转越好。当前已有采集、治理、发布和评测基础；还需跨域控制、审核结果消费、版本数据集、固定基线、返工与发布撤回。经验蒸馏默认关闭，RL 后续另议——见「规划中 🚧」。

> 适用行业不限：零售/电商（经营分析、流失预警）、金融/投研（尽调、风控、合规问答）、制造/供应链（质量与异常）、SaaS/互联网（增长归因、AI 应用评测）等，凡是"既有库表数据、又有文档知识，且对口径与可审计有要求"的组织都能对号入座。

### 用户角色

| 角色 | 典型问题 |
|------|---------|
| **管理层** | "本月业务健康度如何？有什么需要关注？" |
| **业务分析** | "分析 A 渠道 ROI 下降的原因" |
| **业务运营** | "哪些客户有流失风险？给出挽回建议" |
| **IT/数据** | "报表 X 的数据来源是哪里？口径对不对？" |

---

## 能力分级

| 级别 | 定义 | 状态 |
|------|------|------|
| **L1** | 模板化输出：按预设模板生成图表报表 | ✅ |
| **L2** | 自然语言交互：对话式输出分析结论 | ✅ |
| **L3** | 主动决策：拆解任务、规划路径、验证结果、输出行动建议 | 🚧 演进中（Data Agent 已落地）|
| **L4** | 自主进化：自动收集→达标→沉淀→分析→评测→反哺的闭环自转 | 🚧 建设中 |

---

## 项目结构

```
cognida/
├── services/
│   ├── cognida-go/               # Go 主后端（API / 编排 / Agent / RAG / 图谱 / 语义 / 评测编排）
│   │   ├── cmd/                  # 可执行入口：server / migrate-db / seed-*（演示与冷启动灌数）
│   │   ├── internal/
│   │   │   ├── handler/          # HTTP handlers（依赖方向：handler → service → model ← repository）
│   │   │   ├── service/          # 业务逻辑：agent / knowledge / semantic / evaluation / quality …
│   │   │   ├── repository/       # 数据访问实现（MySQL / Milvus / Neo4j / Redis）
│   │   │   ├── model/            # 实体与接口定义
│   │   │   ├── infrastructure/   # 基础设施：llm / mcp / cache / graph / auth / config …
│   │   │   ├── wire/             # 依赖注入装配（google/wire 组合根）
│   │   │   └── prompt/           # 提示词配置（//go:embed 打包）
│   │   ├── migrations/           # golang-migrate 版本化迁移（业务表结构唯一真源）
│   │   ├── skills/               # Agent Skill 定义（SKILL.md，运行时按 ./skills 加载）
│   │   ├── api/ · scripts/ · examples/
│   │   └── go.mod
│   └── cognida-python/           # Python 计算服务（文档解析 / 评测 / 质量 / analytics）
│       ├── grpc_service/ · mcp_service/    # gRPC 服务端 + MCP 服务端
│       ├── services/ · core/ · plugins/    # 业务逻辑 / 内核 / 插件
│       ├── api/ · tools/ · tests/
│       └── main.py · pyproject.toml
├── apps/
│   └── cognida-web/              # Vue 3 前端（Vite）：src / public / docs
├── proto/                        # gRPC 契约（buf 管理，单一数据源）
├── deploy/                       # 部署与日志采集（Loki 等）
├── .github/                      # CI（含 gitleaks 安全扫描门）
├── buf.yaml · buf.gen.yaml       # buf 代码生成配置
└── dev.sh                        # 本地一键起停脚本
```

---

## 快速开始

### 环境要求

- **Go** 1.25+ · **Python** 3.11+（uv）· **Node** 18+ · **MySQL** 8.0+ · **Milvus** 2.6+ · **Neo4j** 5.0+ · **Redis** 7.0+
- **本机开发模式**：中间件（MySQL / Redis / Neo4j / Milvus）需**预先常驻**（本机以 Homebrew services 管理 MySQL/Redis/Neo4j，Milvus 跑在 docker 容器），应用层用下方 `dev.sh` 启动。
- **容器一体化模式**：仓库根目录已提供 [`docker-compose.yml`](docker-compose.yml)，一条命令拉起**全部组件与服务**（前端 + Go + Python + MySQL/Milvus(+etcd+minio)/Neo4j/Redis），无需本机预装任何中间件，见下方「Docker 一键部署」。

### 配置参数

**配置分两层**：非密配置（端口 / 模型名 / base_url / 超时 / 开关…）是**入库真源**，密钥（密码 / Token / API Key）**只走环境变量 `.env`，绝不入库**。加载优先级从低到高：**代码内兜底默认 < `config.yaml` < 环境变量 / `.env`（覆盖，且是密钥唯一来源）**。

| 服务 | 非密真源（入库） | 密钥来源（复制 `.env.example` → `.env`，已 gitignore） |
|------|---------|------|
| Go 后端 | `internal/config/config.yaml`（`//go:embed` 编译期打包进二进制） | `services/cognida-go/.env` |
| Python | `services/cognida-python/config/config.yaml` | `services/cognida-python/.env` |
| 前端 | — | `apps/cognida-web/.env.development`（`VITE_API_BASE_URL`） |

> **Docker 模式更省事**：只需在**仓库根目录一个 `.env`**（`cp .env.docker.example .env`）填全部密钥，compose 会注入到各容器，无需再改服务内的 `.env`。

**必填 / 常用密钥**（缺失则对应能力不可用；`JWT_SECRET` 缺失或不足 32 字节会**直接启动失败**，fail-closed）：

| 变量 | 用途 | 获取 / 生成 |
|------|------|------|
| `JWT_SECRET` | 登录令牌签名 | `openssl rand -hex 32` |
| `DB_PASSWORD` · `NEO4J_PASSWORD` | MySQL / Neo4j 密码 | 自定义 |
| `CHAT_API_KEY` | 聊天大模型（Go 侧，默认 DeepSeek） | 模型厂商控制台 |
| `EMBEDDING_API_KEY` | 向量化（默认阿里云 DashScope） | 阿里云百炼 |
| `LLM_API_KEY` | Python 侧 LLM（文档 / 评测 / 质量） | 模型厂商控制台 |
| `DATASOURCE_SECRET_KEY` | 外部数据源密码 AES-256 加密（多数据源必需） | `openssl rand -base64 32` |
| `METASO_API_KEY` | 联网搜索（DeepResearch，可选） | Metaso |
| `MILVUS_TOKEN` · `REDIS_PASSWORD` | 云 Milvus / 带密 Redis（本机可留空） | 按需 |

**切换模型 / Provider**（改非密 `config.yaml`，Key 仍走 `.env`）：默认聊天模型 `deepseek-chat`（`chat.base_url: https://api.deepseek.com/v1`），Embedding `text-embedding-v3`（DashScope）。换成 OpenAI 或其它 OpenAI 兼容服务，只需改 `chat.base_url` / `chat.model_name` / `chat.provider`，Key 填到 `CHAT_API_KEY` 即可——LLM 统一弹性层承接任意 OpenAI 兼容 Provider。

### Docker 一键部署（全组件）

`docker-compose.yml` 编排全部基础设施与三个应用服务，数据库迁移由一次性 `cognida-migrate` 服务在 Go 启动前跑完（golang-migrate 版本化迁移，不自动建表）。

```bash
git clone https://github.com/your-org/cognida.git && cd cognida

cp .env.docker.example .env      # 填入密钥：DB_PASSWORD / NEO4J_PASSWORD / JWT_SECRET / 各 API Key
docker compose up -d --build     # 首次构建 + 拉起全部服务
docker compose ps                # 查看健康状态
docker compose logs -f cognida-go
docker compose down              # 停止（加 -v 连同数据卷一起清空）
```

访问：前端 `http://localhost:8888` ｜ Go API `http://localhost:8080/api/v1` ｜ Neo4j Browser `http://localhost:7474`。
前端经 nginx 同源反代 `/api → cognida-go:8080`（含 SSE 关缓冲），无需额外配置 CORS。

### 一键启动（本机开发，推荐）

`dev.sh` 是真实的一键启动脚本，负责启停 Go 后端 + 4 个 Python 服务 + 前端，并对中间件做连通性预检。

```bash
git clone https://github.com/your-org/cognida.git
cd cognida

# 先确保中间件在跑（示例：Homebrew + docker）
brew services start mysql@8.0 redis neo4j && docker start milvus-standalone

./dev.sh start      # 启动全部服务
./dev.sh status     # 查看状态
./dev.sh logs go    # 跟踪某个服务日志
./dev.sh stop       # 停止全部服务
```

### 手动分服务启动

```bash
# 1. Go 后端（:8080）
cd services/cognida-go && go run ./cmd/server

# 2. Python 服务（uv）
cd services/cognida-python
uv run python -m grpc_service.server                                   # gRPC 50051 (Document/Quality；Analytics 走 MCP)
uv run python main.py                                                  # 基础 HTTP :8000
uv run uvicorn services.evaluation.fastapi_app:app --port 18888        # 评测 :18888
MCP_MODE=http uv run python -m mcp_service.server                      # MCP :3000（本机 MCP_PORT=3100）

# 3. 前端（:5173）
cd apps/cognida-web && npm install && npm run dev
```

| 服务 | 端口 | 说明 |
|------|------|------|
| cognida-go（后端） | 8080 | REST / gRPC / SSE 网关 |
| cognida-python gRPC | 50051 | Document+Quality |
| cognida-python 基础 HTTP | 8000 | `main.py` 基础接口 |
| cognida-python 评测 | 18888 | FastAPI 评测计算 |
| cognida-python MCP | 3000（本机 3100） | HTTP 模式 MCP |
| cognida-web（前端） | 5173 | Vite 开发服务器（strictPort，代理 /api→:8080） |

> **数据库表结构**：业务表由**版本化迁移**（golang-migrate）管理，运行时**不自动建表**。首次启动（或改 schema 后）需先跑一次迁移——Docker 模式由一次性 `cognida-migrate` 服务在 Go 启动前自动完成；本机 / 手动模式执行：
> ```bash
> cd services/cognida-go && set -a && source .env && set +a && go run ./cmd/migrate-db up
> ```

**调用链路**：浏览器(:5173) → Go 后端(:8080) → gRPC/HTTP/MCP → Python 服务

---

## 规划中 🚧

以下列出**尚未完成的部分**，不把已有采集、清洗、标注、特征字段存储等重新计为从零建设。完整设计和依赖见[实施计划](docs/openspec/changes/complete-governed-data-loop/design.md)。

| 工作组 | 模块 | 本轮计划范围 |
| --- | --- | --- |
| 1 业务闭环 | 上层 Supervisor | 已有任务存储与计划预检；继续接跨域执行、运行时总预算、审批、恢复与任务页面 |
| 1 业务闭环 | Sandbox | 已有 Runner 契约与纯校验；继续实现隔离执行、资源/网络/文件限制、取消回收和真实候选产物校验 |
| 1 业务闭环 | 清洗与两类打标 Agent | 清洗差异复检；知识标签审核后应用；分类/实体/关系/问答样本标注 |
| 1 业务闭环 | 数据集与质量门禁 | 不可变样本版本、划分去重、评测固定输入和配置、候选验证 |
| 1 业务闭环 | 反馈与发布 | 问题定位→返工→复测→审批激活/撤回；源更新、删除与权限失效传播 |
| 1 业务闭环 | 策略／Skill 进化 | 经验与策略候选版本、固定样本回归、确切版本审批、实际加载回执及停用／回退 |
| 2 能力补全 | 接入覆盖 | Web/API/文件/库表分别声明鉴权、分页、格式、增量和真实验收范围 |
| 2 能力补全 | 数仓分析 | ClickHouse/Doris/StarRocks → Hive/BigQuery，逐引擎适配和真实验证，保持只读 |
| 2 能力补全 | 数仓建模与分层加工 | 事实粒度／维度关系、分层模型与有界 DAG、受管派生目标、增量水位／回填和真实表列血缘 |
| 2 能力补全 | 自动特征工程 | 在已有字段/版本/as_of基础上增加受限变换、聚合、物化及批量消费 |
| 2 能力补全 | 漂移监控 | 修正真实基线算法，接入比较、调度、通知和治理任务 |
| 2 能力补全 | 深研增强 | 逐主张证据支持、来源冲突、同源识别及预算内补充调查 |
| 2 能力补全 | 受控写入/ETL | 独立入口、内容绑定审批、派生目标授权、幂等提交核对和安全替换 |

**暂缓范围：** Agentic RL、奖励/策略训练、模型训练与部署回路；K8s/Helm、镜像发布体系、自动扩缩容等独立生产化工程。本轮保留必要的任务恢复、审计、执行限额与现有 Compose 接线，但不据此宣称完整生产化。

**完成标准：** 每项有真实页面入口、后端执行、持久产物、下游消费及拒绝/故障证据。缺少真实数仓环境、模型或凭据的项保持“待验收”；不将 stub、跳过测试或仅接通接口计为完成。质量门槛与人工审核保留，评测反馈不等于自动训练或默认修改生产策略。

---

## 路线图

> 已交付的不是"一个 Agent"，而是**一个平台的多块能力**——底座、Agent 内核、结构化数据智能、非结构化知识、质量评测都已并行建成（阶段一～六 ✅，是平台的不同板块而非严格先后）。阶段七起按本轮计划接通闭环并补全能力；阶段八延后。阶段一～六的已有实现仍受上述边界约束。

### 阶段一：平台底座 ✅

- [x] 多租户 RBAC · 用户 / 租户数据与权限隔离
- [x] 可观测性：request_id 全链路 + 结构化审计（audit_logs）+ Loki 同 rid 关联
- [x] LLM 统一弹性：有序降级链 + 全抖动退避重试 + per-target 三态熔断（eino 化 `LLMClient`）
- [x] 异构服务通信（gRPC / HTTP / MCP，proto 单一源）· Docker 一键部署 + golang-migrate 版本化迁移

### 阶段二：Agent 内核 ✅

- [x] ReAct 编排 · 上下文三级压缩（内嵌真实 BPE 计数）· SSE 流式
- [x] Multi-Agent 协作（上下文防火墙 + 委托轨迹穿透）· 编排原语（Sequential / Parallel / Loop / Conditional / Supervisor）
- [x] 反思 / Critic（Actor-Critic-Memory）· Agent Hooks（意图澄清 / 会话洞察 / 结论生成 / guardrail）· DeepResearch（基础流程，语义核查待增强）
- [x] Skill 系统（`skill_invoke` / `skill_list` + SKILL.md）· Python 数据分析工具（MCP）

### 阶段三：结构化数据智能 ✅

- [x] Data Agent（DaDa，单一 ReAct 内核）· 意图路由 → playbook · 分层子代理委派
- [x] Text2SQL（内核内）· 自我修复（错误分级 / schema 引导 / 列值回注 / 失败护栏 / 动态重规划）
- [x] 指标语义层（NL2Semantics，治理口径 + 覆盖率统计）· 外部多数据源接入（AES-256-GCM 加密，只读）
- [x] A2UI 生成式 UI + Result Store · scope/确认卡/审计等操作基础（默认分析只读，专门操作入口待补）

### 阶段四：非结构化知识 ✅

- [x] 统一检索能力（Milvus 向量 + BM25 全文 + RRF 融合 + 可插拔重排，按库有界并发）
- [x] 知识库管理（PDF / Word / Excel / MD / TXT + 委派 OCR + 智能分块 + 向量化 + hash 去重）
- [x] 知识图谱（LLM 并发抽取三元组 → Neo4j，多跳检索 / 溯源 + 一键补建 + 可视化）
- [x] 知识库范围强制隔离 · 跨轮消息 · 显式私人长期记忆 · RAG 优化器工具
- [x] DeepResearch 有界规划、授权检索和引用校验（语义核查待增强）

### 阶段五：质量与评测 ✅

- [x] 数据质量中心（6 维打分 + 非结构化质量 / PII + 数据清洗，Go 薄代理 + Python 计算）
- [x] 评测子系统（QA / RAG / Agent / Text2SQL 四类，Go 编排执行存储 + Python 打分）
- [x] 指标全套：检索 / 生成 / RAG faithfulness / Agent 轨迹级 / SQL 执行准确率 + LLM-as-Judge
- [x] 独立 FastAPI 评测服务（:18888，可脱离主业务单独使用）

### 阶段六：自进化（雏形）✅ / 🚧

- [x] 会话经验蒸馏（SKILL.md / 图谱 / 经验库），后台异步、默认关
- [x] 两套自进化链路收敛为一套（退役反思异步链路，统一到会话经验蒸馏）
- [ ] **受治理反馈闭环**：评测问题触发有界返工、复测与审批发布；保留权限和发布门槛，不以默认开启共享经验为完成条件

### 阶段七：业务闭环与能力补全 🚧

以下按模块跟踪整体进度，首批基础代码已交付，模块尚未整体完成；逐项证据见[可执行清单](docs/openspec/changes/complete-governed-data-loop/tasks.md)。

- [ ] A：冻结任务/产物/审批契约、来源支持矩阵与质量基线。
- [ ] B：统一控制器 + Sandbox + 任务详情，验证取消、重试和重启恢复。
- [ ] C：清洗差异、知识标签应用、样本标注和版本化数据集。
- [ ] D：先完成旧采集入口候选化与审批迁移，再接固定评测、反馈返工、激活/撤回及失效传播。
- [ ] D+：策略／Skill 版本的隔离评测、审批激活、运行快照及回退。
- [ ] E：接入与数仓适配，先兼容现有数据库，再按引擎逐批验收。
- [ ] E+：主题域／事实／维度建模、分层加工、增量水位及回填。
- [ ] F：自动特征变换/物化、时点正确性和真实基线漂移监控。
- [ ] G：DeepResearch 证据支持与冲突核查、独立受控写入/ETL。
- [ ] H：真实页面、数据库、API、外部引擎及故障恢复综合验收。

B → C → D 先形成可交付闭环；E/F/G 在契约稳定后分模块并行。Hive/BigQuery 等环境依赖项仍属于第 2 组，未验收前不宣布整组完成。

### 阶段八：后续研究与生产化 ⏸

- [ ] Agentic RL、奖励与策略优化、模型训练/微调和部署回路：后续单独调研。
- [ ] K8s/Helm、预构建镜像发布、扩缩容及完整运维体系：按部署目标单独规划。

---

## 开发规范

- [全局开发规范](CLAUDE.md)：`准备 → 评估 → 开发 → 测试 → Review → 提交`，含 Go / Python 编码约定与数据库迁移规则

---

## 参与贡献 · 欢迎提 PR 🙌

**坦诚地说，Cognida 还远没到"完善"——它更像一个把方向想清楚、把主干搭起来、但还有大量待补细节的进行中项目。** 我们更愿意如实标注"哪里是洞"，也非常欢迎你来一起把它补上。

当前优先补齐的地方：

- **已有模块之间的闭环**：上层任务协调、审核标签消费、数据集版本、评测返工、发布撤回和失效传播。
- **能力扩展**：数仓适配、自动特征物化、真实基线漂移、深研证据核查及专门操作入口。
- **执行隔离**：为自定义代码增加 Sandbox；现有白名单算子和工具权限不等同于操作系统隔离。
- **证据与文档**：保持实现状态、支持矩阵和真实验收一致，不把模拟、跳过或基础实现描述为全量支持。
- **后续工程**：RL/训练及完整生产化延后，不混入本轮业务闭环交付。

怎么参与：

1. 先看 [路线图](#路线图) 和 [规划中 🚧](#规划中-)，认领一块你感兴趣的；
2. 单人开发阶段直接在 `main` 上迭代——欢迎 **Issue 讨论方向**、**PR 补实现 / 修 bug / 加测试 / 补文档 / 改进部署**；
3. 遵循 [开发规范](CLAUDE.md)：`准备 → 评估 → 开发 → 测试 → Review → 提交`，核心逻辑请带上测试；
4. 拿不准要不要做？记住设计原则——**一个能力要能落在"数据/知识 × 生命周期"这张网上、且能和相邻工位交换产物，才值得做**。

不确定从哪下手也没关系，开个 Issue 聊聊就行。任何让这个环"多转一格"的贡献都受欢迎。🚀

---

## 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件

---

<div align="center">

**让数据与知识具备智能 · Build Intelligence into Data & Knowledge**

</div>

## 数据生命周期首版

已补齐资产目录与血缘、可组合数据专家与流水线、语义建模、工作空间切换、站内报告看护、标注打标、特征与反馈审核入口。运行与界面说明见 [Web功能说明](apps/cognida-web/FEATURES.md)、[生命周期运行手册](services/cognida-go/internal/service/lifecycle/README.md)、[采集运行手册](services/cognida-go/internal/service/collection/README.md)、[Python计算服务](services/cognida-python/LIFECYCLE.md)。上线前按 [迁移手册](services/cognida-go/migrations/README.md) 应用版本化迁移；未配置的外部能力会明确失败。

新模块后端配置、权限与回滚说明见 [Go 功能运行手册](services/cognida-go/FEATURES.md)。

当前更新后的功能边界与下一阶段交付顺序，以本文能力全景和[业务闭环与能力补全设计](docs/openspec/changes/complete-governed-data-loop/design.md)为准；历史方案和截图不代表所有规划能力已启用。
