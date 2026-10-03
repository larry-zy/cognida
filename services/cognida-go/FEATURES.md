# 数据生命周期首版与验收

本轮按新的可组合数据流水线方案实施，旧 Collection 页面、历史任务与 `/data-agent` 入口继续兼容。采集修复已先合入 main，本文所述功能位于 `codex/complete-documented-features`。

## 可用入口

- 资产目录：MySQL/PostgreSQL 表、视图、主外键同步；可选有界采样、漂移标记、治理属性和业务术语。身份卡给出真实样本完整性、字段风险、可解释相关资产推荐。采样分数不代表准确性或训练适用性。
- 血缘：指标、维度、逻辑表、物理表与列使用独立 Neo4j 命名空间；支持补建、上下游与列影响查询。未对齐元数据标为 unresolved。
- 数据专家与流水线：Access、Quality、Cleaning、Enrichment、Asset、Analysis、Evaluation 可独立执行，也可使用已注册模板组合。版本引用、审批、取消、重试、恢复和 SSE 均持久化；默认分析只接受已发布资产。历史分析 Agent 的写操作能力已从默认授权中剥离。
- 治理工作台：报告与指标监控的定时订阅、运行历史、站内通知；人工标注及审核、规则预标注、批量打标和纠正；特征定义版本、实体值写入、来源计算与按时间读取；待审建议采纳后写入实际术语、知识库或经验仓储。
- 语义模型：可见草稿与已发布模型，支持完整编辑、发布与停用。租户切换由实际成员关系授权并重新签发令牌，刷新保持所选身份。
- 可信决策：`verify_results` 对同一会话的真实结果句柄按键/字段精确比较；`decision_synthesize` 消费实际洞察引用及授权知识库证据。方法、依据、风险和未验证的行动假设随输出保留，不把模型建议标成已验证事实。
- 评测增益：`POST /api/v1/evaluation/compare` 比较当前用户、同一数据集/类型的两个已完成任务，按问题与参考答案配对。输出均值差、近似区间、缺失样本和增益/退化门槛；只给出人工审核或回退建议，不自动修改生产配置。
- 自适应处理：检索模式 `auto` 根据问题形态选择既有策略，无有效证据时最多回退一次，并返回 `strategy_attempts`。知识库分块策略 `auto` 根据段落/标题/长度选择实际分块策略。显式策略和参数仍可配置；知识库及设置在同一事务保存。

## 首版边界

订阅可固定已发布版本，也可选择同一明确来源的 `latest_published`。每次运行先固定实际引用，重试不切换输入；上游需通过正常流程发布新数据。通知仅站内。

标注的规则预标注与打标会标明方法/来源，置信值不是人工验证结果。特征首版通过 MySQL 保存不可变、带时间的值及来源，提供精确在线读取；Python 的图像向量检索使用真实 CLIP/Milvus。身份卡相关推荐使用有界元数据匹配，未宣称模型语义推荐。

专家编排使用固定的声明式 DAG 与输入/输出契约，不开放任意 LLM 生成 DAG；有界并行和无环约束替代递归委派。生命周期执行和授权事件接入现有审计 Writer 与 OpenTelemetry，携带 tenant/user/request/run/stage/job 和资产引用，不记录原始正文。

## 持续运行与数据完整性

流水线取消、重试和推进按 run → stage 的顺序取得行锁，避免旧快照覆盖已取消或已完成的阶段。
恢复读取创建任务时持久化的模板版本；新版本上线后，历史任务仍按原定义执行。缺失或不兼容的
模板只隔离对应任务，保留原因和已有输出，不阻塞其它任务的恢复。

订阅每期执行记录和下一次执行时间在同一事务提交；队列暂时不可用时从数据库恢复待执行记录。
无效计划或失去权限的所有者会使对应订阅停用，并在工作台显示原因，修复后可重新启用。
资产派生、富集、发布和特征计算保留 JSON 数字原值，避免大整数实体 ID 被浮点转换改写；
批量写特征时每个不可变版本在一次请求中只解析和授权一次。

工作台按需加载资产选项，并隔离切换身份或选择对象前发出的旧请求。进度事件流暂时断开后
自动重连，轮询与事件触发的重复读取合并，终态和离开页面时释放连接。
身份卡的相关资产元数据改为一次批量读取，血缘构建每张表只读一次列信息；失效列保留历史治理
记录，但不会被标为仍可用的物理依赖。

## 环境和启动

业务表的唯一真源仍是成对版本迁移。本轮新增 000007–000011；必须先在目标环境备份并应用迁移，再启动新版服务。运行时不会自动建表。

```sh
cd services/cognida-go
set -a && source .env && set +a
go run ./cmd/migrate-db up
go run ./cmd/migrate-db version
go run ./cmd/server
```

Go 与 Python 的 `INTERNAL_HTTP_AUTH_TOKEN` 必须一致；Python 内部计算地址沿用 `evaluation.python_endpoint` 配置。对象存储沿用项目统一配置，采集原始内容、规范化内容及不可变资产均写入该存储。审计、流水线与订阅元数据在 MySQL，任务队列在 Redis，血缘在 Neo4j。

后台任务默认启动；API 验收实例可设置 `BACKGROUND_WORKERS_ENABLED=false`，队列数据会保留。`LINEAGE_ENABLED=true` 开启指标编译后的异步血缘写入；默认关闭时仍可手动补建。临时测试实例应使用独立数据库与 Redis 命名空间，避免恢复其他环境的未完成任务。

## 验证与回滚

本轮使用独立的 `cognida_review_20261003` 数据库，覆盖真实 MySQL、PostgreSQL、Redis、Neo4j、Milvus 及 Go→Python HTTP/MCP。新增迁移在另一个临时空库完成 up(11) → down(5步到6) → up(11)，各次 dirty=false；测试后删除该临时空库。没有迁移生产业务库。

后续按用户授权将本地开发库 `cognida` 从 3 升级至 11（dirty=false），先完整备份并在副本验证，
保留历史采集表后以标准迁移重建，回填后逐字段校验原数据。升级后的 12 项认证读取接口返回
200，跨租户请求返回 403。备份位置和历史 AutoMigrate 接入步骤见 `migrations/README.md`。

```sh
# MYSQL_DSN / DB_NAME 必须指向已迁移的独立测试库
go test ./internal/...
go test -tags=integration ./internal/...
go test -race ./internal/service/lifecycle ./internal/service/governance ./internal/service/agent/resultstore ./internal/service/knowledge

go run ./cmd/frontend-contracts -h
```

外部集成测试的服务/凭据要求见各测试文件和模块 README。新前端 DTO 由 Go 反射生成，CI 检查生成差异并运行类型检查。Go 静态检查工具需使用与项目匹配的 Go 工具链构建。

回滚先关闭后台执行并等待/取消在途任务，再回退应用。迁移 down 会删除新增模块数据或字段，只能在已备份且确认不再需要这些数据时执行；对象存储与 Neo4j 投影不由 SQL down 自动删除。默认保留数据并回退应用比删除新表更易恢复。

细节见 `internal/service/lifecycle/README.md`、`internal/service/governance/README.md`、`internal/service/collection/README.md` 与 Python `LIFECYCLE.md`。
