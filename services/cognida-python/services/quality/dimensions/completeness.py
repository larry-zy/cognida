"""完整性评估维度（薄壳）。

检查逻辑下沉到 rules.builtins.MissingScanRule；本类只保留维度元数据 +
委托规则引擎，并兼容「以 dict 作为第二个位置参数传配置」的历史调用方式。
"""

from typing import Any

import pandas as pd

from .base import DimensionEvaluator
from ..models import DimensionScore, FieldRule
from ..registry import register_evaluator
from ..dimension_names import Dimension
from ..rules.engine import get_engine


@register_evaluator(Dimension.COMPLETENESS.value)
class CompletenessEvaluator(DimensionEvaluator):
    """完整性评估器（委托规则引擎）。"""

    dimension_name = Dimension.COMPLETENESS.value
    description = "检测空值和必填字段验证"

    def evaluate(
        self,
        data: pd.DataFrame,
        rules: list[FieldRule] | None = None,
        config: dict[str, Any] | None = None,
    ) -> DimensionScore:
        """评估数据完整性（委托 :class:`RuleEvaluationEngine`）。"""
        # 兼容历史用法：部分调用方把 config dict 作为第二个位置参数传入
        if rules is not None and not isinstance(rules, list) and isinstance(rules, dict):
            config = rules
            rules = None

        return get_engine().evaluate_dimension(
            self.dimension_name, data, rules, config
        )
