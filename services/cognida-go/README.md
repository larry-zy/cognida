# Cognida Go 服务

Cognida 智能数据系统的 Go 服务端，提供 API、编排、实时处理等核心能力。

## 架构概览

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              API Gateway (HTTP/WebSocket)                      │
└─────────────────────────────────────────────────────────────────────────────────┘
                                              │
┌─────────────────────────────────────────────┼─────────────────────────────────────┐
│                                             ▼                                     │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │                          Application Layer                                │  │
│  │  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐              │  │
│  │  │   Chat    │  │   Agent   │  │    RAG    │  │     KB    │  Use Cases  │  │
│  │  │  Service  │  │  Service  │  │  Service  │  │  Service  │              │  │
│  │  └───────────┘  └───────────┘  └───────────┘  └───────────┘              │  │
│  │  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐              │  │
│  │  │Evaluation │  │  Skill    │  │ Text2SQL  │  │ Semantic  │              │  │
│  │  │  Service  │  │  Service  │  │  Service  │  │  Service  │              │  │
│  │  └───────────┘  └───────────┘  └───────────┘  └───────────┘              │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                             │                                     │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │                             Domain Layer                                  │  │
│  │  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐              │  │
│  │  │   Agent   │  │  Document │  │ Knowledge │  │   Skill   │  Entities    │  │
│  │  │   Domain  │  │   Domain  │  │   Base    │  │   Types   │              │  │
│  │  └───────────┘  └───────────┘  └───────────┘  └───────────┘              │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐     │  │
│  │  │          Agent Tools (Eino Tool Framework)                      │     │  │
│  │  │  ┌───────────┐ ┌───────────┐ ┌────────────┐ ┌───────────────┐    │     │  │
│  │  │  │ rag_query │ │sql_execute│ │semantic_qry│ │  skill_invoke │    │     │  │
│  │  │  └───────────┘ └───────────┘ └────────────┘ └───────────────┘    │     │  │
│  │  │  ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────────┐     │     │  │
│  │  │  │graph_query│ │web_search │ │get_schema │ │ ground_terms  │     │     │  │
│  │  │  └───────────┘ └───────────┘ └───────────┘ └───────────────┘     │     │  │
│  │  └─────────────────────────────────────────────────────────────────┘     │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
│                                             │                                     │
│  ┌──────────────────────────────────────────────────────────────────────────┐  │
│  │                         Infrastructure Layer                              │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │  │
│  │  │    MySQL    │  │   Milvus    │  │   Neo4j     │  │   Redis     │Storage│  │
│  │  │  Repository │  │  Repository │  │  Repository │  │  Repository │      │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘     │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │  │
│  │  │    LLM      │  │   gRPC      │  │    MCP      │  │  Datasource │External│ │
│  │  │  Clients    │  │  Clients    │  │   Client    │  │   Pools     │Services│ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘     │  │
│  │                              ┌─────────────┐                            │  │
│  │                              │   Skill     │ ──────────►                │  │
│  │                              │   Client    │  Python MCP Server         │  │
│  │                              └─────────────┘                            │  │
│  └──────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                              │
                    ┌─────────────────────────┼─────────────────────────┐
                    │                         │                         │
                    ▼                         ▼                         ▼
        ┌───────────────────┐   ┌───────────────────┐   ┌───────────────────┐
        │  Python gRPC      │   │  Python MCP /     │   │   Database        │
        │  Services         │   │  评测 FastAPI     │   │   Cluster         │
        │  ┌─────────────┐  │   │  ┌─────────────┐  │   │  ┌─────────────┐  │
        │  │  Document   │  │   │  │ Data Analysis│  │   │  │   MySQL     │  │
        │  │  Service    │  │   │  │ 评测计算     │  │   │  │   Milvus    │  │
        │  │  (PDF/Word) │  │   │  │ Custom Skills│  │   │  │   Neo4j     │  │
        │  └─────────────┘  │   │  └─────────────┘  │   │  └─────────────┘  │
        └───────────────────┘   └───────────────────┘   └───────────────────┘
