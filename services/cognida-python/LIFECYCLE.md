# Python 生命周期计算接口

Go 负责解析和授权资产引用、不可变版本、任务、质量门禁与通知。Python 只计算有界的内联载荷，不接受任意数据文件路径或 URL。接口挂在现有评测应用，默认使用项目约定的 18888 端口。

```bash
uv sync --extra all
uv run uvicorn services.evaluation.fastapi_app:app --host 127.0.0.1 --port 18888
```

生产环境 `APP_ENV=prod/production` 必须设置 `INTERNAL_HTTP_AUTH_TOKEN`，与 Go 相同；请求使用 `Authorization: Bearer <内部密钥>`，不能传用户 token。`X-Request-ID` 会回写。完成调试后终止自己启动的进程。

## 计算契约

所有路径前缀 `/api/v1/lifecycle`，除 `GET /capabilities` 外均为 POST。成功返回 `schema_version: "1"`、`input_digest`、`method`、`result`、`warnings`。摘要是规范化输入的 SHA-256；它标识输入，不代替 Go 的任务幂等键。

| 路径 | 请求与实际行为 |
|---|---|
| `/profile` | `input` + `options.dimensions/reference_text`；复用 DataQualityEvaluator，返回 0–100 总分、维度、问题、记录数/文本统计及敏感性建议。相关性维度需要参考文本。 |
| `/clean` | `input` + `options.operations/columns/keep`；复用 CleanerRegistry，返回变更数据、操作计数与原始行索引。支持 trim、normalize_ws、drop_empty、dedup、text_cleaner、pii_masker；keep=first/last。 |
| `/enrich` | 文本/表 `input` + `options.method=rules/llm`、features、labels；返回 tags/entities/keywords/summary/category/sentiment/sensitivity 的值、方法、置信度和来源证据。默认规则法；发布动作始终是 recommendation_only。 |
| `/merge` | `inputs` 表数组 + `options.schema_policy=strict/union,deduplicate,keys`；按列名对齐纵向合并，返回每行来源及缺失列填充信息。 |
| `/recommend` | `query`、`candidates[{asset_id,title,description,tags}]`、`limit`；以元数据分词余弦匹配排序，返回匹配词证据，仅能返回给定候选。 |
| `/batch-inference` | `items[{id,prompt}]`、concurrency、max_tokens、temperature；真实调用配置的 OpenAI 兼容客户端，各项单独返回 succeeded/failed。 |
| `/images/resize` | `images[{id,image_base64}]`、width、height、mode=contain/fit/stretch、format=png/jpeg/webp；Pillow 实际处理，输出新图 Base64、尺寸、摘要，去除 EXIF。 |
| `/images/embed` | `images[{id,image_base64}]`；配置的 CLIP 视觉编码器真实推理并 L2 归一化。 |
| `/images/index` | `images[{id,image_base64,asset_id,asset_version}]`，必须正整数 `X-Tenant-ID`；按租户、资产版本和图像 ID 幂等 upsert。 |
| `/images/search` | `image_base64`、`candidate_asset_ids`、limit，必须 `X-Tenant-ID`；租户和已授权候选双重过滤的 Milvus COSINE 搜索。 |

`input` 是下列之一：

```json
{"kind":"text","text":"需要分析的文本"}
```

```json
{"kind":"table","columns":["name","amount"],"rows":[["Alice",12],["Bob",null]]}
```

例如 `/clean` 请求：

```json
{"input":{"kind":"table","columns":["name"],"rows":[[" Alice "],[" Alice "]]},"options":{"operations":["trim","dedup"],"keep":"last"}}
```

表增强保留 input_schema、record_count 和每列类型/非空计数，证据附带原始 row_index/column。模型/规则文本计算最多采样前 20 万字符；截断会在 warnings 和 evidence_truncated 明示，类型计数仍覆盖全部输入行。

