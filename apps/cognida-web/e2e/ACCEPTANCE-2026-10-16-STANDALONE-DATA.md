# 独立数据能力验收：HF、准备、标准与 SFT

日期：2026-10-16。此记录对应新增能力，不复用此前综合流程的 37/37，也不把已有公开数据 benchmark 的质量分数算作本轮界面或模型效果。验收只在隔离环境执行，未启动正式业务采集或训练。**本轮 10 个不同的受影响页面场景已分批通过；历史失败单独保留，不合并为一次无失败的 10 项运行。**

## 可独立调用的能力

| 能力 | 页面 | HTTP（`/api/v1`） | Agent 工具 |
| --- | --- | --- | --- |
| HF 搜索、固定来源检查 | `/hf-datasets` | `GET /huggingface/datasets`、`GET /huggingface/dataset` | `hf_search`、`hf_dataset` |
| HF 预览与导入 | `/hf-datasets` | `POST /huggingface/preview`、`POST /huggingface/imports`、`GET /huggingface/imports/:id` | `hf_preview`、`hf_import`、`hf_import_status` |
| 单项处理与准备 | `/experts`、`/data-preparation` | lifecycle 单项执行/读取、`POST /lifecycle/assets/:id/prepare` | `data_stage_run`、`data_stage_status`、`data_prepare` |
| 标准及标注 | `/label-standards`、`/labeling` | 标准草稿/发布/版本、annotation/tagging、逐项模型建议 | `label_standard_draft`、`annotation_create`、`annotation_preannotate`、`tagging_create`、`label_suggest` |
| SFT 构建、人工审核与导出 | `/sft-datasets` | `/sft-datasets` 创建/读取/审核/发布、`GET /sft-dataset-versions/:id/export` | `sft_dataset_draft`、`data_records_list`、`data_record_get` |
| 特征与数仓 | `/features`、`/warehouse` | 既有版本、物化、变换与状态 API | `feature_job_create`、`feature_job_status`、`warehouse_transform` |

工具调用相同授权服务，来源可以是已有资产。HF/SFT 尚无 Supervisor 持久 DAG adapter，不把工具返回值冒充父任务 ChildHandle。工具不持有人工审核、标准发布或候选批准权限。`label_suggest` 一次针对一个已有 item，绑定固定标准、当前版本与幂等键，输出仍为待审核候选。

新建 annotation/tagging/SFT 必须使用服务器签发的 prepared 来源和已发布的不可变标准。清洗与准备是不同步骤：准备做明确字段映射、类型/必填检查和可选换行规范化，不自动猜测字符串数字、不补答案、不静默删行。标准中的自然语言指南不等于自动证明业务语义。

SFT 只从已有 QA 答案或 messages/tools 构建候选。工具实参校验采用有界 JSON Schema 子集：单一类型、properties/required/additionalProperties、items、标量 enum、长度及精确数值上下界；未知关键字和 `$ref` 拒绝，不做类型强制转换。数据库往返、审核、导出保留精确 schema 数值与 arguments。结构通过不等于执行工具、答案正确或训练有效；本轮不训练/部署模型。

## 已执行的验证

日志目录：`/private/tmp/cognida-scenario-20261016/`。临时路径用于定位本轮原始日志，重新验收须创建自己的隔离环境；测试不依赖这些日志或旧运行 ID。

