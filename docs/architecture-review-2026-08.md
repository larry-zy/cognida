# Cognida 全局架构审查与整改计划

> 审查日期：2026-08-09
> 范围：Go 服务（`services/cognida-go`）、Python 服务（`services/cognida-python`）、前端（`apps/cognida-web`）、跨服务通信（`proto/`、gRPC/MCP/HTTP）、基础设施与横切关注点。
> 方法：五路并行架构审查（Go 后端 / Python 服务 / 跨服务通信 / 前端 / 基础设施），交叉归并去重后成文。

## 总体结论

代码本身工程质量不低：依赖倒置到位、密钥卫生好（无硬编码凭证、`.env` 未入库）、axios/SSE 封装扎实、数据源凭证 AES-GCM 加密落库、LLM 有独立 resilience 模块。

真正的架构债不在"写得烂"，而在**几处半途而废的收敛/迁移**和**多租户 + 限流两个 SaaS 生死线的缺口**。问题高度成主题化，且跨前后端。

---

## 一、贯穿全栈的 5 条主线

### 主线 1｜"半成品迁移"到处留活雷 —— 认知地基不稳
同一模式在三层同时出现（新范式上线、旧的没拆干净）：

- **proto 契约层**：`evaluation.proto` / `judge.proto` / `common.proto` 三份死契约仍在两端生成；analytics 起了个 `:50053` gRPC server 却无人连；evaluation 已迁 HTTP `:18888` 但 proto 未清。
- **前端状态层**：5 个 Pinia store 有 3 个是死代码；evaluation 域同时存在 store 和 composable 两套重叠抽象。
- **Python 评测 API**：`fastapi_app.py` 里 `/api/evaluate/*` 旧范式和 `/api/v1/.../compute-metrics` 新范式并存，registry 抽象被旧端点旁路。
- **命名债**：`link → cognida` 改名 1076 个文件挂在工作区未提交；proto `go_package` 仍是 `link/...`，靠"剥前缀"侥幸编译，一旦有 proto 跨 import 立即断编译；仓库根还有个疑似 `git mv` 事故产生的文件 `a`。

> 影响：无法从代码判断"哪条线真活着"，每一处都是后续演进的认知税和潜在编译/运行地雷。**优先级最高——不修，其他重构都在流沙上做。**

### 主线 2｜SaaS 两条生死线缺失：多租户隔离 + 限流

- **多租户是"选择性"而非"强制"**：`repository/mysql/mysql.go:30-37` 的 `TenantScope` 在 `tenantID==0` 时返回全表无过滤查询；隔离全靠每个 handler 手动记得调 `WithTenantScope`，无 GORM 全局兜底；31 个 mysql repo 有 3 个完全不带 `tenant_id`。**SaaS 最高危缺陷**——一次遗忘 = 跨租户数据泄漏。
- **前端侧同源**：`setCurrentTenant` 零调用、无租户切换 UI、切换不重拉数据——多租户在前端实为"单租户绑定登录"。
- **全局无 HTTP 限流**：`/auth/login` 可暴力破解；`agent/stream`、`rag`、`evaluation` 等昂贵 LLM 端点可被无限并发打爆，DoS + LLM 成本失控。叠加 LLM 层无 token 预算/租户配额。

### 主线 3｜组合根与全局态失控（Go 侧核心债）

- wire 之外 `cmd/server/main.go` 又做 15 处 `SetXxx` 运行时补线，因为 `ToolRegistry ↔ AgentHandler/tools` 有构造期循环依赖 wire 解不了。
- 依赖藏在包级可变全局里：`tools.globalContext`、`genui` 包级 model，以及 Milvus/MySQL/Redis 客户端全是包级单例而非 DI——与项目宣称的 Wire 架构自相矛盾。
- 失败路径大量 `log.Printf("⚠️…")` 静默降级，半接线也能启动，问题拖到运行时。

### 主线 4｜servicer/组件塞业务逻辑 + 同能力多份实现