```

## 目录结构

```
cognida-go/
├── cmd/                    # 应用程序入口（可执行 main）
│   ├── server/            # HTTP 服务器
│   ├── migrate-db/        # 业务表结构版本化迁移（golang-migrate）
│   └── seed/              # 演示/冷启动灌数命令
│       ├── ecommerce/    # 电商演示数据种子
│       ├── semantic/     # 语义层模型种子
│       ├── graph-terms/  # 图谱术语种子
│       └── eval-datasets/ # 评测 golden 数据种子
├── internal/
│   ├── handler/            # 接口层（HTTP handlers + router）
│   ├── service/            # 应用/领域服务（业务逻辑编排）
│   │   ├── agent/          # Agent 框架、预设、编排、工具、语义、术语接地
│   │   ├── evaluation/     # 评测 worker / executor / Python 客户端
│   │   ├── datasource/     # 外部数据源驱动 / 连接池 / 加密 / 健康检查
│   │   └── semantic/       # 语义层建模写入服务
│   ├── model/              # 领域层（实体、接口定义）
│   ├── repository/         # 数据访问实现（mysql / milvus / neo4j / redis）
│   ├── infrastructure/     # 基础设施（LLM 客户端及弹性层、外部依赖）
│   └── wire/               # 依赖注入装配（google/wire 组合根）
├── api/                    # API 定义
│   ├── proto/             # gRPC Proto 文件
│   └── http/              # HTTP API 规范
└── docs/                   # 文档
```

> 依赖方向：`handler → service → model ← repository`。

## 核心功能

### 1. Agent 系统

- **ReAct 编排**：推理-行动循环，统一 Eino Tool 框架
- **Plan-Execute-Reflect**：顺序编排 + 重试（`service/agent/orchestration/sequential.go`）
- **Multi-Agent 协作**：Agent 间委托（delegate）与协作，含「上下文防火墙」+ 委托轨迹穿透（供评测/审计）
- **Deep Research**：深度研究模式（`service/agent/research.go`）
- **反思 / 评审（critic）**：可插拔 LLM / 规则评审子系统（`service/agent/reflection/critic/`），替代早期单一 Hook
- **Data Agent**：单 ReAct 内核 + 查/析/渲/操能力组 + 子 Agent 委托（`service/agent/presets/data_agent`）
- **编排原语**：sequential / parallel / loop / conditional / supervisor（`service/agent/orchestration/`）

#### 内置 Agent

| Spec ID | 名称 | 能力 |
|---------|------|------|
| `default` | 默认助手 | `web_search` `sql_execute` |
| `agent-rag-001` | rag_assistant | `rag_query` `kb_list` `kb_route` `graph_query` |
| `agent-text2sql-per` | Text2SQL（路由别名 `agent-text2sql-001`）| `get_schema` `sql_execute` |
| `agent-chat-001` | chat_assistant | 纯对话（无工具）|
| `agent-data-agent` | Data Agent | 全工具组 |

#### 工具总览（按能力组注册）

| 组 | 工具 |
|----|------|
| sql | `get_schema` `sql_execute` |
| rag | `rag_query` |
| kb | `kb_list` `kb_route` |
| semantic | `semantic_models` `semantic_query` `ground_terms` |
| graph | `graph_query` |
| web | `web_search` `fetch_url` `search_multi` |
| analytics | `data_analysis` |
| render | `render_ui` |
| operation | `sql_mutate` `etl_run` `data_export` |
| skill | `skill_list` `skill_invoke` |

### 2. RAG 系统

- **向量检索**：基于 Milvus 的语义检索
- **图谱检索**：基于 Neo4j 的知识图谱检索
- **混合检索**：向量 + 图谱融合排序
- **文档处理**：支持 PDF、Word、Excel、网页等

### 3. 知识库管理

- **知识库 CRUD**：创建、查询、更新、删除
- **文档管理**：上传、解析、分块
- **版本控制**：文档版本追踪
- **权限控制**：租户隔离

### 4. Text2SQL 系统

专门的自然语言转 SQL 系统，采用 **Plan-Execute-Reflect**（顺序编排 + 重试）而非纯 ReAct，支持多轮对话查询数据库。内置 Agent：`agent-text2sql-per`（保留路由别名 `agent-text2sql-001`）。

| 特性 | 说明 |
|------|------|
| **自然语言转 SQL** | 理解用户查询意图，生成对应 SQL |
| **Schema 感知** | 自动获取库表结构，跨数据源（`get_schema`）|
| **多轮对话** | 支持追问、排序、过滤等交互 |
| **安全执行** | 仅 `SELECT`/`WITH`；关键字黑名单，拒绝注释与多语句；强制 `LIMIT`（默认 100 / 上限 1000）；30s 超时 |
| **结果解释** | 用友好的中文解释查询结果 |
| **多数据源路由** | 显式 `database_id` 或会话默认数据源，详见 [§5 多数据源](#5-多数据源管理) |

#### 工具列表

| 工具名 | 功能 | 参数 |
|--------|------|------|
| `sql_execute` | 执行只读 SQL 查询 | `sql: 查询语句`, `max_rows: 最大行数`, `database_id: 数据源（可选）` |
| `get_schema` | 获取库表结构 | `table_name: 表名（可选）`, `database_id: 数据源（可选）` |

#### 使用示例

```go
// Text2SQL Agent 已在 initializer/init.go 中自动注册（ID: agent-text2sql-per）
agent := agentRegistry.Get("agent-text2sql-per") // 亦可用别名 agent-text2sql-001

response, _ := agent.Chat(ctx, "查询销售额最高的前10个产品") // 简单查询
response, _ = agent.Chat(ctx, "按地区分组统计")             // 追问
response, _ = agent.Chat(ctx, "按销售额降序排列")           // 排序
```

### 5. 多数据源管理

将用户注册的外部查询数据源与应用自身的元数据库彻底分离：外部数据源被视为**只读外部资源**，经原生 `database/sql` 访问，不做 GORM 映射或迁移。

| 能力 | 说明 |
|------|------|
| **引擎支持** | MySQL、PostgreSQL（策略化 `Driver` 注册表，`service/datasource/driver.go`）|
| **凭据安全** | 口令仅以 **AES-256-GCM** 密文存储，API 永不回传；密钥自 `DATASOURCE_SECRET_KEY` 派生，未设置则启动失败 |
| **连接池** | 按 `tenantID:datasourceID` 惰性缓存 `*sql.DB`（保守默认 MaxOpen 4 / MaxIdle 2）；配置变更自动重建、空闲淘汰 |
| **健康检查** | 可选后台探测（`DATASOURCE_HEALTHCHECK_ENABLED`），更新状态/心跳，跳过 `need_credentials` |
| **只读加固** | SQL 校验层（`sql_execute`）+ 运维层专用只读账号（如 `ecommerce_ro`）双重保障 |
| **Schema 内省** | 走 `information_schema` / `pg_catalog`，`get_schema` 跨数据源列表 / 描述表 |

#### 数据源路由（`resolveQueryTarget`）

```
sql_execute / get_schema 的 database_id 解析：
  显式 database_id       ──► ConnectionProvider.Acquire（外部数据源）
  为空                   ──► 会话默认数据源（agentctx / AgentRequest.datasource_id）
  仍为空                 ──► 应用业务库（向后兼容）
  非法 id / 无 provider  ──► 显式报错（绝不静默回落业务库）
```

#### 管理接口 `/api/v1/datasources`

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` / `POST` | `""` | 列表 / 创建 |
| `GET` / `PUT` / `DELETE` | `/:id` | 详情 / 更新 / 删除 |
| `POST` | `/test` | 测试连接（失败返回 200 业务结果，非 500）|
| `GET` | `/:id/tables`、`/:id/tables/:table` | 列表 / 描述表 |

### 6. 语义引擎（治理型语义层 · NL2Semantics）

