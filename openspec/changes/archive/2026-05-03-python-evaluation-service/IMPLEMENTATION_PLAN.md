# Python Evaluation Service - 实现方案

## 1. 架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                         Go API Gateway                          │
│  (任务管理、鉴权、路由、进度查询、结果存储)                        │
└────────────────────┬────────────────────────────────────────────┘
                     │ gRPC Streaming
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Python Evaluation Service                    │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              gRPC EvaluationService Servicer              │  │
│  │  - ExecuteEvaluation (streaming)                          │  │
│  │  - ListGraders                                            │  │
│  │  - ListDatasets                                           │  │
│  └────────────────────┬─────────────────────────────────────┘  │
│                       ▼                                          │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                  EvaluationRunner                         │  │
│  │  - 任务编排                                               │  │
│  │  - 进度追踪                                               │  │
│  │  - 错误处理                                               │  │
│  └──┬──────────┬──────────┬──────────┬──────────────────────┘  │
│     │          │          │          │                          │
│     ▼          ▼          ▼          ▼                          │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────────────┐                  │
│  │Dataset│ │Grader│ │Strategy│ │KB Connector │                  │
│  │Manager│ │Registry│ │      │ │  (Go KB)    │                  │
│  └──────┘ └──────┘ └──────┘ └──────────────┘                  │
│                                                                  │
│  Built-in Graders:                                               │
│  - RetrievalGrader (Precision, Recall, NDCG, MRR, MAP)         │
│  - GenerationGrader (ROUGE, BLEU)                              │
│  - SemanticGrader (sentence-transformers)                       │
│  - LLMJudgeGrader (多维度评分)                                  │
│                                                                  │
│  Custom Graders:                                                │
│  - 支持用户自定义 Python 评测器                                  │
│  - 热加载机制                                                    │
└─────────────────────────────────────────────────────────────────┘
```

## 2. 目录结构

```
services/evaluation/
├── __init__.py
├── service.py                    # gRPC EvaluationService 实现
├── runner.py                     # 评测编排器
├── config.py                     # 配置管理
│
├── graders/                      # 评测器插件系统
│   ├── __init__.py
│   ├── base.py                   # BaseGrader 抽象类
│   ├── registry.py               # GraderRegistry 注册表
│   ├── manager.py                # GraderManager 管理器
│   │
│   ├── builtin/                  # 内置评测器
│   │   ├── __init__.py
│   │   ├── retrieval.py          # 检索评测器
│   │   ├── generation.py         # 生成评测器
│   │   ├── semantic.py           # 语义评测器
│   │   └── llm.py                # LLM 裁判评测器
│   │
│   └── custom/                   # 自定义评测器目录
│       ├── __init__.py
│       └── .gitkeep
│
├── strategies/                   # 评测策略
│   ├── __init__.py
│   ├── base.py                   # BaseStrategy 抽象类
│   ├── zero_shot.py              # 零样本策略
│   ├── data_driven.py            # 数据驱动策略
│   ├── ensemble.py               # 集成策略
│   └── conditional.py            # 条件策略
│
├── metrics/                      # 指标计算工具（底层）
│   ├── __init__.py
│   ├── tokenizer.py              # 分词器（jieba + 英文）
│   ├── retrieval.py              # 检索指标计算
│   ├── generation.py             # 生成指标计算
│   └── semantic.py               # 语义相似度计算
│
├── datasets/                     # 数据集管理
│   ├── __init__.py
│   ├── manager.py                # DatasetManager
│   ├── loader.py                 # 数据集加载器
│   ├── validator.py              # 数据集验证
│   └── data/                     # 数据集文件目录
│       ├── default.json
│       └── .gitkeep
│
└── kb/                           # 知识库连接器
    ├── __init__.py
    └── connector.py              # Go KB 服务连接器
