# 查询适配器、持久查询与验收边界

更新：2026-10-16。本页区分实际引擎验收、真实 MySQL 持久化测试与本地远端协议 fixture。接口存在、协议 fixture 通过均不等于已在对应生产环境验收。

## 引擎与消费者

| 引擎 | 协议与客户端 | 本轮认证与真实环境 | 实际消费者及证据 |
| --- | --- | --- | --- |
| MySQL | MySQL wire；go-sql-driver/mysql 1.9.3 | 独立 MySQL 8.0.46，用户名/密码 | 元数据、抽样、Agent SQL、语义 SQL→Redis ResultStore→资产→七阶段→人工审批→报告实际通过 |
| PostgreSQL | PostgreSQL wire；lib/pq 1.10.9 | 独立 PostgreSQL 17.11，用户名/密码 | 同上实际通过；只读事务与 statement_timeout |
| ClickHouse | Native 协议；clickhouse-go/v2 2.40.3 | 独立 ClickHouse 25.12.11.4，用户名/密码 | 元数据、标量/NULL/精确字符串、只读有界查询和语义完整链实际通过 |
| Doris | MySQL wire；独立 information_schema 元数据实现 | Doris 4.1.3；本地数据库账号 | 真实适配器测试及语义完整链通过，没有用 MySQL fixture 冒充 Doris |
| StarRocks | MySQL wire；独立 information_schema 元数据实现 | StarRocks 3.5；本地数据库账号 | 真实适配器测试及语义完整链通过 |
| Hive | HiveServer2 Thrift；gohive/v2 2.1.0 协议类型及自有 context adapter | Hive 4.0，NONE 认证，本地 TCP | 元数据、语义完整链、持久查询 UI、跨连接恢复、未知提交关闭会话、短页续读实际通过 |
| BigQuery | Google REST v2、OAuth2 服务账号；无 database/sql 连接 | **没有提供真实 GCP 项目/凭据，未执行真实计费查询** | 元数据、dry-run/扫描上限、固定远端 ID、恢复/取消/精确分页有本地 REST fixture；语义 SQL→真实 MySQL query job→ResultStore→资产→真实分析报告有集成 fixture，外部 Google 仍为本地协议服务 |

MySQL/PostgreSQL/ClickHouse/Doris/StarRocks/Hive 六引擎的语义消费链由主控实际执行：MySQL/PostgreSQL/ClickHouse/StarRocks/Hive 五场共 25.5 秒，Doris 5.1 秒，发布后再次校验 `00123` 和精确十进制。该证据不包含 BigQuery live。脱敏原始语义/结果回执保存于 `apps/cognida-web/e2e/evidence/2026-10-16-query-adapters.json`。

`QueryAdapter` 是只读查询接口。MySQL/PostgreSQL/ClickHouse/Doris/StarRocks 提供同步有界结果；Hive 还提供 prepared async；BigQuery 走 async REST。`/query-jobs` 当前只接受 Hive/BigQuery，其余引擎继续使用现有只读查询入口。查询作业成功意味着保存了查询结果，**不代表已发布为知识或可写表**；后续资产治理、审批和发布仍是独立过程。

## 统一限额与精度

- SQL 长度最多 64 KiB；只接受受限单条 SELECT/WITH。拒绝注释、多语句、DML/DDL、文件/远程访问及已知执行型函数；Hive 另拒 TRANSFORM/SCRIPT/REFLECT/JAVA_METHOD/ADD/JAR/DFS，BigQuery 拒外部查询与模型调用。
- 结果 1–10,000 行，最多 512 列、8 MiB，执行截止 1–30 秒。持久 HTTP 查询的最小结果预算为 1 KiB；所有远端分页共用总行数/字节预算，不按页重置。
- 额外读取第 N+1 行只用于识别截断，不把截断结果伪装完整。持久查询每页最多 1,000 行，游标最多 100 个；重复游标或 schema 改变会拒绝。
- 原始 DECIMAL/NUMERIC/BIGNUMERIC、大整数、VARCHAR 前导零以原始字符串保存；ColumnTypes 来自引擎元数据。分析仅对明确数值类型进行统计解释，不猜测 `"00123"`；统计浮点近似不回写原始值。
- ClickHouse 额外连接约束：只读、最大执行 30 秒、256 MiB 内存、8 MiB 结果，超限报错。其他引擎仍需数据库侧只读账号及资源治理；SQL 文本检查不授予数据库权限。
- 通用 SQL adapter 可接受绑定参数；Hive/BigQuery 持久查询目前不支持参数数组，显式拒绝，不静默丢参数。远端 operation ID 由服务端持久作业分配，调用者不能指定。

## 持久性、取消与权限

迁移 `000032_query_jobs` 是 query_jobs 的结构真源。创建原子绑定 tenant/user、幂等键、完整 SQL/预算及数据源版本；同键同输入复用，同键异输入 409。作业保存提交意图、加密远端句柄、分页检查点及最终结果，worker 使用数据库租约和 epoch fence。租约写入/比较均由 SQL 时间完成，公开时间使用 JSON RFC3339，deadline 读取 epoch microseconds；真实测试包括 `loc=Asia/Shanghai`。

每轮提交、读取、分页和结果读取复核当前来源权限与精确配置。凭据、数据源配置改变或撤权会阻断继续消费；owner 仍可请求取消。对外 API 不返回准备会话或远端私有句柄。取消不能抹掉已经提交的结果：未知结果先核对，成功回执保留真实状态。

