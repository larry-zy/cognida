## Context

以 proposal 的授权范围为准。在 main 的 8fff99b 基线上实现，保留已有不可变资产、审批、作业恢复和租户隔离；启用私人记忆时暂停共享经验蒸馏与召回，避免私人回答被共享。现有 service/model/repository 分层和版本化迁移继续有效。

## Goals / Non-Goals

**Goals:** 用户能管理本人长期事实/偏好，新会话能召回；Agent 能组合授权 RAG 优化器；研究页面能执行有界多阶段研究并展示真实来源；网页/文本能经处理审批发布并按目标分析；Compose 能启动相关服务。

**Non-Goals:** 本轮不做跨用户共享记忆、自动隐式采集个人事实、无限自主网络爬取、任意生成 DAG、自动生产策略变更或 Agentic RL。

## Decisions

1. 记忆使用 MySQL user_memories 与成对 000012 迁移，所有仓储操作显式 tenant/user scope；版本 CAS、幂等创建、cursor 分页。删除清除正文/向量并保留幂等墓碑。提供词法检索；配置 embedder 时使用同模型向量在有界本人候选上 cosine 排序并说明限制。管理 API 和工具写入，before hook 每轮新读有界偏好/事实；不使用可能缓存个人输出的共享回答/示例缓存。
2. RAG 优化器复用既有模型及 RAGQueryService，scope 来自认证上下文而非工具参数。图谱适配从租户/知识库命名空间取节点关联 chunk 再读取真实片段，缺依赖/错误可见。
3. DeepResearch 为规划→并发研究→综合→引用核验。复用 ToolRegistry 的 RAG/graph/web 能力，不创建原始网络旁路。限制子问题、并发、来源、上下文与总时间；零证据不能成功，部分失败有 warnings。JSON 与 SSE 共用执行路径，终局帧包含报告，取消沿 ctx 传播。研究入口校验用户会话及知识库范围。
4. 工作台使用注册模板的 stage ID 构造 options.stage_options，独立专家使用 options。发布目标在前后端准入检查；审批后仍复查目标权限。分析目标经现有 Python 受认证 Compute 增加 analyze 操作执行，确定统计作为基线，模型生成有 evidence ID 的解释且显式标记语义未验证。空目标保留确定统计行为；缺模型不能伪称已执行目标。
5. Compose 明确配置共享 HTTP/gRPC 认证、模型和 MCP 地址/端口，统一入口启动 MCP 并暴露 readiness；必需服务错误不能健康降级。测试只使用独立 review 库、随机缓存前缀及自有临时进程。

## Risks / Trade-offs

- 记忆污染/越权 → 私有 scope、显式写入、低权限数据边界、fresh recall 和缓存隔离。
- 模型编造引用 → 证据 ID 白名单、原文摘录校验、引用覆盖/限制展示，不把文本解释冒充客观验证。
- 长任务成本/取消 → 有界并发、预算、请求期限及取消传递。
- 模型缺配置 → API/任务明确失败，不降级为假完成。
- 迁移影响 → 新增表，独立库 up/down/up 测试；现有业务库不自动迁移。

## Migration Plan

测试库应用迁移 12；完成单元、真实 DB/API、前端、跨服务和 Review 后提交 main。部署前备份并手工运行 migrate-db up，设置共享密钥与模型配置。回退应用可保留新表；删除数据的 down 不在业务库执行。

## Open Questions

用户已确认采集范围。实现中只按现有注册工具和可用配置启用外部搜索；缺能力时报告原因。