在 LLM/Text2SQL Agent 与数仓之间引入一层**受治理的指标语义层**：把「LLM 直接裸写 SQL」升级为「引擎依据中心化的指标口径生成 SQL」。全部实现位于 `cognida-go`（Python 端无语义层）。

#### 查询主路径（治理主路）

```
Agent ──► semantic_models（模型目录）
      ──► ground_terms（术语消歧：口语 → 治理口径名）
      ──► semantic_query（结构化「指标 + 维度」请求）
             │
             ▼  metricsql 引擎 Build() → 受治理 SQL + Coverage + database_id
      ──► sql_execute（按 database_id 路由到正确数据源）
```

未覆盖（`covered=false`）时回退到词法路径 `get_schema` + NL2SQL。术语接地两层：模型内同义词倒排索引优先，其次回退 Neo4j 知识图谱（`GraphAdapter` 遍历 `SIMILAR_TO`/`RELATED_TO`/`BELONGS_TO`，把「流水」映射到治理名「营收」）；歧义触发澄清而非猜测。

#### 核心实体（`internal/model/semantic/entity.go`）

| 实体 | 说明 |
|------|------|
| `SemanticModel` | 业务域语义模型，`Version` 作缓存失效锚点，状态 draft/active/deprecated |
| `LogicalTable` | 语义表→物理表映射，携带 `DatabaseID` 数据源绑定 |
| `Dimension` / `Measure` | 可分组维度（含同义词）/ 可聚合度量原子（含默认聚合）|
| `Metric` | 受治理的可复用计算，带 `Caliber`（口径）与同义词 |
| `Relation` / `ModelBundle` | 表间 JOIN 关系 / 引擎装配的聚合视图 |
| `CoverageOutcome` | `covered` / `cache_hit` / `fallback`，`HitRatio=(covered+cache_hit)/total` |

- **模型数据源绑定**：`bundleDatabaseID` 从各逻辑表 `DatabaseID` 派生——全部一致取该 id、全空则回落、冲突则不猜（返回空）；`semantic_query` 回传 `database_id` 并指示 LLM 原样传给 `sql_execute`，使受治理 SQL 命中正确数据源。
- **可观测性**：每次 `semantic_query` 尽力写一条覆盖日志（`agent_semantic_coverage_logs`，按 `request_id` 关联审计/Loki）；`GET /semantic-coverage` 按模型聚合命中率。
- **写入路径**：建模经 REST（`/semantic-models` CRUD + publish/deprecate）与种子命令（`cmd/seed/semantic`、`cmd/seed/graph-terms`）；前端建模 UI 尚未提供。
- **「治理主路」含义**：引擎与实体此前已存在但恒返回 `covered=false`（空壳），因缺三个缝隙——建模写入口、覆盖可观测性、图谱接地数据。补齐三缝隙 + 数据源路由加固后，治理路径可被真实命中、可观测、可接地。

### 7. Agent 评测（轨迹级测评）

面向**运行中的 Agent** 做端到端评测——不只看最终答案，而是对整条**执行轨迹**打分：「选对工具 ≠ 传对参数 ≠ 不绕路」。Go 端负责驱动真实 Agent 并采集轨迹，Python 端负责跨样本聚合打分。

#### 评测维度

| 指标 | 说明 |
|------|------|
| `answer_accuracy` | 最终答案 vs 参考答案的语义相似度（>0.8 记为正确）|
| `tool_selection` | 期望工具集 vs 实际使用工具集的精确率/召回率/`F1` |
| `tool_order` | 期望工具是否为实际调用序列的**有序子序列**（允许中间穿插其它调用）|
| `trajectory_match` | 实际步骤序列 vs 期望步骤：`exact_match` + 语义 `similarity` |
| `step_efficiency` | 实际步数 vs 最优步数：`optimal_ratio = min(opt/act, act/opt)` |
| 运行时指标 | `latency_ms` / `tokens_used` / `llm_calls` / `success_rate`（Go 侧采集，失败亦记录）|

#### 端到端流程

```
EvaluationWorker ──► AgentExecutor.Execute（逐 QA、带超时）
        │                    │
        │                    ▼
        │            agentServiceAdapter ──► AgentInstanceRegistry
        │            （复用前端同款 Agent 实例）
        │                    │  采集 answer + ToolsUsed + Trajectory
        │                    │        + TotalSteps + TokensUsed + LLMCalls
        ▼                    ▼
  ensureAgentGraders（幂等注入 5 个 agent grader）
        │
        ▼  POST /api/v1/evaluation/compute-metrics（Python :18888）
  compute_agent_metrics（跨样本批量聚合）
        │
        ▼  fillMetrics 写回 → augmentRuntimeMetrics 合并运行时指标
```

- **委托轨迹穿透**（`framework/collab_trajectory.go`）：多 Agent 协作时委托边界默认只回传子 Agent 的最终摘要（「上下文防火墙」），会掩盖子 Agent 真实工具调用与 token 消耗。`delegationTrace` 以 `ctx` 侧信道在顶层 `run` 处 `drain` 回注子 Agent 的 `ToolCalls`/tokens/iterations——**仅供评测与审计**，不改变 LLM 所见上下文；并发委托下线程安全。
- Agent grader 在 Python grader 目录注册（`group="agent"`、`eval_types=[AGENT]`）作为单一事实源，前端指标目录据此渲染；批量 `compute_metrics` 走 `_AGENT_GRADERS` 做跨样本聚合，单样本 `/evaluate` 走各 grader 自身的 `_aevaluate`。

### 8. 数据评测优化（评测子系统）

评测子系统对带标注的 QA 数据集做批量评测：**Go 编排/执行/存储，Python 计算打分**，grader 注册表为单一事实源。

#### 评测类型与数据集