```

## 3. 核心组件设计

### 3.1 BaseGrader 抽象类

```python
# graders/base.py
from abc import ABC, abstractmethod
from typing import Any, Dict, List, Optional
from dataclasses import dataclass

@dataclass
class GraderConfig:
    """评测器配置"""
    name: str
    params: Dict[str, Any]
    weight: float = 1.0

@dataclass
class GraderInfo:
    """评测器元信息"""
    name: str
    type: str  # retrieval, generation, semantic, llm, custom
    description: str
    params: List[Dict[str, Any]]
    return_type: str

class BaseGrader(ABC):
    """评测器基类"""

    @classmethod
    @abstractmethod
    def get_info(cls) -> GraderInfo:
        """返回评测器元信息"""
        pass

    @abstractmethod
    def __init__(self, **params):
        """初始化评测器"""
        pass

    @abstractmethod
    def score(self, **kwargs) -> float:
        """计算分数
        Returns:
            float: 分数值 (0.0 - 1.0)
        """
        pass

    def batch_score(self, **kwargs) -> List[float]:
        """批量计算分数"""
        pass
```

### 3.2 GraderRegistry 注册表

```python
# graders/registry.py
from typing import Dict, Type, List
import importlib
import inspect

class GraderRegistry:
    """评测器注册表"""

    _graders: Dict[str, Type[BaseGrader]] = {}
    _builtin_loaded = False

    @classmethod
    def register(cls, name: str):
        """注册评测器装饰器"""
        def decorator(grader_class: Type[BaseGrader]):
            cls._graders[name] = grader_class
            return grader_class
        return decorator

    @classmethod
    def get(cls, name: str) -> Optional[Type[BaseGrader]]:
        """获取评测器类"""
        return cls._graders.get(name)

    @classmethod
    def create(cls, name: str, **params) -> Optional[BaseGrader]:
        """创建评测器实例"""
        grader_class = cls.get(name)
        if grader_class:
            return grader_class(**params)
        return None

    @classmethod
    def list_all(cls) -> Dict[str, GraderInfo]:
        """列出所有评测器"""
        return {
            name: grader_class.get_info()
            for name, grader_class in cls._graders.items()
        }

    @classmethod
    def load_builtin(cls):
        """加载内置评测器"""
        if cls._builtin_loaded:
            return

        from . import builtin

        # 自动发现并注册 builtin 目录下的评测器
        for module_name in ['retrieval', 'generation', 'semantic', 'llm']:
            module = importlib.import_module(f'.builtin.{module_name}', package='services.evaluation.graders')
            for name, obj in inspect.getmembers(module, inspect.isclass):
                if issubclass(obj, BaseGrader) and obj is not BaseGrader:
                    info = obj.get_info()
                    cls._graders[info.name] = obj

        cls._builtin_loaded = True

    @classmethod
    def load_custom(cls, custom_dir: str):
        """加载自定义评测器"""
        # TODO: 实现自定义评测器热加载
        pass
```

### 3.3 检索评测器示例

```python
# graders/builtin/retrieval.py
from ..base import BaseGrader, GraderInfo
from typing import List

@register_grader("precision")
class PrecisionGrader(BaseGrader):
    """Precision@k 评测器"""

    @classmethod
    def get_info(cls) -> GraderInfo:
        return GraderInfo(
            name="precision",
            type="retrieval",
            description="Precision@k: 检索结果中相关文档的比例",
            params=[
                {"name": "k", "type": "int", "default": "5", "description": "Top-k"}
            ],
            return_type="float"
        )

    def __init__(self, k: int = 5, **kwargs):
        self.k = k

    def score(self, retrieved: List[str], relevant: List[str]) -> float:
        """计算 Precision@k"""
        retrieved_at_k = set(retrieved[:self.k])
        relevant_set = set(relevant)
        if not retrieved_at_k:
            return 0.0
        return len(retrieved_at_k & relevant_set) / len(retrieved_at_k)
