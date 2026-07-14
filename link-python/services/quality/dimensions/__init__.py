"""结构化数据质量评估维度。

包含6大维度的质量评估：
- Completeness: 完整性评估
- Accuracy: 准确性评估
- Consistency: 一致性评估
- Validity: 有效性评估
- Uniqueness: 唯一性评估
- Timeliness: 时效性评估
"""

# 触发内置规则注册：维度评估器已退化为委托规则引擎的「薄壳」，
# 其检查逻辑下沉在 rules.builtins 中，必须在这里导入以完成 @register_rule 注册。
from ..rules import builtins as _builtins  # noqa: F401

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