- BigQuery：先以固定 ID 查已有 job，再 dry-run，只有 SELECT 且估算扫描未超预算才提交；请求同时固定 `maximumBytesBilled`。提交响应丢失继续核对同 ID，不换 ID 重发。`jobs.cancel` HTTP 200 只是收到请求；直到 Status 确认真实终态前保持 `reconcile_required/cleanup_pending`。
- Hive：先 Prepare 会话并加密持久化，再写 dispatch fence 后 ExecuteStatement。若仅有准备会话、无法确认 operation，则关闭该会话并记录 `aborted_unknown`，不能重放 SQL。结果保存后关闭会话。分页以 FETCH_FIRST 加有界 FETCH_NEXT 重建前缀，最多 128 次 RPC；短页但 HasMore=true 会继续，空页且 HasMore=true、非法 offset 均拒绝。
- Hive 异步要求服务端 `hive.server2.close.session.on.disconnect=false`，并配有界 idle session 清理。本轮环境使用 2 分钟 idle、10 秒扫描；Prepare 会探测会话能否跨连接保留，不满足时拒绝执行业务查询。NONE 已实际验证；NOSASL、LDAP/CUSTOM 配置接口存在，LDAP/CUSTOM 强制验证 TLS，**本轮未实测 LDAP/TLS，未实现 Kerberos**。

## BigQuery 配置与语义方言

用户只填写 operator 已登记的 `credential_ref`、project、dataset、location 和扫描字节上限。服务端 `COGNIDA_BIGQUERY_CREDENTIALS` 将该引用映射到只允许 operator 提供的本地服务账号文件，并绑定 tenant/project/dataset/location/最大扫描预算。客户端不能上传私钥、选择文件路径或更改 REST endpoint；授权应由 GCP IAM 的最小只读范围进一步约束。

元数据/健康检查走真实 REST dataset/tables API，不打开 SQL 连接；抽样先核对当前表目录，再通过稳定父操作幂等键执行持久查询。Agent get_schema、sql_execute 和 semantic_query 已接同一受认证 metadata/query 路径。没有 owner/request ID 时拒绝远端付费执行。

GoogleSQL 标识符和字符串使用其反斜线转义规则；日期维度支持 DATE/DATETIME/TIMESTAMP 的 day、ISO week、month、quarter、year，使用 FORMAT_DATE 和 CAST(... AS DATE)。TIMESTAMP 日期桶采用 UTC；纯 TIME 没有日期，明确拒绝日历桶。NUMERIC 与 BIGNUMERIC 的声明类型参与数值识别，高精度值始终保持原字符串。依据：[GoogleSQL 词法](https://docs.cloud.google.com/bigquery/docs/reference/standard-sql/lexical)、[日期格式元素](https://docs.cloud.google.com/bigquery/docs/reference/standard-sql/format-elements)、[数值类型](https://docs.cloud.google.com/bigquery/docs/reference/standard-sql/data-types)。

## 可复现验收

| 层级 | 检查及结果 | 文件 |
| --- | --- | --- |
| 核心 race | queryjob 7 项：完整绑定、恢复、未知取消不重发、prepared 关闭、分页取消、BQ cancel ACK 不冒充终态、拒绝丢失参数 | `internal/service/queryjob/service_test.go` |
| 真实 MySQL/HTTP | 3 项：HTTP owner/scope/幂等、重启检查点、精确字符串、租约/截止/取消、非 UTC DSN | `internal/handler/query_job_integration_test.go` |
| BigQuery 协议 | 本地 OAuth/BQ REST fixture；真实 adapter、MySQL query job、语义编译、ResultStore、对象资产和 analysis stage，断言 BIGNUMERIC 原文及 numeric_count=1，同父操作只提交一次 | `internal/handler/bigquery_semantic_integration_test.go` |
| BigQuery 方言/消费 | 五个日期桶×三类时间类型、GoogleSQL 转义、TIME 拒绝、无需 SQL connection 的 metadata→SQL 执行 | `internal/service/agent/metricsql/bigquery_test.go`、`internal/service/agent/tools/remote_query_test.go` |
| 真实 Hive 浏览器 | 2026-10-16，1 passed，实际 4.2 秒：提交→真实 succeeded→精确值→分页→刷新→同键复用与预算变化409；测试后删除专属 datasource | `apps/cognida-web/e2e/query-jobs.spec.ts`；`e2e/evidence/2026-10-16-query-jobs.json` |
| 前端 | QueryJobsPanel 4 项与全项目 vue-tsc 通过；身份切换旧响应丢弃、同输入重试同键、异输入新键、精确分页、BQ预算与防重复提交 | `src/views/datasource/QueryJobsPanel.spec.ts` |

Go 命令（仓库 `services/cognida-go`，DB 环境仅本地注入，不输出凭据）：

```bash
go test -race ./internal/model/datasource ./internal/service/datasource ./internal/service/queryjob ./internal/service/agent/metricsql ./internal/service/agent/tools -count=1
go test -tags=integration -race ./internal/handler -run '^(TestQueryJobMySQL|TestBigQuerySemantic)' -count=1
```

集成测试要求 `MYSQL_DSN` 指向 `cognida_review_` 隔离库并预先应用迁移32或更新版本，使用随机 tenant 清理自身 fixture；不得对业务库执行。BQ fixture 生成临时合成服务账号并限制 HTTP 只能到本地测试服务器，不向 Google 发请求。浏览器命令与登录方式见 `apps/cognida-web/e2e/README.md`，开启 `COGNIDA_E2E_QUERY_JOBS=1` 前须准备 retained-session Hive fixture。最终环境清理由主控统一执行；本子任务未启动常驻服务。