| 验证 | 结果与证据 | 证据范围 |
| --- | --- | --- |
| Go 全 internal race | `go-final-with-egress-race.log`：104 个测试包通过，42 个包无测试，零 FAIL | 含 HF 专用出口与知识应用增量；保留此前 `go-all-final-race.log`。含复用 Go test cache 的包，不等同所有真实数据库集成 |
| Python 全量 | `python-final-preservation.log`：717 passed、3 skipped，24.84 秒 | 含新增四项 clean no-op 原文保真回归，保留此前 713/3 日志。允许本机测试监听后通过；默认沙箱首跑 689 passed 与 24 个 bind 权限错误保留，未删失败或算通过 |
| HF 解码核心 | `tests/huggingface` 24 项通过 | 真实 FastAPI 鉴权与 JSON/CSV/gzip/Parquet 解码；数字精度、边界与非法格式 |
| HF 导入/授权 | `huggingface-integration.log` 2.814 秒；出口增量后 `huggingface-egress-db-regression.log` 2.300 秒 | 真实 MySQL、TCP HTTP、Python decoder、本地对象存储；5 并发同键仅一次文件下载、载荷冲突、owner/撤权/凭据停用/截止栅栏。Hub 是 HTTP fixture，合成凭据不是第三方登录 |
| 标准/SFT | `label_standard_sft_integration_test.go` 与相关治理回归通过 | 真实 MySQL/HTTP、prepared/标准前置、逐行查看、整批人工审核、版本导出、split/group 泄漏拒绝、schema 实参检查与精确数字；未调用训练 |
| 真实动态产物 | `warehouse-feature-chain.log`：2.790 秒通过 | 数仓真实 Result 的 99.25 经固定 feature 得到 100.25；来源与定义 anchor 父链均保留，不把 anchor 原值当实际输入 |
| 动态 embedding 预算 | 根任务的 adapter/governed/warehouse 数据库 race 通过 | 对实际绑定行数核算调用上限；不是外部固定 revision 模型物化验收 |
| QA SFT 页面 | `scenario-workflows.spec.ts`：QA standard → prepared existing table → SFT review → immutable export and refresh，7.4 秒通过 | 真实 UI、独立 API、人工审核、发布、刷新与下载 JSONL |
| messages/tools SFT 页面 | 同文件：existing messages/tool-calling JSON → SFT audit retains exact tool arguments，19.9 秒通过 | 真实 UI，精确工具实参保留、审核及固定导出 |
| 上述两条页面合计 | `ui-e2e-local-final.log`：2 passed，33.0 秒 | 截图和实际 JSONL 在同名目录；不是全部新增场景通过 |
| 受影响既有页面 | `ui-e2e-regressions`：特征流程与四类标注两个 case 通过 | 沿用真实 prepared/已发布标准前置；不假造旧接口已满足新约束 |
| 五节点任务 DAG | `ui-e2e-independent-final`：42.4 秒通过 | 使用真实固定能力与预算；原实际报告附件已提取为 `ui-e2e-independent-final/evidence/multi-specialist-real-results.json`，未重请求或合成状态；单次历史失败及修正原因另行保留 |
| 前端回归 | 382 tests / 58 files，production build、Vue 与 E2E 类型检查通过 | 组件与构建证据；真实页面结果另按下表计数 |
| 独立 Review | 主控及独立 Agent 交叉 Review 通过，无未关闭的确定问题 | 包括 prepared/API/工具注册、动态 DAG 真实来源与预算、HF 出口及精确 CDN、Python 原文保真、标准/SFT/schema 和知识标签父链；实现者不替代自己的独立审查 |

### 最终页面结果：10 个不同场景

每行计一个 case，重复复验不增加场景数。时间为该 case 的实际执行时间；批次总时间包含浏览器启动等开销，不能简单相加成一次完整运行。

| # | 场景 | 最终结果 | 对应日志/证据目录 |
| --- | --- | --- | --- |
| 1 | 特征窗口聚合、物化与 as-of 查询 | 通过，15.7 秒 | `ui-e2e-regressions` |
| 2 | classification/entity/tagging/QA 四类规则标注与审核 | 通过，16.2 秒 | `ui-e2e-regressions` |
| 3 | 质量→特征→清洗→准备→打标五节点任务 DAG | 通过，42.4 秒 | `ui-e2e-independent-final`；`evidence/multi-specialist-real-results.json` 为原报告附件原样提取 |
| 4 | 反馈返工与候选评测、审批流程 | 通过，42.6 秒 | `ui-e2e-release` |
| 5 | 真实模型 QA 单项建议与审核边界 | 通过，4.7 秒 | `ui-e2e-release` |
| 6 | QA 标准→已有准备资产→SFT 审核、固定导出与刷新 | 通过，3.7 秒 | `ui-e2e-release` |
| 7 | messages/tools SFT 审核与精确 arguments 导出 | 通过，6.0 秒 | `ui-e2e-release` |
| 8 | 知识标签审核应用→真实 Milvus 命中→错误标签不命中 | 通过，6.7 秒 | `ui-e2e-final-three` |
| 9 | 已有资产→独立清洗→显式准备映射→标准→分类候选 | 通过，4.7 秒 | `ui-e2e-final-three` |
| 10 | 公开 HF 固定文件→有界导入→清洗→准备→标准→待审标注 | 通过，17.0 秒 | `ui-e2e-final-three` |