- **Python**：analytics 统计逻辑在 gRPC servicer 内联 pandas 重算一份、MCP 路径委托领域层一份，两套口径会漂移；两套重名 `LLMClient` 抽象 + 两套密钥来源；四套互不相同的插件/注册机制。
- **前端**：`KBAssistantView.vue`(1837行) 和 `DataAgentView.vue`(1391行) 把流式消费、会话管理、Markdown 渲染大面积复制粘贴。
- **Go**：图谱在 MySQL + Neo4j 双写，两份近 2000 行平行实现且无跨存储事务/对账，核心数据静默漂移风险。

### 主线 5｜可观测性与生产就绪"看着有，其实是死的"

- Prometheus 指标采集了但没有 `/metrics` 暴露端点；`/health` 静态假健康，不探活任何存储。
- request_id 全链路在 MCP 通路断链；Python 评测 FastAPI 未接 RequestIDMiddleware。
- i18n 形同虚设：2349 行中文硬编码散落 50 个文件，实际只有 `Login.vue` 用 `$t`。
- HTTP server 无超时配置（Slowloris 面）；Dockerfile 入口 `src.main:app` 指向不存在的目录，容器启动即崩且只起空壳。

---

## 二、跨服务通信的两个"生产必炸"点

1. **gRPC 消息大小上下游不对称**：Go 客户端配 100MB（`internal/infrastructure/grpc/client.go:50-51`），Python 服务端没设、沿用默认 4MB。docreader 传大文档/OCR 批量一旦 >4MB → `RESOURCE_EXHAUSTED` 拒收。
2. **Python gRPC 只监听 `127.0.0.1` 且不可配**（`grpc_service/servicer.py:389`）：Go/Python 被迫同机，与"分布式"目标冲突，无法独立部署/横向扩展；且 `insecure` 无 TLS。

---

## 三、分层问题清单

### Go 后端
| # | 级别 | 问题 | 证据 |
|---|------|------|------|
| GO-1 | P0 | 组合根失控：wire 外 15 处 setter 运行时补线 | `cmd/server/main.go:160,372,384,403,415`；`cmd/wire/wire.go:1200-1220` |
| GO-2 | P0 | 包级可变全局单例（隐藏依赖/并发/测试隐患） | `service/agent/tools/context.go:132-160`；`service/agent/genui/compose.go:14` |
| GO-3 | P0 | 图谱 MySQL + Neo4j 双写，两份 ~2000 行，无跨存储事务 | `repository/neo4j/graph_repo.go`(2006)；`repository/mysql/graph_store.go`(1862) |
| GO-4 | P1 | agent 子树占 service 层 44%（135文件/32554行），粒度两极分化 | `service/agent`（21 子包） |
| GO-5 | P1 | `agent/tools` 高扇出枢纽包，import 9 个兄弟子包 | `service/agent/tools`（31文件/9823行） |
| GO-6 | P1 | `eino_agent.go` 上帝对象（接口+23字段实现+3套 sink） | `service/agent/framework/eino_agent.go:36,44,51,416,447,560` |
| GO-7 | P1 | 事务抽象只覆盖 knowledge 一个域，其余内联 `db.Transaction` | `model/knowledge/transaction.go:8`；`mysql/transaction.go` |
| GO-8 | P2 | 接口定义错放 service 层（应在 model） | `service/knowledge/graph.go:26,31` |
| GO-9 | P2 | 部分 handler 直连 repository 绕过 service | `handler/audit_handler.go:22`；`handler/trace_handler.go:18` |
| GO-10 | P2 | request_id 在特定入口"重新生成"而非继承上游 | `handler/agent_handler.go:604,827` |

