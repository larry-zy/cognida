# Python 评测服务文档

## 概述

Python 评测服务提供完整的 RAG/LLM 系统评测能力，支持：

- **检索指标**: Precision, Recall, NDCG, MRR, MAP
- **生成指标**: ROUGE-1/2/L, BLEU-1/2/4
- **语义相似度**: 基于 sentence-transformers
- **LLM-as-Judge**: 使用 LLM 作为评分器
- **自定义评分器**: 插件式评分器系统

## 架构

```
services/evaluation/
├── datasets/           # 数据集管理
│   ├── manager.py     # 数据集加载器
│   └── data/          # 数据集文件
├── graders/           # 评分器系统
│   ├── base.py        # 基类和结果类型
│   ├── registry.py    # 评分器注册表
│   ├── builtin/       # 内置评分器
│   └── custom/        # 自定义评分器
├── metrics/           # 基础指标计算
├── strategies/        # 评测策略
├── runner.py          # 评测运行器
└── service.py         # gRPC 服务
```

## gRPC 接口

### ExecuteEvaluation

流式执行评测任务。

```protobuf
message EvaluationRequest {
  string dataset_id = 1;
  string knowledge_base_id = 2;
  string model_id = 3;
  EvaluationConfig config = 4;
}

message EvaluationResponse {
  oneof {
    Progress progress = 1;
    EvaluationResult result = 2;
    Error error = 3;
  } response;
}
```

### ListGraders

列出可用的评分器。

```protobuf
message ListGradersRequest {
  string type = 1;
}

message ListGradersResponse {
  repeated GraderInfo graders = 1;
}
```

### ListDatasets

列出可用的数据集。

```protobuf
message ListDatasetsRequest {}

message ListDatasetsResponse {
  repeated DatasetInfo datasets = 1;
}
```

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

### EvaluationConfig

```python
from services.evaluation.runner import EvaluationConfig

config = EvaluationConfig(
    top_k=5,
    retrieval_metrics=["precision", "recall", "ndcg"],
    generation_metrics=["rouge_1", "rouge_l", "bleu_4"],
    enable_semantic=True,
    enable_llm_judge=False,
    include_qa_results=True,
    max_concurrent=10,
)
```

## 使用示例

### 完整评测流程

```python
import asyncio
from services.evaluation.runners import EvaluationRunner, EvaluationConfig

async def main():
    config = EvaluationConfig(
        top_k=5,
        enable_semantic=True,
    )

    runner = EvaluationRunner(config)

    result = await runner.run(
        dataset_id="default",
        knowledge_base_id="my_kb",
        model_id="gpt-4",
    )

    print(f"评测完成: {result.success_count}/{result.total_count} 成功")
    print(f"ROUGE-1: {result.generation.rouge_1:.4f}")
    print(f"语义相似度: {result.semantic.similarity:.4f}")

asyncio.run(main())
```

### gRPC 客户端

```python
import grpc
from proto import evaluation_pb2, evaluation_pb2_grpc

channel = grpc.insecure_channel("localhost:50051")
stub = evaluation_pb2_grpc.EvaluationServiceStub(channel)

# 创建请求
request = evaluation_pb2.EvaluationRequest(
    dataset_id="default",
    knowledge_base_id="my_kb",
    model_id="gpt-4",
)

# 流式获取响应
for response in stub.ExecuteEvaluation(request):
    if response.HasField("progress"):
        print(f"进度: {response.progress.message}")
    elif response.HasField("result"):
        print(f"结果: {response.result}")
    elif response.HasField("error"):
        print(f"错误: {response.error.message}")
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
