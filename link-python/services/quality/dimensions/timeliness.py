"""时效性评估器。

检查数据新鲜度。
"""

from datetime import datetime, timedelta, timezone
from typing import Any

import pandas as pd

from .base import DimensionEvaluator
from ..models import DimensionScore, FieldRule, QualityIssue, SeverityLevel
from ..registry import register_evaluator


@register_evaluator("timeliness")
class TimelinessEvaluator(DimensionEvaluator):
    """时效性评估器。

    评估数据的时效性，检查数据是否过期。
    """

    dimension_name = "timeliness"
    description = "检查数据新鲜度"

    def evaluate(
        self,
        data: pd.DataFrame,
        rules: list[FieldRule] | None = None,
        config: dict[str, Any] | None = None,
    ) -> DimensionScore:
        """评估数据时效性。

        Args:
            data: 要评估的数据
            rules: 字段规则列表，可指定时间戳字段
            config: 配置参数，可包含 max_age_days

        Returns:
            时效性维度评分
        """
        config = config or {}
        rules = rules or []
        issues: list[QualityIssue] = []

        max_age_days = config.get("max_age_days", 30)
        stale_count = 0
        total_records = len(data)

        # 获取时间戳字段
        timestamp_field = config.get("timestamp_field")
        if not timestamp_field:
            # 尝试从规则中查找
            for rule in rules:
                if (
                    hasattr(rule, "type")
                    and rule.type in (FieldType.DATE, FieldType.DATETIME)
                    and rule.name in data.columns
                ):
                    timestamp_field = rule.name
                    break

        if not timestamp_field or timestamp_field not in data.columns:
            # 没有时间戳字段，返回信息
            issues.append(
                self._create_issue(
                    severity=SeverityLevel.INFO,
                    field=None,
                    description="未指定时间戳字段，无法评估时效性",
                    count=0,
                )
            )
            return DimensionScore(
                name=self.dimension_name,
                score=100.0,  # 默认满分
                passed=True,
                issues=issues,
            )

        # 转换为 datetime
        try:
            timestamp_col = pd.to_datetime(data[timestamp_field], errors="coerce")
        except Exception:
            issues.append(
                self._create_issue(
                    severity=SeverityLevel.WARNING,
                    field=timestamp_field,
                    description=f"无法解析字段 '{timestamp_field}' 为时间戳",
                    count=0,
                )
            )
            return DimensionScore(
                name=self.dimension_name,
                score=0.0,
                passed=False,
                issues=issues,
            )

        # 计算当前时间
        now = datetime.now(timezone.utc)
        cutoff_date = now - timedelta(days=max_age_days)

        # 确保 timestamp_col 有时区信息（如果它没有，假设为 UTC）
        if timestamp_col.dt.tz is None:
            # 将 cutoff_date 转换为 tz-naive 以便比较
            cutoff_date_naive = cutoff_date.replace(tzinfo=None)
            stale_mask = (timestamp_col < cutoff_date_naive) & timestamp_col.notna()
        else:
            stale_mask = (timestamp_col < cutoff_date) & timestamp_col.notna()
        stale_count = stale_mask.sum()

        if stale_count > 0:
            sample = self._get_sample_rows(
                data, stale_mask[stale_mask].index.tolist(), stale_count
            )
            # 计算最旧数据的年龄
            oldest_date = timestamp_col.min()
            if pd.notna(oldest_date):
                # 确保 oldest_date 和 now 都是 tz-naive 以便比较
                if oldest_date.tzinfo is not None:
                    oldest_date = oldest_date.tz_localize(None)
                # now 总是 tz-aware，需要移除时区信息
                now_for_calc = now.replace(tzinfo=None)
                oldest_age_days = (now_for_calc - oldest_date).days
            else:
                oldest_age_days = 0

            issues.append(
                self._create_issue(
                    severity=SeverityLevel.WARNING,
                    field=timestamp_field,
                    description=(
                        f"有 {stale_count} 条记录已过期（超过 {max_age_days} 天），"
                        f"最旧数据距今 {oldest_age_days} 天"
                    ),
                    count=stale_count,
                    sample=sample,
                )
            )

        # 检查空时间戳
        null_count = timestamp_col.isna().sum()
        if null_count > 0:
            issues.append(
                self._create_issue(
                    severity=SeverityLevel.INFO,
                    field=timestamp_field,
                    description=f"有 {null_count} 条记录缺少时间戳",
                    count=null_count,
                )
            )

        # 计算分数
        if total_records > 0:
            fresh_ratio = (total_records - stale_count) / total_records
            score = fresh_ratio * 100
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


# 导入 FieldType
from ..models import FieldType
