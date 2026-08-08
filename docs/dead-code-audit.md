# 死代码审计报告

> 生成日期：2026-08-08
> 范围：`link-go`（~117k LOC）、`link-python`（~36k LOC）、`link-web`（122 源文件）
> 方法：静态工具（Go `deadcode`/`staticcheck`、Python `vulture`/`ruff`、前端 `knip`）+ grep 逐项交叉验证，已剔除框架反射 / DI 注册 / gRPC 分发 / Pydantic 字段 / Vue 模板 / 动态导入 等误报。
> 状态：**仅审计，未修改任何代码。**

## 总览

| 语言 | 死代码规模 | 整包/整文件可删 | 主要成因 |
|------|-----------|----------------|---------|
| Go | ~11k LOC，924 个不可达函数 | 44 个整死文件 + 2 个整死包 | eino 框架取代旧 chat/LLM 适配器；agent 检索取代旧 knowledge/RAG 栈；语义缓存/RBAC/memory 仓储未接线 |
| Python | ~1.2k LOC + 63 处 ruff 告警 | 3 个整死模块 | `services/judge/` 半删残留；`evaluation/api.py` 被 `fastapi_app.py` 取代；`quality/plugin_loader.py` 未接线 |
| 前端 | ~2k LOC + 6 个无用依赖 | 19 个整死文件 | 未接线的 API 模块/Pinia store/视觉组件/自研 Ui* 组件 |

**合计约 14,000–15,000 行死代码**，Go 占绝大部分。

---

## 一、Go（link-go）

### 方法与可靠性
- `go build ./...` 干净；`deadcode -test ./...`（从全部 `main` 包做 RTA 全程序可达性分析，含测试二进制）为权威依据。
- 已验证其正确性：正确标出 `AgentHandler.Chat`（路由在 `router.go:307` 已注释）与 `ProvideRAGService`（未接线的 Wire provider）。
- `staticcheck ./...` 仅 8 处 U1000，均为 deadcode 已覆盖的子集（staticcheck 只查未导出符号）。

### 高置信度（0 引用，可直接删）

**整死包（全仓 0 import，含 cmd/wire/tests）**
- `internal/infrastructure/auth/jwt/`（jwt.go，14 函数）
- `internal/infrastructure/crypto/`（password.go，6 函数）

**旧 knowledge/RAG 栈**（被 agent 检索取代，见 commit 0818d8e）
- `internal/service/knowledge/service.go`（`NewRAGService` + 13 方法，唯一调用者是未接线的 `wire_gen.go:911 ProvideRAGService`）
- `internal/service/knowledge/knowledge_service.go`（`NewKnowledgeService` + 38 方法）
- `internal/service/knowledge/retriever.go`（`NewRetriever` + 8）
- `internal/service/knowledge/pipeline.go`（`NewPipeline` + 5）
- `internal/service/knowledge/dto.go`（9 个 `To/FromDomainGraph*`）
- `internal/service/knowledge/pipeline/{errors,graph_repository_adapter,query_strengthener}.go`
- `internal/model/knowledge/{errors,retriever,retriever_types}.go`、`internal/model/rag/errors.go`
- 部分死：`knowledge/optimizer.go`、`pipeline/multi_hop.go`、`pipeline/reranker.go` 内若干方法

**语义缓存子系统**（未接线，无 live 调用）
- `internal/handler/cache_handler.go`（整文件 12 函数）
- `internal/handler/middleware/cache_admin_middleware.go`（整文件 7 函数）
- `internal/infrastructure/cache/{management,metrics,semantic_cache}.go`（整文件）
- `internal/infrastructure/cache/feature_flag.go`（20 函数部分死）
- `internal/repository/redis/memory_store.go`（整文件 21 函数）

**旧 chat/LLM 适配器**（被 eino 框架取代）
- `internal/infrastructure/llm/chat/{adapter,sse}.go`（整文件）
- `internal/infrastructure/llm/chat/remote.go:190-220`（7 个 `create*Model`，同时 U1000）
- `internal/infrastructure/llm/chat/provider.go`（`IsQwen3Model`/`GetProviderByName`/`ValidateProvider`）
- `internal/service/chat/{cached_chat,embedding_service,rerank_service,message_converter}.go`（整文件）
- `internal/service/chat/dto.go:142-209`（4 个 embedding/rerank 转换）
- `internal/handler/chat_handler.go`（`Chat`/`chatStream` 等，路由未接线）
- `internal/infrastructure/search/web.go`（`NewTavilySearcher`/`NewMockSearcher`）