```

### 3.4 EvaluationRunner 编排器

```python
# runner.py
from typing import Iterator
from .graders.registry import GraderRegistry
from .datasets import DatasetManager
from .kb import KBConnector

class EvaluationRunner:
    """评测执行器"""

    def __init__(self):
        self.dataset_manager = DatasetManager()
        self.kb_connector = KBConnector()
        GraderRegistry.load_builtin()

    def run(
        self,
        dataset_id: str,
        knowledge_base_id: str,
        config: EvaluationConfig,
        progress_callback: callable
    ) -> EvaluationResult:
        """执行评测"""
        # 1. 加载数据集
        progress_callback(stage="init", current=0, total=1, message="加载数据集")
        dataset = self.dataset_manager.load(dataset_id)

        # 2. 执行检索评测
        progress_callback(stage="retrieval", current=0, total=len(dataset), message="检索评测")
        retrieval_results = self._run_retrieval_evaluation(dataset, knowledge_base_id, config, progress_callback)

        # 3. 执行生成评测
        progress_callback(stage="generation", current=0, total=len(dataset), message="生成评测")
        generation_results = self._run_generation_evaluation(dataset, config, progress_callback)

        # 4. 聚合结果
        progress_callback(stage="evaluation", current=0, total=1, message="聚合结果")
        result = self._aggregate_results(retrieval_results, generation_results)

        progress_callback(stage="complete", current=1, total=1, message="完成")
        return result
```

### 3.5 gRPC Service 实现

```python
# service.py
import grpc
from typing import Iterator
from proto import evaluation_pb2
from proto import evaluation_pb2_grpc
from .runner import EvaluationRunner

class EvaluationService(evaluation_pb2_grpc.EvaluationServiceServicer):
    """评测 gRPC 服务"""

    def __init__(self):
        self.runner = EvaluationRunner()

    def ExecuteEvaluation(
        self,
        request: evaluation_pb2.EvaluationRequest,
        context: grpc.ServicerContext
    ) -> Iterator[evaluation_pb2.EvaluationResponse]:
        """执行评测，流式返回进度"""
        try:
            # 验证请求
            if not request.dataset_id:
                yield self._error_response("INVALID_REQUEST", "dataset_id is required")
                return

            # 进度回调函数
            def progress_callback(stage, current, total, message):
                yield evaluation_pb2.EvaluationResponse(
                    progress=evaluation_pb2.Progress(
                        stage=stage,
                        current=current,
                        total=total,
                        message=message,
                        percentage=current / total if total > 0 else 0
                    )
                )

            # 执行评测
            result = self.runner.run(
                dataset_id=request.dataset_id,
                knowledge_base_id=request.knowledge_base_id,
                config=request.config,
                progress_callback=progress_callback
            )

            # 返回结果
            yield evaluation_pb2.EvaluationResponse(result=self._result_to_proto(result))

        except Exception as e:
            yield self._error_response("INTERNAL_ERROR", str(e))

    def ListGraders(
        self,
        request: evaluation_pb2.ListGradersRequest,
        context: grpc.ServicerContext
    ) -> evaluation_pb2.ListGradersResponse:
        """列出可用的评测器"""
        from .graders.registry import GraderRegistry

        graders_info = GraderRegistry.list_all()

        return evaluation_pb2.ListGradersResponse(
            graders=[self._grader_info_to_proto(info) for info in graders_info.values()]
        )
