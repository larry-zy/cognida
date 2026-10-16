# Supervisor、HF 与 SFT 技术支持场景验收

本记录仅对应本轮隔离环境，不与此前 37 项能力套件或 10 项独立页面验收合并计数。使用真实普通注册账号、真实 HTTP 与页面、实际计算和模型；没有路由响应替换、数据库造完成状态或整例自动重试。最终 **14 个不同真实页面用例分批通过**：主场景、原始空资料负例、11 项独立回归、实际服务重启后的只读恢复。失败批次仍全部保留，不声称一次连续全绿。14 项真实验收完成后，目检恢复截图发现 SFT 布局与回执上下文显示问题；最后前端修复另以组件测试和明确标注的离线视觉回归验证，没有重建已清理的后端或重复模型调用。

## 固定来源与业务边界

- 数据集：`nvidia/TechQA-RAG-Eval`，Apache-2.0，固定 revision `0b5bbc84b7f07d6d09d063130e90b716d8d4a32a`。
- 实际文件 `train.json`：4,019,332 字节；SHA-256 `69d97231509482ed6bd5ec1c4bc0607acb82a88d11169eb8383592d0ca8b93c7`。
- 文件实际 910 条，数据卡声明 908；保留差异。本轮正向场景选择前两条 IBM Streams / Netcool Impact 技术支持问答，不是随机替代数据，也不是全量质量或模型准确率评估。
- SFT 参考答案是显式选择的已有监督数据。知识分支只用同一来源的 `contexts`，由产品资料列投影生成正文，不将问答列追加为知识内容；知识检索和 Agent 回答另核真实来源回执。
- 原第三条 `TRAIN_Q002` 的 `answer="-"`、`is_impossible=true`、`contexts=[]` 均保留。真实负例验证空资料投影被拒绝，不能把 `"-"` 说成空白答案，不能声称通用 SFT 自动识别数据集专用哨兵。空白答案的 SFT 拒绝属于明确合成的后端负例。

## 页面批次与实际结果

全部输出目录位于本轮 `/private/tmp/cognida-supervisor-20261016/`，账号和凭据不纳入证据。每批独立运行，下面未把失败轮次覆盖为通过。

| 批次 | 用例 | 结果与耗时 | 证据目录 / 日志 |
| --- | --- | --- | --- |
| support-first | 真实模型计划 → HF/清洗/准备/SFT → 知识分支 | 四节点实际成功；在后续 HF 链接定位失败，1.1 分钟 | `e2e-support-first/`、`e2e-support-first.log` |
| support-first | 原始第三条空 contexts → 页面投影拒绝 | PASS，9.3 秒 | 同上，`negative-original-import.json`、`negative-empty-contexts-projection.json` |
| support-second | 主场景定位修正后复验 | 四节点实际成功；测试把 RefID 误当带 kind 的 AssetRef，52.4 秒 | `e2e-support-second/`、`e2e-support-second.log` |
| support-third | HF 输出授权修复后的主场景 | 模型候选漏算清洗/准备/SFT 输入调用预留，保存前强断言失败，13.0 秒；未保存/启动 | `e2e-support-third/`、`e2e-support-third.log` |
| support-fourth | 受信节点调用预算补齐后的主场景 | 四节点、SFT 审核发布、资料投影/打标/审批发布及真实 Milvus 检索均通过；知识助手同身份刷新误清空列表，手动选库失败，1.4 分钟 | `e2e-support-fourth/`、`e2e-support-fourth.log` |
| support-fifth | 知识库列表修复后的主场景 | 上述数据链和手动选库通过；Agent 模块开发地址绕过本轮代理，未取得本轮后端 stream 回应，3.1 分钟 | `e2e-support-fifth/`、`e2e-support-fifth.log` |
| support-sixth | Agent 同源修复后的主场景 | 实际 Agent 两次 `rag_query` 成功，回答包含来源中的故障和命令；会话持久化报错，`persistence_failed=true` 强断言失败，1.2 分钟 | `e2e-support-sixth/`、`e2e-support-sixth.log`、`support-agent-actual-events.json` |
| support-seventh | 消息 UUID 修复后的主场景 | 持久化成功，真实会话 GET 含回答和检索步骤；Playwright 对无 charset 的 SSE 正文解码将两个 em dash 显示为乱码，逐字比较失败，1.2 分钟 | `e2e-support-seventh/`、`e2e-support-seventh.log`、`support-agent-persisted-session.json` |
| support-eighth | 显式 UTF-8 SSE 后的主场景 | 真实 Supervisor 返回 `502 invalid_contract`，13.3 秒；在候选阶段失败，未保存/启动或创建 KB | `e2e-support-eighth/`、`e2e-support-eighth.log`，保留脱敏模型诊断 |
| support-ninth | 主控显式发起的独立第九批，原强断言不变 | PASS，1.2 分钟；真实 Agent 回答与已持久化会话正文、成功检索步骤逐字/逐项一致 | `e2e-support-ninth/`、`e2e-support-ninth.log` |
| support-restore | 同二进制真实 Go 重启后的只读恢复 | PASS，用例 2.9 秒 / 批次 3.5 秒；无浏览器领域写请求 | `e2e-support-restore/`、`e2e-support-restore.log`、`go-restart-readback.json` |
| independent-workbenches | 3 个领域专家 + 4 个独立数据工作台 + 知识标签检索 | 8 PASS，总计 1.5 分钟 | `e2e-independent-workbenches/`、同名 `.log` |
| runtime-regression | 四节点分发、双质检并发、真实取消 | 3 PASS，总计 59.9 秒 | `e2e-runtime-regression/`、同名 `.log` |

