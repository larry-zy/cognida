# link-python

Python 工具服务 - 为 Go 服务提供文档处理、评测指标计算、数据分析等**无状态计算**能力增强。

> **边界**：Python 只做「计算」，不做「编排」。评测的流程编排（跑哪些样本、顺序、进度、状态、落库）由 Go worker 权威承担；Python 仅提供无状态的指标计算（给一对 reference/hypothesis 算分），经 FastAPI :18888 供 Go 按需调用。文档解析、OCR、分块、URL 抓取等经 gRPC (:50051) 提供。

## 架构定位

```
┌─────────────────────────────────────────────────────────────┐
│                      Go 服务 (Agent 大脑)                    │
│                    • 任务编排、评测、存储                     │
└──────────────────┬──────────────────────────────────────────┘
                   │ gRPC
                   ▼
┌─────────────────────────────────────────────────────────────┐
│              link-python (工具箱 + 评测 + 分析)              │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ 文档解析     │  │ OCR 识别     │  │ 文本分块     │     │
│  │ PDF/Word/Excel│ │ PaddleOCR/VLM │ │ 5种策略      │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ URL 内容获取  │  │ 评测指标计算 │  │ 数据分析     │     │
│  │ Playwright   │  │ 无状态 compute│ │ 统计/趋势洞察 │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────────────────────────┘
  端口: 50051 (文档 gRPC)  |  50053 (数据分析 gRPC)  |  18888 (评测计算 HTTP)
```

> 通信选型见 [跨服务通信规则](../docs/cross-service-communication.md)：gRPC 为主通道（文档/分析等高性能大数据），HTTP :18888 仅用于评测的无状态指标计算与健康检查。

## 职责划分

| 功能 | Go | Python |
|------|-----|--------|
| 任务编排、存储 | ✓ | |
| 评测**编排**（流程/进度/状态/落库） | ✓ | |
| 评测**指标计算** (BLEU/ROUGE/NDCG/语义/LLM-Judge) | | ✓ (无状态 compute) |
| 文档解析 | | ✓ |
| OCR 识别 | | ✓ |
| 文本分块 | | ✓ |
| URL 内容获取 | | ✓ |
| 向量计算 | | ✓ |
| 数理统计 | | ✓ |
| 趋势分析 | | ✓ |
| 数据洞察 | | ✓ |

## 特性

- **gRPC 高性能**: Protobuf 二进制协议，低延迟
- **文档处理**: PDF、Word、Excel、CSV、Markdown 解析
- **OCR 识别**: PaddleOCR（中英文）、VLM（视觉大模型）
- **文本分块**: 段落、句子、固定大小、语义、递归
- **URL 获取**: 支持动态 JS 渲染（Playwright）
- **评测服务**: RAG/LLM 系统评测，支持自定义评分器
- **数据分析**: 数理统计、趋势分析、数据洞察

## 技术栈

| 类别 | 技术 |
|------|------|
| 协议 | gRPC + Protobuf |
| 文档解析 | pypdf, python-docx, openpyxl |
| OCR | PaddleOCR |
| 网页抓取 | Playwright, httpx, BeautifulSoup4 |
| 评测 | jieba, sentence-transformers, rouge-chinese |
| 数据分析 | pandas, numpy, scipy, statsmodels, scikit-learn |

## 快速开始

### 安装依赖

```bash
# 基础依赖
pip install -e .

# 包含文档处理
pip install -e ".[document]"

# 包含评测功能
pip install -e ".[evaluation]"

# 包含数据分析
pip install -e ".[analytics]"

# 包含所有功能
pip install -e ".[all]"
```

### 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 文件，配置 LLM API Key 等
```

### 启动服务

```bash
# gRPC Server
python -m grpc_service.server
```

## gRPC 服务

### 文档解析服务

```protobuf
service DocumentReaderService {
    rpc ParseDocument(ParseRequest) returns (ParseResponse);
    rpc ParseBatch(ParseBatchRequest) returns (ParseBatchResponse);
    rpc OCRImage(OCRRequest) returns (OCRResponse);
    rpc OCRBatch(OCRBatchRequest) returns (stream OCRBatchResponse);
    rpc ChunkDocument(ChunkRequest) returns (ChunkResponse);
    rpc FetchURL(FetchURLRequest) returns (FetchURLResponse);
}
```

### 评测指标计算（HTTP :18888，无状态 compute）

评测**不经 gRPC 编排**。Python 仅暴露无状态的指标计算与评分器查询，供 Go worker 在编排评测流程时按需调用（`services/evaluation/fastapi_app.py`）：

```
POST /api/v1/evaluation/compute-metrics   # 给一批 reference/hypothesis，算 ROUGE/BLEU/NDCG/语义/LLM-Judge 分
GET  /api/v1/evaluation/graders?eval_type= # 列出某评测类型可用评分器
GET  /health                               # 健康检查
```

### 数据分析服务

```protobuf
service AnalyticsService {
    rpc ComputeMetrics(MetricsRequest) returns (MetricsResponse);
    rpc AnalyzeTrend(TrendRequest) returns (TrendResponse);
    rpc DiscoverInsights(InsightRequest) returns (InsightResponse);
}
```

### 可用方法

| 方法 | 说明 |
|------|------|
| `ParseDocument` | 解析文档（PDF/Word/Excel/CSV/MD/TXT） |
| `ParseBatch` | 批量解析文档 |
| `OCRImage` | OCR 识别图片文字 |
| `OCRBatch` | 批量 OCR 识别（流式） |
| `ChunkDocument` | 文本分块 |
| `FetchURL` | 获取 URL 内容（支持 JS 渲染） |
| `ComputeMetrics` | 计算统计指标（均值、标准差、分位数等） |
| `AnalyzeTrend` | 分析数据趋势（方向、强度、季节性） |
| `DiscoverInsights` | 发现数据洞察（趋势、异常、相关性） |

## 项目结构

```
link-python/
├── api/           # HTTP API（调试用）
├── config/        # 配置管理
├── core/          # 核心模块（logger, exceptions）
├── grpc/          # gRPC 服务层
│   ├── server.py  # 服务启动
│   └── servicer.py # 服务实现
├── proto/         # Protobuf 定义
├── scripts/       # 工具脚本
├── services/      # 服务模块
│   ├── llm/       # LLM 客户端
│   ├── document/  # 文档处理服务
│   ├── evaluation/# 评测服务
│   └── analytics/ # 数据分析服务
├── tests/         # 测试
├── examples/      # 示例
└── docs/          # 文档
```

## 开发

### 运行测试

```bash
pytest tests/ -v
```

### 运行示例

```bash
# 文档处理示例
python examples/document_demo.py