| 概念 | 说明 |
|------|------|
| 评测类型 | `agent` / `rag` / `qa`（归一化为 `llm`）/ `llm` |
| 数据集来源 | `file`（文件）/ `database`（库表）|
| 样本标注 | `question` / `reference_answer` / `relevant_pids`（检索标签）/ `context`；Agent 专属 `expected_tools` / `expected_steps` |

#### Grader 目录（Python `graders/builtin/`，按 eval_type 分组）

| 组 | Grader | 打分内容 |
|----|--------|----------|
| retrieval | `precision` `recall` `ndcg` `mrr` `map` | 命中/排序质量 |
| generation | `rouge*` `bleu*` | n-gram 重叠 |
| semantic | `semantic_similarity` `semantic_relevance` | 向量语义相似度 |
| rule | `exact_match` `contains_match` `regex_match` `numeric_match` | 规则匹配 |
| llm | `llm_judge` `llm_factual` `llm_safety` | LLM-as-judge |
| rag | `faithfulness` `context_relevance` `noise_ratio` | 批级 RAG 指标 |
| agent | `answer_accuracy` `tool_selection` `tool_order` `trajectory_match` `step_efficiency` | 轨迹级（见 §7）|

#### Go Worker 架构

- `EvaluationWorker` 单 `workerLoop` goroutine，基于 slot 的并发队列（`AcquireSlot`/`ReleaseSlot`），空队列指数退避（1s→10s），每任务独立 goroutine 执行。
- 单任务流水线：Redis 进度 → 载入任务 → 解析/校验配置 → 载入数据集 → 由 `ExecutorRegistry` 按类型取执行器 → `Execute` 生成答案/轨迹 → `computeMetrics` 调 Python → `saveResults` 批量落库 → 标记完成。进度阶段 `Loading → Generation → Evaluation → Completed/Failed` 经 SSE 推送。
- **动态 `Scores` JSON 列**：任务级 `TaskMetrics` 与每 QA `EvaluationResult` 除固定浮点列外，都有 registry 驱动的 `Scores map[string]float64` JSON 列，新增 grader **无需迁移**即可落库（评测表主流程无 AutoMigrate）。

#### 近期优化要点

- **Agent 评测闭环**：executor 采集完整工具调用轨迹而非仅最终答案；`AgentChatResult` 增加 `ToolsUsed` / `Trajectory` / `TotalSteps` / `TokensUsed` / `LLMCalls`，`latency_ms` 用挂钟测量、失败也记录。
- **grader 自动注入**：类型为 `agent` 时 `ensureAgentGraders` 幂等补齐 agent grader 家族（answer_accuracy/tool_selection/tool_order/step_efficiency；`trajectory_match` 因期望轨迹为最小锚点集会结构性恒 0，改为需显式请求），避免用户只选了 rouge/bleu 就漏采轨迹指标；`augmentRuntimeMetrics`（agent 与 sql 类型均触发）在 Python 填分后合并 Go 侧运行时指标。
- **grader 注册表为单一事实源**：`GET /api/v1/evaluation/graders?eval_type=` 驱动前端指标选择；`compute-metrics` 对未知 grader 返回显式 `unsupported[]` 而非静默忽略。
- **检索 grader 修复**：`map` 原先误调未定义的 `ap_at_k` 导致 `NameError`，现改用 `map_at_k`；`tool_order`（有序子序列）从 `tool_selection`（集合成员）中拆出为独立指标；`step_efficiency`/`trajectory_match` 加固除零/空轨迹边界。

### 9. Python Skill 集成

Go 端已集成 Python Skill 系统，允许 Agent 调用 Python 实现的动态能力。

#### 架构设计

```
┌─────────────────────────────────────────────────────────────┐
│                        Go Agent                              │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ RAG Query   │  │ SQL Query   │  │   Skill Invoke      │ │
│  │   Tool      │  │    Tool     │  │      Tool           │ │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────────────┘ │
│         │                │                 │                 │
│         └────────────────┴─────────────────┘                 │
│                           │                                   │
│                   ┌───────▼────────┐                         │
│                   │  Tool Registry │                         │
│                   └───────┬────────┘                         │
│                           │                                   │
┌───────────────────────────┼───────────────────────────────────┐
│                    MCP Protocol                             │
│                           │                                   │
│                   ┌───────▼────────┐                         │
│                   │  Skill Client  │                         │
│                   │   (Go Side)    │                         │
│                   └───────┬────────┘                         │
└───────────────────────────┼───────────────────────────────────┘
                            │ HTTP/stdio
┌───────────────────────────┼───────────────────────────────────┐
│                    Python MCP Server                          │
│                           │                                   │
│                   ┌───────▼────────┐                         │
│                   │  Skill Manager │                         │
│                   └───────┬────────┘                         │
│                           │                                   │
│         ┌─────────────────┼─────────────────┐                │
│         │                 │                 │                │
│    ┌────▼────┐      ┌────▼────┐      ┌────▼────┐           │
│    │  Data    │      │  ML     │      │  Custom │           │
│    │ Analysis │      │  Model  │      │  Skills │           │
│    └─────────┘      └─────────┘      └─────────┘           │
└─────────────────────────────────────────────────────────────┘
```

#### 技术特性

| 特性 | 说明 |
|------|------|
| **通信协议** | MCP (Model Context Protocol) |
| **传输层** | HTTP / stdio |
| **工具注册** | `skill_invoke`、`skill_list` |
| **缓存机制** | Skill 列表缓存（TTL）|
| **重连机制** | 指数退避重试 |
| **健康检查** | 连接状态监控 |

#### 使用示例

```go
import (
    "link/internal/service/agent/tools"
    "link/internal/infrastructure/skill"
)

// 1. 初始化 Skill Client
skillClient, _ := skill.NewMCPClient(&skill.Config{
    Endpoint: "http://localhost:8080/mcp",
    Timeout:  30 * time.Second,
    CacheTTL: 60 * time.Second,
})

// 2. 注册全局客户端
tools.InitGlobalSkillClient(skillClient)

// 3. 创建 Skill 工具
skillInvokeTool, _ := tools.NewSkillInvokeTool()
skillListTool, _ := tools.NewSkillListTool()

// 4. 注册到 Agent
agent := builder.
    Name("Data Analyst").
    Tools(skillInvokeTool, skillListTool).
    Build()
```

