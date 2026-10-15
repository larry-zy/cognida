# 实现与验证记录

日期：2026-10-15。开发直接在 main，既有用户改动与独立文档未纳入本次提交。

## 可用入口与行为

- `/memories`：本人事实/偏好管理，创建幂等、版本修正/遗忘、cursor 列表与召回预览。迁移 12 建立 `user_memories`；所有仓储请求绑定 tenant + user。删除清正文与向量，旧创建键不会复活数据。
- 默认助手、知识库助手、聊天助手、Data Agent 每轮新读有界个人背景。四个显式记忆工具注册到默认/RAG/Data Agent。共享回答/few-shot 缓存、共享经验召回和自动经验/全局技能蒸馏在私人记忆生效时停用；记忆路由跳过通用响应重放缓存，依赖自己的持久幂等。通用 SQL/画像/导出/ETL 不允许访问私人记忆表。
- `/research`：子问题规划、授权 RAG/图谱/可选网络来源、综合、引用 ID 与逐字摘录核验。JSON 与 SSE 共用执行，终态含完整报告；无证据失败、部分失败有说明、可取消。引用核验不等于独立验证来源真实性或结论语义。
- RAG 的 rewrite/HyDE/multi-hop 工具复用同一受治理检索能力；每跳保留授权范围，假设文档不作为证据。图谱节点到 chunk 的读取经过真实 Neo4j 与 MySQL 双重范围过滤。
- 专家/流水线页面按真实 stage ID 传递参数。文本发布必须选择当前租户有效知识库，提交前及审批后写入前均复查。独立产出可打开分析证据。
- 分析和订阅报告的 `prompt` 实际调用 Python `/api/v1/lifecycle/analyze`，确定统计保留，模型解读与可定位证据分开。输出通过持久 checkpoint/已有 Insight 恢复，通知写失败后不会重新生成冲突内容。
- Python 统一入口监督 HTTP、计算 HTTP、MCP、gRPC；就绪检查包含存储与实际监听状态，必需服务启动失败会关闭同组服务。内部 HTTP/gRPC 认证由 Compose 同时注入两端。

## 验证结果

- Go：最终冻结代码后再次执行 `go test ./internal/...`，全量内部包通过；生命周期、治理、组合根集成，以及记忆/研究 HTTP、SQL 防护、Neo4j 真实集成均复跑通过。记忆、SQL 工具和 Result Store 的并发竞争检查通过。
- MySQL/Redis：专用 `cognida_review_*` 库和随机 Redis 前缀验证记忆持久化、用户/租户隔离、幂等/版本冲突、删除；生命周期发布预检/撤权、流水线、目标报告、分析重试与通知故障恢复均通过。所有测试只清自身 scope。
- 图谱：真实 Neo4j 随机 namespace + MySQL 验证节点到原文、范围隔离、错误跨库 chunk 引用与停用/删除过滤；三个优化器也经过真实数据库授权/检索。
- Python：离线模式 `HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 pytest tests/ -m 'not integration'`：650 项通过，18 项外部/集成用例未在这次命令执行。离线模式避免既有评测测试下载 Hugging Face 模型。
- 四协议真实监听/鉴权/取消/清理与既有 HTTP、gRPC、MCP 回归：43 项通过。占用端口能传播启动失败并关闭兄弟监听；挂起工具取消后端口关闭。
- Web：27 个测试文件、113 项测试通过；Vue 类型检查和生产构建通过。保留既有 GraphView 包体积提示。
- 真实 Go 组合根：临时端口 + 隔离数据库成功启动，注册研究/记忆工具；真实注册用户后记忆 CRUD、召回及研究路由校验通过。
- 迁移：另一专用空库完整 up → down 1 → up，最终 `version: 12 dirty: false`；迁移测试库已删除。
- Compose：缺内部凭据时正确拒绝解析；提供仅用于解析的测试占位后 `docker compose config -q` 通过。
- 独立 Review：覆盖私人数据缓存与 SQL 旁路、分析恢复、RAG 工具 schema、多跳 JSON 解析、检索失败语义；确认问题均增加修复/回归，最终独立复审批准。中文订阅目标按 4000 字符统一校验，边界回归通过。

## 环境限制

- 已实际尝试配置中的 DeepSeek：分析及 DeepResearch 的规划请求返回 HTTP 402 / Insufficient Balance。因此真实模型完整报告未通过，不能据测试替身宣称模型质量验收完成；生产需要有效模型凭据与额度。失败路径和受控模型全流程已验证。
- 已尝试 Docker 生产构建，但 Docker Hub 在基础镜像 token/manifest 拉取阶段超时；尚未验证完整容器构建。Compose 配置和本机相同运行入口已验证。

## 部署

先备份业务库，再按 migrations/README.md 手动运行 `go run ./cmd/migrate-db up`。不得运行时自动建表。设置 `.env.docker.example` 中两个随机内部服务凭据，Go 与 Python 各使用对应同值；配置聊天模型、Python 分析模型与可选搜索服务。`/health` 只代表存活，编排依赖 `/ready`。

回滚应用时可保留新表；`down 1` 会删除全部个人记忆，仅用于空库测试或明确授权的数据回退。

## 清理

已关闭本任务启动的 Go/Python 临时监听，删除两个专用 MySQL 测试库以及临时凭据、服务二进制。原有开发服务保留。
