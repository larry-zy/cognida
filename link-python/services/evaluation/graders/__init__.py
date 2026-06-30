"""评分器模块。"""

from .base import (
    BaseGrader,
    GraderError,
    GraderMode,
    GraderRank,
    GraderResult,
    GraderScore,
    MetricType,
    function_grader,
)
from .registry import (
    GraderRegistry,
    get_global_registry,
    get_grader,
    list_graders,
    register_grader,
    register_grader_class,
)
from .builtin.rule_based import (
    RuleBasedGrader,
    compute_rule_metrics,
    contains_match,
    exact_match,
    numeric_match,
    regex_match,
)

__all__ = [
    # Base classes
    "BaseGrader",
    "GraderResult",
    "GraderScore",
    "GraderRank",
    "GraderError",
    "MetricType",
    "GraderMode",
    "function_grader",
    # Registry
    "GraderRegistry",
    "get_global_registry",
    "get_grader",
    "list_graders",
    "register_grader",
    "register_grader_class",
    # Built-in graders
    "RuleBasedGrader",
    "exact_match",
    "contains_match",
    "regex_match",
    "numeric_match",
    "compute_rule_metrics",
]
