# Link

<div align="center">

**企业级「数据 + 知识」Agent 平台**

一个受治理、可评测、可安全执行的数据智能体，覆盖数据与知识的全生命周期

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Python Version](https://img.shields.io/badge/Python-3.10+-3776AB?style=flat&logo=python)](https://www.python.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

</div>

---

## 项目简介

**Link** 是一个面向企业的 **「数据 + 知识」Agent 平台**，采用 Go + Python 异构多服务架构。

它用**一个企业级硬化的 Agent 内核**——口径受治理、操作可管控、过程可审计、质量可评测——统一处理：

- **结构化数据**的接入、查询、分析与安全操作
- **非结构化知识**的收集、沉淀、问答与溯源

目标是让 Agent 覆盖数据与知识从「**进来**」到「**被用起来**」再到「**被评估改进**」的全过程。

### 系统定位

> Link 不是「又一个能问数据的 bot」。市面上的 ChatBI 只会查结构化数据、RAG 工具只会答文档，且都停在"能不能答"。Link 补的是**答之后那道信任鸿沟**——让企业**敢**把真实数据和真实决策托付给 Agent，并且**能验证它到底好不好**。

一句话记忆点：

> **别人是「能问数据的助手」，Link 是「敢把数据和知识托付给它、还能度量它好不好的企业级数据智能体」。**

### 核心差异

| | ChatBI | RAG 机器人 | 数据中台 | **Link** |
|---|--------|-----------|---------|----------|
| 结构化数据 | ✅ | ✗ | ✅(底座) | ✅ |
| 非结构化知识 | ✗ | ✅ | 部分 | ✅ |
| 口径受治理(可信 SQL) | ✗ | — | 部分 | ✅ |
| 可安全执行(读/写/ETL 分级) | ✗ | ✗ | ✗ | ✅ |
| 可评测(轨迹级度量) | ✗ | ✗ | ✗ | ✅ |
| 全过程可审计 | ✗ | ✗ | 部分 | ✅ |
| Agent 驱动全生命周期 | ✗ | ✗ | ✗ | ✅(建设中) |

**护城河 = 三者的交集：企业级硬化的 Agent 内核 × 数据/知识双栖 × 全生命周期覆盖。** 单看每一格都是商品，叠加起来才是"组织敢长期托付、还能持续进化"的数据智能体。

---

## 能力全景（数据/知识 × 全生命周期）

Link 的每一项能力都落在一张网上：横轴是 **Agent 驱动的数据/知识生命周期**，纵轴是**处理对象**。✅ 已实现 · 🚧 规划中。

| 生命周期 → | ①收集/接入 | ②清洗/达标 | ③沉淀/资产化 | ④分析/决策 | ⑤评测/进化 |
|-----------|-----------|-----------|-------------|-----------|-----------|
| **结构化数据** | 多数据源接入 ✅<br>数仓接入分析 🚧<br>Agent 自动收集 🚧 | 数据质量中心 ✅<br>数据清洗 ✅<br>数据达标(自动+人工) 🚧 | 指标语义层(治理口径) ✅<br>Result Store ✅<br>数据特征化(自动) 🚧<br>特征存储 🚧 | Data Agent(DaDa) ✅<br>Text2SQL ✅<br>自我修复(错误分级/schema 引导/失败护栏/重规划) ✅<br>数仓分析 🚧<br>A2UI 生成式 UI ✅ | 轨迹级 Agent 评测 ✅<br>数据漂移检测 ✅<br>Agent 自进化/经验沉淀(默认关) ✅<br>进化回路(评测反哺) 🚧 |
| **非结构化知识** | 知识库上传(多格式/OCR) ✅<br>Agent 自动收集 🚧 | 非结构化质量评估 ✅<br>上传去重 ✅<br>数据标注/达标 🚧 | 知识库/智能分块/向量化 ✅<br>知识图谱抽取 ✅<br>Memory 长期记忆 ✅ | 知识库助手(Agentic RAG) ✅<br>图谱检索/多跳 ✅<br>DeepResearch ✅ | RAG 评测(faithfulness 等) ✅<br>检索/生成指标 ✅ |
| **贯穿层（两者共用）** | \ | Agent 内核(ReAct / DeepResearch / 多 Agent 协作 / 编排 / 反思 / Hooks)、安全与治理(scope 门控/写确认/只读加固)、LLM 统一弹性、多租户 RBAC、审计 + request_id + Loki 可观测、Skill/MCP 扩展 | | | / |

> **诚实标注**：结构化侧的「①自动收集」「③特征化」是当前主要的"洞"。⑤的**经验沉淀链路已接线但默认关**（反思 + 会话蒸馏两套并存，见下），真正缺的是「**评测结论自动反哺收集/沉淀策略**」这段回路——把已有工位之间的传送带接上、并默认打开，这个环才算真正**自己转起来**。

---

## 已实现功能 ✅

### Agent 内核

- **ReAct 编排**：推理-行动循环，统一 Eino Tool 框架
- **DeepResearch**：深度研究模式，多步推理与验证，生成结构化报告
- **Multi-Agent 协作**：任务分解、智能分发、结果聚合；委托含「上下文防火墙」+ 委托轨迹穿透（供评测/审计）
- **编排原语**：Sequential / Parallel / Loop / Conditional / Supervisor
- **反思 / 评审（critic）**：可插拔 LLM / 规则评审子系统，答前自检
- **Agent Hooks**：数据结论生成、意图澄清、反思与自我修正
- **流式响应**：SSE 实时输出，结论/图表边推理边下发
- **Agent 自进化（经验沉淀，默认关）**：后台异步、不阻塞对话——**会话经验蒸馏**：空闲会话经 LLM 蒸馏为 SKILL.md + 知识图谱 + 经验库，SKILL.md 经渐进式披露注入后续回答（`EXPERIENCE_DISTILL_ENABLED`）

### 结构化数据：查询 · 分析 · 安全操作

- **Data Agent（DaDa）**：单一 ReAct 内核驱动的 取数 → 分析 → 渲染 → 操作 闭环，子代理委派
- **Text2SQL**：Plan-Execute-Reflect（顺序编排 + 重试），Schema 感知、多轮对话、结果解释
- **指标语义层（NL2Semantics）**：在 LLM 与数仓之间引入受治理的指标语义层，把「裸写 SQL」升级为「按中心化口径生成 SQL」，附覆盖率统计（covered / cache_hit / fallback）
- **多数据源接入**：MySQL / PostgreSQL，凭据 AES-256-GCM 加密、连接池、健康检查、`information_schema` 内省，外部源视为**只读外部资源**
- **A2UI 生成式 UI**：`render_ui` 随流下发 UISpec，图表/表格/结论实时绘制到结果画布；Result Store 中间结果落库、会话重开恢复画布
- **安全执行**：会话按 `read / write / etl` 分级，写操作走危险操作确认卡（二次确认 + 令牌时效）；查询默认只读、自动 LIMIT、关键字黑名单、超时保护
- **自我修复（业务层闭环）**：SQL/取数报错时不止于重试——① **结构化错误分级**（`syntax / unknown_column / unknown_table / permission / timeout / transient`，按 MySQL 错误码，`sql_error.go` `classifySQLError`）；② **schema 线索回注**（列不存在回传该表可用列、表不存在回传近似候选表，经 `information_schema` 现取，`buildSchemaHint`）；③ **重复失败护栏**（按 `tool:error_kind` 计数，触顶提前收尾并转部分结论，`self_repair_guard.go`，防无限失败循环）；④ **动态重规划**（同一失败签名达阈值注入再规划提示，引导换路径而非原样重试）

### 非结构化知识：检索 · 图谱 · 记忆

- **Agentic RAG**：Milvus 语义向量 + BM25 全文双路召回，重排序精排；HyDE / 查询重写 / 多跳检索可组合开启
- **知识库范围强制**：范围经会话入口选定并由 ctx 强制透传，工具层求交拒绝越权
- **知识图谱**：文档入库时 LLM 并发抽取实体关系三元组，Neo4j 存储；图谱检索支撑关系/关联/溯源类问答；图谱提取=库级开关、检索=提问级开关；一键补建、图谱管理与可视化
- **知识库管理**：PDF / Word / Excel / Markdown / TXT，智能分块、向量化、OCR、按 file hash 去重预检
- **Memory 系统**：长期记忆、用户偏好、跨轮对话记忆，基于 Milvus 的语义检索

### 质量 · 评测（可信与度量）

- **数据质量中心**：完整性 / 一致性 / 准确性 / 有效性 / 唯一性打分；非结构化质量（可读性/信息密度/语言质量）；数据清洗（去噪/去重/格式转换）；质量流水线；数据漂移检测
- **评测子系统**：QA / RAG / Agent 三类评测，**Go 编排/执行/存储，Python 计算打分**，grader 注册表为单一事实源
  - 检索指标：Recall@K、Precision@K、MRR、NDCG、MAP
  - 生成指标：BLEU、ROUGE、语义相似度
  - RAG 专项：faithfulness、context_relevance、noise_ratio
  - **Agent 轨迹级**：答案准确性、工具选择、工具顺序、轨迹匹配、步骤效率
  - LLM-as-a-Judge：多维裁判（事实正确性、内容安全性等）
  - 独立 FastAPI 评测服务（默认 :18888），前端指标可视化配置与结果明细一体化

### 平台底座

- **多租户**：数据与权限隔离、RBAC、用户管理
- **可观测性**：request_id 全链路透传、结构化审计（audit_logs）、Loki 日志采集，同 rid 关联
- **LLM 统一弹性**：跨目标有序降级链 + 同目标指数退避重试 + per-target 三态熔断，透明装饰、成功路径零改动
- **Python Skill 集成（MCP）**：Agent 调用 Python 动态能力，`skill_invoke` / `skill_list`，缓存 + 指数退避重连 + 健康检查

---

## 规划中 🚧

围绕"把生命周期闭环接通"展开，而非无限加功能：

| 模块 | 能力 | 落在生命周期 | 优先级 |
|------|------|-------------|--------|
| **Agent 自动收集数据** | Agent 主动从 Web / API / 文件 / 库表发现并采集数据、自动沉淀为知识与数据集 | ①收集/接入 | P1 |
| **数仓分析** | 接入并分析主流数仓（ClickHouse / Doris / StarRocks / Hive / BigQuery 等），作为**消费方**而非自建数仓 | ①接入 + ④分析 | P1 |
| **数据达标（自动 + 人工）** | 自动清洗 + 人工标注/校准的 human-in-the-loop 达标流程，让入库数据可信可用 | ②清洗/达标 | P1 |
| **数据特征化（自动）** | 从达标数据自动抽取特征、沉淀为可复用的特征资产（对接特征存储） | ③沉淀/资产化 | P2 |
| **特征存储** | 离线特征宽表（元数据 + 向量特征），供分析与预测复用 | ③沉淀/资产化 | P2 |
| **进化回路** | 评测结论自动反哺收集/沉淀策略，让"自主进化"从能力变成回路 | ⑤评测/进化 | P2 |
| **Agentic RL** | Agent 强化学习、自主优化、策略迭代 | ⑤评测/进化 | P2 |
| **AI 原生能力** | 数据自描述、自适应处理、模型-数据闭环、自主学习 | 全生命周期 | P3 |

> **设计原则**：一个能力要落在"数据/知识 × 生命周期"这张网上、且能和相邻工位交换产物，才值得做；落不上、不交换的，就是负债。先让一小段环端到端跑通（收集 → 达标 → 沉淀 → 分析 → 评测 → 反哺），再加宽。

---

## 技术架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              API Gateway (REST / gRPC / SSE)                  │
└─────────────────────────────────────────────────────────────────────────────┘
                                     │
        ┌────────────────────────────┴────────────────────────────┐
        ▼                                                          ▼
┌───────────────────────────────────┐        ┌───────────────────────────────────┐
│           Go 主后端                 │        │         Python 计算服务             │
│      (API / 编排 / Agent)          │        │      (AI/ML / 重计算 / gRPC·MCP)   │
├───────────────────────────────────┤        ├───────────────────────────────────┤
│  Agent 内核                         │        │  gRPC / HTTP 服务                   │
│   ├─ ReAct / DeepResearch          │        │   ├─ Document (50051)              │
│   ├─ Multi-Agent 协作 / 编排        │        │   ├─ Quality  (50051)              │
│   ├─ 反思 / Hooks                   │        │   ├─ Analytics(50053)              │
│   └─ 安全与治理(scope/写确认)        │        │   ├─ Evaluation(HTTP 18888)        │
│                                    │        │   ├─ ML / 特征化 (TODO)             │
│  Data Agent (DaDa)                  │        │   └─ 数据收集 / 达标 (TODO)         │
│   ├─ 语义选表 / 指标语义层           │        │                                    │
│   └─ A2UI / Result Store            │        │  MCP Server                        │
│                                    │        │   ├─ Skill Manager                 │
│  知识库助手 (Agentic RAG)            │        │   ├─ Data Analysis 工具            │
│   ├─ 混合检索 / HyDE / 多跳          │        │   └─ Custom Skills                 │
│   └─ 图谱增强 / 跨轮记忆             │        │                                    │
│                                    │        │  Document Service                  │
│  评测编排 · 质量 · 语义建模          │        │   └─ PDF/Word/OCR/分块/URL          │
│  Memory · Chat/Session · LLM 弹性   │        │                                    │
│  User/Tenant · 审计 · 可观测         │        │  Evaluation / Quality / Analytics  │
└───────────────────────────────────┘        └───────────────────────────────────┘
        │                                                          │
        └────────────────────────────┬────────────────────────────┘
                                     ▼
        ┌──────────┬──────────┬──────────┬──────────┐
        │  MySQL   │  Milvus  │  Neo4j   │  Redis   │
        │ (元数据) │(向量/特征)│  (图谱)  │(缓存/队列)│
        └──────────┴──────────┴──────────┴──────────┘
```

**依赖方向**：`handler → service → model ← repository`

### 存储约定

| 存储 | 用途 |
|------|------|
| MySQL | 元数据、配置、任务状态、语义模型、评测结果、特征宽表 |
| Milvus | 向量、语义记忆、特征向量 |
| Neo4j | 知识图谱、实体关系、血缘 |
| Redis | 缓存、任务队列、进度 |

---

## 服务通信

### Go → Python

| 方式 | 端口 | Python 服务 | 状态 | 用途 |
|------|------|------------|------|------|
| gRPC | 50051 | Document Service | ✅ | 文档解析、OCR、分块、URL 抓取 |
| gRPC | 50051 | Quality Service | ✅ | 质量检查、数据清洗、漂移检测 |
| gRPC | 50053 | Analytics Service | ✅ | 趋势、洞察、统计、归因 |
| HTTP | 18888 | Evaluation Service | ✅ | 评测计算（compute-metrics，无状态） |
| MCP  | 3000 / 8080 | Skill / Data Analysis | ✅ | Agent 调用 Python 动态能力 |
| gRPC | 50054 | ML / 特征化 Service | 🚧 | 特征抽取、模型推理 |
| gRPC | 50055 | 数据收集 / 达标 Service | 🚧 | 自动收集、标注达标 |

- **Proto 单一数据源**：`link-go/api/proto/*.proto`，Python 经 `python scripts/generate_grpc.py` 生成
- **MCP**：JSON-RPC 2.0 over HTTP/stdio；Go 端 `internal/infrastructure/skill`，Python 端 `link-python/mcp_service`

---

## 应用场景

| 场景 | 说明 | Link 能力 |
|------|------|----------|
| **可信智能分析** | 自然语言查数，口径受治理、结果可审计、直接出图 | Data Agent · 语义层 · A2UI ✅ |
| **深度研究分析** | 复杂问题多步推理，多源整合生成结构化报告 | DeepResearch · Memory ✅ |
| **知识问答与溯源** | 企业知识沉淀、检索、关系溯源 | Agentic RAG · 知识图谱 ✅ |
| **数据驱动决策** | 基于分析输出可落地行动建议 | Agent 决策 · 结论生成 ✅ |
| **自动数据沉淀** | Agent 自动收集、达标、沉淀为知识与数据集 | 收集 Agent · 达标 · 特征化 🚧 |

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
link/
├── link-go/        # Go 主后端（API / 编排 / Agent / RAG / 图谱 / 语义 / 评测编排）
├── link-python/    # Python 计算服务（文档解析 / 评测 / 质量 / analytics，gRPC + MCP + HTTP）
├── link-web/       # Vue 3 前端（Vite）
├── proto/          # gRPC 契约（buf 管理，单一数据源）
├── deploy/         # 部署与依赖编排（docker-compose、Loki 等）
├── config/         # 配置
├── datasets/       # 演示 / 评测数据集
├── skills/         # Agent Skill 定义
├── openspec/       # OpenSpec 变更提案与规范
└── docs/           # 文档
```

---

## 快速开始

### 环境要求

- **Go** 1.21+ · **Python** 3.10+ · **MySQL** 8.0+ · **Milvus** 2.6+ · **Neo4j** 5.0+ · **Redis** 7.0+

### 使用 Docker Compose（推荐）

```bash
git clone https://github.com/your-org/link.git
cd link
docker-compose up -d
# API: http://localhost:8080 · Milvus: :19530 · Neo4j: :7474
```

### 本地开发

启动顺序：**依赖服务 → Go 后端 → Python 服务 → 前端**。

```bash
# 依赖服务（MySQL / Milvus / Neo4j / Redis）
docker-compose -f docker/docker-deps.yml up -d

# 1. Go 后端（:8080）
cd link-go && make deps && make run

# 2. Python 服务（gRPC :50051 / Analytics :50053 / 评测 HTTP :18888 / MCP :3000）
cd link-python && pip install -e ".[all]"
python -m grpc_service.server          # gRPC 主服务
uv run uvicorn services.evaluation.fastapi_app:app --port 18888   # 评测
python -m mcp_service.server           # MCP（可选）

# 3. 前端（:5173）
cd link-web && npm install && npm run dev
```

| 服务 | 端口 | 说明 |
|------|------|------|
| link-go（后端） | 8080 | REST / gRPC / SSE 网关 |
| link-python gRPC | 50051 / 50053 | Document+Quality / Analytics |
| link-python 评测 | 18888 | FastAPI 评测计算 |
| link-python MCP | 3000 | HTTP 模式 MCP（默认 stdio） |
| link-web（前端） | 5173 | Vite 开发服务器 |

**调用链路**：浏览器(:5173) → Go 后端(:8080) → gRPC/HTTP/MCP → Python 服务

---

## 路线图

### Phase 1：Agent 核心能力 ✅

- [x] Multi-Agent 协作编排 · 生成式 UI（A2UI）· 意图澄清 · 会话洞察 · 反思机制
- [x] Data Agent（DaDa，单一 ReAct 内核）· Python Skill 集成（MCP）
- [x] Data Agent 自我修复（结构化错误分级 / schema 线索引导 / 重复失败护栏 / 动态重规划）
- [x] 外部多数据源接入 · 指标语义层建模入口 + 治理覆盖统计
- [x] 可观测性（request_id 全链路 + 审计 + Loki）· LLM 统一弹性

### Phase 2：数据/知识生命周期接通 🚧

- [ ] **Agent 自动收集数据**（Web / API / 文件 / 库表 → 自动沉淀）
- [ ] **数仓分析**（接入并分析 ClickHouse / Doris / StarRocks / Hive / BigQuery）
- [ ] **数据达标**（自动清洗 + 人工标注/校准，human-in-the-loop）
- [ ] **数据特征化（自动）** + 特征存储

### Phase 3：自主进化闭环

- [x] Agent 自进化：会话经验蒸馏（SKILL.md/图谱/经验库），后台异步、默认关
- [x] 两套自进化链路收敛为一套（退役反思异步链路，统一到会话经验蒸馏）
- [ ] **进化回路**：把评测结论自动反哺收集/沉淀策略（当前缺的一段），并默认打开
- [ ] Agentic RL（Agent 强化学习）
- [ ] 数据自描述 · 自适应处理 · 模型-数据闭环 · 自主学习

---

## 开发规范

- [Go 语言规范](link-go/CLAUDE.md) · [Python 语言规范](link-python/CLAUDE.md) · [全局开发规范](CLAUDE.md)

---

## 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件

---

<div align="center">

**让数据与知识具备智能 · Build Intelligence into Data & Knowledge**

</div>
