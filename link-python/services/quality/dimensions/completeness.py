"""完整性评估器。

检测数据中的空值和必填字段验证。
"""

from typing import Any

import pandas as pd

from .base import DimensionEvaluator
from ..models import DimensionScore, FieldRule, QualityIssue, SeverityLevel
from ..registry import register_evaluator


@register_evaluator("completeness")
class CompletenessEvaluator(DimensionEvaluator):
    """完整性评估器。

    评估数据的完整性，检测空值和必填字段。
    """

    dimension_name = "completeness"
    description = "检测空值和必填字段验证"

    def evaluate(
        self,
        data: pd.DataFrame,
        rules: list[FieldRule] | None = None,
        config: dict[str, Any] | None = None,
    ) -> DimensionScore:
        """评估数据完整性。

        Args:
            data: 要评估的数据
            rules: 字段规则列表
            config: 配置参数

        Returns:
            完整性维度评分
        """
        # 处理测试中直接传递 dict 作为第二个参数的情况
        if rules is not None and not isinstance(rules, list) and isinstance(rules, dict):
            config = rules
            rules = None

        config = config or {}
        rules = rules or []
        issues: list[QualityIssue] = []

        # 检查必填字段，记录已检查的字段
        required_fields = [r for r in rules if r.required]
        required_fields_with_issues = set()
        for field_rule in required_fields:
            if field_rule.name in data.columns:
                null_mask = data[field_rule.name].isna()
                null_count = null_mask.sum()
                if null_count > 0:
                    null_indices = null_mask[null_mask].index.tolist()
                    sample = self._get_sample_rows(data, null_indices, null_count)
                    issues.append(
                        self._create_issue(
                            severity=SeverityLevel.CRITICAL,
                            field=field_rule.name,
                            description=f"必填字段 '{field_rule.name}' 有 {null_count} 个空值",
                            count=null_count,
                            sample=sample,
                        )
                    )
                    required_fields_with_issues.add(field_rule.name)

        # 检查所有字段的缺失率
        high_missing_threshold = config.get("high_missing_threshold", 0.5)
        for col in data.columns:
            # 跳过已经作为必填字段报告的字段
            if col in required_fields_with_issues:
                continue

            null_mask = data[col].isna()
            null_count = null_mask.sum()
            missing_rate = null_count / len(data)

            if missing_rate > 0:
                # 检查是否是高缺失率
                if missing_rate >= high_missing_threshold:
                    null_indices = null_mask[null_mask].index.tolist()
                    sample = self._get_sample_rows(data, null_indices, null_count)
                    issues.append(
                        self._create_issue(
                            severity=SeverityLevel.WARNING,
                            field=col,
                            description=f"字段 '{col}' 缺失率 {missing_rate:.1%} 超过阈值",
                            count=null_count,
                            sample=sample,
                        )
                    )
                elif missing_rate > 0:
                    # 记录信息级别的缺失
                    null_indices = null_mask[null_mask].index.tolist()
                    sample = self._get_sample_rows(
                        data, null_indices, min(5, null_count)
                    )
                    issues.append(
                        self._create_issue(
                            severity=SeverityLevel.INFO,
                            field=col,
                            description=f"字段 '{col}' 有 {null_count} 个空值 ({missing_rate:.1%})",
                            count=null_count,
                            sample=sample,
                        )
                    )

        # 计算分数
        total_cells = len(data) * len(data.columns)
        missing_cells = data.isna().sum().sum()
        score = self._calculate_score(missing_cells, total_cells)

        threshold = config.get("threshold", 80)
        passed = self.is_passed(score, threshold)

        return DimensionScore(
            name=self.dimension_name,
            score=score,
            passed=passed,
            issues=issues,
        )