### Python 服务
| # | 级别 | 问题 | 证据 |
|---|------|------|------|
| PY-1 | P0 | Dockerfile 入口 `uvicorn src.main:app` 指向不存在目录，且只起空壳 | `Dockerfile:29,67`（无 `src/`）；`core/app.py:76-79` |
| PY-2 | P0 | 四进程碎片化无统一入口，主 FastAPI 是空壳；request_id 在评测 FastAPI 断链 | `scripts/dev-all.sh`；`services/evaluation/fastapi_app.py:908` |
| PY-3 | P1 | 两套重名 `LLMClient`，provider/密钥来源分裂 | `services/llm/base.py:7` vs `services/evaluation/llm/client.py:49` |
| PY-4 | P1 | analytics 在 gRPC servicer 内联重算，与 MCP 路径实现分叉 | `grpc_service/analytics_servicer.py:141-211` vs `tools/analytics/*` |
| PY-5 | P1 | 四套互不相同的注册/插件机制；含 gRPC 请求驱动的任意模块导入 | `quality/registry.py`、`quality/plugin_loader.py`、`quality/servicer.py:448-459`、`evaluation/graders/registry.py`、`tools/registry.py` |
| PY-6 | P2 | `fastapi_app.py` 908行 god module，新旧两套评测 API 并存 | `services/evaluation/fastapi_app.py:122,168,476` |
| PY-7 | P2 | 生成的 proto 死代码，与根 `proto/` 不同步 | `proto/{judge,evaluation,common}_pb2*.py`（无引用） |
| PY-8 | P2 | 硬编码 `D:/cognida` 路径 + import 期 `load_dotenv` 副作用 | `fastapi_app.py:18`；`evaluation/llm/client.py:20` |
| PY-9 | P2 | 端口/密钥配置碎片化（18888/50053 硬编码，LLM 密钥直读 env） | `config/settings.py`；`evaluation/llm/client.py:73,79,82` |
| PY-10 | P2 | quality servicer 承载编排 + 运行时任意导入 | `services/quality/servicer.py:128,200,249,448` |
| PY-11 | P3 | 同步 servicer 内每请求 `asyncio.run()` | `grpc_service/servicer.py:54,160,249,311` |

### 跨服务通信
| # | 级别 | 问题 | 证据 |
|---|------|------|------|
| X-1 | P0 | gRPC 消息大小上下游不对称（Go 100MB / Py 默认 4MB） | `grpc/client.go:50-51` vs `grpc_service/servicer.py:365-368` |
| X-2 | P0 | 三份死 proto + 无人连的 analytics gRPC server | `proto/{evaluation,judge,common}.proto`；`analytics_servicer.py:422-435` |
| X-3 | P0 | gRPC/MCP/HTTP 三通路边界混乱，analytics 同能力两条路 | `tools/data_analysis.go:25-57` vs `:50053` |
| X-4 | P1 | 可靠性策略三套并存，gRPC 侧最弱（无重试/无熔断） | `grpc/docreader/client.go:148-155` vs `python_client.go:26-53` vs `mcp/client.go:343-388` |
| X-5 | P1 | 服务地址配置散落四处，docreader 硬编码 | `wire.go:544`（`localhost:50051`）；`config.go:470,660` |
| X-6 | P1 | Python gRPC 仅监听 `127.0.0.1` 且不可配，无 TLS | `servicer.py:389`；`analytics_servicer.py:24` |
| X-7 | P1 | request_id 在 MCP 通路断链 | `mcp/client.go:320-330` |
| X-8 | P2 | docreader `OCRBatch` 假流式（先算完再 `return iter`） | `servicer.py:233-243` vs `docreader/client.go:212-298` |
| X-9 | P2 | CI 无契约漂移门（无 `buf generate` diff / `buf breaking`） | `.github/workflows/*.yml` |
| X-10 | P2 | 单一源迁移半成品，`go_package=link/...` 编译地雷 | `buf.yaml:8-15`；`buf.gen.yaml:17-29`；`proto/*.proto:5` |
| X-11 | P2 | 跨服务错误无结构化映射（`common.ErrorCode` 空转，靠字符串匹配判错） | `python_client.go:283-289`；`docreader/client.go:83,138` |