独立回归的 11 个不同用例：

| 用例 | 实际耗时 |
| --- | --- |
| 特征窗口预览、固定版本、物化、历史查询和专家任务 | 15.5 秒 |
| 分类、实体、知识打标、QA 的证据与人工审核/导出 | 6.8 秒 |
| 多领域专家 DAG 及真实产物绑定 | 37.0 秒 |
| 知识标签审核、应用回执、真实向量标签过滤 | 6.3 秒 |
| 已有表格 → 独立清洗 → 类型准备 → 已发布分类标准 → 标注 | 4.6 秒 |
| QA 标准 → 准备数据 → SFT 审核 → 固定导出/刷新 | 3.4 秒 |
| messages/tool-calling → SFT，保留精确工具参数 | 2.9 秒 |
| 独立 HF 固定样本 → 清洗 → 准备 → 标准约束标注 | 15.4 秒 |
| 四个真实专家节点 → 终态产物 → 页面刷新恢复 | 37.3 秒 |
| 两个独立质检根节点按并发预算分发 | 14.1 秒 |
| 取消申请与实际取消终态分离 | 8.0 秒 |

双质检领域从创建到终态的实际窗口为 `11:37:44.453122Z–11:37:44.739312Z` 和 `11:37:44.581186Z–11:37:44.953868Z`，重叠 **158.126 ms**。这证明两个独立领域任务窗口重叠，不等同于 Python CPU 同时计算或性能 SLA。取消场景实际先返回 `cancel_requested`，最终 `cancelled`、成功节点数 0；断言仍允许已有真实成功产物时合法进入 `partially_completed`。

## 首轮发现与修复边界

