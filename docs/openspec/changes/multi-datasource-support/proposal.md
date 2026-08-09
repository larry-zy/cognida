# 多数据源支持（multi-datasource-support）

## Why

当前系统只能连接自身的单一业务库（全局 DSN + 全局 `gorm.DB` 单例），Data Agent / text2sql 虽预留了 `database_id` 参数但未实装（`resolveDatabase()` 恒回落到当前库）。用户无法把自己的外部数据库注册进来做查询分析，这限制了 Data Agent 与数据质量模块的实际可用性。

## What Changes

- 新增 `data_sources` 业务表与数据源 CRUD API：注册/编辑/删除外部数据库（MySQL 起步），凭证 AES-GCM 加密存储、API 永不回显密码。
- 新增"测试连接"与 schema 探查接口：保存前可 ping，可列出目标库的表/字段。
- 新增 ConnectionManager：按 `datasource_id` 懒加载 `*sql.DB` 连接池（保守池参数），LRU/空闲回收，配置变更失效缓存；用户数据源只走 `database/sql` 原生查询，不引入 GORM。
- 实装 text2sql 工具链的 `database_id`：`get_schema` / `sql_execute` 非空时经 ConnectionManager 路由到外部数据源，为空保持现状（当前业务库），完全向后兼容。
- 新增外部数据源查询只读防护：SQL 语句类型拦截（仅 SELECT）、强制 LIMIT 上限、查询超时。
- 前端新增数据源管理页（列表/新建/编辑/测试连接）与 Data Agent 会话中的数据源选择器（自研 Ui* 组件）。
- Phase 3（本变更内规划、可后置实现）：PostgreSQL 支持（driver Strategy 抽象）、数据源健康检查定时任务、数据质量模块经 Go 抽样取数接入。

## Capabilities

### New Capabilities

- `datasource-registry`: 数据源元数据管理——`data_sources` 表、CRUD API、凭证加密存储与不回显语义、测试连接、schema 探查。
- `datasource-connection`: 数据源连接生命周期——ConnectionManager 懒加载连接池、保守池配置、LRU/空闲回收、配置变更失效、多 driver（MySQL 起步，PostgreSQL 后续）。
- `datasource-query-safety`: 外部数据源查询安全边界——只读拦截、强制 LIMIT、查询超时、错误信息脱敏。
- `datasource-management-ui`: 前端数据源管理页与数据源选择器。

### Modified Capabilities

- `agent-tools`: `sql_execute` / `get_schema` 的 `database_id` 参数由"预留未实装"变为"非空时路由到已注册数据源执行，且外部数据源强制只读防护"。
- `data-agent`: 会话可携带数据源上下文；指定数据源后，本会话查询类工具 SHALL 在该数据源上执行。

## Impact

- **cognida-go**：新增 `internal/model/datasource`、`internal/repository/mysql`（DataSource model）、`internal/service/datasource`（含 ConnectionManager、凭证加解密）、`internal/handler/datasource_handler.go` 与路由；修改 `internal/service/agent/tools/get_schema.go`、`sql_execute` 相关工具；`cmd/migrate-db` 覆盖新表（GORM model 同步，无手写 ALTER）。
- **cognida-web**：新增数据源管理视图与 API 模块；`DataAgentView.vue` 增加数据源选择器；`streamDataChat()` 透传 `datasource_id`。
- **cognida-python**：本变更不直连数据库（维持"Python 只做计算"分工）；Phase 3 由 Go 抽样后推数据/落 Parquet 复用现有 `load_data()`。
- **依赖**：新增 SQL 解析依赖（语句类型判断，如 `sqlparser`）；`DATASOURCE_SECRET_KEY` 环境变量（凭证加密密钥）。
- **兼容性**：无破坏性变更；`database_id` 为空时行为与现状一致。