**未接线仓储 / RBAC / memory**
- `internal/repository/mysql/rbac_repo.go`（整文件 51 函数）
- `internal/model/user/rbac/entity.go`（整文件）
- `internal/repository/mysql/memory_repo.go`（整文件 36 函数，`memory/` 包已删）
- `internal/repository/mysql/user_repo.go:526-676`（`tenantUserRepository.*` 15 个，部分死）
- `internal/repository/mysql/knowledge_repo.go:944-1119`（`tagRepository.*` 14 个，部分死）

**gRPC client/interceptor 辅助**（包保留 `requestid.go` + `NewClient`）
- `internal/infrastructure/grpc/interceptor/{auth,breaker,logging,retry,tracing}.go`（整文件）
- `internal/infrastructure/grpc/client.go`（`NewClientWithInterceptors`、`ClientPool.*` 等 11 函数部分死；`NewClient`/`DefaultConfig` 为 live，保留）

**Agent 协作 / 编排 / 技能库**
- `internal/service/agent/collaboration/task.go`（整文件 21 函数）
- `internal/service/agent/collaboration/aggregator.go`（`SynthesizerAgent.*` 等 7 部分死）
- `internal/service/agent/orchestration/func.go`（`FuncStream/Map/Filter/Memoize/Concat/Race/All` 等 12 组合子）
- `internal/service/agent/skills/{convenience,manager,middleware,init,loader,adapter,agent_integration}.go`（~40 函数）
- `internal/service/agent/framework/eino_builder.go:302-804`（`NewSimpleAgent`/`NewToolAgent`/`EnableAsk/Handoff` 等 10 个旧 builder 变体）
- `internal/service/agent/framework/collab_errors.go`（5）
- `internal/service/agent/{agent,persistence,research}.go`（旧 pre-registry 路径 ~25 函数，路由已注释）
- `internal/handler/agent_handler.go:120-419`（`Chat`/`ChatStream`/`DeepResearch` 等，路由在 `router.go:307-308` 注释）

**Model / util / misc 死文件**
- `internal/model/errors/errors.go`（18）、`internal/model/common/page.go`（16）、`internal/model/conversation/errors.go`（6）、`internal/model/agent/errors.go`（6）、`internal/model/agent/service.go`（5）
- `internal/handler/sse/events.go`（11 `New*Event`）
- `internal/handler/common.go`（`PageSuccess`/`BindQuery` 等 8）、`internal/handler/web/handler.go`（`ServeFile`/`Index`）
- `internal/repository/mysql/models.go:897-1072`（`GraphNodeModel`/`GraphRelationModel` 的 GORM 方法，已验证父结构体无引用）
- `internal/repository/mysql/{transaction.go,model_repo.go,mysql.go}` 若干、`internal/repository/milvus/retriever/repository.go`（`MetadataToJSON`）、`internal/infrastructure/cache/redis/transaction.go`（8）
- `internal/service/agent/context/bpecount/bpecount.go:73 New`（注：该目录为 git 未跟踪的新增文件）
- evaluation/account 侧若干未用 API：`evaluation/service.go GetTaskProgress`、`task_service.go`（11）、`python_client.go`、`account/service.go`（6）

### 需人工确认（medium）—— 实现完整但当前不在 DI 图内，非腐烂废弃
| 文件 | 原因 |
|------|------|
| `internal/repository/mysql/graph_store.go`（46 函数） | 有完整 integration 套件 `graph_store_integration_test.go`，经 `wire_gen.go:273 ProvideGraphRepository` 接线（当前未走），是 MySQL 图谱**备用后端** |
| `internal/repository/milvus/cache_collection.go`（13 函数） | 有 `cache_collection_integration_test.go` + 单测覆盖，属语义缓存子系统但被主动测试 |
| `internal/service/agent/skills/*` builder/convenience | 大面积 Builder/validator 公共 API，疑似有意的扩展点 |
| `internal/service/agent/orchestration/func.go` 组合子 | 通用库表面，同上 |
| `experience/worker.go`、`framework/reflection/metrics.go` | 自进化子系统 feature-flag（`AGENT_REFLECTION_ENABLED` 默认关），部分 metrics 辅助或为开启时预留 |