1. 同一 HF/SFT 回执同时出现在节点列表与折叠的投递历史，原测试全页唯一性定位错误。已限定到实际“节点执行状态”列表，仍按真实 domain ID 精确匹配。
2. 任务汇总的 `output_refs` 是仅含 ID/版本的 RefID。测试现通过授权资产 GET 取得真实 kind 后选用清洗/准备产物，不推断未返回的字段。
3. 同时发现 HF 真实产物在汇总被隐藏：零输入能力的输入验证被用于产物读授权。后端已增加独立输出授权并经核心/真实数据库验证；第四轮实际通过汇总产物必须等于真实 HF import 产物的强断言。
4. 第三轮真实模型将三个节点的调用预留填成基础次数，遗漏每项输入解析成本。编译器现按受信能力的基础调用与输入数量确定最低预留，展示逐节点与汇总调整，仍不能超出原任务预算、不改业务参数或重试次数；手工计划也必须满足相同执行门槛。第四轮真实模型提案、显式保存及启动成功。
5. 第四轮知识助手读库返回 200，但同身份资料对象刷新触发了数组 getter watcher，误清空缓存并丢弃在途回包。这是生产页面缺陷，非定位器问题。已改为分别观察身份 primitive 值；同 ID 刷新保留在途列表与缓存，真实身份变化仍同步清空并拒收旧响应，补充回归测试。
6. 第五轮实际选库成功后，Agent 模块仍优先读取开发环境旧的 `VITE_API_BASE_URL`，绕过普通 HTTP 所用的 Vite 同源代理。现统一 Agent SSE、来源分页和操作确认到 `/api/v1`，不改变鉴权、AbortSignal 或请求体；回归覆盖旧环境变量存在时仍同源，不能将本次失败说成模型回答失败。
7. 第六轮实际回答的 `partial=false`，两个真实检索工具分别返回 4 与 3 个片段，但数据库保存消息时返回 `Data too long for column 'id'`，故终局明确 `persistence_failed=true`。后端改为完整 36 字符 UUID，不截断随机标识、不扩大 schema；第七轮 `persistence_failed=false`，并通过真实会话 GET 确认已存回答与步骤。测试不把有回答等同于已可靠保存。
8. 第七轮额外逐字回读断言发现无显式 charset 的 SSE 经 Playwright/Chrome 文本读取后，两处 UTF-8 em dash 被错误解码；页面和数据库正文均是正确 Unicode。共享 SSE 现明确声明 `text/event-stream; charset=utf-8`，原始事件与会话 JSON 均保留；测试不替换字符，继续逐字比较，并核对实际 charset。第九批原样逐字断言通过。第八批真实 Supervisor 因 `invalid_contract` 明确拒绝；没有保存或启动，不放宽模型合同、不使用模板回退。主控单独授权第九批一次执行，不能将其描述为自动重试或所有模型提案均成功。

## 代码验证与证据读取

最终前端全量 `npm test`：**416 tests / 60 files PASS**（4.48 秒）；最终 `npm run build`（包含 `vue-tsc --noEmit`，Vite 4.41 秒）、`npm run test:e2e:check` 通过。日志为 `frontend-unit-layout-final.log`、`frontend-build-layout-final.log`、`frontend-e2e-type-layout-final.log`。构建只有既有 Sass 与 chunk-size 提示。新增核心组件覆盖固定 HF/SFT 参数、零输入、已准备资产、发布包装读取、资料投影、发布桥绑定、身份/迟到响应失效以及实际检索来源卡。

截图已目检：原始空资料的失败/零产物提示、独立 HF 标注审核区、SFT 固定发布样本、真实向量知识标签结果正文可读。第九批 `support-four-node-receipts.png`、`support-agent-answer.png`、`support-agent-source-receipts.png` 已目检：四节点、真实故障/命令和工具来源可读。恢复截图曾暴露 SFT 详情落入窄列，见下方单独修复说明。

重跑使用 [E2E README](README.md) 中显式隔离 URL、账号路径与开关；服务由主控启动和管理。`support-restore.spec.ts` 只能在实际 Go 服务重启后，传入主场景生成的 `support-restore-manifest.json` 运行。普通页面刷新不能算作服务重启恢复。

## 持久恢复的固定回执

第九批实际 task `e5b36234-7de6-4abd-9efa-4fb2bdad70d1`（task_version 25）的四个子任务均 attempt 1：

| 节点 | Child ID | 领域回执 |
| --- | --- | --- |
| import | `6499f6fe-0e7f-4fac-942e-3f9dc515f5b2` | huggingface_import `6e043e7f-1bf6-4b6c-bffb-7112e8b9b2bc` |
| clean | `59711b6e-7204-4ece-9960-332bba9c917b` | invocation `b3b941f6-4f07-4414-81e6-ca7a89b79102` |
| prepare | `5d3a7dd1-2a4b-48a5-8dcf-0fe10a87f9e1` | invocation `bba32171-2125-4db8-81a2-0ff5d27e355a` |
| sft | `be1ffe11-8587-4358-9470-d19c6b709ae3` | sft_execution `bc659e74-d348-5a41-a859-97cb960f1e85` |

