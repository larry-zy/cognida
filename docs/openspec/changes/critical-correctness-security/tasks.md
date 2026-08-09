# 关键正确性与安全修复 - Tasks

> **原则**：安全优先。按「安全 → 评测正确性 → Go core → 跨轮记忆 → 前端 SSE」顺序推进，每 Phase 结束可独立验证。

---

## Phase 0：准备

- [x] 0.1 备份当前 openspec/changes 目录
- [x] 0.2 审计现有调用是否依赖越权访问（grep handler 中直接用请求体 `tenant_id` 的位置）
- [x] 0.3 确认 `JWT_SECRET`、`CORS_ALLOWED_ORIGINS` 在部署环境已就绪
- [x] 0.4 运行 `cd cognida-go && go build ./...`、`cd cognida-python && pytest -q` 记录基线

---

## Phase 1：安全（最高优先级）

### 1.1 移除 X-API-Key 空认证分支

- [x] 1.1.1 删除 `internal/handler/middleware/auth.go` 中「非空 `X-API-Key` 即放行」分支
- [x] 1.1.2 确认无合法 `Authorization` 且非 DEV_MODE 时返回 401、不设默认 user/tenant
- [x] 1.1.3 单测：仅带 `X-API-Key` 的请求断言 401

### 1.2 JWT 密钥启动强校验

- [x] 1.2.1 在启动装配（`cmd/server/main.go` 或 `infrastructure/config`）读取 `JWT_SECRET`
- [x] 1.2.2 缺失 / 等于 `"your-secret-key"` / 长度 < 32 字节时 `log.Fatal`
- [x] 1.2.3 更新 `.env.example`：占位标注「必须替换」，注释最小长度要求
- [x] 1.2.4 单测：三类非法密钥各断言启动校验失败

### 1.3 CORS 白名单

- [x] 1.3.1 `CORSMiddleware` 默认 `AllowedOrigins` 不再为 `["*"]`，改从配置读取
- [x] 1.3.2 命中白名单才回写该 `Origin` + `Allow-Credentials: true`；不命中不回写 `Allow-Origin`
- [x] 1.3.3 单测：白名单内 Origin 被反射、白名单外 Origin 不带凭证

### 1.4 会话鉴权 fail-closed

- [x] 1.4.1 `authorizeSession` 同时校验 `tenant_id` 与 `user_id`
- [x] 1.4.2 context 缺 tenant/user 时拒绝，不用默认身份
- [x] 1.4.3 单测：tenant 不符 / 缺身份两种场景断言拒绝

### 1.5 跨租户 IDOR 修复（tenant-isolation）

- [x] 1.5.1 service 签名加 `tenantID`：`knowledgeBaseService.FindByID(ctx, id, tenantID)`、`GetChunks(ctx, kbID, tenantID, ...)`、`Update`/`Delete` 校验归属
- [x] 1.5.2 repository 读写 SQL 一律 `WHERE id=? AND tenant_id=?`（纵深防御）
- [x] 1.5.3 handler（`knowledge_base_handler`、`agent_handler`、`session_handler`）统一传 `GetTenantID(c)`，不信任请求体 `tenant_id`
- [x] 1.5.4 归属不符时返回 not-found / access-denied，不泄露存在性
- [x] 1.5.5 单测：tenant A 访问 tenant B 资源 id 断言 not-found；repo 层断言 0 行受影响

### 1.6 Phase 1 验证

- [x] 1.6.1 `cd cognida-go && go build ./...`
- [x] 1.6.2 `cd cognida-go && go test ./internal/handler/... ./internal/service/knowledge/... -v`

---

## Phase 2：评测正确性

### 2.1 修复 LLM 裁判 grader

- [x] 2.1.1 `graders/builtin/llm.py` 改调 `LLMClient.generate_json(...)` 取结构化维度分
- [x] 2.1.2 去掉 `except Exception` 后恒返回 `total_score=3.0` 的静默固化，改为显式错误 / 失败标记
- [x] 2.1.3 统一分数量纲（单一口径），维度、`total_score`、下游读取一致
- [x] 2.1.4 测试：mock 一个真实返回的 `generate_json` 断言分数按预期计算；断言失败时抛错而非返回 3.0

### 2.2 修复 runner 的 LLM 评判调用

- [x] 2.2.1 `runner.py::compute_llm_judge_metrics_async` 按正确签名逐条调用
- [x] 2.2.2 读 `result["dimension_scores"]`
- [x] 2.2.3 测试：多条样本断言逐条调用次数与读取字段正确

### 2.3 打通流式评测进度

- [x] 2.3.1 `service.py` 用 `asyncio.Queue` 桥接 worker 产出与 gRPC/SSE 流
- [x] 2.3.2 worker `put(Progress)`、流侧 `await queue.get()` 后 yield，收到哨兵终止
- [x] 2.3.3 grader 注册失败记录日志、不静默固化 ready
- [x] 2.3.4 测试：断言 producer 放入的 Progress 全部被 consumer yield；哨兵后流关闭

### 2.4 Phase 2 验证

- [x] 2.4.1 `cd cognida-python && pytest tests/ -v`（评测相关用例）
- [x] 2.4.2 集成：`pytest -m integration tests/ -v`（如涉及真实 LLMClient）

