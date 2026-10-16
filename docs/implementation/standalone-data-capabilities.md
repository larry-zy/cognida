# 独立数据能力：HF 接入、数据准备、标注标准与 SFT

更新：2026-10-16。本页描述新增能力的实际接口与验收边界。每项能力接受已有输入、返回自己的记录或版本资产，不要求用户每次从采集开始。Agent 工具与页面调用同一授权服务；跨阶段流程只组合这些能力，不另建执行器。

## 支持矩阵

以下 HTTP 路径统一以 `/api/v1` 为前缀。所有调用使用当前登录用户与工作空间身份，工具不能通过参数指定其他用户或租户。

| 能力 | 独立页面 | 独立 API | Agent 工具及实际产物 |
| --- | --- | --- | --- |
| HF 搜索与来源检查 | `/hf-datasets` | `GET /huggingface/datasets`、`GET /huggingface/dataset` | `hf_search`、`hf_dataset`；返回仓库、数据卡、固定 commit、配置/切分与静态文件列表，不创建资产 |
| HF 预览与导入 | `/hf-datasets` | `POST /huggingface/preview`、`POST /huggingface/imports`、`GET /huggingface/imports/:id` | `hf_preview`、`hf_import`、`hf_import_status`；预览不写资产，导入返回持久回执、manifest 与 raw_asset 引用 |
| 单项质量/清洗/增强/分析/评估 | `/experts`、现有各领域页面 | 现有 lifecycle 单项调用及状态 API | `data_stage_run`、`data_stage_status`；返回真实 StageRun，完成状态与输出由后续读取确认 |
| 数据准备 | `/data-preparation` | `POST /lifecycle/assets/:id/prepare`，body 含精确 `version` | `data_prepare`；接受已有 cleaned_asset，输出带 prepared 元数据的 cleaned_asset，绑定源/输出/schema 摘要 |
| 标注标准 | `/label-standards` | `/label-standards` 草稿、更新、版本与 `/publish`；`GET /label-standard-versions/:id` | `label_standard_draft` 创建草稿，`data_records_list` / `data_record_get` 读取；标准发布由人工权限 API 完成 |
| 知识打标与样本标注 | `/labeling` | 现有 tagging/annotation API，新建需 `standard_version_id` 与 prepared 来源 | `tagging_create`、`annotation_create`、`annotation_preannotate`、`label_suggest`；后者按已发布标准、当前 item 版本与稳定键生成一次模型候选，仍需人工审核 |
| SFT 构建 | `/sft-datasets` | `/sft-datasets` 创建、逐行读取、`/review`、`/publish`；`GET /sft-dataset-versions/:id/export` | `sft_dataset_draft` 构建候选，通用记录工具读取；发布后可导出固定 JSONL，不执行训练 |
| 特征物化 | `/features` | 既有版本、预览、物化作业 API | `feature_job_create`、`feature_job_status`；使用已有定义与显式源快照，保留分区、截止时间与提交回执 |
| 数仓变换 | `/warehouse` | 既有模型/运行/审批 API | `warehouse_transform`；固定模型版本与事实快照，产生待审批候选 |

工具注册到默认 Agent 和 Data Agent 的 `data_capabilities` 分组。`data_stage_run` 可以申请发布阶段，但发布执行仍停在已有人工审批门禁。工具集合没有标准发布、标签审核、SFT 审核或审批工具。

HF 和 SFT **尚未登记为 Supervisor 持久 DAG adapter**。它们的独立服务回执不等同于父任务的 ChildHandle。现有治理特征与数仓 adapter 可以消费授权的已解析上游产物；动态绑定在分发时检查真实来源和模式，不能凭模型猜测资产 ID，也不能变更已固定定义。

## HF 静态文件合同

`GET /data-connectors` 当前返回 `scope=huggingface_connector` 的单项能力说明，不代表全站所有连接器矩阵。`decoder_ready=false` 时仍可搜索和查看数据卡；预览、导入返回不可用。Go 与 Python 必须配置一致的 `INTERNAL_HTTP_AUTH_TOKEN`，Python 运行既有 FastAPI 应用中的内部解码路由。受限或私有仓库使用受管 Bearer 凭据 ID，凭据须属于当前用户且 origin 精确为 `https://huggingface.co`。

