# 个人 Agent 默认配置

`GET /api/v1/agent/config` 与 `POST /api/v1/agent/config` 使用已认证身份的
`tenant_id + user_id` 保存私人配置。请求体不能指定其他主体，不修改全局模型、工具权限或审批规则。
启用新版本前须通过 `cmd/migrate-db up` 应用迁移 `000036_agent_preferences`；运行时不自动建表。

GET 返回 `data.version`、`data.agentic_rag` 和 `data.deep_research`。
首次读取版本为 0，字段为 `null`，表示沿用现有运行时默认值。

更新先读取当前版本，再提交补丁和独立的 `Idempotency-Key`：

```http
POST /api/v1/agent/config
Content-Type: application/json
Idempotency-Key: personal-config-edit-1

{
  "expected_version": 0,
  "agentic_rag": {
    "max_iterations": 3,
    "default_top_k": 8,
    "min_score": 0,
    "rerank_enabled": false,
    "alpha": 0.5
  },
  "deep_research": {
    "max_sub_questions": 2,
    "max_concurrency": 1,
    "max_sources": 12,
    "timeout_seconds": 90
  }
}
```

省略字段保持原值；显式 `null` 清除该字段覆盖。空补丁、未知字段、错误类型、超出范围、缺少版本或幂等键返回 400。
版本过期或同一键提交不同内容返回 409。成功更新原子保存完整快照和回执，版本增加 1。
同一主体、同一键与相同补丁重试返回原回执；之后已经发生新更新时，重放结果仍是原版本，须再 GET 获取最新状态。
不同用户或工作空间可独立使用同一键。数据库异常返回 503，不假装保存成功。

| 字段 | 允许值 | 实际生效位置 |
| --- | --- | --- |
| `agentic_rag.max_iterations` | 1–20 或 null | 知识 Agent 的每次工具循环；实际取该值与 Agent 预设上限的较小值 |
| `agentic_rag.default_top_k` | 1–100 或 null | 授权知识检索候选数 |
| `agentic_rag.min_score` | 0–1 或 null | 检索分数阈值；0明确关闭分数过滤，不被工具默认值覆盖 |
| `agentic_rag.rerank_enabled` | boolean 或 null | 跨知识库候选合并后的重排开关 |
| `agentic_rag.alpha` | 大于0且不超过1，或null | 现有混合检索融合权重；0在底层表示默认值，因此本接口拒绝0，恢复默认请用null |
| `deep_research.max_sub_questions` | 1–5 或 null | 交互研究的问题拆分上限 |
| `deep_research.max_concurrency` | 1–3 或 null | 交互研究的并发检索上限 |
| `deep_research.max_sources` | 1–30 或 null | 交互研究保留的来源上限 |
| `deep_research.timeout_seconds` | 30–300 或 null | 交互研究的总时限 |

知识 Agent 在新运行开始时读取一次快照。会话中明确保存的检索参数优先于个人默认值；Data Agent 保持独立配置。
交互研究的请求级显式选项优先于个人研究默认值。受治理研究任务继续使用控制器确定的预算，不套用后来修改的个人默认值。
已启动运行不随配置更新改变；权限、来源有效性和预算检查仍按原路径执行。

本接口只公布实际接入消费者的字段。旧占位响应中的名称、描述、搜索凭据及未生效的评估/事实检查开关不作为可写项，提交这些字段会明确拒绝。