### 生成代码（勿手删，应重生成）
- `cmd/wire/wire_gen.go` 含若干死 Wire provider（`ProvideRAGService`、`ProvideConfig` 及旧 RAG 链）。应从 `wire.go` injector set 剔除后重跑 `go generate`/`wire`，而非直接改生成文件。
- `api/`/`proto/` 生成代码未出现在死代码列表中。

### 副带发现（非死代码，独立 bug）
- **集成测试构建已损坏**：`internal/handler/get_database_schema_integration_test.go:77` 调 `NewAgentHandler(nil,nil,nil,nil,nil)`，但构造器现为 6 参数（新增 `RetrievalSettingRepository`）。这使 `go test -tags=integration` 与 `deadcode -tags=integration` 无法编译，建议独立修复。

---

## 二、Python（link-python）

### 入口
`main.py`→`core.app.create_app`；`services/evaluation/fastapi_app.py`（评测 :18888）；`grpc_service/server.py`→`serve_grpc`（DocumentReader + Quality，Analytics 经 `servicer.py:425` 懒加载）；`mcp_service/server.py`。

### 整死模块（建议删除）
1. **`services/judge/`** 整树（10 文件 ~685 LOC）—— 孤儿。无生产 import，唯一调用者 `scripts/test_judge.py` 自身已损坏（import 了不存在的 `judge/client.py`、`judge/executor.py`）。是 `services/evaluation/` 的半删前身。
2. **`services/evaluation/api.py`**（158 LOC）—— 被取代。`EvaluationService`/`get_evaluation_service` 全仓 0 引用，`__init__.py` 未导出，已被 `fastapi_app.py` 替代。
3. **`services/quality/plugin_loader.py`**（213 LOC）—— 未接线脚手架。`PluginLoader`/`load_plugins` 在 quality 管线内 0 引用（仅 `plugins/__init__.py:3` docstring 提及）。删前确认无运行时 `importlib` 字符串加载。

配套：删 `scripts/test_judge.py`（引用不存在的模块）。

### 高置信度孤儿符号（live 模块内，0 引用）
| 位置 | 符号 | 说明 |
|------|------|------|
| `grpc_service/analytics_servicer.py:408` | `serve_analytics_grpc` | 0 调用者，`servicer.py:425` 改用 `create_analytics_server` |
| `grpc_service/analytics_servicer.py:73` | `_dataframe_to_proto` | 仅测试调用 |
| `grpc_service/servicer.py:351/361` | `include_analytics`/`futures` | 未读局部变量 / F811 重复导入 |
| `config/settings.py:87` | `is_prod` | 属性，0 引用（注：`validate_app_env` 是 validator，非死代码） |
| `services/evaluation/graders/registry.py` | `register_instance`/`register_function`/`unregister`/`reload_custom` | 4 个方法 0 外部调用 |
| `services/evaluation/metrics/tokenizer.py:160/167` | `COMMON_CHINESE_WORDS`/`COMMON_ENGLISH_WORDS` | 模块常量 0 引用 |

### ruff 一键可清（63 处）
`cd link-python && uv run ruff check . --fix --select F401,F811,F841`
- F401 无用 import ×56（集中在 `grpc_service/analytics_servicer.py`、`services/quality/*`、`services/evaluation/*`）
- F811 重复定义 ×1、F841 无用局部 ×4（`analytics/statistics.py:339`、`evaluation/strategies/data_driven.py:135`、`quality/evaluator.py:149`、`quality/rules/builtins.py:473`）

### 中置信度（仅在 examples/scripts 中 live，生产已死）
- `services/evaluation/strategies/`（仅 `examples/evaluation_demo.py` 用）
- `services/dataset/loader.py`（仅 examples/scripts 用）
- `services/document/fetcher.py:18 FetchResult`、`mcp_service/handlers.py:132 get_handlers`

