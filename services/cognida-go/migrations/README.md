# 数据库迁移（golang-migrate，〔INF-4〕）

业务表结构的**唯一真源**。所有 schema 变更一律新增版本化迁移文件，由 `cmd/migrate-db`
驱动执行；**运行时（`cmd/server`）与生产库不做任何自动建表/改表**（不再用 GORM AutoMigrate）。

> 图谱本体以 Neo4j 为唯一真源（见〔GO-3〕），不在本迁移范围；MySQL 不保存 `graph_*` 表。

## 文件约定

golang-migrate 命名：`NNNNNN_描述.up.sql` / `NNNNNN_描述.down.sql`，成对出现。

- `000001_baseline.up.sql` —— 32 张业务表基线（由切换时的 GORM model AutoMigrate 空库同步后
  `mysqldump --no-data` 生成，忠实反映既有结构）。`down` 删除全部业务表。
- `000036_agent_preferences.up.sql` —— 用户＋工作空间范围的 Agent 默认配置与幂等更新回执。
  新服务启动前应用；`down` 会删除这两张表及其中的个人配置，须先停止相关写入并备份。

迁移文件经 `migrations/embed.go` 以 `embed.FS` 内嵌进二进制，`cmd/migrate-db` 用 iofs 源读取，
无需在运行环境额外拷贝 `.sql`。

## 常用命令

先加载环境变量（`DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME`）：

```bash
cd services/cognida-go && set -a && source .env && set +a

go run ./cmd/migrate-db up            # 应用全部未执行迁移（默认动作）
go run ./cmd/migrate-db up N          # 只应用后续 N 个迁移，N 必须为正整数
go run ./cmd/migrate-db version       # 查看当前版本 / dirty 状态
go run ./cmd/migrate-db down [N]      # 回滚 N 步；省略 N 则回滚全部（谨慎）
go run ./cmd/migrate-db force <V>     # 将版本强制标记为 V（不执行 SQL）
```

## 新增一次变更

1. 建一对文件，版本号递增，例如：
   `000002_add_users_locale.up.sql` / `000002_add_users_locale.down.sql`。
2. `up` 写正向 DDL（`ALTER TABLE ...`），`down` 写可逆回滚。
3. 本地 `go run ./cmd/migrate-db up` 验证，再 `down 1` / `up` 验证可逆。
4. 若变更契约相关，同步刷新对应基线/文档。

> GORM model 结构体仍是应用读写的映射，但**不再**用它自动同步 schema；改字段时
> model 与迁移文件需同时更新，二者保持一致。

## 存量库接入（一次性）

既有库已由旧 AutoMigrate 建好 32 张表，切换到版本化迁移时先把基线标记为已应用，避免重复建表：

```bash
go run ./cmd/migrate-db force 1   # 建 schema_migrations 并标记 000001 已应用
go run ./cmd/migrate-db up        # 此后应用增量（当前无增量则 no change）
```

全新库直接 `up`，由 `000001_baseline` 建表。

## dirty 状态处理

迁移中途失败会置 `dirty=true` 并停在半应用状态。人工核对该版本 SQL 的实际落库情况后，
用 `force <上一个干净版本>` 复位，修正迁移文件，再 `up`。

## 历史 Collection AutoMigrate 表接入

早期开发环境可能停在版本 3，却已存在 `collection_tasks/sources/runs/items`，
且字段是 nullable longtext、缺少唯一键和外键，甚至提前带有调度字段。这不等价于
迁移 5/6 已完成。`migrate-db up` 会在执行任何增量之前检查此状态并报错；不要用
`force 5`、`CREATE TABLE IF NOT EXISTS` 或直接删除旧表绕过结构差异。

接入步骤：

1. 暂停向该库写入的 Go 服务；完整备份数据、触发器、存储过程和事件，并在独立库恢复验证。
2. 确认迁移版本干净、四表的全部列、引用关系及记录数。检查旧数据满足新 schema 的长度、
   NOT NULL、唯一键和外键约束；如果不满足，先明确修复数据的规则，不截断或丢弃记录。
3. 若四表没有外部外键引用，将其在**一条原子 `RENAME TABLE`**中改成独立的保留名
   （例如 `legacy_v3_collection_tasks` 等），避免被新迁移覆盖。存在外部引用时需另行设计迁移。
4. 正常运行 `migrate-db up`，由版本化 SQL 建立新表。使用 `STRICT_ALL_TABLES`，在一个事务中
   按 tasks → sources → runs → items 的顺序，显式列名复制旧表的每一列；不得使用 `INSERT IGNORE`。
   旧表有新 schema 未定义的列时先停止核对，不能静默丢列。
5. 验证每表记录数、主键，以及所有原列的逐字段二进制 NULL-safe 等值；核对其它业务表记录数
   （迁移 9 对成员关系的补全属于预期变化），最后确认 migration version 和 dirty=false。
6. 确认新接口正常后再恢复服务。保留备份与旧表，直到单独决定归档期限；升级失败时保留现场，
   依据备份和执行记录恢复，不通过强制版本号假装成功。

本地 `cognida` 已按上述过程完成版本 3 → 11：四张旧表各 2 条记录全部逐字段匹配，
原表以 `legacy_v3_*` 保留。备份与验证报告在仓库忽略目录
`var/backups/cognida-20261003-v3/`，备份文件权限为 0600；该目录不进入 Git。
