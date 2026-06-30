"""有效性评估器。

验证数据类型和值约束。
"""

from typing import Any

import pandas as pd

from .base import DimensionEvaluator
from ..models import DimensionRule, DimensionScore, FieldRule, FieldType, QualityIssue, SeverityLevel
from ..registry import register_evaluator


@register_evaluator("validity")
class ValidityEvaluator(DimensionEvaluator):
    """有效性评估器。

    评估数据的有效性，包括数据类型验证和值约束。
    """

    dimension_name = "validity"
    description = "数据类型和值约束验证"

    # 类型转换映射
    _TYPE_CASTERS = {
        FieldType.INTEGER: pd.to_numeric,
        FieldType.FLOAT: pd.to_numeric,
        FieldType.BOOLEAN: lambda x: x.astype("boolean"),
        FieldType.STRING: lambda x: x.astype(str),
    }

    def evaluate(
        self,
        data: pd.DataFrame,
        rules: list[FieldRule] | None = None,
        config: dict[str, Any] | None = None,
    ) -> DimensionScore:
        """评估数据有效性。

        Args:
            data: 要评估的数据
            rules: 字段规则列表
            config: 配置参数

        Returns:
            有效性维度评分
        """
        config = config or {}
        rules = rules or []
        issues: list[QualityIssue] = []

        total_issues = 0

        for field_rule in rules:
            if field_rule.name not in data.columns:
                continue

            column = data[field_rule.name]

            # 数据类型验证
            if field_rule.type != FieldType.STRING:
                type_issues, type_count = self._check_data_type(column, field_rule)
                issues.extend(type_issues)
                total_issues += type_count

            # 长度验证
            if field_rule.min_length is not None or field_rule.max_length is not None:
                length_issues, length_count = self._check_length(column, field_rule)
                issues.extend(length_issues)
                total_issues += length_count

            # 枚举值验证（已在 ConsistencyEvaluator 中处理，这里可跳过）
            # 这里只处理没有在规则中定义的情况

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

    def _check_data_type(
        self, column: pd.Series, field_rule: FieldRule
    ) -> tuple[list[QualityIssue], int]:
        """检查数据类型。

        Args:
            column: 数据列
            field_rule: 字段规则

        Returns:
            (问题列表, 问题总数)
        """
        issues = []

        if field_rule.type not in self._TYPE_CASTERS:
            return issues, 0

        caster = self._TYPE_CASTERS[field_rule.type]

        try:
            # 尝试转换
            if field_rule.type == FieldType.BOOLEAN:
                converted = column.astype(str).str.lower().map(
                    {"true": True, "false": False, "1": True, "0": False}
                )
                invalid_mask = converted.isna() & column.notna()
            else:
                if field_rule.type in (FieldType.INTEGER, FieldType.FLOAT):
                    converted = pd.to_numeric(column, errors="coerce")
                    invalid_mask = converted.isna() & column.notna() & (
                        column.astype(str).str.strip() != ""
                    )
                else:
                    invalid_mask = pd.Series(False, index=column.index)

            invalid_count = invalid_mask.sum()

            if invalid_count > 0:
                sample = self._get_sample_rows_from_series(column, invalid_mask, invalid_count)
                issues.append(
                    self._create_issue(
                        severity=SeverityLevel.CRITICAL,
                        field=field_rule.name,
                        description=(
                            f"字段 '{field_rule.name}' 有 {invalid_count} 个值 "
                            f"无法转换为 {field_rule.type.value}"
                        ),
                        count=invalid_count,
                        sample=sample,
                    )
                )

        except Exception:
            pass

        return issues, sum(i.count for i in issues)

    def _check_length(
        self, column: pd.Series, field_rule: FieldRule
    ) -> tuple[list[QualityIssue], int]:
        """检查长度约束。

        Args:
            column: 数据列
            field_rule: 字段规则

        Returns:
            (问题列表, 问题总数)
        """
        issues = []

        # 转换为字符串检查长度
        str_col = column.astype(str)

        if field_rule.min_length is not None:
            invalid_mask = (str_col.str.len() < field_rule.min_length) & column.notna()
            invalid_count = invalid_mask.sum()

            if invalid_count > 0:
                sample = self._get_sample_rows_from_series(column, invalid_mask, invalid_count)
                issues.append(
                    self._create_issue(
                        severity=SeverityLevel.WARNING,
                        field=field_rule.name,
                        description=(
                            f"字段 '{field_rule.name}' 有 {invalid_count} 个值 "
                            f"长度小于最小值 {field_rule.min_length}"
                        ),
                        count=invalid_count,
                        sample=sample,
                    )
                )

        if field_rule.max_length is not None:
            invalid_mask = (str_col.str.len() > field_rule.max_length) & column.notna()
            invalid_count = invalid_mask.sum()

            if invalid_count > 0:
                sample = self._get_sample_rows_from_series(column, invalid_mask, invalid_count)
                issues.append(
                    self._create_issue(
                        severity=SeverityLevel.WARNING,
                        field=field_rule.name,
                        description=(
                            f"字段 '{field_rule.name}' 有 {invalid_count} 个值 "
                            f"长度超过最大值 {field_rule.max_length}"
                        ),
                        count=invalid_count,
                        sample=sample,
                    )
                )

        return issues, sum(i.count for i in issues)

    def _get_sample_rows_from_series(
        self, series: pd.Series, mask: pd.Series, max_samples: int = 5
    ) -> list[dict[str, Any]]:
        """从 Series 获取样本。"""
        indices = mask[mask].index.tolist()[:max_samples]
        return [{series.name: series.iloc[i]} for i in indices]