### 已剔除的误报类别（勿标记为死）
Pydantic 模型/settings 字段、`@field_validator`/`@model_validator`、`@register_grader` 装饰的 grader 类、匹配 proto 的 gRPC RPC handler、FastAPI 路由、MCP tools、抽象接口多态实现。

---

## 三、前端（link-web）

工具 `knip`（自动识别 Vite/Vue），逐项 grep 复核。包管理器为 **pnpm**（另有一份陈旧 `package-lock.json` 并存，建议删除）。

### 高置信度整死文件（19 个，~2k LOC）
| 文件 | 类型 | LOC |
|------|------|-----|
| `src/api/auth/index.ts` | API 模块 | 39 |
| `src/api/chat/index.ts` | API 模块 | 18 |
| `src/api/message/index.ts` | API 模块 | 39 |
| `src/api/tenant/index.ts` | API 模块 | 53 |
| `src/components/transitions/PageLoader.vue` | 组件 | 105 |
| `src/components/visual/AnimatedBorder.vue` | 组件 | 144 |
| `src/components/visual/GradientText.vue` | 组件 | 84 |
| `src/composables/useToast.ts` | composable | 42（应用改用 `utils/toast.ts`） |
| `src/directives/animations.ts` | 指令 | 99（无注册） |
| `src/stores/evaluation.ts` | Pinia store | 236 |
| `src/stores/knowledge.ts` | Pinia store | 82 |
| `src/stores/ui.ts` | Pinia store | 42 |
| `src/views/settings/GeneralSettings.vue` | 视图 | 76（不在 router 内） |

**仅经无用 barrel re-export 可达的自研组件（实为死，6 文件 937 LOC）**：`UiRadio.vue`、`UiSkeleton.vue`、`UiTooltip.vue`、`UiBreadcrumb.vue`、`layout/UiPageSection.vue`、`layout/UiContainer.vue`（仅被同为死的 `UiPageSection` 引用）。Ui* 组件非全局注册，模板与 import 均 0 引用。

### live 文件内的死导出（删 export + 实现，~150 LOC）
`src/utils/index.ts`：`debounce`/`throttle`/`deepClone`/`generateUUID`/`truncateText`/`highlightKeyword`；`src/utils/security.ts`：`generateRandomString`/`copyToClipboard`；`src/types/index.ts`：`defaultRAGConfig`。

### 无用依赖（可从 package.json 移除）
运行时：`dayjs`、`dompurify`、`highlight.js`；开发：`@types/dompurify`、`@vue/test-utils`、`globals`。

### 误报（勿删）
- **~60 个图标导出**：`components/icons/UiIcon.vue` 通过 `import * as icons` + `icons[props.name]` 动态按名解析，静态工具看不到，全部为 live。
- 经对象命名空间调用的 API（`traceApi.listTraces`、`agentApi.streamAgentChat` 等）—— 仅 `export` 冗余，代码非死。
- **Element Plus 仍在用**（`main.ts` 全局注册 + 6 个视图用其图标 + `utils/element.ts` 包装 `ElMessage`），不可删；自研 Ui 层与其共存。

---

## 建议的清理路径（供后续决策）

1. **第一优先（0 风险）**：Go 2 个整死包 + Python 3 个整死模块 + 前端 19 个整死文件 + 三侧无用依赖/import 清理（Python 跑 `ruff --fix`）。
2. **第二优先**：Go 各"整死文件"（44 个）与部分死方法，配合从 `wire.go` 剔除死 provider 后重跑 wire。
3. **需人工确认**：Go 的 `graph_store.go`（备用图谱后端）、`cache_collection.go`、`skills/*` 与 `orchestration/func.go`（疑似有意保留的扩展点）—— 确认产品意图后再定。
4. **独立修复**：集成测试编译错误（`NewAgentHandler` 参数个数）。

> 所有删除操作前应逐语言执行：Go `go build ./... && go test ./internal/...`；Python `uv run pytest`；前端 `pnpm build && pnpm test`，确保绿灯。
