# Collection 首版运行保障

采集任务由 MySQL 记录状态、Redis job queue 投递，文档入库保留原有幂等 content hash。迁移 `000011_collection_artifacts` 添加可空语义（默认空字符串/0）的留存字段，旧任务可继续读取。

生产组合必须注入 `Service.SetArtifactStore(objectstore.Store)` 和 `SetRunSubmitter`。抓取成功后先把原始响应写入对象存储（JSON envelope，包含原始字节的 Base64、canonical URL、HTTP 状态、内容类型和 SHA-256），并提交 `raw_object_key/raw_hash/raw_size`，再规范化、留存 `normalized_object_key/normalized_size`、执行知识库入库。键包含租户和任务/运行/条目身份的哈希，不包含原始 URL 或任意路径。重复内容也保留本次抓取快照；拒绝规范化的响应保留 raw 证据。

存储或数据库临时失败返回 worker 重试，条目保持 pending。即使 raw 写成功后 DB 指针更新失败，重试也先查确定性 raw key 恢复 envelope；已保存 raw 的重试从对象存储读取并验证大小与 SHA-256，避免抓取已变化的页面。未配置对象存储的旧组合保持兼容行为（不写留存字段），不应据此声称启用了留存；生产 wire 注入配置好的 store。存储生命周期/删除策略由运维按租户留存要求设置，本实现不自动删除审计原文。

调度 `ClaimDueTasks` 在同一数据库事务中锁住到期且可启动的任务，创建 running Run，标记任务 running，再推进 schedule。调度进程在提交 Redis 前崩溃，run 仍会被每次 scheduler tick 的 recovery 扫描重新投递。Redis Submit 失败不把 run 标为 failed，错误返回调用方；再次 Start 会重新提交同一个 run。Dispatcher 聚合每项错误，不因一项失败跳过其他任务。取消/运行中的任务不再 claim；运行完成后到期的下一次调度才可启动。

生产 fetcher 注入共享 `politeness.Gate`，默认每主机并发 1、请求开始间隔 1 秒。gate 位于每个 HTTP RoundTrip，初始请求和重定向目标都受限，直到响应体关闭才释放并发许可。它在进程内共享；部署多个 worker 进程时应按域分片或使用共享上层限速。公网 HTTPS/SSRF/DNS 绑定校验仍由 safe fetcher 执行。

验证：

```bash
go test ./internal/service/collection/... ./internal/model/collection/...
# 先按项目 migrations 流程准备显式指定的测试数据库
# 以下测试用独立任务ID、临时目录及Redis key前缀，结束清理自己的内容
go test -tags=integration ./internal/repository/mysql -run TestCollection -count=1
```

真实集成覆盖：并发 claim 只创建一次、claim 后未提交仍可恢复、Redis 幂等投递、运行期间不重复调度、完成后下一周期可启动、对象字段往返。单元测试使用真实本地对象存储验证规范化写失败后的 raw 快照恢复、redirect host gate、Redis 提交错误返回与恢复状态。
