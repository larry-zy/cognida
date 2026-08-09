# 关键正确性与安全修复 - Design

## Context

本次 change 聚焦一批「生产可达 / 静默损坏」的缺陷：它们不抛错、不告警，却返回错误结果或直接构成认证绕过。这类问题在主路径上（登录用户即可触达、评测每次运行即触发），风险最高且最易被漏掉。

现状（只读抽查确认）：

- `cognida-go/internal/handler/middleware/auth.go`：`X-API-Key` 分支「任意非空 key 即放行为 user=default/tenant=1」；`CORSMiddleware` 反射任意 `Origin` 且同时带 `Allow-Credentials: true`；JWT 密钥无启动校验（占位符 `"your-secret-key"` 可用）。
- `cognida-go/internal/service/knowledge/knowledge_base_service.go`：`FindByID(ctx, id)`、`Update(ctx, kb)`、`GetChunks(...)` 等读写不带 `tenant_id` 边界，仅凭 `id` 即可跨租户命中（IDOR）。
- `cognida-go/internal/service/agent/memory/long_term.go`：`randomString` 用 `time.Now().UnixNano()%len` 生成，同一纳秒内所有字符相同、极易碰撞覆盖主键；`RetrieveMemoryUseCase.Execute` 在 goroutine 里对返回给调用方的同一 `*mem` 调 `RecordAccess()` 并 `Update`，与调用方读取构成 data race。
- `cognida-python/services/evaluation/graders/builtin/llm.py`：LLM 裁判用 `client.complete(...)`；若该方法不存在/签名不符，异常被 `except Exception` 吞掉后恒返回 `total_score=3.0`，大模型可能从未被真正调用；分数量纲混用（1-5 与 0-5 默认值混杂）。
- `cognida-web/src/utils/request.ts`：401 直接 `authStore.logout()`，从不尝试已实现的 `refreshAccessToken`；SSE 由各处裸 `fetch`/`EventSource` 自行拼装，未复用统一鉴权与错误处理。

这些都属于「先止血」级别，需按安全优先的顺序修复并可测。

## Goals / Non-Goals

### Goals

- 所有租户数据的读/写/删在 handler、service、repository 三层强制 `tenant_id` 边界，越权访问被拒（fail-closed）。
- 移除空 `X-API-Key` 认证分支；JWT 密钥缺失/占位/过短时启动即失败；CORS 改白名单。
- 会话鉴权同时强制 tenant 匹配，缺失身份时拒绝。
- 评测 LLM 裁判真正调用大模型、失败显式暴露（不静默固化固定分）、分数量纲统一。
- 流式评测进度真正下发客户端。
- Go core：记忆 ID 用 `crypto/rand`、消除记忆 data race、RAG 流式不丢块、流错误正确区分 EOF、StreamReader 不泄漏、SSE 感知客户端断开。
- 跨轮摘要 write-through 落库；记忆分支与非记忆分支共用同一前置流程（hooks/middleware）。
- 前端统一 `readSSE` 注入鉴权、401 走 refresh 再登出、4xx 停止无效重连。

### Non-Goals

- 不移除 `ExecuteEvaluation` / 不做评测执行架构重构（属另一 architecture change）。
- 不实现完整的 API Key 库校验体系（本 change 只删空分支，待后续 change 落地真实校验）。
- 不改动业务表迁移主流程、不新增 SQL 迁移文件（字段变更走 `cmd/migrate-db`）。
- 不引入 RBAC/细粒度权限模型（仅 tenant + user 纵深边界）。

## Decisions

### D1 租户隔离在三层强制（handler + service + repository 纵深）

**为什么**：单层易被绕过。约定分工：

- **Handler**：统一从 `GetTenantID(c)` 取当前租户，作为显式参数下传，绝不信任请求体里的 `tenant_id`。
- **Service**：签名带 `tenantID int64`（如 `FindByID(ctx, id, tenantID)`、`GetChunks(ctx, kbID, tenantID, ...)`），命中记录归属不符时返回「未找到 / 无权限」而非返回对象。
- **Repository**：SQL 一律 `WHERE id=? AND tenant_id=?`，作为最后一道纵深防御，即便上层漏传也不越权。

**BREAKING**：此前依赖越权访问的调用会被拒（预期行为）。

### D2 移除 X-API-Key 空认证分支

**为什么**：现分支「任意非空 key 放行为 user=default/tenant=1」等于无认证。替代方案：直接删除该分支；在真实 `api_keys` 表校验（查表 + 校验有效期/权限/租户归属）落地前，不提供 API Key 认证方式。请求无合法 `Authorization` 即 401。`DEV_MODE` 绕过保留但仅限开发环境，且打安全警告日志。

### D3 JWT 密钥启动强校验