最后批次 `ui-e2e-final-three.log` 为 **3 passed / 29.4 秒**。HF case 的 `hf-preview-response.json`、`hf-imports-response.json`、`hf-clean-prepare-annotation-evidence.json` 保存真实请求/响应与派生链；`hf-prepared-annotation.png` 为实际审核区截图。知识 case 保留正向真实 `milvus_vector` 命中、错误标签空态与每次查询回执，未降低阈值或替换为词法 fallback。独立分类检查两行实际 `annotation.labels` 均为 `["train"]` 且 pending。最终三项截图已目检，无加载遮挡；浏览器上下文已关闭。

### 保留的失败与复验

中间七场 `ui-e2e-release.log` 为 **4 passed / 3 failed**（1.5 分钟），保留原批次：反馈、真实模型 QA、QA SFT、messages SFT 通过；知识向量召回、分类测试 DTO 路径、HF preview 失败。定向 `ui-e2e-label-recheck-real.log` 为 **2 passed / 10.9 秒**：知识标签 5.4 秒与独立清洗→准备→标准→分类 4.5 秒通过。知识该次首次查询即命中真实 Milvus，不追认之前空召回的具体原因。最终三项又在同一最新版服务上通过，旧批次不覆盖、不删除。

首轮分类标准页面因后端省略可选 `terms/keywords` 导致渲染异常，已补空数组归一与组件回归；测试还纠正了读取 `annotation.labels` 的 DTO 路径，并严格检查两行均为 `["train"]` 且 pending。另保留旧模型 502、反馈投影真实 33.5 秒返回 409 而原测试仅等待 20 秒的超时，以及知识标签应用来源合同阻断；修复后的通过证据不覆盖原失败。默认沙箱定向 Chrome 启动失败未发业务请求，另保留 `ui-e2e-label-recheck`，与产品断言失败分开。

## HF 网络：失败、定位与修复分别保留

首次真实页面固定 `mteb/banking77` commit `18072d2685ea682290f7b8924d94c62acc19c0b2` 的来源检查在约 38 秒返回 504，未进入预览/导入。后续独立直连生产 fetch 与强制 `netdns=cgo` 诊断均在 DNS 前检 30 秒截止，未产生 TCP/TLS/HTTP；这只能说明后续诊断的阶段，不能追认最早 504 的具体根因。

系统已配置本机代理而 Go 安全采集默认直连。显式使用既有代理的只读诊断收到同一 metadata 200，说明远端不是普遍不可达。随后新增 **仅 HF** 的 operator 配置 `HUGGINGFACE_SOCKS5_ADDRESS`：

- 空值保留直连；只接受数字 loopback host:port，不从请求、工具或普通代理环境变量继承出口。
- 固定 Cloudflare DoH endpoint 与官方 bootstrap 地址，正常验证证书；A/AAAA 共享 8 秒预算，任一失败即拒绝，无陈旧缓存或本地 DNS 回退。
- 全部地址先做既有公网检查，SOCKS CONNECT 只传已批准数字 IP；HF/CDN 的 TLS/SNI 仍验证原 hostname。
- DoH 禁止重定向；HF 每跳保留域名/DNS 策略，跨域后永久去除 Bearer；不自动重试 HTTP 错误。通用采集不改。

