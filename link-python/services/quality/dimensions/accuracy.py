"""准确性评估器。

验证数据范围和格式。
"""

import re
from typing import Any

import pandas as pd

from .base import DimensionEvaluator
from ..models import DimensionScore, FieldRule, QualityIssue, SeverityLevel
from ..registry import register_evaluator


@register_evaluator("accuracy")
class AccuracyEvaluator(DimensionEvaluator):
    """准确性评估器。

    评估数据的准确性，包括范围检查和格式验证。
    """

    dimension_name = "accuracy"
    description = "范围检查和格式验证"

    def evaluate(
        self,
        data: pd.DataFrame,
        rules: list[FieldRule] | None = None,
        config: dict[str, Any] | None = None,
    ) -> DimensionScore:
        """评估数据准确性。

        Args:
            data: 要评估的数据
            rules: 字段规则列表
            config: 配置参数

        Returns:
            准确性维度评分
        """
        config = config or {}
        rules = rules or []
        issues: list[QualityIssue] = []

        total_issues = 0

        for field_rule in rules:
            if field_rule.name not in data.columns:
                continue

            column = data[field_rule.name]
            non_null_data = column.dropna()

            if len(non_null_data) == 0:
                continue

            # 范围检查
            if field_rule.min_value is not None or field_rule.max_value is not None:
                violation_mask = pd.Series(False, index=non_null_data.index)

                if field_rule.min_value is not None:
                    violation_mask |= non_null_data < field_rule.min_value

                if field_rule.max_value is not None:
                    violation_mask |= non_null_data > field_rule.max_value

                violation_count = violation_mask.sum()
                if violation_count > 0:
                    total_issues += violation_count
                    sample = self._get_sample_rows(
                        data, violation_mask[violation_mask].index.tolist(), violation_count
                    )
                    issues.append(
                        self._create_issue(
                            severity=SeverityLevel.WARNING,
                            field=field_rule.name,
                            description=(
                                f"字段 '{field_rule.name}' 有 {violation_count} 个值超出范围 "
                                f"[{field_rule.min_value}, {field_rule.max_value}]"
                            ),
                            count=violation_count,
                            sample=sample,
                        )
                    )

            # 模式检查
            if field_rule.pattern:
                try:
                    pattern = re.compile(field_rule.pattern)
                    violation_mask = ~non_null_data.astype(str).str.match(pattern, na=False)
                    violation_count = violation_mask.sum()

                    if violation_count > 0:
                        total_issues += violation_count
                        sample = self._get_sample_rows(
                            data,
                            violation_mask[violation_mask].index.tolist(),
                            violation_count,
                        )
                        issues.append(
                            self._create_issue(
                                severity=SeverityLevel.WARNING,
                                field=field_rule.name,
                                description=(
                                    f"字段 '{field_rule.name}' 有 {violation_count} 个值 "
                                    f"不匹配模式 '{field_rule.pattern}'"
                                ),
                                count=violation_count,
                                sample=sample,
                            )
                        )
                except re.error:
                    pass

        # 计算分数
        total_records = len(data)
        score = self._calculate_score(total_issues, total_records)

        threshold = config.get("threshold", 80)
        passed = self.is_passed(score, threshold)

        return DimensionScore(
            name=self.dimension_name,
            score=score,
            passed=passed,
            issues=issues,
        )
