"""结构化数据质量评估维度。

包含6大维度的质量评估：
- Completeness: 完整性评估
- Accuracy: 准确性评估
- Consistency: 一致性评估
- Validity: 有效性评估
- Uniqueness: 唯一性评估
- Timeliness: 时效性评估
"""

from .base import DimensionEvaluator
from .completeness import CompletenessEvaluator
from .accuracy import AccuracyEvaluator
from .consistency import ConsistencyEvaluator
from .validity import ValidityEvaluator
from .uniqueness import UniquenessEvaluator
from .timeliness import TimelinessEvaluator

__all__ = [
    "DimensionEvaluator",
    "CompletenessEvaluator",
    "AccuracyEvaluator",
    "ConsistencyEvaluator",
    "ValidityEvaluator",
    "UniquenessEvaluator",
    "TimelinessEvaluator",
]
