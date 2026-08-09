"""内置评分器模块。"""

from .rule_based import (
    RuleBasedGrader,
    compute_rule_metrics,
    contains_match,
    exact_match,
    numeric_match,
    regex_match,
)

__all__ = [
    "RuleBasedGrader",
    "exact_match",
    "contains_match",
    "regex_match",
    "numeric_match",
    "compute_rule_metrics",
]

# 导入时自动注册所有评分器
from . import retrieval  # noqa: F401
from . import generation  # noqa: F401
from . import semantic  # noqa: F401
from . import llm  # noqa: F401
