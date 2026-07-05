# Design: 多数据源支持

## Context

link-go 目前以单一全局 DSN 初始化一个 `gorm.DB` 单例（`internal/repository/mysql/mysql.go`），所有查询都落在自身业务库上。text2sql 工具链已预留 `database_id` 参数（`internal/service/agent/tools/get_schema.go` 的 `resolveDatabase()`），但恒回落当前库。link-python 只从文件读数据，前端无数据源相关 UI。

约束：
- 遵循 `handler → service → model ← repository` 依赖方向。
- Python 只做计算/分析，Go 承担主后端与 UI 契约（见项目分工约定）。
- 业务表结构经 `cmd/migrate-db` 从 GORM model 同步，无手写迁移。
- 前端优先自研 `Ui*` 组件，不直接用 Element Plus。

## Goals / Non-Goals

**Goals:**
- 用户可注册/管理外部 MySQL 数据源，凭证安全存储。
- Data Agent / text2sql 可按 `database_id` 在指定数据源上执行 schema 探查与 SELECT 查询。
- 外部数据源查询有强制的只读/限量/超时安全边界。
- 连接生命周期受控：懒加载、保守池参数、空闲回收、配置变更失效。
- 为 PostgreSQL 等更多 driver 留好 Strategy 扩展点。

**Non-Goals:**
- 不做 Kafka/API/文件等异构 connector 统一抽象（等第二类需求出现再抽象）。
- 不允许对外部数据源做任何写操作（`sql_mutate`/ETL 仍仅限自身业务库）。
- link-python 不直连外部数据库（Phase 3 由 Go 抽样后推数据/落 Parquet）。
- 不做数据源级别的多租户权限模型（当前单人使用，预留 `created_by` 字段即可）。

## Decisions

### D1: 外部数据源用 `database/sql` 原生连接，不用 GORM

外部库 schema 不受我们控制，ORM 映射无意义；且与业务库的 GORM 世界物理隔离，杜绝误把用户库当业务库迁移/写入。备选"多 GORM 实例"被否：AutoMigrate 等能力对外部库是危险面而非便利。

### D2: ConnectionManager 懒加载 + 版本失效

`datasource_id → *sql.DB` 的进程内缓存，首次使用才建池。池参数保守（`MaxOpenConns≈4`、`MaxIdleConns≈2`、`ConnMaxIdleTime≈5min`、`ConnMaxLifetime≈30min`），因为连的是用户的库。缓存条目记录数据源的 `updated_at`（或版本号），取用时与 DB 元数据比对，不一致则关旧池重建——避免改密码后旧连接继续用。空闲回收由 `ConnMaxIdleTime` + 定期清理长期未使用条目实现。备选"启动时全量预连"被否：数据源可能不可达，会拖慢启动且浪费连接。

### D3: 凭证 AES-GCM 加密，密钥走 `DATASOURCE_SECRET_KEY` 环境变量

密码列存 `password_encrypted`（base64(nonce+ciphertext)）。API 响应一律不含密码字段；编辑接口"密码留空=不修改"。备选：明文（否）、单向 hash（不可行，需要还原连接）、外部 KMS（当前部署形态过重）。

### D4: 只读防护三层叠加

1. **SQL 语句类型拦截**：解析语句类型，仅放行 SELECT（含 CTE 的 SELECT）；`INSERT/UPDATE/DELETE/DDL/SET/USE` 等一律拒绝。实现优先用现有 sqlparser 类库判断语句首类型，不自研完整解析器。
2. **强制 LIMIT 上限**：无 LIMIT 或超上限时改写/拒绝，沿用现有 1000 行保护。
3. **查询超时**：`context.WithTimeout`（默认 30s，可配）。
另外建议（文档提示，不强制）用户提供只读账号。错误信息回传前脱敏（不含 DSN/密码）。

### D5: `database_id` 语义——空值向后兼容

`get_schema`/`sql_execute` 的 `database_id` 为空 → 现状行为（当前业务库）；非空 → 经 ConnectionManager 路由到已注册数据源，且强制 D4 防护。Data Agent 会话可携带数据源上下文，作为本会话工具调用的默认 `database_id`。不存在或被禁用的数据源返回明确错误的合成 ToolMessage，不静默回落业务库（防止越权/混淆）。

### D6: driver Strategy 抽象

`type Driver interface { BuildDSN(cfg) ; Ping ; ListTables ; DescribeTable ; QuoteIdent }`，Phase 1 只有 MySQL 实现，PostgreSQL 在 Phase 3 以新增实现接入，注册进 Factory。schema 探查语句（`information_schema` 查询）归属各 driver。

### D7: 表结构与分层落位

- `data_sources` 表：`id / name / type / host / port / database_name / username / password_encrypted / status / extra(JSON) / last_health_check_at / created_by / created_at / updated_at`，`name` 唯一。
- model：`internal/model/datasource`（实体 + repository/service 接口）；repository：`internal/repository/mysql`；service：`internal/service/datasource`（CRUD、加解密、测试连接、ConnectionManager、schema 探查）；handler：`internal/handler/datasource_handler.go`，路由挂 `/datasources`。
- `cmd/migrate-db` 注册新 model 后跑一次同步建表。

### D8: 前端

新增 `views/datasource/` 管理页（列表/新建/编辑/测试连接，复用自研 `Ui*` 组件）与 `api/datasource/` 模块；`DataAgentView.vue` 顶部加数据源下拉（默认"当前库"），`streamDataChat()` 透传 `datasource_id`。

## Risks / Trade-offs

- [用户库被慢查询拖垮] → 保守池参数 + 强制 LIMIT + 超时；建议只读账号。
- [sqlparser 对方言 SQL 误判] → 拦截层"默认拒绝"：解析失败即拒绝执行，宁可误杀。
- [密钥丢失导致所有凭证不可解] → 文档明确 `DATASOURCE_SECRET_KEY` 必须持久保存；解密失败时数据源标记为需重新录入密码，不崩溃。
- [进程内连接缓存在多实例部署下各自为政] → 当前单实例可接受；缓存仅是性能优化，正确性靠版本失效兜底。
- [外部 schema 巨大导致 get_schema 上下文爆炸] → 复用现有有界选表逻辑（keywords 相关度 + 轻量目录上限）。

## Migration Plan

1. Phase 1 合入后跑 `cd link-go && set -a && source .env && set +a && go run ./cmd/migrate-db` 建 `data_sources` 表；配置 `DATASOURCE_SECRET_KEY`。
2. `database_id` 为空全程走旧路径，无需灰度；回滚即不再使用新接口，表可留存。
3. Phase 3 的健康检查定时任务独立开关，默认关闭。

## Open Questions

- SQL 解析库选型：`vitess sqlparser` vs `xwb1989/sqlparser` vs 轻量前缀+token 判断——实现时按依赖体积定。
- Phase 3 数据质量取数走"Go 推数据给 Python gRPC"还是"落临时 Parquet"——到时按数据量实测定。
