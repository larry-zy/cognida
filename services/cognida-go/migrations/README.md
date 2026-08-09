# 数据库迁移（golang-migrate，〔INF-4〕）

业务表结构的**唯一真源**。所有 schema 变更一律新增版本化迁移文件，由 `cmd/migrate-db`
驱动执行；**运行时（`cmd/server`）与生产库不做任何自动建表/改表**（不再用 GORM AutoMigrate）。

> 图谱本体以 Neo4j 为唯一真源（见〔GO-3〕），不在本迁移范围；MySQL 不保存 `graph_*` 表。

## 文件约定

golang-migrate 命名：`NNNNNN_描述.up.sql` / `NNNNNN_描述.down.sql`，成对出现。

- `000001_baseline.up.sql` —— 32 张业务表基线（由切换时的 GORM model AutoMigrate 空库同步后
  `mysqldump --no-data` 生成，忠实反映既有结构）。`down` 删除全部业务表。

迁移文件经 `migrations/embed.go` 以 `embed.FS` 内嵌进二进制，`cmd/migrate-db` 用 iofs 源读取，
无需在运行环境额外拷贝 `.sql`。

## 常用命令

先加载环境变量（`DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME`）：

```bash
cd services/cognida-go && set -a && source .env && set +a

go run ./cmd/migrate-db up            # 应用全部未执行迁移（默认动作）
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
