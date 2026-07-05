"""
评测服务。

提供基于评分器的评测能力，支持：
- 检索指标（Precision, Recall, NDCG, MRR, MAP）
- 生成指标（ROUGE, BLEU）
- 语义相似度
- LLM-as-Judge
- 自定义评分器
"""

__version__ = "0.1.0"

from .datasets import (
    Dataset,
    DatasetManager,
    DatasetMetadata,
    QAPair,
    RelevantChunk,
    get_dataset_manager,
)
from .graders import (
    BaseGrader,
    GraderError,
    GraderMode,
    GraderRank,
    GraderResult,
    GraderScore,
    MetricType,
    get_global_registry,
    get_grader,
    list_graders,
    register_grader,
)
from .metrics import (
    AgentMetrics,
    GenerationMetrics,
    LLMJudgeMetrics,
    RetrievalMetrics,
    SemanticMetrics,
)

__all__ = [
    # Version
    "__version__",
    # Datasets
    "Dataset",
    "DatasetManager",
    "DatasetMetadata",
    "QAPair",
    "RelevantChunk",
    "get_dataset_manager",
    # Graders
    "BaseGrader",
    "GraderResult",
    "GraderScore",
    "GraderRank",
    "GraderError",
    "MetricType",
    "GraderMode",
    "get_global_registry",
    "get_grader",
    "list_graders",
    "register_grader",
    # Metrics
    "RetrievalMetrics",
    "GenerationMetrics",
    "SemanticMetrics",
    "LLMJudgeMetrics",
    "AgentMetrics",
]