#### Skill 类型

```go
// Skill 来源
type SkillSource string
const (
    SkillSourceBundled SkillSource = "bundled"  // 内置
    SkillSourcePrivate  SkillSource = "private"   // 私有
    SkillSourcePublic   SkillSource = "public"    // 公共
    SkillSourcePlugin   SkillSource = "plugin"    // 插件
)

// 执行模式
type ContextMode string
const (
    ContextModeInline ContextMode = "inline"  // 内联执行
    ContextModeFork   ContextMode = "fork"    // Fork 子 Agent
)
```

### 10. LLM 统一弹性（降级 / 重试 / 熔断）

在领域接口缝隙处以**透明装饰器**为所有 LLM 调用（chat / embedding / rerank）提供统一容错，对上层业务零改动、成功路径逐字节透传。默认开启，可经环境变量调参或整体关闭。

#### 两层容错模型

```
┌──────────────────────────────────────────────────────────────┐
│  外层：跨目标有序降级链（fallback chain）                    │
│  primary ──失败──► backup#1 ──失败──► backup#2 ──► 聚合错误  │
│    │                                                          │
│    ▼ 内层：同目标指数退避 + 全抖动重试                        │
│  attempt0 ─► attempt1 ─► attempt2（至 MaxAttempts）           │
│    │                                                          │
│    ▼ per-target 三态熔断器（closed → open → half-open）       │
│  连续失败达阈值 → open（冷却窗口内快速失败，跳过该目标）      │
│  冷却到期 → half-open 放行有限探测 → 成功则 closed            │
└──────────────────────────────────────────────────────────────┘
```

| 特性 | 说明 |
|------|------|
| **类型化错误分级** | `APIError` + `ErrorClass`：`rate_limited` / `transient` / `terminal` / `canceled` |
| **智能重试** | 仅 `transient`/`rate_limited` 重试；`terminal`/`canceled` 立即停止 |
| **Retry-After 感知** | 解析响应头（秒 / HTTP-date），退避封顶 `RetryAfterCap` |
| **有序降级** | 主目标耗尽后按配置链降级到后备模型 |
| **共享熔断器** | 按 `provider/model` 进程内共享，`terminal`/`canceled` 不计入失败 |
| **流式失败转移** | 首 chunk 前失败可转移；已产出后作为流终止透传，不重放 |
| **可观测性** | 指标计数 + 结构化日志（`request_id` 透传、脱敏），组合观测器 |

#### 降级链装配

`service/chat` 为同租户下 `Enabled` 的同类型模型自动组链：主模型（`IsDefault`）居首、其余稳定排序作为后备；单模型租户退化为「仅重试 + 熔断」。可用工厂选项关闭：

```go
// 默认：环境变量装配弹性
factory := llm.NewModelFactoryWithObservability()

// 显式覆盖配置
factory := llm.NewModelFactory(llm.WithResilience(cfg))

// opt-out：直连底层客户端，零装饰开销
factory := llm.NewModelFactory(llm.WithoutResilience())
```

## Agent 开发指南（新建 · 配置 · 使用）

本节回答三个问题：**这个项目里的 Agent 是怎么组织的、如何从零新建一个、以及如何配置和调用它。**

### 核心前提：Agent 是「代码声明式」的，没有 per-agent 外部配置文件

本项目**不存在**「一个 Agent 一份 YAML/JSON」的配置文件。每个 Agent 都是在代码里通过 `AgentSpec` 声明、用链式 `Builder` 装配出来的。`config.yaml` 与环境变量配置的是**共享依赖**（模型、工具后端、Skill 端点）和**特性开关**，而不是某个 Agent 的行为。改 Agent 行为 = 改代码，不是改配置文件。

### 三层结构

| 层 | 位置 | 职责 |
|----|------|------|
| **Builder** | `service/agent/framework/eino_builder.go` | 链式装配**单个**可运行 Agent（模型 / 工具 / prompt / hook / 预算 / 压缩 / 护栏…） |
| **AgentSpec** | `service/agent/framework/agent_spec.go` | Agent 的**声明**：`ID/Name/Description/Type/ToolNames/Aliases/Metadata` + `Build` 闭包（真正装配逻辑） |
| **Initializer** | `service/agent/initializer/init.go` | 集中**注册**内置 Agent（default / rag / chat / data），把共享依赖注入各 `Build` 闭包 |
| **SpecRegistry** | `service/agent/framework/spec_registry.go` | 注册表：`RegisterSpec` 时**立即** `Build` 出实例入表；`GetInstance(id)` 只查表不重建 |
| **Orchestrator** | `service/agent/framework/registry_orchestrator.go` | 运行时以 `registry.GetInstance` 为 `AgentGetter`，按 ID 路由到实例并调用 |

> `Build` 用闭包而非直接持有 Agent，是为了打破包依赖环：`tools` 包 import `framework`，若 `framework` 反向 import `tools` 会成环，所以「按名取工具再装配」的逻辑必须留在 initializer/preset 层。

### 装配链路（从进程启动到可被调用）

```
config.yaml (//go:embed) + .env
        │
        ▼  cmd/server/main.go 组合根
  ├─ toolModel  = llmchat.NewToolCallingChatModel(app.ChatConfig)      # 模型
  ├─ reg        = tools.NewToolRegistry(ToolDeps{...})                 # 工具（按能力组注册）
  └─ initializer= agentinit.NewInitializer(AgentRegistry, reg, msgRepo)
                    .WithEmbedder(...).WithExperienceRecall(...)...     # 链式注入可选能力
        │
        ▼  initializer.Initialize(ctx, toolModel)
  registerDefaultAgent / registerRAGAgent / registerChatAgent / registerDataAgent
        │  每个 = 声明 AgentSpec → 在 Build 闭包里用 Builder 装配
        ▼
  registry.RegisterSpec(ctx, spec)  ──►  立即 spec.Build(ctx) 得到实例，按 ID + Aliases 入表
        │
        ▼  运行时
  Orchestrator / evaluation ──► registry.GetInstance(agentID) ──► Agent.Chat / Stream
```

