# Link

<div align="center">

**AI-Native 智能 Data 平台**

让每个组织都能构建自主进化的数据大脑

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Python Version](https://img.shields.io/badge/Python-3.10+-3776AB?style=flat&logo=python)](https://www.python.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

</div>

---

## 项目简介

**Link** 是一个 **AI-Native 智能 Data 系统**，采用 Go + Python 异构多服务架构，提供数据全生命周期管理能力。

### 系统定位

Link 不是简单的"数据助手"，而是新一代企业级 **AI 数据专家**。我们通过"**大模型 + 领域知识引擎 + 工具链**"的架构，让 AI 具备主动思考、深度分析、输出行动建议的能力，实现从"数据助手"到"业务伙伴"的跃迁。


### 核心定位

| 传统数据工具 | Link AI 数据专家 |
|-------------|-----------------|
| 被动响应指令 | 主动思考拆解任务 |
| 生成图表报表 | 输出决策建议 |
| 描述"发生了什么" | 回答"如何行动" |
| 工具属性 | **业务伙伴属性** |

### 核心价值

- **数据 + 知识融合**：将隐性知识转化为显性知识，沉淀为企业资产
- **洞察到行动闭环**：从数据分析延伸到业务决策建议
- **人机协作进化**：AI 处理确定性，人类专注创造性
- **全生命周期管理**：从数据收集到价值应用的完整链路

---

## 核心特性

### 已实现功能 ✅

#### Agent 系统
- **ReAct 编排**：推理 + 行动循环模式
- **DeepResearch**：深度研究模式，多步推理与验证
- **Multi-Agent 协作**：任务分解、智能分发、结果聚合
- **Agent Hooks**：
  - **数据结论生成**：自动分析数据并生成结论建议
  - **意图澄清**：模糊问题智能澄清
  - **反思机制**：基于 Hook 的反思与自我修正
- **流式响应**：支持 SSE 实时输出
- **协作编排**：Sequential、Parallel、Loop、Conditional 模式
- **Supervisor**：多 Agent 协调与任务分发

#### 工具系统 (Tools)
- **数据查询类**：SQL 查询生成、数据查询执行
- **检索类**：RAG 查询、图谱查询、知识库查询
- **分析类**：交叉验证、数据分析
- **外部类**：网页搜索、URL 内容获取
- **工具注册**：统一工具注册表，支持动态扩展

#### Text2SQL 系统
- **自然语言转 SQL**：理解查询意图，生成对应 SQL
- **Schema 感知**：自动获取数据库表结构
- **多轮对话**：支持追问、排序、过滤等交互
- **安全执行**：只允许 SELECT 查询，自动 LIMIT
- **Text2SQL Agent**：专门的数据库查询 Agent (`agent-text2sql-001`)

#### Python Skill 集成
- **MCP 协议**：支持 Model Context Protocol
- **Skill 调用**：Agent 可调用 Python 实现的动态能力
- **工具集成**：`skill_invoke`、`skill_list` 工具
- **缓存机制**：Skill 列表缓存（TTL）
- **重连机制**：指数退避重试
- **健康检查**：连接状态监控

#### Memory 系统
- **长期记忆**：Agent 执行经验、知识积累
- **记忆管理**：记忆存储、检索、遗忘机制
- **向量检索**：基于 Milvus 的语义记忆检索
- **用户偏好**：保存用户个性化偏好

#### RAG 检索系统
- **向量检索**：基于 Milvus 的语义向量检索
- **BM25 全文检索**：支持关键词精准匹配
- **混合检索**：向量 + 全文融合，自动重排序
- **多路召回**：支持自定义检索策略组合
- **跨库检索**：支持同时检索多个知识库

#### 知识图谱
- **Neo4j 存储**：高性能图数据库支持
- **实体关系抽取**：从文本自动抽取知识三元组
- **图谱检索**：基于关系的多跳查询
- **图谱可视化**：直观展示知识网络

#### 评测系统 (Python)
- **检索评测**：Recall@K、MRR、NDCG 等指标
- **生成评测**：BLEU、ROUGE 等生成质量指标
- **LLM 评测**：基于大模型的智能评分
- **评测策略**：零样本、数据驱动、集成评测
- **自定义指标**：支持扩展评测维度

#### 质量服务 (Quality Service)
- **质量维度**：完整性、一致性、准确性、有效性、唯一性
- **数据清洗**：去噪、去重、格式转换
- **非结构化质量**：可读性、信息密度、语言质量
- **质量流水线**：可配置的质量检查流程
- **数据漂移检测**：监控数据分布变化

#### 知识库管理
- **多格式支持**：PDF、Word、Excel、Markdown、TXT
- **智能分块**：语义分块、固定分块多种策略
- **向量化**：支持多种 Embedding 模型
- **OCR 识别**：图片/扫描件文字提取

#### 多租户
- **租户隔离**：数据与权限完全隔离
- **权限管理**：细粒度的 RBAC 权限控制
- **用户管理**：用户注册、登录、个人资料
- **审计日志**：操作审计与追溯

### 规划中 🚧

| 模块 | 功能 | 优先级 |
|------|------|--------|
| **Agent 核心能力** | 洞察报告(GenUI) | P0 |
| **Agentic RL** | Agent 强化学习、自主优化、策略迭代 | P1 |
| **AI 数据能力** | 数据收集、数据标注、智能打标、特征存储 | P1 |
| **AI 原生能力** | 数据自描述、自适应处理、模型数据闭环、自主学习 | P2 |

**注**：Multi-Agent 协作、会话洞察、反思机制已完成 ✅

详见 [功能规划文档](docs/feature-roadmap.md)

---

<details>
<summary><b>🔧 架构优化方向</b></summary>

以下为长期架构演进方向，暂无明确排期：

| 优化项 | 说明 | 触发条件 |
|--------|------|----------|
| **事件驱动通信** | Agent 间异步事件通信，基于 Redis MQ | Agent 跨机器部署或需要异步处理时 |
| **服务发现** | 动态 Agent 注册与发现，支持水平扩展 | 多实例部署场景 |
| **A2A 通信协议** | 标准化 Agent 间消息格式 | 需要跨系统互操作时 |
| **MCP 协议** | 支持 Model Context Protocol | ✅ 已实现 |

**优先原则**：先解决实际业务问题，架构演进是渐进式的。

</details>

---

## 技术架构

### 整体架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              API Gateway                                     │
│                         (REST / gRPC / SSE)                                  │
└─────────────────────────────────────────────────────────────────────────────┘
│
┌───────────────────────┴───────────────────────┐
▼                                               ▼
┌───────────────────────────────────┐   ┌───────────────────────────────────┐
│          Go Services               │   │          Python Services         │
│     (API / 编排 / 实时)             │   │     (AI/ML / 重计算 / gRPC)     │
├───────────────────────────────────┤   ├───────────────────────────────────┤
│  ┌─────────────────────────────┐  │   │  ┌─────────────────────────────┐  │
│  │  Agent Service              │  │   │  │  gRPC Services             │  │
│  │  ├─ ReAct/DeepResearch      │  │   │  │  ├─ Document (50051)       │  │
│  │  ├─ Multi-Agent 协作        │  │   │  │  ├─ Evaluation (50052)      │  │
│  │  ├─ 工具调用 (Tools)         │  │   │  │  ├─ Quality (50053)         │  │
│  │  ├─ 协作编排 (Orchestration) │  │   │  │  └─ ML/Annotation (TODO)    │  │
│  │  └─ Agent Hooks             │  │   │  └─────────────────────────────┘  │
│  │    ├─ 数据结论生成          │  │   │                                   │
│  │    ├─ 意图澄清              │  │   │  ┌─────────────────────────────┐  │
│  │    └─ 反思机制              │  │   │  │  MCP Server                 │  │
│  └─────────────────────────────┘  │   │  │  ├─ Skill Manager           │  │
│                                   │   │  │  ├─ Data Analysis           │  │
│  ┌─────────────────────────────┐  │   │  │  ├─ ML Models               │  │
│  │  Text2SQL Service           │  │   │  │  └─ Custom Skills           │  │
│  │  ├─ Text2SQL Agent          │  │   │  └─────────────────────────────┘  │
│  │  ├─ Schema 感知             │  │   │                                   │
│  │  └─ SQL 安全执行            │  │   │  ┌─────────────────────────────┐  │
│  └─────────────────────────────┘  │   │  │  Document Service           │  │
│                                   │   │  │  ├─ 文档解析 (PDF/Word)     │  │
│  ┌─────────────────────────────┐  │   │  │  ├─ OCR 识别                │  │
│  │  Skill Service (MCP Client) │  │   │  │  ├─ 文本分块                │  │
│  │  ├─ skill_invoke           │  │   │  │  └─ URL 内容获取            │  │
│  │  ├─ skill_list             │  │   │  └─────────────────────────────┘  │
│  │  └─ 缓存/健康检查           │  │   │                                   │
│  └─────────────────────────────┘  │   │  ┌─────────────────────────────┐  │
│                                   │   │  │  Evaluation Service         │  │
│  ┌─────────────────────────────┐  │   │  │  ├─ 检索评测                │  │
│  │  Memory Service             │  │   │  │  ├─ 生成评测                │  │
│  │  ├─ 长期记忆                │  │   │  │  ├─ LLM 评测                │  │
│  │  ├─ 记忆管理                │  │   │  │  └─ 自定义指标              │  │
│  │  └─ 向量检索                │  │   │  └─────────────────────────────┘  │
│  └─────────────────────────────┘  │   │                                   │
│                                   │   │  ┌─────────────────────────────┐  │
│  ┌─────────────────────────────┐  │   │  │  Quality Service             │  │
│  │  Tools System               │  │   │  │  ├─ 质量检查                │  │
│  │  ├─ SQL 查询/执行           │  │   │  │  ├─ 数据清洗                │  │
│  │  ├─ RAG 查询                │  │   │  │  ├─ 异常检测                │  │
│  │  ├─ 图谱查询                │  │   │  │  ├─ 完整性/一致性          │  │
│  │  ├─ 网页搜索                │  │   │  │  └─ 数据漂移检测            │  │
│  │  ├─ Skill 调用              │  │   │  └─────────────────────────────┘  │
│  │  └─ 知识库查询              │  │   │                                   │
│  └─────────────────────────────┘  │   │                                   │
│  ┌─────────────────────────────┐  │   │  ┌─────────────────────────────┐  │
│  │  RAG Service                │  │   │  │  ML Service (TODO)          │  │
│  │  ├─ 向量检索 (Milvus)       │  │   │  │  ├─ 特征工程                │  │
│  │  ├─ BM25 全文检索           │  │   │  │  ├─ 模型训练/推理           │  │
│  │  └─ 混合检索                │  │   │  │  └─ 模型评估                │  │
│  └─────────────────────────────┘  │   │  └─────────────────────────────┘  │
│                                   │   │                                   │
│  ┌─────────────────────────────┐  │   │  ┌─────────────────────────────┐  │
│  │  Knowledge Service          │  │   │  │  Annotation Service (TODO)  │  │
│  │  ├─ 知识库管理              │  │   │  │  └─ 数据标注                 │  │
│  │  ├─ 文档向量化              │  │   │  └─────────────────────────────┘  │
│  │  └─ 智能分块                │  │   │                                   │
│  └─────────────────────────────┘  │   │  ┌─────────────────────────────┐  │
│                                   │   │  │  Metadata Service (TODO)    │  │
│  ┌─────────────────────────────┐  │   │  │  └─ 元数据提取               │  │
│  │  Graph Service              │  │   │  └─────────────────────────────┘  │
│  │  ├─ 图谱存储 (Neo4j)        │  │   │                                   │
│  │  ├─ 实体关系抽取            │  │   │                                   │
│  │  └─ 图谱检索                │  │   │                                   │
│  └─────────────────────────────┘  │   │                                   │
│                                   │   │                                   │
│  ┌─────────────────────────────┐  │   │                                   │
│  │  Chat/Session Service       │  │   │                                   │
│  │  ├─ 会话管理                │  │   │                                   │
│  │  ├─ 消息历史                │  │   │                                   │
│  │  └─ 流式响应                │  │   │                                   │
│  └─────────────────────────────┘  │   │                                   │
│                                   │   │                                   │
│  ┌─────────────────────────────┐  │   │                                   │
│  │  LLM Service                │  │   │                                   │
│  │  ├─ 聊天模型                │  │   │                                   │
│  │  ├─ 向量模型                │  │   │                                   │
│  │  └─ 重排序模型              │  │   │                                   │
│  └─────────────────────────────┘  │   │                                   │
│                                   │   │                                   │
│  ┌─────────────────────────────┐  │   │                                   │
│  │  User/Tenant Service        │  │   │                                   │
│  │  ├─ 用户管理                │  │   │                                   │
│  │  ├─ 租户管理                │  │   │                                   │
│  │  └─ 权限控制 (RBAC)         │  │   │                                   │
│  └─────────────────────────────┘  │   │                                   │
└───────────────────────────────────┘   └───────────────────────────────────┘
│
┌───────────────────────┴───────────────────────┐
▼
┌───────────────────────────────────────┐
│         Message Queue (Redis)         │
│         ┌─────────┐  ┌─────────┐      │
│         │  任务队列 │  │  缓存    │      │
│         └─────────┘  └─────────┘      │
└───────────────────────────────────────┘
│
┌───────────────┼───────────────┬───────────────┐
▼               ▼               ▼
┌───────────┐   ┌───────────┐   ┌───────────┐   ┌───────────┐
│  MySQL    │   │  Milvus   │   │  Neo4j    │   │  Redis    │
│ (元数据)  │   │ (向量/特征)│   │  (图谱)   │   │ (缓存/队列)│
└───────────┘   └───────────┘   └───────────┘   └───────────┘
```

### Agent 工具系统

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Agent Tools System                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐       │
│  │ 数据查询类   │  │ 检索类      │  │ 分析类      │  │ 外部类      │       │
│  ├─────────────┤  ├─────────────┤  ├─────────────┤  ├─────────────┤       │
│  │ sql_query  │  │ rag_query   │  │ data_query  │  │ web_search  │       │
│  │ sql_generator│ │ graph_query │  │ cross_validator│ │           │       │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘       │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Tool Registry (工具注册表)                         │   │
│  │  统一管理所有工具的注册、发现、调用                                     │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Memory 系统

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Memory System                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         长期记忆 (Long-term Memory)                    │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │   │
│  │  │ 执行经验    │  │ 知识积累    │  │ 用户偏好    │  │ 上下文记忆  │  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                      │                                      │
│                                      ▼                                      │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         Memory Store (向量存储)                        │   │
│  │                    基于 Milvus 的语义向量检索                          │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Orchestration 编排系统

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Agent Orchestration System                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐  │
│  │ Sequential  │    │  Parallel   │    │  Loop       │    │ Conditional  │  │
│  │  顺序执行    │    │  并行执行    │    │  循环执行    │    │  条件分支    │  │
│  └─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘  │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         Supervisor (监督者)                            │   │
│  │              协调多个 Agent，处理复杂任务的分发与聚合                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```



### 数据流架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              数据流架构                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────────┐              │
│  │  数据源      │ ───▶ │  数据收集    │ ───▶ │  数据处理    │              │
│  │  (Web/API/DB)│      │  (Agent驱动) │      │  (清洗/标注) │              │
│  └──────────────┘      └──────────────┘      └──────┬───────┘              │
│                                                        │                     │
│                                                        ▼                     │
│  ┌──────────────┐      ┌──────────────┐      ┌──────────────┐              │
│  │  数据应用    │ ◀─── │  数据服务    │ ◀─── │  数据存储    │              │
│  │  (分析/决策) │      │  (检索/推理) │      │  (KB/Graph)  │              │
│  └──────────────┘      └──────────────┘      └──────────────┘              │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 服务通信方式

### Go → Python (gRPC)

| Go 端口 | Python 服务 | Proto 文件 | 状态 | 用途 |
|---------|------------|-----------|------|------|
| 50051 | Document Service | docreader.proto | ✅ | 文档解析、OCR、分块、URL抓取 |
| 50052 | Evaluation Service | evaluation.proto | ✅ | 检索评测、生成评测、LLM评测 |
| 50053 | Quality Service | quality.proto | ✅ | 质量检查、数据清洗、异常检测 |
| 50054 | ML Service | ml.proto | 📋 | ML 模型推理 |
| 50055 | Annotation Service | annotation.proto | 📋 | 数据标注 |

### Proto 文件管理

- **单一数据源**：`link-go/api/proto/*.proto`
- **同步机制**：Python 通过 `python scripts/generate_grpc.py` 生成代码
- **变更流程**：修改 proto → 更新 Go 代码 → 同步到 Python

### Go → Python (MCP)

| 协议 | 传输层 | 状态 | 用途 |
|------|--------|------|------|
| MCP | HTTP / stdio | ✅ | Agent 调用 Python Skills |
| Tools | skill_invoke, skill_list | ✅ | Skill 执行与列表查询 |

**MCP 架构**：
- Go 端：MCP Client (`internal/infrastructure/skill`)
- Python 端：MCP Server (`link-python/services/mcp`)
- 通信：JSON-RPC 2.0 over HTTP/stdio

---

## 应用场景

### 核心场景

| 场景 | 说明 | Link 能力 |
|------|------|----------|
| **智能数据分析** | 自然语言查询数据，获得可视化图表和结论 | NL2SQL ✅ · Tools ✅ · 结论生成 ✅ |
| **深度研究分析** | 复杂问题的多步推理，生成结构化报告 | DeepResearch ✅ · 多源整合 ✅ · Memory ✅ |
| **自动化数据收集** | Agent 自动发现、收集、清洗并沉淀数据 | Agent 驱动 ✅ · Quality ✅ |
| **知识管理应用** | 企业知识沉淀、检索与应用 | 知识图谱 ✅ · RAG ✅ |
| **数据驱动决策** | 基于数据分析，输出可落地建议 | Agent 决策 ✅ · Orchestration ✅ |

### 行业解决方案

<details>
<summary><b>🛒 零售行业</b></summary>

| 场景 | 解决方案 | 价值 |
|------|---------|------|
| 销售分析 | 自然语言查询 + 自动洞察 | 效率提升 80% |
| 选品决策 | 历史数据 + 市场分析 | 库存积压降低 30% |
| 会员运营 | 知识图谱 + 交易数据 | 复购率提升 25% |
| 竞品数据收集 | 自动采集价格/促销/新品 | 人工投入减少 90% |
| 用户反馈收集 | 多渠道评论自动收集汇总 | 响应速度提升 10 倍 |

</details>

<details>
<summary><b>💰 金融行业</b></summary>

| 场景 | 解决方案 | 价值 |
|------|---------|------|
| 智能投研 | 自动研读公告研报 | 研究效率提升 10 倍 |
| 行业资讯沉淀 | 自动收集新闻/政策/动态 | 持续积累数据资产 |
| 风险预警 | 实时监控 + 智能识别 | 风险发现提前 2-3 天 |
| 智能客服 | 知识库 + 业务系统 | 客服人力节省 60% |
| 合规分析 | 自动检测 + 报告生成 | 合规成本降低 40% |

</details>

<details>
<summary><b>🏭 制造行业</b></summary>

| 场景 | 解决方案 | 价值 |
|------|---------|------|
| 设备健康 | IoT 数据 + 知识图谱 | 停机时间减少 50% |
| 生产优化 | 历史数据 + 实时数据 | 生产效率提升 20% |
| 质量追溯 | 知识图谱根因分析 | 追溯时间缩短 70% |
| 供应链协同 | Agent 协作协调 | 缺货率降低 30% |

</details>

<details>
<summary><b>🏥 医疗行业</b></summary>

| 场景 | 解决方案 | 价值 |
|------|---------|------|
| 医学知识助手 | 整合指南文献 | 查询效率提升 5 倍 |
| 科研分析 | 自动分析临床数据 | 科研周期缩短 40% |
| 患者咨询 | 智能问答 | 响应速度提升 10 倍 |
| 临床决策 | 指南 + 患者数据 | 决策准确性提升 |

</details>

### 用户角色

| 角色 | 核心需求 | 典型问题 |
|------|---------|---------|
| **管理层** | 快速了解业务、获取决策建议 | "本月业务健康度如何？有什么需要关注？" |
| **业务分析** | 灵活探索、深度归因 | "分析 A 渠道 ROI 下降的原因" |
| **业务运营** | 快速获取信息、行动建议 | "哪些客户有流失风险？给出挽回建议" |
| **IT/数据** | 数据管理、质量监控 | "报表 X 的数据来源是哪里？" |

详见 [应用场景与落地背景](docs/implementation-background.md)

---

## 能力分级

| 级别 | 定义 | 能力 | 状态 |
|------|------|------|------|
| **L1** | 模板化输出 | 根据预设模板生成图表、报表 | ✅ 已实现 |
| **L2** | 自然语言交互 | 通过对话输出分析结论 | ✅ 已实现 |
| **L3** | 主动决策 | 主动拆解任务、规划路径、验证结果、输出行动建议 | 🚧 开发中 |

---

## 项目结构

Link 采用 **Go + Python** 异构多服务架构：

```

---

## 快速开始

### 环境要求

- **Go**: 1.21+
- **Python**: 3.10+
- **MySQL**: 8.0+
- **Milvus**: 2.6+
- **Neo4j**: 5.0+
- **Redis**: 7.0+

### 使用 Docker Compose（推荐）

```bash
# 克隆仓库
git clone https://github.com/your-org/link.git
cd link

# 启动所有服务（包括依赖）
docker-compose up -d

# 查看日志
docker-compose logs -f

# 访问服务
# API: http://localhost:8080
# Milvus: http://localhost:19530
# Neo4j: http://localhost:7474
```

### 本地开发

三个服务分别启动，建议使用独立终端。启动顺序：**依赖服务 → Go 后端 → Python 服务 → 前端**。

#### 启动依赖服务

```bash
# 启动 MySQL、Milvus、Neo4j、Redis
docker-compose -f docker/docker-deps.yml up -d
```

#### 1. 启动 Go 服务（后端 · 端口 8080）

入口：`link-go/cmd/server/main.go`

```bash
cd link-go

# 安装依赖
make deps                       # 等价于 go mod download && go mod tidy

# 开发模式启动
make run                        # 等价于 go run ./cmd/server/main.go

# 或生产模式（编译后运行）
make build && ./bin/link
```

监听 `0.0.0.0:8080`（可用环境变量 `SERVER_PORT` 覆盖）。

#### 2. 启动 Python 服务（gRPC / MCP · 端口 50051 / 3000）

```bash
cd link-python

# 安装依赖（推荐安装全部功能）
pip install -e ".[all]"         # 或 uv pip install -e ".[all]"

# 启动 gRPC 服务（主服务，端口 50051）
python -m grpc_service.server   # 或 console script: link-python-grpc

# 启动 MCP 服务（可选，AI 工具调用）
python -m mcp.server            # 或 console script: link-python-mcp
```

- gRPC：默认 `50051`（环境变量 `GRPC_PORT`）
- MCP：默认 `stdio` 模式；HTTP 模式监听 `3000`（`MCP_MODE=http`、`MCP_PORT`）

#### 3. 启动前端（Vue 3 + Vite · 端口 5173）

```bash
cd link-web

# 安装依赖
npm install

# 开发模式启动
npm run dev

# 生产构建 / 预览
npm run build
npm run preview
```

浏览器访问 `http://localhost:5173`，Vite 已配置将 `/api` 代理到 Go 后端 `http://localhost:8080`。

#### 端口一览

| 服务 | 端口 | 说明 |
|------|------|------|
| link-go（后端） | 8080 | REST / gRPC / SSE 网关 |
| link-python（gRPC） | 50051 | Document 服务（另有 50052 Evaluation、50053 Quality） |
| link-python（MCP） | 3000 | HTTP 模式下的 MCP 服务（默认 stdio） |
| link-web（前端） | 5173 | Vite 开发服务器 |

**调用链路**：浏览器(:5173) → Go 后端(:8080) → gRPC → Python 服务(:50051)

---

## 使用示例

### RAG 检索

```go
package main

import (
    "context"
    "fmt"
    "link/internal/application/usecases/rag"
)

func main() {
    ctx := context.Background()

    // 创建 RAG 服务
    ragService := rag.NewService(/* 依赖 */)

    // 执行检索
    resp, err := ragService.Search(ctx, &rag.SearchRequest{
        Query:          "什么是知识图谱？",
        KnowledgeBaseID: "kb_001",
        TopK:           5,
        SearchType:     "hybrid", // 混合检索
    })
    if err != nil {
        panic(err)
    }

    for _, doc := range resp.Documents {
        fmt.Printf("Score: %.2f, Content: %s\n", doc.Score, doc.Content)
    }
}
```

### Agent 对话（流式）

```go
package main

import (
    "context"
    "link/internal/application/usecases/agent"
)

func main() {
    ctx := context.Background()

    // 创建 Agent 服务
    agentService := agent.NewService(/* 依赖 */)

    // 流式对话
    stream, err := agentService.ChatStream(ctx, &agent.ChatRequest{
        AgentID: "agent_001",
        Message: "分析最近一周的销售数据，给出结论和建议",
    })
    if err != nil {
        panic(err)
    }

    for chunk := range stream {
        fmt.Print(chunk.Content)
    }
}
```

### Agent Hooks（数据结论生成）

```go
package main

import (
    "context"
    "time"

    "link/internal/domain/agent"
    "link/internal/infrastructure/agent/hooks"
    "link/internal/infrastructure/llm/chat"
)

func main() {
    ctx := context.Background()

    // 创建 LLM 客户端
    llm := chat.NewClient(/* 配置 */)

    // 配置结论生成 Hook
    conclusionGen := hooks.NewConclusionGenerator(llm).
        Enable().
        AddDataTools("sql_query", "data_query").
        WithTimeout(30 * time.Second)

    // 通过 Builder 配置 Agent
    builder := agent.New(llm).
        Name("数据分析 Agent").
        WithConclusion(conclusionGen)

    ag, err := builder.Build(ctx)
    if err != nil {
        panic(err)
    }

    // Agent 响应会自动包含数据结论
    resp, err := ag.Chat(ctx, "分析最近一周的销售数据")
    if err != nil {
        panic(err)
    }

    // resp.Metadata["conclusion"] 包含结构化结论
    fmt.Println(resp.Metadata["conclusion"])
}
```

### Agent Hooks（意图澄清）

```go
package main

import (
    "context"
    "link/internal/domain/agent"
    "link/internal/infrastructure/agent/hooks"
)

func main() {
    ctx := context.Background()

    // 配置意图澄清 Hook
    clarifier := hooks.NewIntentClarifier(llm).
        Enable().
        WithMaxRounds(2).
        WithBusinessContext("销售分析")

    // 通过 Builder 配置 Agent
    builder := agent.New(llm).
        Name("销售分析 Agent").
        WithClarification(clarifier)

    ag, err := builder.Build(ctx)
    if err != nil {
        panic(err)
    }

    // 用户查询模糊时自动澄清
    resp, err := ag.Chat(ctx, "分析销售数据")
    if err != nil {
        // 返回 ClarificationNeededError，包含澄清问题
        if clarErr, ok := err.(*agent.ClarificationNeededError); ok {
            fmt.Printf("需要澄清：%v\n", clarErr.Questions)
        }
    }
}
```

### 从配置文件创建 Agent

```go
package main

import (
    "context"
    "link/internal/domain/agent"
)

func main() {
    ctx := context.Background()

    config := &agent.AgentConfig{
        MaxIterations: 10,
        HookConfig: &agent.HookConfig{
            EnableConclusion: true,
            DataTools:        []string{"sql_query", "data_query"},
            Timeout:          30,
            EnableClarification: true,
            BusinessContext:      "销售数据分析",
            MaxRounds:            2,
        },
    }

    ag, err := agent.NewAgentFromConfig(chatModel, config)
    if err != nil {
        panic(err)
    }

    // 使用 Agent
    resp, err := ag.Chat(ctx, "分析销售数据")
    // ...
}
```

### 知识图谱查询

```go
package main

import (
    "context"
    "fmt"
    "link/internal/application/usecases/graph"
)

func main() {
    ctx := context.Background()

    // 创建图谱服务
    graphService := graph.NewService(/* 依赖 */)

    // 执行图谱查询
    resp, err := graphService.Query(ctx, &graph.QueryRequest{
        EntityType: "Person",
        Conditions: map[string]interface{}{
            "name": "张三",
        },
        Depth: 2, // 两跳关系
    })
    if err != nil {
        panic(err)
    }

    for _, node := range resp.Nodes {
        fmt.Printf("Entity: %s, Properties: %v\n", node.ID, node.Properties)
    }

    for _, edge := range resp.Edges {
        fmt.Printf("Relation: %s -> %s [%s]\n", edge.From, edge.To, edge.Type)
    }
}
```

### Python 文档处理

```python
from link_python.services.document import DocumentProcessor
from link_python.services.document.chunking import ChunkingStrategy

# 创建文档处理器
processor = DocumentProcessor()

# 处理文档
result = processor.process(
    file_path="document.pdf",
    chunking_strategy=ChunkingStrategy.SEMANTIC,
    chunk_size=500,
    chunk_overlap=50
)

# 获取分块结果
for chunk in result.chunks:
    print(f"Content: {chunk.content}")
    print(f"Metadata: {chunk.metadata}")
```

---

## 开发规范

- [Go 语言规范](link-go/CLAUDE.md) - Clean Architecture、设计模式应用
- [Python 语言规范](link-python/CLAUDE.md) - 分层架构、代码风格
- [全局开发规范](CLAUDE.md) - 公共约定、数据流、存储约定

### 核心设计原则

1. **优先使用设计模式**：根据场景选择合适模式
2. **抽取公共逻辑**：重复代码提取为函数/方法
3. **接口隔离**：定义小而专的接口

### 常用设计模式

| 模式 | 用途 |
|------|------|
| Builder | 构建复杂对象（Agent 配置、Pipeline 定义） |
| Factory | 创建对象（多 Provider 模型） |
| Repository | 封装数据访问 |
| Middleware | 横切关注点（日志、监控） |
| Strategy | 算法族（检索策略、评估策略） |

---

## 路线图

### Phase 1：Agent 核心能力 (2025 Q2-Q3)

- [x] Multi-Agent 协作编排
- [ ] 洞察报告生成（GenUI）
- [x] 意图澄清机制
- [x] 会话洞察（结论生成）
- [x] 反思机制
- [x] Text2SQL Agent
- [x] Python Skill 集成 (MCP)

### Phase 2：AI 数据能力 (2025 Q4-Q1)

- [ ] Agent 驱动的数据收集
- [ ] 数据标注系统
- [ ] 智能打标（非结构化数据）
- [ ] 特征存储服务
- [ ] Agentic RL（Agent 强化学习）

### Phase 3：AI 原生能力 (2026 Q2+)

- [ ] 数据自描述
- [ ] 自适应数据处理
- [ ] 模型-数据闭环
- [ ] 自主学习

---


## 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件

---

## 联系方式

- **文档**: [docs/](docs/)
- **Issue**: [GitHub Issues](https://github.com/your-org/link/issues)
- **讨论**: [GitHub Discussions](https://github.com/your-org/link/discussions)

---

<div align="center">

**让数据具备智能 · Build Intelligence into Data**

</div>