### 前端
| # | 级别 | 问题 | 证据 |
|---|------|------|------|
| FE-1 | P0 | i18n 形同虚设，2349 行中文硬编码散落 50 文件 | 仅 `views/auth/Login.vue` 用 `$t`；`i18n/locales/*` 各 140 行 |
| FE-2 | P0 | 5 个 Pinia store 中 3 个死代码，状态管理无统一模型 | `stores/{ui,knowledge,evaluation}.ts`（零引用） |
| FE-3 | P1 | evaluation 域 store 与 composable 两套重叠抽象 | `stores/evaluation.ts` vs `views/evaluation/composables/useEvaluationList.ts:20` |
| FE-4 | P1 | 两个 Agent 对话视图大面积复制粘贴，缺 `useAgentChat` | `KBAssistantView.vue`(1837) 与 `DataAgentView.vue`(1391) |
| FE-5 | P1 | v-html 渲染 AI 内容且 DOMPurify 声明却从未使用 | `KBAssistantView.vue:241,305`；`grep DOMPurify` 零命中 |
| FE-6 | P1 | 超大 view 职责过载、状态散落 | `GraphView.vue`(1907)、`DatasetList.vue`(1110)、`QualityCenter.vue`(1070) |
| FE-7 | P2 | 路由守卫无 RBAC、无多租户处理 | `router/index.ts:120`；`setCurrentTenant` 零调用 |
| FE-8 | P2 | Element Plus 全量注册 + 自研 UI 库并存，双设计体系臃肿 | `main.ts:20-24`；`components/ui/index.ts` |
| FE-9 | P2 | 类型与后端契约手工同步无 codegen | `src/types/index.ts`；api 层内联 interface |
| FE-10 | P2 | 组件绕过 store 直连 api + local state，无共享缓存 | `KBAssistantView.vue:383` |
| FE-11 | P3 | `any` 泛滥（152 处），核心可视化/列表丢类型保护 | `views/knowledge`(31)、`components/ui`(18) |
| FE-12 | P3 | Token 存 localStorage 且"加密"仅 base64 混淆 | `utils/security.ts`（`btoa(encodeURIComponent(...))`） |
| FE-13 | P3 | 鉴权/baseURL 逻辑多处重复实现 | `utils/request.ts` vs `api/agent/index.ts:30` vs `utils/sse.ts` |

### 基础设施与横切
| # | 级别 | 问题 | 证据 |
|---|------|------|------|
| INF-1 | P0 | 多租户"选择性"隔离，`tenantID==0` 放行全表，无全局兜底 | `repository/mysql/mysql.go:30-37,107-114` |
| INF-2 | P0 | 无任何 HTTP 层限流（登录暴破 / LLM 端点被打爆） | `router/router.go:77-82,176` |
| INF-3 | P0 | DEV_MODE 整体绕过认证 + token 允许走 URL query | `middleware/auth.go:44-65,41` |
| INF-4 | P0 | 无迁移文件、AutoMigrate 不具生产 schema 演进能力 | `cmd/migrate-db/main.go:44-95`；`graph_store.go:284` |
| INF-5 | P1 | `/health` 静态假健康，不探活任何存储 | `router/router.go:115-121`；`main.go:104-136` |
| INF-6 | P1 | Prometheus 指标采集了但无 `/metrics` 暴露端点 | `observability/global_metrics.go`（无 promhttp） |
| INF-7 | P1 | HTTP Server 无超时配置（Slowloris/DoS 面） | `router/router.go:551-553` |
| INF-8 | P1 | 存储客户端包级全局单例，非 DI | `milvus/milvus_client.go:16`；mysql/redis 包级 |
| INF-9 | P1 | 日志非结构化、访问日志重复、tracing 仅覆盖 agent span | `auth.go:225,262-266`；`main.go:153-164` |
| INF-10 | P1 | LLM 基础设施缺成本控制（无 token 预算/租户配额） | `llm/chat_repo.go:60,332,392` |
| INF-11 | P2 | `config.go` 811行上帝配置 + 重复加载路径 | `config/config.go:307-326,329-341` |
| INF-12 | P2 | Neo4j 连接池配置被读取但未生效（死配置） | `config.go:368`；`cmd/wire/provider.go:23-39` |
| INF-13 | P2 | CI 质量门非阻断、无安全扫描、不跑集成测试 | `.github/workflows/ci-go.yml`、`ci-python.yml` |
| INF-14 | P3 | 迁移期遗留脏文件（根目录文件 `a`）；密钥派生用 sha256 非 KDF | `git status`；`datasource/crypto.go:35` |

