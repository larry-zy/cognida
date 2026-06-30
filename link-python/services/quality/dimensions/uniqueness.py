"""唯一性评估器。

检测重复记录。
"""

from typing import Any

import pandas as pd

from .base import DimensionEvaluator
from ..models import DimensionScore, FieldRule, QualityIssue, SeverityLevel
from ..registry import register_evaluator


@register_evaluator("uniqueness")
class UniquenessEvaluator(DimensionEvaluator):
    """唯一性评估器。

    评估数据的唯一性，检测重复记录。
    """

    dimension_name = "uniqueness"
    description = "检测重复记录"

    def evaluate(
        self,
        data: pd.DataFrame,
        rules: list[FieldRule] | None = None,
        config: dict[str, Any] | None = None,
    ) -> DimensionScore:
        """评估数据唯一性。

        Args:
            data: 要评估的数据
            rules: 字段规则列表，可指定唯一字段
            config: 配置参数

        Returns:
            唯一性维度评分
        """
        config = config or {}
        rules = rules or []
        issues: list[QualityIssue] = []

        total_duplicates = 0

        # 检查指定为唯一的字段
        unique_fields = [r for r in rules if hasattr(r, "unique") and r.unique]
        for field_rule in unique_fields:
            if field_rule.name in data.columns:
                column = data[field_rule.name]
                non_null_data = column.dropna()

                if len(non_null_data) == 0:
                    continue

                # 找出重复值
                duplicates = non_null_data[non_null_data.duplicated(keep=False)]
                duplicate_count = len(duplicates)

                if duplicate_count > 0:
                    total_duplicates += duplicate_count
                    sample_indices = duplicates.index.tolist()[:5]
                    sample = data.iloc[sample_indices].to_dict("records")
                    issues.append(
                        self._create_issue(
                            severity=SeverityLevel.CRITICAL,
                            field=field_rule.name,
                            description=(
                                f"唯一字段 '{field_rule.name}' 有 {duplicate_count} 个重复值"
                            ),
                            count=duplicate_count,
                            sample=sample,
                        )
                    )

        # 检查完全重复的行
        full_row_duplicates = data[data.duplicated(keep=False)]
        duplicate_row_count = len(full_row_duplicates)

        if duplicate_row_count > 0:
            total_duplicates += duplicate_row_count
            sample_indices = full_row_duplicates.index.tolist()[:5]
            sample = data.iloc[sample_indices].to_dict("records")
            issues.append(
                self._create_issue(
                    severity=SeverityLevel.WARNING,
                    field=None,
                    description=f"有 {duplicate_row_count} 行完全重复的记录",
                    count=duplicate_row_count,
                    sample=sample,
                )
            )

        # 计算分数
        total_records = len(data)
        if total_records > 0:
            unique_records = total_records - total_duplicates
            score = (unique_records / total_records) * 100
        else:
            score = 0.0

        threshold = config.get("threshold", 80)
        passed = self.is_passed(score, threshold)

        return DimensionScore(
            name=self.dimension_name,
            score=score,
            passed=passed,
            issues=issues,
        )