**为什么**：占位符/短密钥可被离线爆破并自签合法 token。方案：应用启动装配阶段读取 `JWT_SECRET`，当缺失、等于占位符（如 `"your-secret-key"`）、或长度 < 32 字节时 `log.Fatal` 终止启动，杜绝弱密钥进入生产。`.env.example` 用明显占位并注释「必须替换」。

### D4 CORS 白名单

**为什么**：反射任意 `Origin` + `Allow-Credentials: true` 允许任意站点携带凭证发起跨域请求。方案：`CORSMiddleware` 维护 `AllowedOrigins` 白名单（从配置读取），仅当请求 `Origin` 命中白名单时才回写该 `Origin` 与 `Allow-Credentials: true`；不命中则不回写 `Allow-Origin`。默认不再是 `*`。

### D5 会话鉴权 fail-closed + tenant 匹配

**为什么**：`authorizeSession` 仅比对 userID，跨租户同 ID 仍可能放行；缺失身份时若默认放行则等于无鉴权。方案：`authorizeSession` 同时校验 `session.TenantID == ctxTenantID && session.UserID == ctxUserID`；上下文缺失 tenant/user 时直接拒绝。

### D6 评测 LLM 裁判改调 generate_json 并统一量纲

**为什么**：`complete(...)` 可能不存在或返回自由文本需再解析，失败被吞后恒返回固定分——大模型从未真正影响结果。方案：grader 改调 `LLMClient.generate_json(...)` 直接拿结构化维度分；调用失败/解析失败时 **抛出或返回显式错误标记**，不再 `except` 后返回 `3.0` 固化「看似正常」的结果。统一分数量纲为单一口径（如 0-1），所有维度、`total_score`、下游 `runner` 读取一致。`runner.compute_llm_judge_metrics_async` 按正确签名逐条调用并读 `result["dimension_scores"]`。

### D7 流式评测进度用 asyncio.Queue 下发

**为什么**：进度从 Redis/内部取出后未真正 `yield` 给 gRPC 流，客户端收不到。方案：`service.py` 用 `asyncio.Queue` 作为 producer/consumer 桥，worker 侧 `put` `Progress`，gRPC 流侧 `await queue.get()` 后 `yield`，直到收到终止哨兵。grader 注册失败记录日志、不静默把状态固化为 ready。

### D8 记忆 ID 用 crypto/rand

**为什么**：`randomString` 用 `time.Now().UnixNano()%len` 同一纳秒内每个字符相同，主键高概率碰撞导致覆盖他人记忆。方案：`randomString`/`generateID` 改用 `crypto/rand`（或 `github.com/google/uuid`）生成，保证唯一性与不可预测性。

### D9 消除记忆 data race（拷贝 vs repo 层原子自增）

**为什么**：`Retrieve` 把 `*mem` 返回给调用方后，又在 goroutine 里 `mem.RecordAccess()` + `Update`，两处并发读写同一结构体。方案二选一并落地其一：

- 首选：**repo 层 SQL 原子自增**——`Update`/或新增 `RecordAccess(ctx, id)` 用 `UPDATE ... SET access_count = access_count + 1, last_accessed_at = ?` 完成，不触碰返回给调用方的内存对象。
- 备选：返回前**深拷贝**，goroutine 只操作副本。

本 change 采用 repo 层原子自增，兼顾正确性与去重逻辑单点。

### D10 RAG 流式阻塞发送 + ctx.Done()

**为什么**：`pipeline.go` 用 `select { case ch<-x: default: }`，缓冲满时静默丢块，检索结果被无声吞掉。方案：改为阻塞发送并感知取消：`select { case ch<-x: case <-ctx.Done(): return ctx.Err() }`，既不丢块又能在客户端断开时及时退出。

### D11 流错误 errors.Is(io.EOF)

**为什么**：`eino_agent.go` 把所有 stream 错误一律当正常结束吞掉，真实错误被隐藏。方案：`errors.Is(err, io.EOF)` 才视为正常结束；否则记录并上报错误，让上层能感知失败。

### D12 StreamReader 生命周期（每轮 Close）

**为什么**：在循环内 `defer reader.Close()` 会把关闭堆积到函数返回，长循环期间累积泄漏。方案：每轮迭代取得 `StreamReader` 后在本轮末显式 `Close()`（或用小闭包限定 defer 作用域），不在外层循环累积 defer。

### D13 Go 端 SSE 取消感知

**为什么**：`streamAgentChunks` / `streamInternal` 的 `ch <- chunk` 无取消分支，客户端断开后上游仍持续空跑、占用资源。方案：所有向下游 channel 的发送改为 `select { case ch<-chunk: case <-ctx.Done(): return }`，客户端断开即终止上游生成。

### D14 跨轮摘要 write-through 落库

**为什么**：摘要只写 Redis（TTL 2h），重启/过期即丢，跨轮记忆不成立。方案：`UpdateSummary` write-through：先写 MySQL（持久），再写 Redis 缓存；`GetSummary` 先查缓存，未命中回源 MySQL 并回填，且区分「未命中（返回空）」与「错误（返回 err）」两种语义。