此例输出一行 `Alice`，来源行索引 `[1]`。所有索引从 0 开始。清洗仅生成新载荷，Go 保存新版本，不覆盖输入资产。规则情感/分类置信度是启发式建议，不是校准概率；LLM 证据必须逐字来自原文，否则返回 503。PII 规则覆盖电话号码、邮箱和身份证，不能声称检测全部敏感信息。

## 限制和失败

整个 HTTP 请求最多 20 MiB（流式传输同样限制），文本最多 20 万字符；表最多 1 万行、200 列、10 万单元格、4 MiB JSON，且必须矩形、列名唯一、标量有限值。合并最多 10 张表、总共 1 万行。推荐最多 200 个已授权候选。图像最多 8 张，单图压缩大小 10 MiB、1600 万像素，仅 JPEG/PNG/WebP；resize 最大边长 4096。批推理最多 20 项、并发 4、每项 60 秒。

`LIFECYCLE_TIMEOUT_SECONDS` 默认 90（范围 1–300）。请求断开会取消异步任务；CPU 线程无法强制中断，但有界输入、最多 4 个 CPU 任务和单模型推理锁限制后台消耗。取消不保证撤销已被 Milvus 接受的 upsert；相同资产版本和图像 ID 可安全重试。Go 必须保证同一不可变资产版本的数据内容一致。

未认证 401，超过请求大小 413，无效载荷 422，断开 499，超时 504；缺模型/依赖/配置、上游不可达、容量忙返回 503。批推理允许部分失败，HTTP 200 的 `result.failed` 必须检查。Go 负责队列、重试策略和用户取消状态。

## 图像模型和 Milvus

安装 `image` extra，预先提供可信的训练完成 CLIP 模型与 image processor（Hugging Face CLIP 格式）。设置 `IMAGE_MODEL_ID` 为本地目录或缓存模型 ID；生产固定 `IMAGE_MODEL_REVISION`，默认不允许下载，明确启用 `IMAGE_ALLOW_MODEL_DOWNLOAD=true` 才尝试联网。不会执行模型仓库自定义代码。

设置 `IMAGE_MILVUS_URI`（如 `http://localhost:19530`），按部署需要填写 `IMAGE_MILVUS_TOKEN`。集合名是 `IMAGE_COLLECTION_PREFIX` 加模型标识/修订/维度的哈希，与文本向量空间独立；不要修改一个已使用修订对应的模型文件。向量包含 tenant_id、asset_id、asset_version、image_id；Go 必须先授权资产和候选，内部服务密钥只授予可信服务。

实现依据：[Transformers CLIP](https://huggingface.co/docs/transformers/model_doc/clip)、[Milvus asyncio client](https://milvus.io/docs/use-async-milvus-client-with-asyncio.md)。

```bash
# 自包含单元及 ASGI API 验证
HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 uv run pytest tests/lifecycle -q
# 实际模型计算 + 本机 Milvus，创建随机命名测试集合并 finally 删除
RUN_IMAGE_MILVUS_INTEGRATION=1 HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 uv run pytest tests/lifecycle/test_image_milvus_integration.py -q
```

集成测试生成微型随机权重 CLIP，验证真实推理、入库、搜索、幂等、租户/候选隔离。它不验证语义检索质量；生产应使用训练好的模型并用业务图片验收排序质量。未配置模型时能力不可用，不能用随机模型或哈希向量代替生产语义模型。

## 文档格式

`parse_document` 和现有 MCP/gRPC 文档入口已支持离线 HTML/HTM、PPTX。HTML 不执行脚本、不抓取图片，图片返回引用元数据；PPTX 读取幻灯片文字、表格、嵌入图像，最多 1000 页、50 MiB 压缩/100 MiB 解压。Python/MCP 返回图像 Base64；gRPC 按已有 proto 只返回图像元数据。旧二进制 DOC/XLS/PPT 必须先转换为 OOXML，明确失败，不伪装支持。