### 一、从零新建一个 Agent

假设要新增内置 Agent `agent-foo`。**二选一**：

#### 方式 A：简单 Agent（内联声明，仿 `registerDefaultAgent`）

在 `internal/service/agent/initializer/init.go` 加一个方法：

```go
func (init *Initializer) registerFooAgent(ctx context.Context, chatModel any) error {
    toolModel, ok := chatModel.(model.ToolCallingChatModel)
    if !ok {
        return fmt.Errorf("foo agent 需要 ToolCallingChatModel")
    }

    spec := infraagent.AgentSpec{
        ID:          "agent-foo",
        Name:        "foo_assistant",
        Description: "示例 Agent",
        Type:        agent.AgentTypeNormal, // normal / agentic_rag / deep_research / multi_agent
        Metadata:    map[string]string{"builtin": "true", "version": "1.0.0"},
        Build: func(ctx context.Context) (infraagent.Agent, error) {
            // 1) 按名 / 按能力组从注入的 ToolRegistry 取工具
            var tools []tool.BaseTool
            if t, ok := init.tools.Get("sql_execute"); ok {
                tools = append(tools, t)
            }
            tools = append(tools, init.tools.GetByGroup("skill")...)

            // 2) 用 Builder 装配
            builder := infraagent.New(nil).
                Name("foo_assistant").
                Prompt(skills.AugmentPromptWithCatalog(`你是示例助手……`)).
                WithToolModel(toolModel).
                Before(skills.AutoInjectHook(skills.FallbackInjectThreshold)).
                WithMaxIterations(6)
            if len(tools) > 0 {
                builder = builder.Tools(tools...)
            }

            // 3) 护栏装配（默认恒等，未接线时不改变行为）
            builder = init.guardrail.Apply(builder)
            return builder.Build(ctx)
        },
    }
    return init.registry.RegisterSpec(ctx, spec)
}
```

然后在 `Initialize(...)` 里挂上调用（与 `registerDefaultAgent` 等并列一行）：

```go
if err := init.registerFooAgent(ctx, chatModel); err != nil {
    return err
}
```

#### 方式 B：复杂 Agent（独立预设包，仿 `presets/data_agent`）

当 Agent 需要子代理委托、意图路由、多 hook、三级上下文压缩、软/硬缓存时，抽成独立包 `internal/service/agent/presets/foo/`，对外只导出一个 `Spec(...) infraagent.AgentSpec` 工厂 + 内部 `buildFooAgent(...)`。initializer 里只需一行：

```go
if err := init.registry.RegisterSpec(ctx, foo.Spec(toolModel, init.messageRepo, init.tools, /* …可选能力… */)); err != nil {
    return err
}
```

`data_agent` 即此范式：`Spec()` 返回声明，`buildDataAgent()` 里建协作注册表 + 注册子代理（`RegisterDataSubAgents`）+ 按能力组收集工具（`collectCapabilityTools` 用 `registry.GetByGroup`）+ 挂多个 `Before` hook + `WithCollaboration(reg, EnableDelegate())` + 缓存装饰。

#### 需要改哪些文件、按什么顺序

| 步骤 | 文件 | 说明 |
|------|------|------|
| 1（可选）新增工具 | `tools/*.go` + `tools/init.go` + `tools/registry.go` + `tools/deps.go` | 实现 eino `tool.BaseTool` → 在某 `registerXxxTools` 里 `r.Register("<group>", tool)` → 新分组要登记进 `registry.go` 的 registrars 列表 → 新依赖加进 `ToolDeps` 并在 `main.go` 装配处填入。已有工具够用则跳过 |
| 2 声明并注册 Spec | `initializer/init.go`（简单）**或** `presets/foo/`（复杂） | 见方式 A / B |
| 3 挂上调用 | `initializer/init.go` 的 `Initialize` | 加 `init.registerFooAgent(...)` 或 `RegisterSpec(foo.Spec(...))` |
| 4（可选）新增注入依赖 | `initializer/init.go` + `cmd/server/main.go` | `Initializer` 加字段 + `WithXxx` 链式方法（仿 `WithEmbedder`），组合根 `.WithXxx(...)` 注入 |
| — 无需改动 | `framework/spec_registry.go`、`agent_spec.go`、`registry_orchestrator.go`、`eino_agent.go`、`eino_builder.go` | 稳定基建。`GetInstance` 自动能取到新 Agent；要兼容历史 ID 就在 Spec 填 `Aliases`，无需改编排器 |

### 二、Builder 装配项一览

一个 Agent 能做什么，取决于 `Builder` 链上挂了哪些方法（`eino_builder.go`）：

| 分组 | 方法 | 作用 |
|------|------|------|
| 基础 | `New(model)` `Name` `Description` `Prompt` `WithToolModel` `Tools` / `ToolsFromRegistry` / `ToolsAutoSelect` `WithMaxIterations` | 模型、系统提示、工具集、ReAct 最大迭代 |
| 预算护栏 | `WithTokenBudget` `WithWallClock` `WithToolTimeout` | token / 挂钟 / 单工具超时兜底 |
| 上下文压缩 | `WithContextCompaction(trigger,target)` `WithMaxMessageTokens` `WithReasoningEviction` `WithTokenCounter` `WithContextSummarizer` | 三级压缩：单条截断 / 推理淘汰 / 整体摘要 |
| 记忆 | `WithContextBuilder` | 接入会话历史回放（需 `messageRepo`） |
| Hook | `Before` `After` `Observe` `Middleware` | 请求前/后钩子、观测、中间件 |
| 协作 | `WithCollaboration(reg, EnableDelegate()/EnableAsk()/EnableHandoff())` | 多 Agent 委托 / 询问 / 交接 |
| 高级能力 | `WithReflection` `WithConclusion` `WithClarification` `WithAutoCompress` `WithInputGuardrail` `WithOutputGuardrail` `WithToolOutputGuardrail` | 反思、结论生成、意图澄清、输入/输出/工具输出护栏 |