```

## 4. 数据格式

### 4.1 数据集格式

```json
{
  "dataset_id": "default",
  "description": "默认评测数据集",
  "metadata": {
    "domain": "general",
    "language": "zh-CN"
  },
  "qa_pairs": [
    {
      "id": "q1",
      "question": "什么是 RAG？",
      "answer": "RAG（Retrieval-Augmented Generation）是一种结合检索和生成的AI技术...",
      "relevant_chunks": [
        "doc1_chunk1",
        "doc2_chunk3"
      ]
    }
  ]
}
```

### 4.2 评测配置格式

```python
EvaluationConfig(
    top_k=5,
    retrieval_metrics=["precision", "recall", "ndcg"],
    generation_metrics=["rouge-1", "rouge-2", "rouge-l", "bleu-4"],
    enable_semantic=True,
    enable_llm_judge=True,
    llm_judge_dimensions=["accuracy", "completeness", "relevance"],
    graders=[
        GraderConfig(name="custom_relevance", params={"threshold": "0.8"}, weight=1.0)
    ],
    strategy="zero_shot",
    include_qa_results=True,
    max_concurrent=5
)
```

## 5. 实现阶段

### Phase 1: 基础框架
1. 创建目录结构
2. 定义 proto 文件
3. 实现 BaseGrader 和 GraderRegistry
4. 实现 gRPC 服务骨架

### Phase 2: 数据集管理
1. 实现 DatasetManager
2. 实现数据集验证
3. 实现数据集热加载

### Phase 3: 内置评测器
1. 实现检索评测器
2. 实现生成评测器（带正确中文分词）
3. 实现语义评测器
4. 实现 LLM 裁判评测器

### Phase 4: 集成和测试
1. 实现 EvaluationRunner
2. 集成 Go KB 连接
3. 端到端测试
4. 性能优化

## 6. 依赖项

```toml
[project.dependencies]
jieba = ">=0.42.1"
sentence-transformers = ">=2.2.0"
rouge-chinese = ">=1.0.3"
numpy = ">=1.24.0"
scipy = ">=1.10.0"
```

## 7. 环境变量

```bash
# Go 知识库服务地址
GO_KB_SERVICE_ADDRESS=localhost:50051

# LLM 配置
LLM_MODEL=gpt-4
LLM_API_KEY=sk-xxx
LLM_API_BASE=https://api.openai.com/v1

# 评测配置
EVALUATION_DATASET_DIR=./services/evaluation/datasets/data
EVALUATION_CUSTOM_GRADER_DIR=./services/evaluation/graders/custom
EVALUATION_MAX_CONCURRENT=5

# 语义模型
SEMANTIC_MODEL_PATH=paraphrase-multilingual-MiniLM-L12-v2
```

## 8. 调用流程

```
Go API Gateway
    │
    │ 1. 创建评测任务
    ▼
Database (保存任务状态: pending)
    │
    │ 2. 调用 Python ExecuteEvaluation
    ▼
Python EvaluationService
    │
    │ 3. 流式返回进度
    ▼
Go API Gateway (更新数据库进度)
    │
    │ 4. 前端轮询进度
    ▼
Client (实时查看进度)
    │
    │ 5. 评测完成
    ▼
Python 返回最终结果
    │
    │ 6. Go 保存结果到数据库
    ▼
Database (任务状态: completed)
```

## 9. 自定义评测器示例

```python
# services/evaluation/graders/custom/my_grader.py
from services.evaluation.graders.base import BaseGrader, GraderInfo
from services.evaluation.graders.registry import register_grader

@register_grader("custom_relevance")
class CustomRelevanceGrader(BaseGrader):
    """自定义相关性评测器"""

    @classmethod
    def get_info(cls) -> GraderInfo:
        return GraderInfo(
            name="custom_relevance",
            type="custom",
            description="自定义相关性评测",
            params=[
                {"name": "threshold", "type": "float", "default": "0.5"}
            ],
            return_type="float"
        )

    def __init__(self, threshold: float = 0.5, **kwargs):
        self.threshold = threshold

    def score(self, question: str, generated: str, reference: str) -> float:
        """自定义评分逻辑"""
        # 实现自定义逻辑
        return 0.85
```

## 10. 测试计划

1. **单元测试**: 每个评测器独立测试
2. **集成测试**: 完整评测流程测试
3. **性能测试**: 大数据集评测性能
4. **并发测试**: 多任务并发评测
5. **自定义评测器测试**: 验证插件系统