`huggingface-egress-core.log`：完整 fetcher race 8.321 秒通过，使用真实本机 SOCKS/TLS/HTTP2 fixture，覆盖混合私网、重绑定、错误证书零业务 HTTP、取消关闭、重定向凭据、HTTP 503 不重放。固定 DoH 合同见 [Cloudflare 官方文档](https://developers.cloudflare.com/1.1.1.1/encryption/dns-over-https/make-api-requests/dns-json/)。

`huggingface-egress-public.log`：**新生产工厂**读取固定公开 metadata，HTTP 200、2252 字节、1.814 秒，repo/commit 匹配，未发凭据、未创建资产。该结果不是完整公网文件导入、私有 HF 鉴权或业务采集通过。

真实页面随后已读取 metadata，但 preview 返回 400。只读固定文件入口诊断确认其 302 目标为旧白名单没有的 `us.aws.cdn.hf.co`，属于生产有限域名名单滞后。[HF 官方下载文档](https://huggingface.co/docs/hub/main/datasets-downloading)确认该精确 CDN 后，仅增加此域，不接受任意子域或后缀；原 TLS、全 IP 公网检查与永久去凭据规则不变。`huggingface-cdn-core.log` 的完整 fetcher race 再次 8.334 秒通过。

`huggingface-cdn-public-preview.log`：一次真实固定 Parquet→生产安全出口→已运行且认证的 Python decoder→3 行预览，6.658 秒通过；文件 294377 字节，SHA256 `482d07d5f8027debcaca8fb3b8fa9da9d81a01318bba746eb36e5bb4fcd785af`，sampled/truncated 均 true，排除 `label` 与 `label_text`。该诊断未接仓储/资产服务，不能生成导入回执或资产；没有重复执行另一个文件下载诊断。

之后完整页面在 `ui-e2e-final-three` 中以 17.0 秒通过。真实预览与导入均绑定上述 commit、`default/train` 和 `data/train-00000-of-00001.parquet`，实际文件总量 9993 行，只取前 3 行，`sampled/truncated=true`；导入回执 `145863fb-dc03-431e-b38b-fbb554db320d` 状态为 succeeded，真实 raw asset 为 `460ebae3-c2b3-5fa4-bc9f-0420e5ed150f:1`。后续独立清洗、准备及固定标准 `4bc512db-591a-553a-a1ad-98f5e123c19c` 生成 3 条 pending 标注。原始正文、prepared 摘要和父链见实际 JSON 附件；`label`、`label_text` 的值没有进入导入资产。这是公开数据的有界功能链路验收，不是私有仓库认证、完整 split 导入或标注质量评测，原 504/400 与 4/3 批次继续保留。

## 可重跑入口与未验收边界

先建立专用 `cognida_review_` 数据库并用正式迁移应用到 37；不要在生产库跑 fixture。将连接注入受保护环境，Go 与 Python 的内部 HTTP token 必须一致。HF 默认不需要外部凭据即可读取公开仓库；私有仓库仅使用本人受管 Bearer 引用。不要把 key/token 写到命令记录或版本库。

```bash
cd services/cognida-go
go test -race ./internal/service/collection/fetcher ./internal/service/huggingface ./internal/model/huggingface -count=1
go test -race -tags=integration ./internal/service/huggingface -run '^TestHuggingFaceMySQLHTTPImportIndependentAndFenced$' -count=1
```

```bash
cd services/cognida-python
.venv/bin/python -m pytest tests/huggingface -q
```

浏览器使用 `scenario-workflows.spec.ts` 和 `scenario.helpers.ts` 中的真实登录/固定环境变量合同；既有回归另见 `domain-specialists.spec.ts`、`governance-release.spec.ts` 和 `feedback.spec.ts`。已有回执只是结果，不是用来绕过页面流程的 Mock API。每轮用独立操作键、真实来源与授权工作空间，保留失败和产物。

## 清理与环境边界

最终知识 fixture 的 DELETE 返回 200；随后真实登录并读取该专用账号的知识库列表，`total=0/items=[]`，回执为 `fixture-kb-final-inventory.json`。`cleanup-receipt.json` 记录本轮 Python 95238、Go 5316、Vite 43060 已停止，按精确 ID 删除本轮专用 `cognida-scenario-mysql`、`cognida-scenario-redis` 容器及其所属卷；主控同时删除本轮临时凭据与服务二进制。清理后的 `62601/62602/62610/62611/62613` 端口均确认关闭，`temporary_credentials_removed=true`。

清理回执中的 `original_running_unchanged=false` **保留原值**。原先 14 个容器中，`powercontext-push-validation-8PWh83` 当前已不存在，其余 13 个仍运行；清理脚本只对上述两个本轮容器执行删除，未对该 PowerContext 容器执行操作。其消失时间和原因未核实，不能声明“所有原有容器均未改变”，也不把该差异归因于本轮清理。

外部真实 GCP BigQuery 项目、可信固定 revision 的特征 embedding 仍未验收。HF bounded import 每次最多 1000 行、1–4 文件、单文件 8 MiB/数据总量 16 MiB；只有前缀样本时必须显示 sampled/truncated，未知总量保持 null。SFT 发布不自动成为生产训练批准，新增标准与准备流程也不重写既有 335 条模型基准结果。