SFT 数据集 `3241ad09-4032-5884-9448-36430e5a4949` 修订 3，发布版本 `de482984-f53d-5650-971a-e2aa87fe9beb`，内容摘要 `f8b4ac9da1ce9d916aecd1b73d925ac896a0682a9ffe655d89aff5b5328d19a4`。主控实际将 Go PID 1644 重启为 3689，二进制 SHA-256 前后同为 `08e204113625f32d88f0dec4b041a9b22c505799b13c3591c8356ed2945218b2`；证据 `go-restart-readback.json`。恢复用例只执行登录、GET、页面打开和刷新，核对 task/执行版本、所有 child/domain/attempt、HF request/manifest/output refs、SFT 包装和版本摘要均保持一致，`browser_domain_writes=[]`。这验证已完成持久任务的重启后读取，不声称中断中的付费模型可自动重放。

原始回执：`e2e-support-ninth/support-supervisor-technic-ef9c5--knowledge-and-Agent-answer/support-restore-manifest.json`、同目录 `hf-supervisor-sft-evidence.json` / `support-agent-persisted-session.json` / `support-sft.jsonl`，以及 `e2e-support-restore/support-restore-technical--275a6-vive-a-real-service-restart/support-after-service-restart.json`。

## 最后布局与回执上下文修复的独立验证

真实恢复截图显示 `execution_id` 模式将“任务回执”和“数据集详情”作为独立 grid 子项，详情掉入第二行 290px 左栏。现将回执、构建表单和详情放入同一右侧 stack，沿用原有 900px 单列断点；长摘要在窄屏换行。同时修复从左栏打开另一数据集时旧执行回执残留的问题：先失效在途请求，再清除不匹配的回执；同一数据集刷新保留回执。未改变接口、审核、发布或导出语义。

新增组件回归验证 grid 仅有侧栏与详情两项、回执/样本/版本同属右栏、同数据集刷新保留回执、跨数据集清回执且不写入。全部工作台 26 项与全前端 416 项通过，独立 Review 通过。

已清理真实环境后，只启动临时 Vite + Chrome，以前述实际恢复 JSON 和实际导出 JSONL 作为**明确的离线 fixture**（合成登录，仅本机 GET 回包替换，不是真实 API/E2E 验收）检查实际 CSS：1440px 视口右栏 836px，390px 视口单列 358px，卡片同列且文档无横向溢出。截图 `offline-sft-layout/desktop-offline-sft-detail.png`、`mobile-offline-sft-detail.png` 和两个 `*-offline-sft-samples.png` 已目检；布局和零写请求回执为 `offline-sft-layout/offline-layout-receipt.json`。脚本 `sft-offline-layout.mjs`、日志 `offline-sft-layout.log` 均仅保存在临时证据目录。Google Fonts 被离线脚本阻止，使用应用字体回退；没有访问模型或真实后端。临时浏览器与 62715 端口 Vite（最后 PID 7654）在 finally 关闭。

因此：**14 项真实 E2E 在最后布局/只读上下文补丁之前完成；补丁后证据是组件测试、全量前端测试/构建/类型检查及离线视觉回归。** 两者分别记账。

## 清理

独立知识标签回归创建的 KB `20261016193557b9798fd423575270` 已通过正常删除 API 返回 200，回执为 `e2e-independent-workbenches/governance-release-reviewe-09a78--→-real-vector-label-filter/knowledge-fixture-cleanup.json`。前三轮主场景都在创建 KB 前停止。第四至六轮的 KB `20261016195107db4d8ec9a28a68cc`、`20261016195758187b200ac489a835`、`20261016200357fe15c95f7cf46b2e` 均在各轮 `finally` 经正常 DELETE 返回 200，对应 `support-knowledge-cleanup.json` 保留。第七批 KB `2026101620113716d2da457129b461` 与第九批 KB `2026101620194789bb1152ae7d7aa1` 也均由正常 DELETE 返回 200，回执在各批 `support-knowledge-cleanup.json`。第八批未创建 KB。服务、数据库和临时凭据清理见下方“环境回收验证”；上述离线回归不连接它们。

## 后端与数据完整性验证