---

## 四、整改批次计划

### 第 0 批 · 止血（安全/生产必炸，投入小，优先做）
1. **多租户**：`tenantID==0` 改报错而非放行全表；上 GORM 全局 `Query/Update/Delete` callback 强制注入 tenant_id。〔INF-1〕
2. **gRPC 消息大小**：Python `grpc.server` 显式设 `max_receive/send_message_length` 对齐 100MB。〔X-1〕
3. **限流**：接入 IP+账号维度限流中间件（`golang.org/x/time/rate` 或 Redis 令牌桶），登录端点单独收紧。〔INF-2〕
4. **HTTP 超时**：显式构造 `http.Server` 设 Read/Write/Idle/ReadHeader 超时（SSE 端点豁免 WriteTimeout）；DEV_MODE 绕过加 localhost 硬约束。〔INF-7 / INF-3〕
5. **Dockerfile**：修正入口路径，决定容器真实拓扑（supervisor 编排四进程或收敛单进程多协议）。〔PY-1〕

### 第 1 批 · 清认知地基（消除半成品，投入中）
6. 删三份死 proto（evaluation/judge/common）+ 无人连的 analytics gRPC server；每个能力钉死唯一通路并文档登记。〔X-2 / X-3〕
7. proto `go_package` 改 `cognida/api/proto/...`、`buf.gen opt: module=cognida`；完成 `link→cognida` 收敛并提交；删根目录文件 `a`。〔X-10 / INF-14〕
8. 删 3 个死 Pinia store；evaluation 双抽象二选一（保 composable）；定 i18n 去留（删依赖或建 key 提取规范 + lint 禁模板 CJK）。〔FE-2 / FE-3 / FE-1〕
9. 淘汰 Python 旧 `/api/evaluate/*` 端点，统一走 grader registry。〔PY-6〕

### 第 2 批 · 结构性重构（投入大，可排期）
10. Go 组合根：拆 `ToolRegistry ↔ AgentHandler/tools` 循环依赖为窄接口 + provider，让 wire 一次装配；删包级全局态；`Initialize` 失败必须 `return err` 中止启动。〔GO-1 / GO-2 / INF-8〕
11. 图谱双写改"MySQL 真源 + Neo4j 投影/outbox"，拆分两个 2000 行文件按聚合分文件。〔GO-3〕
12. 抽 `useAgentChat` / `useMarkdown` / `useChatSessions` composable 并在 `renderMarkdown` 接 `DOMPurify.sanitize()`，两个对话 view 收敛到 <500 行。〔FE-4 / FE-5〕
13. Python：统一单一 LLM 抽象层（provider 工厂 + 统一配置源）；统一 Registry/Plugin 契约；servicer 只做协议适配，编排下沉。〔PY-3 / PY-4 / PY-5〕
14. 补真实 `/health` + `/ready`（Ping MySQL/Redis/Milvus/Neo4j）、暴露 `/metrics`、MCP client 透传 request_id、评测 FastAPI 接 RequestIDMiddleware。〔INF-5 / INF-6 / X-7 / PY-2〕