# 评测示例
python examples/evaluation_demo.py

# 数据分析示例
python examples/analytics_demo.py
```

### 生成 gRPC 代码

```bash
python scripts/generate_grpc.py
```

## 评测指标计算

详细文档请参阅 [docs/evaluation.md](docs/evaluation.md)

> 评测**流程编排在 Go worker**；Python 仅提供下列**无状态**指标计算能力，不持有评测流程/进度/状态。

### 功能

- **检索指标**: Precision, Recall, NDCG, MRR, MAP
- **生成指标**: ROUGE-1/2/L, BLEU-1/2/4
- **语义相似度**: 基于 sentence-transformers
- **LLM-as-Judge**: 使用 LLM 作为评分器
- **自定义评分器**: 插件式评分器系统

### 使用示例（无状态算分）

```python
from services.evaluation.metrics import GenerationMetrics, SemanticMetrics

# 给定一对 参考答案/生成答案，直接算分——不涉及数据集拉取、检索、进度或落库
gen = GenerationMetrics.compute(reference="参考答案", hypothesis="生成答案")
print(f"ROUGE-1: {gen.rouge_1:.4f}")

sem = SemanticMetrics.compute(reference="参考答案", hypothesis="生成答案")
print(f"语义相似度: {sem.similarity:.4f}")
```

Go worker 在编排评测流程时，经 `POST /api/v1/evaluation/compute-metrics`（:18888）批量取回这些分数。

## 数据分析服务

详细文档请参阅 [services/analytics/README.md](services/analytics/README.md)

### 功能

- **数理统计**: 描述统计、分布分析、相关性分析、假设检验
- **趋势分析**: 线性趋势、移动平均、季节分解、增长率
- **数据洞察**: 趋势洞察、异常检测、相关性发现

### 使用示例

```python
from services.analytics import DescriptiveStats, LinearTrendAnalyzer, InsightGenerator
import pandas as pd

# 描述统计
data = pd.Series([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])
result = DescriptiveStats.describe(data)
print(f"均值: {result.mean}, 标准差: {result.std}")

# 趋势分析
trend = LinearTrendAnalyzer.analyze(data)
print(f"趋势方向: {trend.direction}, 强度: {trend.strength}")

# 洞察生成
df = pd.DataFrame({
    "sales": [100, 110, 105, 120, 130, 125, 140, 150, 145, 160],
    "visitors": [1000, 1100, 1050, 1200, 1300, 1250, 1400, 1500, 1450, 1600]
})
generator = InsightGenerator()
insights = generator.generate(df)
for insight in insights:
    print(f"[{insight.severity}] {insight.title}: {insight.description}")
```

## Go 集成

### 文档处理

```go
// gRPC 客户端
conn, _ := grpc.Dial("localhost:50051", grpc.WithInsecure())
client := docreader.NewDocumentReaderServiceClient(conn)

// 解析文档
resp, _ := client.ParseDocument(ctx, &docreader.ParseRequest{
    Source: &docreader.ParseRequest_FilePath{FilePath: "test.pdf"},
    Format: "pdf",
})
```

### 评测指标计算

评测流程由 Go worker 编排；算分这一步经 HTTP :18888 调 Python（见 `internal/service/evaluation/python_client.go`）：

```go
// 无状态算分：Go worker 在编排每批样本时调用，取回指标结果
client := evaluation.NewPythonClient(cfg) // baseURL = http://localhost:18888
scores, err := client.ComputeMetrics(ctx, &evaluation.ComputeMetricsRequest{
    EvalType: "rag",
    Items: []evaluation.MetricItem{
        {Reference: "参考答案", Hypothesis: "生成答案"},
    },
})
// Go 负责：拉数据集、检索、生成、进度、状态、聚合与落库；Python 只回分数。
```

### 数据分析

```go
// 数据分析客户端 (端口 50053)
conn, _ := grpc.Dial("localhost:50053", grpc.WithInsecure())
client := analytics.NewAnalyticsServiceClient(conn)

// 计算统计指标
resp, _ := client.ComputeMetrics(ctx, &analytics.MetricsRequest{
    Data: &analytics.DataFrame{
        Columns: []string{"value"},
        Rows: []*analytics.Row{
            {Values: []string{"1"}},
            {Values: []string{"2"}},
            {Values: []string{"3"}},
        },
    },
    Metrics: []*analytics.MetricConfig{
        {Type: analytics.MetricType_MEAN, Column: "value"},
        {Type: analytics.MetricType_STD, Column: "value"},
    },
})

for _, result := range resp.Results {
    fmt.Printf("%s: %f\n", result.Name, result.Value)
}
```

## 部署

```yaml
# docker-compose.yml
services:
  link-python:
    ports:
      - "50051:50051"
    build: .
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
```

## 许可证

MIT