来源检查允许 `revision=main` 以获取当前固定 SHA；预览和导入只接受 **40 位小写 commit SHA**。配置/切分从数据卡的 `configs/data_files` 映射到实际仓库文件；不支持的配置形状明确提示，未声明可验证映射时只提供标明含义的 `static_files/selected` 文件选择，不声称是发布者定义的 train/test。Hub 的版本元数据与配置概念可参照其[官方 API 合同](https://huggingface.co/docs/huggingface_hub/en/package_reference/hf_api)和[数据文件配置说明](https://huggingface.co/docs/hub/en/datasets-manual-configuration)。

支持 JSON 数组、JSONL、CSV、Parquet；JSON/JSONL/CSV 可使用 gzip。不会调用 `load_dataset`、执行数据集脚本、加载 pickle，或接受任意 URL/宿主文件路径。下载从固定 HF 域出发，逐跳校验公共 DNS 与有限 CDN 域；首次跨域后永久移除 Bearer，不将签名重定向 URL 写入回执。此次真实固定文件重定向使用的 `us.aws.cdn.hf.co` 已按 [HF 官方下载文档](https://huggingface.co/docs/hub/main/datasets-downloading)加入精确白名单，不接受任意 `hf.co` 子域。

网络默认直连，不自动继承系统或 `HTTP_PROXY`/`HTTPS_PROXY`。运营者可仅为 HF 设置 `HUGGINGFACE_SOCKS5_ADDRESS=127.0.0.1:7897`（示例端口须对应实际已配置的本机服务）。只接受数字 loopback 地址和端口，不接受代理 URL、账号密码或请求级覆盖。启用后使用固定 Cloudflare DoH endpoint 与官方 bootstrap 地址，经 SOCKS 连接、正常验证 DoH 证书；A/AAAA 共享 8 秒预算，任一族解析失败即拒绝，完整地址集合继续检查公网属性。HF/CDN 的 SOCKS CONNECT 只发送已验证的数字 IP，仍按原域名验证 TLS/SNI；DoH 不跟随重定向、不缓存过期结果，HTTP 失败不自动重试。通用网页/API 采集路径不受此配置影响。合同依据为 [Cloudflare DoH JSON API](https://developers.cloudflare.com/1.1.1.1/encryption/dns-over-https/make-api-requests/dns-json/)；本机代理属于运营者信任边界，不等于所有网络均可用。

| 限额 | 当前值与含义 |
| --- | --- |
| 文件与输入 | 每次 1–4 个实际静态文件；单文件最多 8 MiB，选定数据文件总下载最多 16 MiB |
| 行数 | `max_rows` 为 1–1000；按所选文件顺序取有界前缀，不作随机/分层采样，也不代表完整数据集 |
| 解码 | gzip 展开最多 32 MiB；Parquet 仅处理内存中的受限 bytes，并检查列数、row group 和声明展开量；不提供通用大文件流式导入 |
| 输出 | lifecycle payload 最多 4 MiB；显式映射最多 32 列，预览最多展示 20 行 |
| 时间 | 预览总上下文最多 90 秒；导入持久截止时间 120 秒，单次内部解码请求最多 30 秒 |

`manifest` 保存 repo/commit/config/split、每文件实际字节与 SHA256、字段映射、导入行数、排除字段和请求摘要。`sampled/truncated` 表示有界前缀，`total_rows=null` 表示总量未知；不能据此声称完整读取仓库或 split。文件未全部读取时不从已有前缀推算总量。

字段映射有两个互斥分支：

```json
{"text_field":"utt","id_field":"id"}
```

该分支仅导入 `source_id` 和 `text`，不自动带入其他列的值；没有 ID 字段时由固定文件路径和行号派生。数据卡内容与排除列名仍可见，因此这不是数据卡级别的 gold 隔离机制，实验者仍需单独隔离评测真值。

```json
{"columns":[{"source":"id","target":"source_id"},{"source":"messages","target":"messages","encoding":"json"},{"source":"amount","target":"amount"}]}
```

该分支只导入明确选择的列；监督答案可由用户显式选择，nested messages/tools 必须显式编码为 JSON 字符串。标量数字保留精度，Parquet 大整数不转为 float，CSV 的 `00123` 保持文本；预览以 `number_encoding=exact_string` 避免浏览器舍入，导入资产保留实际数值类型。

`POST /huggingface/imports` 必须附 `Idempotency-Key`。键绑定 tenant/user 与完整请求摘要；同键不同载荷冲突，并发同键只发起一次导入。状态为 `running/succeeded/failed`，客户端遇未知结果应按同键核对或读取已知 ID。进程中断不自动重新下载；截止后转失败，终态重放不产生新资产。当前没有独立后台 worker、取消 API 或断点续传。若需要新的尝试，用户必须明确发起新操作。

读取回执和派生产物会重查当前授权、所有者、受管凭据是否启用以及回执绑定。它不会每次访问远端仓库重新验证站外权限，也不声称能实时侦测 HF 仓库删除或远端 token 撤销；下载时真实上游拒绝仍返回失败。

## 清洗、prepared 资产与标准的关系

在 `/data-preparation` 选择已有资产：原始资产先调用独立清洗能力；已有 cleaned_asset 可以直接格式化。格式化只做显式字段白名单/重命名、必填和类型检查，以及可选的 CRLF/CR→LF 规范化。它不猜测字符串数字、不填补缺失答案、不静默删除行；失败不会生成可用 prepared 标记。

prepared 是服务器生成的资产来源证明，包含固定输入、输出与字段规则摘要。新建 annotation/tagging/SFT 任务必须同时绑定 prepared 来源和已发布的标准版本；仅手工填一个 `prepared=true` 无效。正文在这之后改变会产生新版本，旧标注区间不能直接套用。

标准包含任务类型、标签定义、纳入/排除说明、正反例和数量/空结果约束。规则直接使用受支持关键词与排除关键词；模型建议接收固定标准，服务校验标签、字段、证据位置和约束。自然语言指南被传入不代表自动证明了全部业务语义。修改草稿不会追改已启动任务的标准快照，发布新标准需显式操作。

## SFT 的完整范围

最小路径为：在 `/label-standards` 发布 QA 标准；从已有源经过清洗与字段格式化取得 prepared 表格；在 `/sft-datasets` 选择该版本和 QA 标准，明确问题/已有参考答案列，或 messages/tools JSON 列；查看逐行结果，人工审核通过后发布固定版本，再导出 JSONL。

SFT 支持 1–1000 行、每行 2–100 个消息、最多 30 个工具定义。校验精确字段、角色顺序、工具调用 ID/响应闭合、最终 assistant 答复、非空现有答案；同一提示跨 train/validation/test 或同 group 跨 split 会被拒绝。它不生成合成答案，不从分类标签自动编造问答，不执行工具调用，也不训练/部署模型。结构检查与人工批准不等同于答案事实正确或训练收益已经达标。

工具实参按有界、无网络的 JSON Schema 子集校验：单一 `type`（object/array/string/number/integer/boolean/null）、`properties/required/additionalProperties`（布尔或子 schema）、`items`、标量 `enum`、`minLength/maxLength/minItems/maxItems/minimum/maximum`。未知关键字、`$ref`、组合 schema 和类型强制转换均拒绝；schema 深度最多 12、节点最多 256，实参深度最多 32、节点最多 10000。数据库保存、审核及导出保持 schema 的精确数字和工具 arguments 原文；通过这些检查不表示实际调用过工具。

## 验收记录与复现

本页与历史的[综合 37 项 E2E](../../apps/cognida-web/e2e/ACCEPTANCE-2026-10-16-COMPLETE-LOOP.md)分开计数。本轮 10 个不同的受影响页面场景已分批通过，逐项映射和原失败批次见[独立能力验收](../../apps/cognida-web/e2e/ACCEPTANCE-2026-10-16-STANDALONE-DATA.md)，不是一次未失败的 10 项运行。HF 最终以显式受信出口完成真实公开固定文件的 3 行有界导入及下游待审标注；此前来源检查 504、遗漏精确 CDN 域导致的 preview 400 均保留。

| 验证 | 已有证据 | 不能据此声称 |
| --- | --- | --- |
| HF 核心 | Go model/service/fetcher race；Python 24 项真实 decoder/FastAPI 测试 | 已登录真实私有 HF 仓库或导入完整大型数据集 |
| HF 集成 | `TestHuggingFaceMySQLHTTPImportIndependentAndFenced`：真实 MySQL、TCP HTTP、生产 Python decoder、本地对象存储；5 并发同键单次文件下载，owner/撤权/停用/截止栅栏，精确数字与默认 gold 列排除；2.814 秒通过 | 上游 Hub fixture 是公网 Hugging Face；测试合成 token 是真实第三方认证 |
| HF 公网读取 | 直连及 cgo 诊断均在 DNS 预检 30 秒截止，未发出 TCP/TLS/HTTP；系统已有代理出口单独验证可用。新显式 SOCKS+DoH 生产工厂读取固定 BANKING77 metadata 为 200、2252 字节、1.814 秒，repo/SHA 匹配，不带凭据、不创建资产 | 不能追认原页面 38 秒 504 的具体阶段；公开元数据成功不等于完整文件导入、私有 HF 登录或站外权限持续有效 |
| HF 出口安全 | 完整 fetcher race 8.321 秒通过；真实本机 SOCKS 协议/TLS/HTTP2，完整地址与重绑定拒绝、DoH/HF 错误证书零业务 HTTP、取消关闭、重定向不泄露 Bearer、503 不重放；原 MySQL/HTTP 回归另以 2.300 秒通过 | 本机协议 fixture 不等于真实代理服务的所有配置；原数据库回归上游仍是 fixture |
| HF 真实文件预览 | `TestHuggingFacePublicBoundedPreviewDiagnostic`：固定 BANKING77 Parquet 294377 字节，经生产出口与认证 Python decoder 取 3 行，sampled/truncated 为 true，排除 label/label_text，6.658 秒通过，无持久化导入 | 不能算全库导入、全量 split 解码或业务采集完成 |
| HF 真实页面导入 | `ui-e2e-final-three` 中 HF case 17.0 秒通过：同一固定 commit/文件真实预览和导入，9993 行中取前 3 行，raw asset→清洗→prepared→固定标准→3 条 pending 标注，输出不含 label/label_text | 不能声称私有 HF 鉴权、全量数据集导入、自动审核或标签准确率已验收 |
| 标准/SFT | governance/model 核心 race；`label_standard_sft_integration_test.go` 的真实 MySQL/HTTP 发布、审核、拒绝未准备来源及 split 泄漏；既有标签建议和固定版本回归通过 | 新标准已经达到具体业务准确率或 SFT 已经训练 |
| 页面与工具 | 10 个不同场景分批通过：特征、四类标注、五节点 DAG、反馈、真实模型 QA、两种 SFT、知识标签、独立分类和 HF。最后三场 3 passed / 29.4 秒；中间七场 4 通过/3 失败及定向复验均保留 | 不合并为一次无失败的完整批次，不把规则候选当模型质量；HF/SFT 不因此成为持久 DAG adapter |
| 全量与动态消费 | Go 全 internal race（含 HF 出口与知识应用增量）104 个测试包通过、42 个包无测试；Python 717 通过/3 跳过。warehouse 真实 Result 的 99.25 输入经受控 feature 得到 100.25，保留实际源与定义 anchor 父链；动态 embedding 使用实际行数预算的数据库回归通过 | embedding 预算 fixture 不代表外部固定 revision 的真实模型物化已验收；Python 默认沙箱初跑的 24 个监听权限错误仍保留 |

HF 回归命令在专用测试库完成 migration 37 后执行。数据库连接由本机受保护环境文件注入 `COGNIDA_DOMAIN_TEST_DSN`，库名须以 `cognida_review_` 开头；不在生产库运行 fixture。Go 集成依赖 Python 项目已安装依赖的 `.venv/bin/python`，上游 HTTP fixture 与子进程均由测试关闭。

```bash
cd services/cognida-go
go test -race ./internal/model/huggingface ./internal/service/huggingface ./internal/service/collection/fetcher -count=1
go test -race -tags=integration ./internal/service/huggingface -run '^TestHuggingFaceMySQLHTTPImportIndependentAndFenced$' -count=1
```

```bash
cd services/cognida-python
.venv/bin/python -m pytest tests/huggingface -q
```

`unavailable` 应检查内部解码认证/服务是否配置；`invalid_request` 检查固定 SHA、实际文件、互斥映射和字段类型；`limit_exceeded` 必须减少选定文件或明确预算；`conflict` 需核对原幂等载荷；`forbidden` 需重新核实当前身份与凭据授权。不要通过修改键、自动重复调用或扩大范围隐藏原失败。

本轮不补计外部验收：真实 GCP BigQuery 项目及可信固定 revision 的特征 embedding 仍未提供可用验收条件。历史公开数据 benchmark 的模型成绩保留原协议，不能由新增 prepared/标准接口重写为新业务质量结论。
