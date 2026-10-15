# 私人长期记忆

`user_memories` 保存用户通过管理页或显式工具写入的事实与偏好。所有仓储操作强制传入 `Scope{TenantID, UserID}`，没有租户共享或全局记忆。运行时不创建表；先通过迁移工具应用 `000012_personal_memories`。

API 在认证租户组下注册 `/memories`：列表使用 `cursor/page_size/category/q`，创建要求 `Idempotency-Key`，更新要求 JSON `expected_version`，删除要求 query `expected_version`。请求 JSON 不能提供用户或租户 ID。创建重试不覆盖现有内容；删除清除正文、向量及访问统计，保留幂等墓碑，旧创建请求不能复活已删除内容。

`NewService(repo, embedder, embeddingModelID)` 提供 CRUD 与召回。模型标识应包括 provider/model/version 或配置身份；无模型、向量失败或模型不匹配时使用词法检索。同模型的向量采用 cosine，结果注明排序方法、500 条最近更新候选上限与截断状态；分数不是置信概率。内容修正时旧向量一定清除，不复用过期语义。当前不做后台自动提取、用户资料共享或全库 ANN 检索。

`NewRecallHook(service)` 每轮按本人身份新读最多 3 条偏好与 5 条相关事实，用 JSON 包装不可信资料，只加入当前 user turn，不修改系统指令或持久化聊天历史。数据库/嵌入读取有期限；存储失败时不使用旧记忆副本。挂载该 Hook 的 Agent 必须禁用共享回答、few-shot 写穿缓存与租户共享的自动经验蒸馏路径，避免个人输出被重新发布。删除只影响长期记忆仓储及未来新召回，不删除用户已有聊天记录。

`tools.NewMemoryTools(service)` 提供 `memory_remember/memory_recall/memory_update/memory_forget`。写工具只用于用户明确要求保存、修正或遗忘的内容；工具从认证 context 读取所有者，调用参数不能指定其他用户。HTTP 响应 `Cache-Control: no-store`，工具本身不缓存结果、不记录正文审计。

验证：

```sh
go test -race ./internal/service/memory ./internal/service/agent/tools -run '^TestMemory'
# MYSQL_DSN 必须指向已应用迁移 12 的 cognida_review_* 独立库。
go test -tags=integration ./internal/handler -run '^TestMemoryHTTPIntegration' -count=1
```

真实 MySQL 集成覆盖持久向量与进程重建、跨用户/租户拒绝、幂等并发、版本竞争、cursor、SQL 通配符字面查询、删除清空及新会话召回刷新；只清理测试自行创建的随机租户数据。