### 第 3 批 · 工程治理
15. CI 加 `buf generate` 后 `git diff --exit-code` 漂移门 + `buf breaking` 对基线 + `govulncheck` + secret-scan；存量清零后 lint 转阻断门禁。〔X-9 / INF-13〕
16. 评估引入版本化迁移（goose / golang-migrate，up/down + schema_migrations），生产库禁用运行时 AutoMigrate。〔INF-4〕
17. ~~统一跨通路重试/超时/熔断策略；gRPC 侧补 retryPolicy + 熔断；服务地址集中配置、docreader 复用统一 target。〔X-4 / X-5〕~~ ✅ 新增 `infrastructure/reliability`（三态 per-target 熔断 + gRPC 透明重试服务配置），接入 gRPC 基础客户端（docreader/quality 自动获得）与 HTTP 评测 compute 路径；docreader 复用 `PYTHON_GRPC_TARGET` 不再硬编码。见 `docs/reliability-strategy.md`。
18. ~~前端：EP 按需引入、类型从 proto/OpenAPI codegen、共享域数据统一走 store/带缓存 composable。〔FE-8 / FE-9 / FE-10〕~~
    - **〔FE-8〕✅ EP 按需引入**：`vite.config.ts` 接 `unplugin-auto-import` + `unplugin-vue-components` 的 `ElementPlusResolver`（`importStyle:false`），`main.ts` 删除全量 `app.use(ElementPlus)` 与全量图标注册循环，模板 `<el-*>` 按需解析并 tree-shake JS；样式仍保留全量 `element-plus/dist/index.css` 覆盖显式导入的 `ElMessage/ElMessageBox`，零样式回退。生成的 `src/types/{auto-imports,components}.d.ts` 已 gitignore（构建时再生）。验证：`vite build` / `vue-tsc --noEmit`（含 clean-CI 无 dts 场景）/ vitest 全通过。
    - **〔FE-10〕✅ 共享域走带缓存 store**：新增 `stores/knowledge.ts`（单一事实源 + 60s TTL 缓存 + in-flight 去重 + `refresh`/`invalidate`），4 处知识库列表消费方（`KBAssistantView`/`KnowledgeBaseList`/`GraphList`/`useEvaluationList`）统一 `storeToRefs` 消费，消除各视图重复 `knowledgeApi.getList()` + 本地 state；顺带修掉 `useEvaluationList` 只识别 `{items}` 不识别裸数组的归一化隐患。
    - **〔FE-9〕⏸ 类型 codegen —— 暂缓（记录原因与路径，非遗漏）**：现状不具备可用 codegen 源。Go handler 仅 8/21 文件有 swaggo 注解，且 75 条 `@Router` 中绝大多数只有 `@Summary`+`@Router`，全库仅 12 条 `@Success` / 6 条 `@Param`，其中真正带 struct 模型的只有 7 个 `domaincache.*`（缓存管理面，前端几乎不消费），其余全是 `map[string]interface{}`/`map[string]string`。`swag` 亦未安装。若强行 `swag init → openapi-typescript`，只会产出约 75 条无 schema 的空路径类型，严重劣于现有 467 行手写 `src/types/index.ts`。要做成需先给全部 21 个 handler 补全 request/response struct 注解（体量与回归风险都远超一个 P2 前端项，且无法在浏览器侧校验类型正确性）。**后续路径**：先补齐 handler 的 `@Param`/`@Success` 到真实 DTO → 引入 swaggo 生成 OpenAPI（2.0）→ `swagger2openapi` 转 3.0 → `openapi-typescript` 生成前端类型 → 增量替换手写类型（先并存不做大爆炸替换）。

---

## 五、做得好的地方（重构时勿破坏）

- **Go**：`model` 零反向依赖、repository 接口全定义在 model（DIP 正确）；agent `framework` 与业务 service 经 `adapters.go` 单点桥接（seam 设计正确）；mysql 有干净的 `gormTransactionManager` + `contextWithTx`（问题在未推广）。
- **基础设施**：无硬编码凭证；`.env` 未入库；JWT 密钥 fail-closed 强校验；CORS fail-closed 白名单；数据源凭证 AES-GCM 加密落库；LLM 独立 resilience 模块。
- **前端**：`utils/request.ts` 统一封装 + 401 单飞 refresh；`utils/sse.ts` 带鉴权 fetch 流、4xx 停重连、`finally` cancel 通知后端；生命周期无泄漏（`network.destroy()`、`onUnmounted` abort）。
- **跨服务**：gRPC/HTTP 两条通路均已透传 request_id（仅 MCP 断链）。
