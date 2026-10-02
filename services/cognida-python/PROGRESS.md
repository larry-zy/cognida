# cognida-python 功能进度

更新：2026-10-03。本文件按可执行代码和测试记录状态，替换旧版百分比与过时目录清单。

Go 负责租户授权、资产引用、版本、任务持久化和调度；Python 提供有界计算能力。生命周期详细契约、配置和验收见 [LIFECYCLE.md](LIFECYCLE.md)。

| 能力 | 状态与实现 |
|---|---|
| MCP stdio/HTTP | 已实现，`mcp_service/server.py`；tools/list、tools/call、共享内部认证、request_id。 |
| gRPC | 已实现，`grpc_service/servicer.py`、生成 proto；文档解析、URL 获取、分块、清洗、质量评估；共享认证、租户和 request_id 上下文。 |
| 评测 HTTP | 已实现，`services/evaluation/fastapi_app.py`；批指标、自定义 grader、SQL/RAG/Agent 评测。模型类指标需要模型配置。 |
| 质量 | 已实现，`services/quality`；结构化及文本维度、清洗注册、质量报告；生命周期 `/profile` 真正复用 evaluator。 |
| 清洗 | 已实现，生命周期 `/clean` 复用 trim/normalize_ws/drop_empty/dedup/text_cleaner/pii_masker，输出来源行索引。 |
| 增强 | 已实现可用首版，生命周期 `/enrich`；文本与表结构保留的规则标签/关键词/抽取摘要/类别/PII/情感和可选真实 LLM；返回方法、置信度、来源证据、发布建议。 |
| 数据集合并 | 已实现 `/merge`；严格/联合 schema、去重、逐行来源映射。 |
| 候选推荐 | 已实现 `/recommend`；仅在 Go 提供的授权候选中做元数据分词相似度排名。 |
| 批量推理 | 已实现 `/batch-inference`；复用 OpenAI 兼容 provider，限制并发/时限，单项失败可见。未配置时 503。 |
| 文档格式 | TXT/Markdown/PDF/DOCX/CSV/XLSX 已有；新增离线 HTML/HTM、PPTX，文字/表格/图片提取按格式支持。旧二进制 Office 明确要求转 OOXML。 |
| OCR/URL 采集 | 已有 OCR、HTTP/浏览器提取与动态页面处理；URL 公网 HTTPS 默认策略、重定向/DNS 校验、文件访问根限制。OCR 需安装对应依赖。 |
| 图像 resize | 已实现 `/images/resize`；实际 Pillow 变换、去 EXIF、返回内容与摘要。 |
| 图像向量/搜索 | 已实现 CLIP→Milvus 可执行链路：embed/index/search、独立模型空间、租户与授权候选过滤。需要训练模型和 Milvus 配置；缺配置 503。 |
| 分析 | `services/analytics` 已实现统计、相关性、趋势、归因与 MCP 工具。 |
| 通用向量搜索/Collection/流式导出 | 由 Go 持有权限与存储访问；Python 不重复实现资产控制面。 |
| 特征/版本/编排/通知 | Go 生命周期、资产及 Feature Store 控制面；Python 仅返回计算载荷。 |

## 验证记录

- 第一阶段缺陷修复基线：618 项通过。
- 本阶段全套：646 passed、1 skipped（显式 opt-in Milvus 测试），包含新增 ASGI API、转换真实性、格式、认证、请求限制、超时/断开取消回归。
- 显式真实图像集成：1 passed；本地 CLIP 数学计算与本机 Milvus 的入库、幂等重试、搜索、跨租户/候选隔离，随机命名测试集合已删除。
- 图像测试使用微型随机权重，仅证明管线正确。生产语义质量必须另用训练模型和业务集验收，未配置能力不标为已可用。

```bash
uv sync --extra all
HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 uv run pytest tests -q
RUN_IMAGE_MILVUS_INTEGRATION=1 HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 uv run pytest tests/lifecycle/test_image_milvus_integration.py -q
uv run python -m mcp_service.server
uv run python -m grpc_service.server
uv run uvicorn services.evaluation.fastapi_app:app --host 127.0.0.1 --port 18888
```

MCP 默认 3000、gRPC 默认 50051、评测/生命周期 HTTP 使用 18888。运行前设置必要依赖和内部密钥；任务完成后停止自己启动的服务。依赖配置见 `.env.example`，不用示例密钥代替真实凭据。
