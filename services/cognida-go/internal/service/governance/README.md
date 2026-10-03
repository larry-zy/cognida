# 治理服务首版

服务提供站内报告订阅和指标监控、三种标注与人工审核、规则打标与人工纠正、带版本的特征定义和时点查询。HTTP 入口由 `GovernanceHandler` 注册，MySQL 保存任务与审计状态，Redis 通用工作队列执行报告和打标；源数据与输出使用 lifecycle 不可变引用和 object store。

## 权限与写入

所有请求必须有有效 tenant/user，派生标注正文、打标结果和特征值每次读取都重新检查源资产权限。跨租户不可访问；订阅、运行和通知还限制归属用户。批量列表包含无权访问的源时整体返回 403，不返回部分敏感数据。改进建议仅创建者或管理员可读，普通成员列表只包含自己的建议；私聊来源与证据权限每次读取时复查。创建重放也检查原记录的源权限。订阅配置与标注审批要求 admin；读取敏感源还要求 sensitive。

创建可传 `Idempotency-Key`；手工运行订阅必须传入非空键（最长 55 字符）。修改传 `expected_version`，冲突返回 409。特征定义更新与不可变版本写入在同一事务内完成；版本列表的 `version` 是定义版本。特征值按 tenant/feature/version/entity/timestamp 幂等，时间规范到数据库微秒精度，JSON 按语义比较（包含大整数、数值表示和键顺序）。同坐标不同值返回 409，整批回滚。

## 固定快照和动态订阅

默认 `input.asset_ref_id` 固定指向已审批发布快照，所有运行分析同一版本。需要持续观察新数据时显式配置：

```json
{
  "asset_ref_id": "<published-asset-uuid>:1",
  "time_column": "day",
  "value_column": "revenue",
  "refresh": {
    "mode": "latest_published",
    "source_type": "datasource",
    "source_id": "<datasource-id>/<schema>/<table>"
  }
}
```

`source_type` 支持 datasource/web/knowledge/collection/result/evaluation；`source_id` 必须与 anchor 自身或祖先 provenance 中的来源完全相同。注册目录资产的 datasource 来源 ID 使用 catalog asset ID；web 使用规范 URL；其它类型使用实际源对象 ID。客户端应从真实资产 provenance 选择，不能凭显示名拼接。

每次执行只选择同租户、当前用户可读、已发布且 `policy.approved=true` 的匹配来源，按创建时间选最新版本。不会自动采集来源，也不会使用未发布的原始数据。候选查询先按发布/审批过滤，使用 `(asset_id,version)` 复合游标；最多检查 10,000 个发布候选和每候选 128 个祖先，超过明确报错，不默默回退旧数据。运行在生成输出之前持久化 `resolved_ref_id`，故障重试沿用原选择，通知按运行幂等。

Cron 是五字段，支持数字、列表、范围与步长（`5/10` 从 5 到字段上限每隔 10）。星期 0/7 均为周日；月日和星期都为具体限制时取 OR，存在星号字段时取 AND。使用 IANA timezone，按真实 UTC 分钟遍历，因此夏令时重复时刻可各运行一次，缺失时刻跳过；最长搜索八年以覆盖闰日。调度器错过多个触发点时补一次并推进到下一未来时刻，不进行无限历史回放。语义参考 [crontab(5)](https://www.man7.org/linux/man-pages/man5/crontab.5.html)。

调度领取在一个 MySQL 事务里创建当期运行并推进下一触发时间，提交后才投递 Redis。队列不可用时运行保持 queued，由恢复器重新投递；不会让最早一页订阅长期阻塞后续任务。并发调度器通过版本 CAS 竞争，失败事务不遗留运行。旧配置无效或创建者访问不可用时自动停用，保留 `schedule_error`（`invalid_schedule_configuration` / `owner_access_unavailable`）；修复权限或配置后，创建者需以管理员身份显式保存启用，成功保存清除错误。

## 指标与归因

`zscore` 至少需要四个观察周期；设置 `time_column` 时，同一时间的行按 value 求和，然后以最后一期对比此前各期的均值与总体标准差。没有时间列时按行序观察。恒定历史基线遇到变化直接触发，zscore 返回 null。

`attribution_dimensions` 最多 10 个且必须是真实列，使用维度时必须提供时间列。返回每个维度组合最后一期值、历史均值、差值及其占总差值比例；缺失组合视为 0。首版适用于可加指标；这是描述性贡献分解，不推断因果。阈值范围 `(0,20]`。

## 标注、打标与特征

标注支持 classification/named_entity/relation，输入与行文本在创建记录前验证。实体偏移按 Unicode 字符位置，审核仅导出 approved 的 JSONL。规则预标注不覆盖人工修改或已审批项，保留方法、证据和置信信息。

打标复用 Python `/api/v1/lifecycle/enrich` 的真实规则结果，保存完整 evidence，人工改动明确标记 human/manual，并重置为 pending。管理员通过 `POST /tagging-results/:id/review` 提交 decision、comment 和 expected_version，持久化 approved/rejected、审核人和说明；仅改审核字段，保留原始 evidence、provenance 与置信信息。标签 schema 最多 30 项，每项 80 字符；不返回伪造质量分。表输入每行转为带列名文本，最多 1,000 行，每行最多 200,000 字符，空行文本拒绝。每次工作尝试最多五分钟，打标逐项持久化检查点，重试跳过已完成结果并保留人工修改。Python 不可用时任务按通用队列重试并最终标记 failed。

特征定义首版为 field 投影，支持 numerical/categorical/vector，源字段和 entity_field 必须存在。计算保留源 ref 与源观察时间，查询支持 as_of；没有符合时间的数据就不返回该实体。该模块提供版本元数据和 SQL 特征值；图像向量检索走 Python CLIP/Milvus 独立空间。

## 验证

特征投影保留 JSON 大整数的原始数值，避免把相邻实体 ID 转成相同浮点数。批量写入每次请求只解析和授权各不可变特征版本一次，避免按实体重复读取整个源对象。通知并发标记已读是幂等操作；重新预标注会清除旧审核人和说明，重新等待审核。

核心单测覆盖 cron、维度贡献、版本展示、输入预校验、来源刷新与敏感派生读取。`go test -tags=integration ./internal/service/governance -run TestGovernanceIntegration` 使用显式隔离 `MYSQL_DSN`（库名必须以 `cognida_review_` 开头）、真实 Redis 独立前缀及临时本地对象目录，结束精确清理自有测试行与队列。

集成覆盖 HTTP 特征版本/查询、JSON 规范化重放、跨租户和敏感权限、特征事务失败回滚、标注与打标、动态订阅终态写入失败重试检查点和单次通知。打标集成使用确定性 compute fixture 验证字段契约；真实 Go→Python 链路另由 lifecycle 集成验证。