### D15 记忆分支统一 pre-processing

**为什么**：记忆分支绕过 beforeHooks/middleware，导致鉴权/审计/限流等横切逻辑在该分支缺失。方案：抽出统一前置流程函数，记忆分支与非记忆分支都先执行 beforeHooks/middleware，再进入各自后续。

### D16 前端统一 readSSE 与 401/refresh

**为什么**：各处裸 `fetch`/`EventSource` 各自拼装、绕过鉴权与统一错误处理；401 直接 logout 丢失可续期会话。方案：

- 抽 `readSSE(url, body, signal)` 共享工具：注入 `Authorization`（及 `X-Tenant-ID`）、对非 2xx 走统一错误/登出、透传 `AbortSignal`；删除 `chat/stream.ts` 重复实现。
- 401 处理改为：本地 `clearAuth` → 尝试 `refreshAccessToken` 重放原请求 → 失败才跳登录页；接通已实现但从未调用的 refresh。
- 评测进度 SSE 4xx 时停止重连（或改 fetch 流以携带鉴权头），不再无限重连。

## Risks / Trade-offs

- **[风险] IDOR 修复后越权调用被拒，可能打断此前依赖越权访问的前端/脚本** → 缓解：属预期 BREAKING；上线前审计现有调用是否传对 tenant，前端统一走注入 `X-Tenant-ID` 的请求层。
- **[风险] JWT 强校验导致未配置密钥的环境启动失败** → 缓解：`.env.example` 显式标注必填，部署脚本预检；这是刻意 fail-closed，避免弱密钥进生产。
- **[风险] CORS 白名单遗漏合法前端域名导致跨域被拦** → 缓解：白名单从配置读取，支持多域名；本地开发通过 Vite proxy 走同源不受影响。
- **[风险] LLM 裁判改抛错后，评测任务可能从「有分」变「报错」** → 缓解：这是暴露真实失败的正确行为；grader 层返回明确错误原因，runner 记录并计入失败项而非固化 3.0。
- **[风险] 记忆改 repo 层原子自增，需 repo 支持对应 SQL** → 缓解：若 repo 暂不支持，回退到「返回前深拷贝」保证无 race。
- **[风险] SSE 阻塞发送若下游长时间不消费可能阻塞上游** → 缓解：配合 `ctx.Done()` 与合理缓冲；客户端断开即取消，不会无限阻塞。
- **[风险] 前端 refresh 重放引入重复请求或竞态** → 缓解：refresh 单飞（并发 401 共享同一 refresh promise），成功后统一重放。

## Migration Plan

按「安全优先」顺序上线，每步可独立回滚：

1. **安全（最先）**：JWT 强校验 + 移除空 API Key 分支 + CORS 白名单 + 会话 fail-closed。先配置好 `JWT_SECRET` 与 `CORS_ALLOWED_ORIGINS` 再发布，避免启动失败。回滚：恢复中间件旧逻辑。
2. **租户隔离**：service 签名加 `tenantID`、repo 加 `WHERE tenant_id=?`、handler 传 `GetTenantID(c)`。灰度观察被拒请求量，确认无合法调用被误伤。回滚：临时放宽 repo where（不推荐）。
3. **评测正确性**：grader 改 `generate_json` + 统一量纲 + runner 修调用 + 进度 Queue 下发。回滚：Python 侧独立部署，可单独回退。
4. **Go core**：记忆 ID/race、RAG 阻塞发送、EOF 区分、StreamReader、SSE 取消。回滚：按文件粒度回退。
5. **跨轮记忆**：摘要落库需先跑 `cmd/migrate-db` 同步表结构，再发布 write-through 逻辑。回滚：GetSummary 回源逻辑幂等，回退安全。
6. **前端 SSE**：`readSSE` 统一 + 401/refresh + 4xx 停重连。与后端 SSE 取消感知配套发布。回滚：前端独立回退。

回滚总原则：各层改动尽量按文件/服务粒度隔离，安全项与租户项优先且不可逆方向为「更严」，回退到「更严」不会产生越权。

## Open Questions

- JWT 密钥最小长度阈值取 32 字节是否足够，是否需要强制 HMAC-SHA256 以上算法校验？
- CORS 白名单的来源：走 `CORS_ALLOWED_ORIGINS` 环境变量（逗号分隔）还是配置中心？多租户自定义域名如何纳入？
- LLM 裁判统一量纲选 0-1 还是 0-100？需与前端展示、历史评测数据的兼容性对齐。
- 记忆 `RecordAccess` 原子自增：repo 层是否已有对应字段（access_count/last_accessed_at），是否需 `cmd/migrate-db` 加列？
- 前端 refresh 单飞的实现位置：放在 `request.ts` 拦截器还是 `stores/auth.ts`？
