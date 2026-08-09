# Python 评测指标计算文档

## 概述

> **边界**：评测**流程编排在 Go worker**（跑哪些样本、顺序、检索、生成、进度、状态、聚合、落库）。
> Python 只提供**无状态的指标计算**——给定 reference/hypothesis 直接算分，不持有流程与状态。
> 通信见 [跨服务通信规则](../../docs/cross-service-communication.md)。

Python 侧提供的算分能力：

- **检索指标**: Precision, Recall, NDCG, MRR, MAP
- **生成指标**: ROUGE-1/2/L, BLEU-1/2/4
- **语义相似度**: 基于 sentence-transformers
- **LLM-as-Judge**: 使用 LLM 作为评分器
- **自定义评分器**: 插件式评分器系统

## 架构

```
services/evaluation/
├── datasets/           # 数据集管理（供计算取参考数据）
│   ├── manager.py     # 数据集加载器
│   └── data/          # 数据集文件
├── graders/           # 评分器系统
│   ├── base.py        # 基类和结果类型
│   ├── registry.py    # 评分器注册表
│   ├── builtin/       # 内置评分器
│   └── custom/        # 自定义评分器
├── metrics/           # 无状态指标计算（核心）
├── strategies/        # 评分策略（组合评分器）
└── fastapi_app.py     # 无状态 compute 薄壳（HTTP :18888）
```

> 注：原 `runner.py`（评测状态机）与 `service.py`（`ExecuteEvaluation` gRPC 编排）已删除——
> 那是与 Go worker 重复的第二套编排引擎，且 Go 从未调用它。评测编排现由 Go 唯一权威承担。

## HTTP 接口（无状态 compute，:18888）

Go worker 在编排评测流程时，按需调用以下无状态端点取回指标结果。

### POST /api/v1/evaluation/compute-metrics

给定一批 `reference/hypothesis`（+可选 question/context），返回各评测族的指标分数。
无状态：进来一次算一次，不涉及数据集拉取、检索、进度或落库。

### GET /api/v1/evaluation/graders?eval_type=

列出某评测类型（generation/retrieval/semantic/rag/llm_judge）下可用的评分器。

### GET /health

健康检查。

## 数据集格式

评测数据集使用 JSON 格式：

```json
{
  "metadata": {
    "dataset_id": "my_dataset",
    "description": "我的评测数据集",
    "version": "1.0.0",
    "domains": ["general"],
    "languages": ["zh"]
  },
  "qa_pairs": [
    {
      "question": "什么是机器学习？",
      "answer": "机器学习是人工智能的一个分支...",
      "relevant_chunks": [
        {
          "chunk_id": "chunk_1",
          "content": "机器学习相关内容...",
          "score": 0.95
        }
      ],
      "metadata": {
        "category": "ai",
        "difficulty": "basic"
      }
    }
  ]
}
```

## 创建自定义评分器

### 方法 1: 类继承

```python
from services.evaluation.graders import BaseGrader, GraderScore, MetricType, register_grader

@register_grader("my_custom_grader")
class MyCustomGrader(BaseGrader):
    def __init__(self):
        super().__init__(
            name="my_custom_grader",
            description="我的自定义评分器",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        **kwargs
    ) -> GraderScore:
        # 你的评分逻辑
        score = calculate_score(response)

        return GraderScore(
            name=self.name,
            metric_type=MetricType.SCORE,
            reason="评分理由",
            metrics={"my_score": score},
        )
```

### 方法 2: 函数装饰器

```python
from services.evaluation.graders.base import function_grader, GraderScore, MetricType

@function_grader(name="simple_grader")
async def simple_grader(response: str, **kwargs) -> GraderScore:
    score = len(response) / 100  # 示例：基于长度评分

    return GraderScore(
        name="simple_grader",
        metric_type=MetricType.SCORE,
        metrics={"score": score},
    )
```

## 评测策略

### ZeroShotStrategy

直接执行评分器：

```python
from services.evaluation.strategies import get_strategy

strategy = get_strategy("zero_shot")
result = await strategy.execute(
    query="问题",
    response="答案",
    reference="参考答案",
    graders=["rouge_1", "bleu_4"],
)
```

### EnsembleStrategy

组合多个评分器：

```python
strategy = get_strategy("ensemble")
result = await strategy.execute(
    query="问题",
    response="答案",
    reference="参考答案",
    graders=["rouge_1", "bleu_4"],
    aggregation="average",  # 或 "max", "min", "weighted"
)
```

### ConditionalStrategy

根据问题类型选择评分器：

```python
strategy = get_strategy("conditional")
result = await strategy.execute(
    query="1 + 1 = ?",  # 数学问题
    response="2",
    reference="2",
)
# 自动选择数学相关的评分器
```

## 配置选项

### 环境变量

```bash
# LLM 配置
OPENAI_API_KEY=sk-xxx
OPENAI_BASE_URL=https://api.openai.com/v1
DEFAULT_LLM_MODEL=gpt-4o-mini

# Go 知识库服务
GO_KB_SERVICE_ADDRESS=localhost:50051

# 评测配置
DEFAULT_TOP_K=5
ENABLE_SEMANTIC=true
ENABLE_LLM_JUDGE=false
```

## 使用示例

### 无状态算分（Python 内直接调用）

```python
from services.evaluation.metrics import GenerationMetrics, SemanticMetrics

# 给定一对 参考答案/生成答案，直接算分——不涉及数据集拉取、检索、进度或落库
gen = GenerationMetrics.compute(reference="参考答案", hypothesis="生成答案")
print(f"ROUGE-1: {gen.rouge_1:.4f}")

sem = SemanticMetrics.compute(reference="参考答案", hypothesis="生成答案")
print(f"语义相似度: {sem.similarity:.4f}")
```

### Go worker 经 HTTP 取分（编排在 Go）

```python
# 等价的 HTTP 调用形态（Go 侧 python_client.go 即如此）
import httpx

resp = httpx.post(
    "http://localhost:18888/api/v1/evaluation/compute-metrics",
    json={
        "eval_type": "rag",
        "items": [{"reference": "参考答案", "hypothesis": "生成答案", "question": "问题"}],
    },
)
print(resp.json())  # 各评测族指标分数；Go 负责聚合/进度/落库
```

## 可用评分器列表

### 检索评分器
- `precision`: 精确率
- `recall`: 召回率
- `ndcg`: NDCG@k
- `mrr`: 平均倒数排名
- `map`: 平均精确率

### 生成评分器
- `rouge_1`: ROUGE-1
- `rouge_2`: ROUGE-2
- `rouge_l`: ROUGE-L
- `bleu_1`: BLEU-1
- `bleu_2`: BLEU-2
- `bleu_4`: BLEU-4

### 语义评分器
- `semantic_similarity`: 语义相似度
- `semantic_relevance`: 语义相关性

### LLM 评分器
- `llm_judge`: LLM 裁判（支持自定义维度）
- `llm_factual`: 事实性检查
- `llm_safety`: 安全性检查

### 规则评分器
- `exact_match`: 精确匹配
- `contains_match`: 包含匹配
- `numeric_match`: 数值匹配
- `regex_match`: 正则匹配

### 自定义评分器
- `answer_length`: 答案长度评分
- `conciseness`: 简洁性评分
- `keyword_match`: 关键词匹配
