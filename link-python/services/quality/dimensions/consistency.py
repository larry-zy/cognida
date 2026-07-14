"""一致性评估维度（薄壳）。

检查逻辑下沉到 rules.builtins 的 EnumRule / CrossFieldRule（内置
end_after_start / positive_value / non_negative）；本类只保留维度元数据 + 委托规则引擎。
"""

from typing import Any

import pandas as pd

from .base import DimensionEvaluator
from ..models import DimensionScore, FieldRule
from ..registry import register_evaluator
from ..rules.engine import get_engine


@register_evaluator("consistency")
class ConsistencyEvaluator(DimensionEvaluator):
    """一致性评估器（委托规则引擎）。"""

    dimension_name = "consistency"
    description = "跨字段逻辑规则验证"

    def evaluate(
        self,
        data: pd.DataFrame,
        rules: list[FieldRule] | None = None,
        config: dict[str, Any] | None = None,
    ) -> DimensionScore:
        """评估数据一致性（委托 :class:`RuleEvaluationEngine`）。"""
        return get_engine().evaluate_dimension(
            self.dimension_name, data, rules, config
        )
