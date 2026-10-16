# Agent 与基础能力收尾验收

验收日期：2026-10-16。范围为 Agent 配置、图谱关系统计、语义缓存孤儿清理、Skill 标签筛选和旧 Agent 占位入口收尾。

## 实现结果

| 能力 | 实际行为 | 说明 |
| --- | --- | --- |
| Agent 配置 | MySQL 按 `tenant_id + user_id` 保存；版本冲突检查、持久幂等回执、部分更新及 null 重置；新知识问答与交互研究读取个人默认值 | [配置契约](../../services/cognida-go/internal/service/agent/preferences.md) |
| 图谱关系统计 | MySQL 验证知识库权限，Neo4j 对完整范围聚合；单条新增关系写入范围字段，缺少节点或提交失败不再假成功 | [统计与存量数据边界](../../services/cognida-go/internal/service/knowledge/graph_relation_stats.md) |
| 语义缓存孤儿清理 | 完整枚举、Redis 主记录核验、原子清理标记、精确删除身份和实际计数；异常不假装成功 | [清理契约与恢复](../../services/cognida-go/internal/infrastructure/cache/cleanup.md) |
| Skill 标签筛选 | 两条实际工具路径使用相同过滤规则；category 与 tags 取交集，标签默认 any，可显式 all；非法参数拒绝执行 | `internal/service/agent/tools`、`internal/service/agent/skills/tools` |
| 旧 Agent 框架 | 删除没有消费者的旧计划拆解、派发和注册代码；保留实际协作结果协议；可执行工具注册拒绝 nil 和无执行能力的占位项 | `internal/service/agent/collaboration`、`internal/service/agent/framework` |

个人配置不授予工具或审批权限；已经启动的运行固定使用原快照。受治理研究任务继续遵守控制器预算。

## 验证记录

命令均在 `services/cognida-go` 执行。真实存储使用本次独立的 MySQL 8、Neo4j 5、Redis 7 和 Milvus v3.0-beta 容器，未修改共享实例。

| 检查 | 结果与覆盖 |
| --- | --- |
| `go test ./...` | 通过：105 个有测试的包，另有 50 个无测试文件的包完成编译 |
| 配置、适配器、工具循环、幂等中间件的定向 `-race` | 通过：字段边界、显式零分数、null 重置、会话优先级、研究预算隔离和运行快照 |
| 两条 Skill 工具路径、工具注册、协作协议的四包 `-race` | 通过：生产注册覆盖路径、组合过滤、非法参数、不可执行工具与禁用工具 |
| 图谱 service、Neo4j repository、handler 三包 `-race` | 通过：授权、聚合、关系写入和失败返回 |
| 缓存相关四包 `-race` | 通过：完整预检、并发写入保护、真实计数、明确失败重试与未知回执保留 |
| 配置＋图谱真实存储 HTTP 集成，`-race` | 最终合并复跑通过，3.308s；两个测试均实际执行，未跳过 |
| Redis＋Milvus 集成，`-race` | 通过，10.270s；保留有效缓存和其他租户数据，删除确实孤立的旧向量，异常和并发保护符合约定 |
| 版本化迁移 | 空库正常应用至 36；迁移 36 回退至 35，再应用至 36，均 `dirty: false`；未使用运行时自动建表 |
| 独立 Code Review | 四个实现分组均通过交叉 Review；修复评审发现的问题后重新验证 |

HTTP 集成的具体场景：

- 配置：不同用户／工作空间隔离，相同键与载荷重放，不同载荷冲突，8 个并发更新只有一个成功，重建服务后读取持久配置，以及实际执行服务读取新默认值并保留旧运行上下文。
- 图谱：初始 1,004 条关系完整统计，经 HTTP 创建后为 1,005；跨范围关系排除；同租户公共知识库可读而非 owner 不可写；知识库禁用／删除后拒绝访问；空图、缺节点、损坏分块引用均有明确结果。

复跑真实存储测试前，配置专用环境并正常执行 `go run ./cmd/migrate-db up`。MySQL 数据库名必须以 `cognida_review_` 开头，图谱地址须为本机隔离端点。测试所需环境变量：

- `COGNIDA_AGENT_CONFIG_TEST_DSN`
- `COGNIDA_GRAPH_TEST_DSN`、`COGNIDA_GRAPH_TEST_NEO4J_URI`、`COGNIDA_GRAPH_TEST_NEO4J_USER`、`COGNIDA_GRAPH_TEST_NEO4J_PASSWORD`、`COGNIDA_GRAPH_TEST_NEO4J_DATABASE`
- `COGNIDA_CACHE_TEST_REDIS_ADDR`、`COGNIDA_CACHE_TEST_MILVUS_ADDR`；有认证时另设 Redis password／Milvus token 对应测试变量。

```bash
go test -tags=integration -race ./internal/handler \
  -run '^(TestAgentConfigMySQLHTTPPersistenceIsolationReplayAndRuntime|TestGraphRelationStatsMySQLNeo4jHTTP)$' -count=1 -v

go test -tags=integration -race ./internal/infrastructure/cache \
  -run '^TestOrphanCleanupRedisMilvusIntegration$' -count=1 -v
```

未配置专用环境时这些集成测试会跳过，因此单独一次 `go test ./...` 不代表完成真实存储验收。HTTP 测试注入明确的认证身份；配置测试使用 TCP HTTP，图谱测试通过 HTTP handler 请求／响应链路连接真实存储；不据此宣称登录、模型回答质量或浏览器页面已验收。本次未重新运行之前的浏览器全流程套件。

## 上线与运维边界

- 上线前应用迁移 `000036_agent_preferences`。回退会删除个人默认配置和更新回执，应先备份并停止对应写入。
- 历史缺少租户／知识库范围的图谱关系不会被自动认领，需审计后另行迁移；统计不会将其混入当前用户数据。
- 缓存清理要求显式租户，目前没有自动接入生产调度或 HTTP 入口。未知删除结果保留清理标记并要求核对，不能按超时直接解除。
- 本次专用的 4 个测试容器、4 个匿名卷和临时凭据已删除，6 个测试端口已关闭；原有 17 个容器及其运行状态保持不变。