---

## Phase 3：Go core 数据正确性与资源

### 3.1 修复记忆 ID 生成

- [x] 3.1.1 `memory/long_term.go::randomString`/`generateID` 改用 `crypto/rand`（或 `google/uuid`）
- [x] 3.1.2 测试：`-race` 下并发生成大量 ID 断言无重复

### 3.2 消除记忆 data race

- [x] 3.2.1 `RetrieveMemoryUseCase.Execute` 不再在 goroutine 里改返回给调用方的 `*mem`
- [x] 3.2.2 采用 repo 层原子自增：`UPDATE ... SET access_count=access_count+1, last_accessed_at=?`（如缺列跑 `cmd/migrate-db`）；备选返回前深拷贝
- [x] 3.2.3 测试：`go test -race ./internal/service/agent/memory/...` 断言无 data race

### 3.3 RAG 流式不丢块

- [x] 3.3.1 `pipeline.go` 的 `select{ default }` 改 `select { case ch<-x: case <-ctx.Done(): return ctx.Err() }`
- [x] 3.3.2 测试：断言慢消费者场景无丢块、取消时及时返回

### 3.4 流错误正确区分

- [x] 3.4.1 `eino_agent.go` 用 `errors.Is(err, io.EOF)` 区分正常结束与真实错误，后者上报
- [x] 3.4.2 测试：注入非 EOF 错误断言被上报；EOF 断言正常结束

### 3.5 修复 StreamReader 泄漏

- [x] 3.5.1 每轮迭代显式 `Close()`，不在循环内累积 defer（可用闭包限定 defer 作用域）
- [x] 3.5.2 review：确认循环体内无累积 defer

### 3.6 SSE 感知取消（Go 端）

- [x] 3.6.1 `streamAgentChunks` 与 `streamInternal` 的 `ch<-` 全部改 `select { case ch<-x: case <-ctx.Done(): return }`
- [x] 3.6.2 测试：取消 ctx 后断言不再向 channel 发送、上游停止

### 3.7 Phase 3 验证

- [x] 3.7.1 `cd cognida-go && go build ./...`
- [x] 3.7.2 `cd cognida-go && go test -race ./internal/service/agent/... -v`

---

## Phase 4：跨轮记忆

### 4.1 摘要 write-through 落库

- [x] 4.1.1 `UpdateSummary` 先写 MySQL，再写 Redis 缓存
- [x] 4.1.2 `GetSummary` 缓存未命中回源 MySQL 并回填；区分 miss（空）与 error（err）
- [x] 4.1.3 如新增列/表，跑 `cd cognida-go && set -a && source .env && set +a && go run ./cmd/migrate-db` 同步
- [x] 4.1.4 测试：write-through 后清缓存断言回源命中；miss 与 error 语义区分

### 4.2 记忆分支统一 pre-processing

- [x] 4.2.1 抽出统一前置流程函数（beforeHooks/middleware）
- [x] 4.2.2 记忆分支与非记忆分支都调用该前置流程
- [x] 4.2.3 测试：记忆分支断言 beforeHooks/middleware 被执行

### 4.3 Phase 4 验证

- [x] 4.3.1 `cd cognida-go && go build ./...`
- [x] 4.3.2 `cd cognida-go && go test ./internal/service/chat/... ./internal/service/agent/... -v`

---

## Phase 5：前端 SSE 与鉴权

### 5.1 统一 readSSE

- [x] 5.1.1 新增 `src/utils/sse.ts::readSSE(url, body, signal)`：注入 `Authorization` + `X-Tenant-ID`、非 2xx 走统一错误/登出、透传 `AbortSignal`
- [x] 5.1.2 `src/api/agent/index.ts`、评测进度改用 `readSSE`
- [x] 5.1.3 删除 `src/api/chat/stream.ts` 重复实现，chat 流改走 `readSSE`

### 5.2 401 处理正确化

- [x] 5.2.1 `request.ts` 拦截器 401 改为：`clearAuth` → `refreshAccessToken` 重放 → 失败跳登录页
- [x] 5.2.2 接通 `stores/auth.ts` 中已实现但未调用的 `refreshAccessToken`；并发 401 单飞共享同一 refresh
- [x] 5.2.3 不再无条件先 `logout`

### 5.3 评测进度 SSE 停止无效重连

- [x] 5.3.1 4xx 时停止重连并抛出错误
- [x] 5.3.2 改 fetch 流以携带鉴权头（或复用 `readSSE`），不用裸 `EventSource`

### 5.4 Phase 5 验证

- [x] 5.4.1 前端类型检查 / lint 通过
- [x] 5.4.2 手测：401 触发 refresh 重放；客户端断开时后端上游停止（配合 3.6）

---

## Phase 6：Review 与收尾

- [x] 6.1 触发 code-review skill，修复问题
- [x] 6.2 `cd cognida-go && go build ./... && go test ./... && go vet ./...`
- [x] 6.3 `cd cognida-python && pytest tests/ -v`
- [x] 6.4 `openspec validate critical-correctness-security --strict`
- [ ] 6.5 按 design.md「Migration Plan」的安全优先顺序分批上线，并确认各步回滚路径