本轮 `go test -race ./internal/...`：**106 个测试包通过，42 个包无测试，0 失败**（`go-all-internal-race-final.log`）；随后针对消息持久化与 SSE 编码增量，重新运行 handler/SSE race 和真实数据库测试，见下表。Python HF 解码定向回归 **24 passed / 0.43 秒**（`python-hf-decoder.log`），本轮没有修改 Python 生产代码，也没有将历史 717 项结果记为本轮重跑。

| 范围 | 本轮真实环境证据 |
| --- | --- |
| HF 持久提交、一次认领、取消/撤权、重建服务读取与原独立导入 | `hf-governed-integration-final.log` 3.737 秒；使用真实 MySQL、HTTP 和 Python decoder，外部下载用明确的本地 HTTP fixture；公开 HF 下载另由页面验收覆盖 |
| HF 输出与任务摘要授权 | `hf-output-integration.log` 3.839 秒；完成产物可见、跨用户/租户拒绝、撤权后不可见、重建服务不重复下载。任务收尾与摘要核心回归 `output-authorization-core.log` 2.074 秒 |
| SFT 持久草案、原子提交、期限、取消和重复投递 | `sft-adapter-integration.log` 3.335 秒；原标准/SFT 独立 HTTP 回归 `sft-independent-regression.log` 2.262 秒。空白答案负例是合成样本，不冒充 TechQA 第三条 |
| 显式资料正文投影 | `projection-integration.log` 1.807 秒；核验来源与 prepared 证明，空/畸形资料失败且无可用产物 |
| 审核标签关联真实发布 | `knowledge-publication-integration.log` 2.581 秒；真实生命周期审批、发布资产、来源链和 MySQL/HTTP 应用；`knowledge-label-snapshot-final.log` 2.667 秒覆盖当前内容/审核版本变更失效 |
| 知识入库原始/分块证明 | `knowledge-ingestion-final-db.log` 1.940 秒；正式迁移后的真实数据库验证最终提交、历史证明及普通上传不伪造字节证明。既有上传 FileHash 元数据保留 |
| Agent 回答与来源持久化 | `stream-message-id-core.log` handler race 2.545 秒、`stream-message-id-db.log` 真实迁移库/SSE 2.151 秒；完整 UUID 与正式 36 字符主键相容，回读实际问答、工具回执和会话计数，无 schema 扩容 |
| SSE 明确编码 | `sse-charset-core.log`：SSE race 1.915 秒、handler race 1.950 秒；明确 UTF-8 Content-Type，与原始 JSON 字节保持一致，保留页面/数据库逐字读回断言 |
| Supervisor 调用预算 | `supervisor-execution-budget-db.log` 3.230 秒；注入明确的合成 planner，通过真实 HTTP/数据库验证逐节点补数可见、预览零写、人工精确保存、原始不足预算拒绝及超任务上限拒绝；真实模型另见页面批次 |

正式迁移 **38、39** 已分别用于隔离审查库和页面应用库，均为 `version: 39, dirty: false`，证据 `migrations-test-39-approved.log`、`migrations-app-39-approved.log`。部署时仍需按 `services/cognida-go/migrations/README.md` 执行版本化迁移；不会运行时自动改表，也不会用当前分块回填旧文档的历史入库证明。

独立 Review 覆盖 HF/SFT adapter、队列/取消/恢复、调用预算、知识入库/发布证明与前端/测试接线；发现项修复后复查通过。旧的失败批次、普通沙箱数据库连接限制、早期测试定位错误以及真实模型预算漏算记录均保留，未覆盖为成功。

## 环境回收验证

真实 E2E 结束后，按已登记 PID/命令及容器 ID/任务标签核对，停止本轮 Python、Go、Vite，删除本轮 MySQL/Redis 容器及其专用卷。原先 13 个运行容器均仍运行，共享 Milvus 与原有服务未被停止。独立检查 62701、62702、62710、62711、62713 五个端口全部关闭；临时 app/test/MySQL 环境文件、账号文件、测试二进制与配置均已删除。证据为 `cleanup-receipt.json` 与 `cleanup-independent-check.json`。测试日志、脱敏回执和截图保留在权限 0700 的本轮证据目录；不将凭据提交到仓库。

最后离线视觉回归结束后，主控再次独立检查上述五个端口及 62715，六个端口全部关闭，证据 `cleanup-final-ports.json`。