> 便捷工厂：`NewSimpleAgent` / `NewToolAgent` / `NewAgentFromRegistry` / `NewAgentFromConfig`（用领域层 `AgentConfig` 结构体声明式建 Agent，见下）。

### 三、配置从哪来（三类）

**① Agent 本体** —— 在代码里（`initializer/init.go` 或 `presets/`），无外部配置文件。

**② 共享依赖（config.yaml + 环境变量）** —— 模型、DB、Skill 端点等，见下方 [§配置](#配置)。优先级：`代码默认 < config.yaml < 环境变量`；**密钥只走环境变量**。

**③ 声明式配置结构体（可选路径）** —— `internal/model/agent/config.go` 的 `AgentConfig`（`MaxIterations` / `DefaultTopK` / `HookConfig` / `MemoryConfig` / `ReflectionConfig` 等），配合 `Builder.NewAgentFromConfig` 使用；`DefaultAgentConfig()` 给出默认值、`Validate()` 校验。当前四个内置 Agent 走的是直接链式装配，未走此路径。

**④ 高级能力的特性开关（环境变量，默认全关＝零回归）** —— 在 `cmd/server/main.go` 组合根按环境变量装配：

| 能力 | 关键环境变量 | 默认 |
|------|-------------|------|
| Data Agent 工具超时兜底 | `DATA_AGENT_TOOL_TIMEOUT_SECONDS`（0/负=关） | 90s |
| 经验蒸馏（写侧） | `EXPERIENCE_DISTILL_ENABLED` / `EXPERIENCE_PREGATE_ENABLED` / `EXPERIENCE_SKILL_SINK_ENABLED` / `EXPERIENCE_MIN_CONFIDENCE`… | 蒸馏关，前置门开 |
| 经验召回（读侧） | `EXPERIENCE_RECALL_ENABLED`（需图谱可用） | 关 |
| 语义缓存 / few-shot | `SEMANTIC_CACHE_ENABLED` / `RAG_HARD_CACHE_ENABLED` / `DATA_SOFT_CACHE_ENABLED`（+ `*_THRESHOLD` / `*_TTL` / `*_TOP_K`） | 关 |
| 技能目录 | `COGNIDA_SKILL_DIRS` | `./skills,../skills,../../skills` |
| 调用链追踪 | `AGENT_TRACING_ENABLED` | 关 |

> 反思（`WithReflection`）与护栏（`WithGuardrail`）**已实现但默认未接线**：组合根未调用对应装配缝，故当前对四个内置 Agent 无效果，属「能力就绪、待接线」。

### 四、如何调用运行中的 Agent

Agent 在 `RegisterSpec` 时已被构建好，运行时按 ID 取实例即可：

```go
// 编排器 / 评测适配器持有 registry.GetInstance 作为 AgentGetter
ag, ok := registry.GetInstance("agent-foo") // agentID 为空回退 "default"
if !ok {
    return fmt.Errorf("agent 未注册")
}

// 一次性完整响应（含工具调用轨迹）
resp, err := ag.Chat(ctx, "查询销售额最高的前 10 个产品")
// resp.Content / resp.ToolCalls / resp.Metadata

// 流式响应
ch, err := ag.Stream(ctx, "……")
for chunk := range ch { // chunk.Content / chunk.Done
    _ = chunk
}
```

`Agent` 接口（`framework/eino_agent.go`）只有三个方法：`Chat(ctx, msg) (*Response, error)`、`Stream(ctx, msg) (<-chan *Chunk, error)`、`Name() string`。内置 Agent 的 ID 见 [§核心功能 → 内置 Agent](#1-agent-系统) 表。

### 五、开发流程

遵循仓库根 `CLAUDE.md` 的强制流程：`准备 → 评估 → 开发 → 测试 → Review → 提交`。

```bash
# 装配 / 运行
go run cmd/server/main.go                              # 开发模式启动
go generate ./cmd/wire                                 # 改了依赖注入后重新生成 wire

# 测试
go test ./internal/service/agent/... -v                # Agent 单元测试
go test -tags=integration ./internal/... -v            # 集成测试（真实 DB / 评测服务）

# 若新增/修改了业务表（加了 model 字段等），配套写迁移并应用
set -a && source .env && set +a && go run ./cmd/migrate-db up
```

> 提交前必过 code-review；任务完成后终止启动的服务进程（`CLAUDE.md` 强制规则）。

## 配置

### 环境变量

```bash
# 服务配置
SERVER_HOST=0.0.0.0
SERVER_PORT=8080

# 数据库
MYSQL_DSN=root:password@tcp(localhost:3306)/cognida

# 向量库
MILVUS_ADDRESS=localhost:19530

# 图数据库
NEO4J_URI=bolt://localhost:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=password

# LLM
LLM_API_KEY=sk-xxx
LLM_BASE_URL=https://api.openai.com/v1
LLM_MODEL=gpt-4

# LLM 弹性（降级/重试/熔断，未设置则用安全默认值）
LLM_RESILIENCE_ENABLED=true          # 整体开关
LLM_RESILIENCE_MAX_ATTEMPTS=3        # 单目标最大尝试次数（含首次）
LLM_RESILIENCE_BASE_BACKOFF=200ms    # 退避基准
LLM_RESILIENCE_MAX_BACKOFF=5s        # 退避上限
LLM_RESILIENCE_RETRY_AFTER_CAP=30s   # Retry-After 封顶
LLM_RESILIENCE_FAIL_THRESHOLD=5      # 熔断连续失败阈值
LLM_RESILIENCE_COOLDOWN=30s          # 熔断冷却时长
LLM_RESILIENCE_HALF_OPEN_PROBES=1    # half-open 探测放行数

# 多数据源（外部查询数据源）
DATASOURCE_SECRET_KEY=change-me           # 凭据加密密钥（AES-256-GCM，未设置则启动失败）
DATASOURCE_HEALTHCHECK_ENABLED=false      # 后台健康检查开关
DATASOURCE_HEALTHCHECK_INTERVAL=5m        # 健康检查间隔

# 评测（Python 计算服务）
PYTHON_EVALUATION_ENDPOINT=http://localhost:18888  # 独立 FastAPI 评测服务

# Text2SQL Agent
TEXT2SQL_ENABLED=true
TEXT2SQL_MAX_ROWS=1000
TEXT2SQL_TIMEOUT=30

# Python Skill MCP
SKILL_ENABLED=true
SKILL_ENDPOINT=http://localhost:8080/mcp
SKILL_TIMEOUT=30
SKILL_CACHE_TTL=60
```

### 配置文件

非密配置的唯一真源是 `internal/config/config.yaml`，由 `//go:embed` 在**编译期**打包进二进制，运行时无需关心工作目录、也不依赖磁盘副本。

- **加载优先级（从低到高）**：`代码内兜底默认  <  config.yaml  <  环境变量（覆盖）`
- **只放非密配置**：Host/Port、开关、超时、阈值、池大小、端点、模型名、目录、TTL、分页等。
- **严禁出现任何密钥**（`DB_PASSWORD` / `MILVUS_TOKEN` / `NEO4J_PASSWORD` / `JWT_SECRET` / `CHAT_API_KEY` / `METASO_API_KEY` / `EMBEDDING_API_KEY` / `REDIS_PASSWORD` / `DATASOURCE_SECRET_KEY`）——密钥类字段没有对应 yaml key，只能来自环境变量（见 `.env` / `.env.example`）。
- **免重编覆盖**：设置环境变量 `CONFIG_FILE` 指向外部 yaml，即可叠加在内嵌真源之上（缺失/解析失败静默跳过）。

真实结构摘录（完整见 `internal/config/config.yaml`）：

```yaml
# 数据库（MySQL）—— 密码走 DB_PASSWORD 环境变量
database:
  host: localhost
  port: "3306"
  user: root
  database: cognida

# 聊天模型 —— APIKey 走 CHAT_API_KEY 环境变量
chat:
  source: remote                       # local / remote
  base_url: https://api.deepseek.com/v1
  model_name: deepseek-chat
  provider: deepseek

# Embedding —— APIKey 走 EMBEDDING_API_KEY 环境变量
embedding:
  provider: dashscope
  model: text-embedding-v3
  base_url: https://dashscope.aliyuncs.com/compatible-mode/v1/embeddings

# HTTP 服务
server:
  port: "8080"
  mode: debug                          # debug / release
  host: 0.0.0.0

# 评测系统（独立评测 FastAPI :18888）
evaluation:
  agent_timeout: 180                   # Agent 单条评测超时（秒）

# Skill / MCP 工具端点（指向 Python MCP 服务 :3100）
skill:
  enabled: false
  endpoint: http://localhost:3100/mcp
  timeout: 30
  cache_ttl: 60
```

## 依赖注入

使用 [Wire](https://github.com/google/wire) 进行依赖注入：

```bash
# 生成 wire 代码
go generate ./cmd/wire
```

## 数据库表结构同步（版本化迁移，golang-migrate）

业务表结构的唯一真源是 `migrations/`（成对的 `NNNNNN_*.up.sql` / `.down.sql`），由 `cmd/migrate-db` 驱动执行；**运行时与生产库不做任何自动建表/改表**（已弃用 GORM AutoMigrate）。改 schema 时新增一对迁移文件并同步更新对应 model，二者保持一致。

```bash
cd cognida-go && set -a && source .env && set +a
go run ./cmd/migrate-db up          # 应用全部未执行迁移（默认动作）
go run ./cmd/migrate-db version     # 查看当前版本 / dirty
go run ./cmd/migrate-db down [N]    # 回滚 N 步（省略=全部，谨慎）
go run ./cmd/migrate-db force <V>   # 存量库接入：force 1 标记基线已应用
```

> 新增变更、存量库接入、dirty 处理详见 `migrations/README.md`。图谱表（`graph_*`）以 Neo4j 为唯一真源，不在本迁移范围；外部数据源为只读外部资源，不参与迁移。

## 开发

### 运行服务

```bash
# 开发模式
go run cmd/server/main.go

# 编译运行
go build -o bin/server cmd/server/main.go
./bin/server
```

### 运行测试

```bash
# 全部测试
go test ./...

# 覆盖率
go test -cover ./...

# 特定包
go test ./internal/service/agent/...

# 集成测试（真实数据库 / 评测服务）
go test -tags=integration ./internal/... -v
```

### 代码生成

```bash
# Proto 生成
python scripts/generate_grpc.py

# Wire 生成
go generate ./cmd/wire
```

## 部署

### Docker

```bash
# 构建
docker build -t cognida-go:latest .

# 运行
docker run -d \
  -p 8080:8080 \
  -e MYSQL_DSN=... \
  -e LLM_API_KEY=... \
  -e DATASOURCE_SECRET_KEY=... \
  cognida-go:latest
```

### Docker Compose

```bash
docker-compose up -d
```

## 文档

- [架构设计](../docs/architecture.md)
- [API 文档](../docs/api.md)
- [Agent 配置](../docs/agent-config.md)
- [Agent Hooks](../docs/agent/agent-hooks.md)
- [Text2SQL 架构](../docs/text2sql-architecture.md)
- [Text2SQL 测试](../docs/text2sql-testing.md)
- [Skill 集成](../docs/skill-integration.md)
- [统一 Chat API](../docs/unified-chat-api.md)

## 许可证

Copyright © 2024 Cognida Project
