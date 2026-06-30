"""一致性评估器。

检查跨字段逻辑规则。
"""

from typing import Any

import pandas as pd

from .base import DimensionEvaluator
from ..models import DimensionScore, FieldRule, QualityIssue, SeverityLevel
from ..registry import register_evaluator


@register_evaluator("consistency")
class ConsistencyEvaluator(DimensionEvaluator):
    """一致性评估器。

    评估数据的一致性，包括跨字段逻辑规则和枚举值一致性。
    """

    dimension_name = "consistency"
    description = "跨字段逻辑规则验证"

    def __init__(self) -> None:
        """初始化一致性评估器。"""
        super().__init__()
        # 内置逻辑规则
        self._builtin_rules = {
            "end_after_start": self._check_end_after_start,
            "positive_value": self._check_positive_value,
            "non_negative": self._check_non_negative,
        }

    def evaluate(
        self,
        data: pd.DataFrame,
        rules: list[FieldRule] | None = None,
        config: dict[str, Any] | None = None,
    ) -> DimensionScore:
        """评估数据一致性。

        Args:
            data: 要评估的数据
            rules: 字段规则列表
            config: 配置参数，可包含 consistency_rules

        Returns:
            一致性维度评分
        """
        config = config or {}
        rules = rules or []
        issues: list[QualityIssue] = []

        total_issues = 0

        # 检查枚举值一致性
        for field_rule in rules:
            if (
                field_rule.name in data.columns
                and field_rule.allowed_values is not None
            ):
                column = data[field_rule.name]
                non_null_data = column.dropna()

                if len(non_null_data) == 0:
                    continue

                # 检查是否有不在允许列表中的值
                invalid_mask = ~non_null_data.isin(field_rule.allowed_values)
                invalid_count = invalid_mask.sum()
                invalid_values = non_null_data[invalid_mask].unique().tolist()

                if invalid_count > 0:
                    total_issues += invalid_count
                    sample = self._get_sample_rows(
                        data, invalid_mask[invalid_mask].index.tolist(), invalid_count
                    )
                    issues.append(
                        self._create_issue(
                            severity=SeverityLevel.WARNING,
                            field=field_rule.name,
                            description=(
                                f"字段 '{field_rule.name}' 有 {invalid_count} 个值不在允许列表中。"
                                f"无效值: {invalid_values[:5]}"
                            ),
                            count=invalid_count,
                            sample=sample,
                        )
                    )

        # 执行配置中的一致性规则
        consistency_rules = config.get("consistency_rules", [])
        for rule_config in consistency_rules:
            rule_name = rule_config.get("rule")
            rule_params = rule_config.get("params", {})

            if rule_name in self._builtin_rules:
                rule_issues, rule_count = self._builtin_rules[rule_name](
                    data, **rule_params
                )
                issues.extend(rule_issues)
                total_issues += rule_count

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

    def _check_end_after_start(
        self, data: pd.DataFrame, start_field: str, end_field: str
    ) -> tuple[list[QualityIssue], int]:
        """检查结束时间是否晚于开始时间。"""
        issues = []

        if start_field not in data.columns or end_field not in data.columns:
            return issues, 0

        # 确保是 datetime 类型
        start_col = pd.to_datetime(data[start_field], errors="coerce")
        end_col = pd.to_datetime(data[end_field], errors="coerce")

        # 找出无效的行
        invalid_mask = (end_col < start_col) & start_col.notna() & end_col.notna()
        invalid_count = invalid_mask.sum()

        if invalid_count > 0:
            sample = self._get_sample_rows(
                data, invalid_mask[invalid_mask].index.tolist(), invalid_count
            )
            issues.append(
                self._create_issue(
                    severity=SeverityLevel.WARNING,
                    field=f"{start_field},{end_field}",
                    description=f"有 {invalid_count} 条记录的 {end_field} 早于 {start_field}",
                    count=invalid_count,
                    sample=sample,
                )
            )

        return issues, invalid_count

    def _check_positive_value(
        self, data: pd.DataFrame, field: str
    ) -> tuple[list[QualityIssue], int]:
        """检查字段值是否为正数。"""
        issues = []

        if field not in data.columns:
            return issues, 0

        col = pd.to_numeric(data[field], errors="coerce")
        invalid_mask = (col <= 0) & col.notna()
        invalid_count = invalid_mask.sum()

        if invalid_count > 0:
            sample = self._get_sample_rows(
                data, invalid_mask[invalid_mask].index.tolist(), invalid_count
            )
            issues.append(
                self._create_issue(
                    severity=SeverityLevel.WARNING,
                    field=field,
                    description=f"字段 '{field}' 有 {invalid_count} 个非正数值",
                    count=invalid_count,
                    sample=sample,
                )
            )

        return issues, invalid_count

    def _check_non_negative(
        self, data: pd.DataFrame, field: str
    ) -> tuple[list[QualityIssue], int]:
        """检查字段值是否非负。"""
        issues = []

        if field not in data.columns:
            return issues, 0

        col = pd.to_numeric(data[field], errors="coerce")
        invalid_mask = (col < 0) & col.notna()
        invalid_count = invalid_mask.sum()

        if invalid_count > 0:
            sample = self._get_sample_rows(
                data, invalid_mask[invalid_mask].index.tolist(), invalid_count
            )
            issues.append(
                self._create_issue(
                    severity=SeverityLevel.WARNING,
                    field=field,
                    description=f"字段 '{field}' 有 {invalid_count} 个负数值",
                    count=invalid_count,
                    sample=sample,
                )
            )

        return issues, invalid_count
