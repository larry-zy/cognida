# Tasks: 多数据源支持

## 1. Phase 1 — 数据源注册表（link-go）

- [x] 1.1 新增 `internal/model/datasource`：DataSource 实体（含 status/extra/last_health_check_at）+ Repository/Service 接口定义
- [x] 1.2 新增 `internal/repository/mysql` DataSource GORM model 与 repository 实现（name 唯一索引），注册进 `cmd/migrate-db`，跑一次同步建表验证
- [x] 1.3 实现凭证加解密（AES-GCM，`DATASOURCE_SECRET_KEY` 环境变量）：加密落库、解密失败标记需重录、单元测试覆盖加解密与密钥缺失
- [x] 1.4 实现 `internal/service/datasource` CRUD service：创建/更新（密码留空不修改）/删除/列表，响应不含密码字段
- [x] 1.5 新增 `datasource_handler.go` 与 `/datasources` 路由（CRUD），API 测试验证不回显密码与 name 冲突错误

## 2. Phase 1 — 连接管理与探查（link-go）

- [x] 2.1 定义 driver Strategy 接口（BuildDSN/Ping/ListTables/DescribeTable）+ Factory，注册 MySQL driver 实现
- [x] 2.2 实现 ConnectionManager：`datasource_id → *sql.DB` 懒加载缓存、保守池参数（可配置）、updated_at 版本比对失效重建、空闲回收、删除数据源联动关池；单元测试覆盖懒加载/失效/回收
- [x] 2.3 实现"测试连接"接口（表单配置或已存 id，临时连接不进缓存，超时限时返回，错误脱敏）
- [x] 2.4 实现外部数据源 schema 探查接口（表清单/表结构，经 driver 的 information_schema 查询）
- [x] 2.5 集成测试：用本地 MySQL 作为"外部数据源"验证注册→测试连接→探查→改密码失效重建全链路（`datasource_integration_test.go` TestDataSourceLifecycle，含密码留空不改/错密码失效/重录重建/删除关池，已跑通）

## 3. Phase 2 — 查询安全与 text2sql 实装（link-go）

- [x] 3.1 引入 SQL 解析依赖并实现只读拦截（仅 SELECT/CTE 放行，解析失败 fail-closed），单元测试覆盖写语句/DDL/SET/解析失败用例（复用既有 `validateSQL` 关键词黑名单+SELECT/WITH 白名单，外部路径同一代码路径生效；TestValidateSQL 覆盖）
- [x] 3.2 实现强制 LIMIT 改写与查询超时（context.WithTimeout，默认 30s 可配）、错误信息脱敏（复用 `ensureLimit`+30s 超时；外部错误统一包装"外部数据源查询执行失败"，Acquire 层 sanitizeErr 已脱敏 DSN/密码）
- [x] 3.3 实装 `get_schema` 的 `database_id`：非空经 ConnectionProvider 路由（`resolveQueryTarget`），外部数据源复用有界选表规则；无效 id 显式报错不回落业务库
- [x] 3.4 实装 `sql_execute` 的 `database_id`：外部数据源执行 + 只读防护 + 结果照常入 Result Store 回传信封（queryTarget 统一 *sql.DB 执行）
- [x] 3.5 Data Agent 会话数据源上下文：`AgenticRAGRequest.DatasourceID` → `agentctx.WithDatasourceID` 注入，查询类工具以 ctx 值为默认；外部数据源会话硬拦 `sql_mutate`/`etl_run`；单测覆盖空值向后兼容（datasource_provider_test.go）

## 4. Phase 2 — 前端（link-web）

- [x] 4.1 新增 `api/datasource/` 模块（CRUD/测试连接/schema 探查）
- [x] 4.2 新增数据源管理页 `views/datasource/`（列表/新建/编辑/删除/测试连接，自研 Ui* 组件，编辑密码留空语义）+ 路由 `/datasource` 与导航入口（platform 导航、i18n 中英文案）
- [x] 4.3 `DataAgentView.vue` 增加数据源选择器（默认"当前库"），`streamDataChat()` 透传 `datasource_id`（空值不下发，向后兼容），会话内记忆选择；`npm run type-check` 通过

## 5. Phase 3 — 扩展（可后置）

- [ ] 5.1 PostgreSQL driver 实现并注册（BuildDSN/Ping/ListTables/DescribeTable），前端类型下拉放开
- [ ] 5.2 数据源健康检查定时任务（默认关闭，更新 status/last_health_check_at，列表展示状态）
- [ ] 5.3 数据质量模块接入：Go 从数据源抽样取数推给 Python（或落 Parquet 复用 load_data），Python 不直连数据库

## 6. 验证与收尾

- [x] 6.1 `go test ./internal/... -v` 与 `-tags=integration` 全量通过；涉及 HTTP 接口跑 API 测试（datasource handler API 测试含不回显密码/name 冲突；唯一失败为预先存在的 TestDataAnalysisE2E_RealMCP，依赖 :8899 Python MCP 服务未启动，与本变更无关）
- [x] 6.2 触发 code-review skill 并修复问题；文档补充 `DATASOURCE_SECRET_KEY` 配置说明；终止开启的服务进程（无 code-review skill，改为委派独立 Agent 评审：修复 validateSQL 多语句检测反转、ensureLimit 数值上限旁路、rows.Err() 缺失三处并补回归测试，全量单测复跑通过；`DATASOURCE_SECRET_KEY` 已补充至 `.env.example`；本任务未启动任何服务进程，无需终止）
