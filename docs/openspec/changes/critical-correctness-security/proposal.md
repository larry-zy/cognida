# 关键正确性与安全修复 - Proposal

## Why

本次全量扫描发现一批"生产可达 / 静默损坏"的功能与安全缺陷，它们不报错却返回错误结果、或直接构成认证绕过，是当前风险最高、最易被漏掉的一类问题：

- **认证/授权形同虚设**：跨租户 IDOR、X-API-Key 空认证、JWT 默认密钥——任意登录用户可越权读写他人租户数据，或自签合法 token。
- **评测静默损坏**：LLM 裁判调用不存在的方法被吞异常后恒返回固定分，大模型从未被真正调用；流式评测进度取出后不发客户端。用户拿到"看似正常"的错误结果。
- **Go core 数据正确性与资源泄漏**：非随机 ID 高概率碰撞覆盖记忆、返回对象被 goroutine 并发写 data race、RAG 流式满缓冲无声丢块、流错误当 EOF 吞掉、StreamReader 循环内 defer 泄漏。
- **跨轮记忆不成立**：摘要只写 Redis/TTL 2h 不落库、记忆分支绕过全部 hooks/middleware。
- **SSE 全链路缺陷**：前端裸 fetch 绕过鉴权、401 不处理、EventSource 无限重连；Go 端不检测客户端断开导致上游持续空跑。

现在做，是因为这些缺陷已在主路径上（登录用户即可触达、评测每次运行即触发），属于"先止血"级别。

## What Changes

### 安全（最高优先级）
- **修复跨租户 IDOR**：知识库/文档/分块的读取·更新·删除（`FindByID`/`Update`/`GetChunks` 等）一律强制 `WHERE id=? AND tenant_id=?`，Handler 统一传入 `GetTenantID(c)`；仓储层加 tenant/user 纵深防御。**BREAKING**（越权访问被拒）。
- **移除 X-API-Key 空认证分支**：删除"任意非空 key 即放行为 user=1/tenant=1"的中间件分支，未真正实现库校验前不提供该认证方式。
- **JWT 密钥强校验**：无 `JWT_SECRET` 或等于占位符/长度不足时启动即 `log.Fatal`，禁止默认 `"your-secret-key"`。
- **CORS 收紧**：`Access-Control-Allow-Origin` 改为白名单匹配，不再反射任意 Origin 同时带 `Allow-Credentials`。
- **会话鉴权 fail-closed**：`authorizeSession` 同时强制 tenant 匹配，缺失身份时拒绝而非放行。

### 评测正确性
- **修复 LLM 裁判 grader**：`graders/builtin/llm.py` 改调正确的 `LLMClient` 方法（`generate_json`），去掉吞异常恒返回 3.0；统一 LLM 裁判分数量纲（0-100 或 0-1 单一口径）。
- **修复 runner 的 LLM 评判调用**：`compute_llm_judge_metrics_async` 按正确签名逐条调用并读 `result["dimension_scores"]`。
- **打通流式评测进度**：`service.py` 用 asyncio.Queue 把 `Progress` 真正 yield 给 gRPC 流；grader 注册失败记录日志、不静默固化 ready。

### Go core 数据正确性与资源
- **修复记忆 ID 生成**：`randomString` 改用 `crypto/rand` 或 `uuid`，消除主键碰撞。
- **消除记忆 data race**：返回前拷贝或把 `RecordAccess` 收敛到 repo 层 SQL 原子自增。
- **RAG 流式不丢块**：`pipeline.go` 的 `select{default}` 改为阻塞发送 + `ctx.Done()` 感知取消。
- **流错误正确区分**：`eino_agent.go` 用 `errors.Is(err, io.EOF)` 区分正常结束与真实错误，后者上报。
- **修复 StreamReader 泄漏**：每轮迭代显式 `Close()`，不在循环内累积 defer。
- **SSE 感知取消**：Go 端 `streamAgentChunks` 与 streamInternal 的 `ch<-` 全部 `select { case …: case <-ctx.Done(): return }`，客户端断开即终止上游。

### 跨轮记忆
- **摘要落库**：`UpdateSummary` write-through 到 MySQL，Redis 仅作缓存；`GetSummary` 区分未命中与错误。
- **记忆分支统一 pre-processing**：抽出统一前置流程，记忆分支与非记忆分支都执行 beforeHooks/middleware。

### 前端 SSE 与鉴权
- **401 处理正确化**：不再递归 logout，本地 clearAuth → 尝试 refreshAccessToken 重放 → 失败跳登录页；接通已实现但从未调用的 refresh。
- **SSE 走统一鉴权**：抽 `readSSE(url, body, signal)` 共享工具注入 auth header、解析非 2xx 走统一错误/登出、透传 AbortSignal；删除 `chat/stream.ts` 重复实现。
- **评测进度 SSE 停止无效重连**：4xx 时停止重连，或改 fetch 流以携带鉴权头。

## Capabilities

### New Capabilities
- `tenant-isolation`: 租户隔离契约——所有租户数据的读写删强制 tenant_id 边界校验（handler + service + repository 纵深）。
- `auth-hardening`: 认证加固契约——移除空 API Key 分支、JWT 密钥强校验、CORS 白名单、会话鉴权 fail-closed。
- `sse-auth-lifecycle`: SSE 鉴权与生命周期契约——前后端统一鉴权注入、断线/取消/重连语义、401 处理。

### Modified Capabilities
- `evaluation-graders`: LLM 裁判 grader 真正调用大模型、统一分数量纲、失败不静默固化。
- `evaluation-progress`: 流式评测进度真正下发客户端。
- `agent-core`: 流式错误区分、StreamReader 生命周期、SSE 取消感知。
- `chat-service`: 会话鉴权强制 tenant 匹配、跨轮摘要落库、记忆分支统一 hooks。
- `sse-helper`: SSE helper 统一鉴权与取消。

## Impact

- **cognida-go**：`internal/handler/{knowledge_base_handler,agent_handler,session_handler}.go`、`internal/handler/middleware/auth.go`、`internal/service/knowledge/knowledge_base_service.go`、`internal/service/chat/session_service.go`、`internal/service/agent/framework/eino_agent.go`、`internal/service/agent/memory/{long_term,manage}.go`、`internal/service/knowledge/pipeline/pipeline.go`、`internal/repository/mysql/chat_repo.go`、`internal/infrastructure/config/config.go`、`.env.example`。
- **cognida-python**：`services/evaluation/graders/builtin/llm.py`、`services/evaluation/runner.py`、`services/evaluation/service.py`、`services/evaluation/metrics/llm_judge.py`。
- **cognida-web**：`src/utils/request.ts`、`src/stores/auth.ts`、`src/api/agent/index.ts`、`src/api/chat/stream.ts`、`src/utils/sse.ts`。
- **测试**：新增 middleware/auth 与 tenant 隔离单测、评测 grader 调用真实性测试、记忆 ID/race 的 `-race` 测试、SSE 取消测试。
- **数据/兼容**：IDOR 修复后此前依赖越权访问的调用会被拒（预期）；移除 ExecuteEvaluation 不在本 change 范围（属架构 change）。
