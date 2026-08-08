"""内置质量规则。

把历史 6 个维度评估器里的命令式检查逻辑，拆成一组可注册、可插拔的原子规则。
每条规则忠实平移原实现（命中数、严重级别、描述文案、样本取法均保持一致），
以保证「收敛替换」后行为等价（tests/quality 作为等价安全网）。

维度归属：
- completeness → missing_scan（必填空值 + 全列缺失率扫描，与打分的单元格口径解耦）
- accuracy    → range / pattern
- validity    → type / length
- consistency → enum / cross_field（配置驱动的跨字段逻辑）
- uniqueness  → unique / duplicate_rows
- timeliness  → staleness
"""

from __future__ import annotations

import re
from datetime import datetime, timezone, timedelta

import pandas as pd

from ..models import FieldType, SeverityLevel
from .engine import Rule, RuleResult, register_rule


# ---------------------------------------------------------------------------
# completeness
# ---------------------------------------------------------------------------
@register_rule("missing_scan")
class MissingScanRule(Rule):
    """必填字段空值 + 全列缺失率扫描。"""

    dimension = "completeness"

    def run(self, data, field_rules, config):
        results: list[RuleResult] = []

        # 必填字段空值（CRITICAL），记录已报告字段避免重复计入下方扫描
        required_with_issues: set[str] = set()
        for fr in (r for r in field_rules if r.required):
            if fr.name not in data.columns:
                continue
            null_mask = data[fr.name].isna()
            null_count = int(null_mask.sum())
            if null_count > 0:
                indices = null_mask[null_mask].index.tolist()
                results.append(
                    RuleResult(
                        field=fr.name,
                        count=null_count,
                        severity=SeverityLevel.CRITICAL,
                        description=f"必填字段 '{fr.name}' 有 {null_count} 个空值",
                        sample=self.sample_rows(data, indices, null_count),
                    )
                )
                required_with_issues.add(fr.name)

        # 全列缺失率扫描：高缺失率(WARNING) / 低缺失率(INFO)
        high_missing_threshold = config.get("high_missing_threshold", 0.5)
        for col in data.columns:
            if col in required_with_issues:
                continue
            null_mask = data[col].isna()
            null_count = int(null_mask.sum())
            missing_rate = null_count / len(data)
            if missing_rate <= 0:
                continue
            indices = null_mask[null_mask].index.tolist()
            if missing_rate >= high_missing_threshold:
                results.append(
                    RuleResult(
                        field=col,
                        count=null_count,
                        severity=SeverityLevel.WARNING,
                        description=f"字段 '{col}' 缺失率 {missing_rate:.1%} 超过阈值",
                        sample=self.sample_rows(data, indices, null_count),
                    )
                )
            else:
                results.append(
                    RuleResult(
                        field=col,
                        count=null_count,
                        severity=SeverityLevel.INFO,
                        description=f"字段 '{col}' 有 {null_count} 个空值 ({missing_rate:.1%})",
                        sample=self.sample_rows(data, indices, min(5, null_count)),
                    )
                )

        return results


# ---------------------------------------------------------------------------
# accuracy
# ---------------------------------------------------------------------------
@register_rule("range")
class RangeRule(Rule):
    """数值范围检查（min_value / max_value）。"""

    dimension = "accuracy"

    def run(self, data, field_rules, config):
        results: list[RuleResult] = []
        for fr in field_rules:
            if fr.name not in data.columns:
                continue
            if fr.min_value is None and fr.max_value is None:
                continue
            non_null = data[fr.name].dropna()
            if len(non_null) == 0:
                continue

            violation_mask = pd.Series(False, index=non_null.index)
            if fr.min_value is not None:
                violation_mask |= non_null < fr.min_value
            if fr.max_value is not None:
                violation_mask |= non_null > fr.max_value

            count = int(violation_mask.sum())
            if count > 0:
                indices = violation_mask[violation_mask].index.tolist()
                results.append(
                    RuleResult(
                        field=fr.name,
                        count=count,
                        severity=SeverityLevel.WARNING,
                        description=(
                            f"字段 '{fr.name}' 有 {count} 个值超出范围 "
                            f"[{fr.min_value}, {fr.max_value}]"
                        ),
                        sample=self.sample_rows(data, indices, count),
                    )
                )
        return results


@register_rule("pattern")
class PatternRule(Rule):
    """正则模式检查（pattern）。"""

    dimension = "accuracy"

    def run(self, data, field_rules, config):
        results: list[RuleResult] = []
        for fr in field_rules:
            if fr.name not in data.columns or not fr.pattern:
                continue
            non_null = data[fr.name].dropna()
            if len(non_null) == 0:
                continue
            try:
                pattern = re.compile(fr.pattern)
            except re.error:
                continue
            violation_mask = ~non_null.astype(str).str.match(pattern, na=False)
            count = int(violation_mask.sum())
            if count > 0:
                indices = violation_mask[violation_mask].index.tolist()
                results.append(
                    RuleResult(
                        field=fr.name,
                        count=count,
                        severity=SeverityLevel.WARNING,
                        description=(
                            f"字段 '{fr.name}' 有 {count} 个值 "
                            f"不匹配模式 '{fr.pattern}'"
                        ),
                        sample=self.sample_rows(data, indices, count),
                    )
                )
        return results


# ---------------------------------------------------------------------------
# validity
# ---------------------------------------------------------------------------
# 仅这些类型执行「无法转换」检查（沿用旧 _TYPE_CASTERS 覆盖范围）
_CASTABLE_TYPES = {FieldType.INTEGER, FieldType.FLOAT, FieldType.BOOLEAN}


@register_rule("type")
class TypeRule(Rule):
    """数据类型可转换性检查（type 非 string）。"""

    dimension = "validity"

    def run(self, data, field_rules, config):
        results: list[RuleResult] = []
        for fr in field_rules:
            if fr.name not in data.columns:
                continue
            if fr.type == FieldType.STRING or fr.type not in _CASTABLE_TYPES:
                continue
            column = data[fr.name]
            try:
                if fr.type == FieldType.BOOLEAN:
                    converted = column.astype(str).str.lower().map(
                        {"true": True, "false": False, "1": True, "0": False}
                    )
                    invalid_mask = converted.isna() & column.notna()
                else:  # INTEGER / FLOAT
                    converted = pd.to_numeric(column, errors="coerce")
                    invalid_mask = (
                        converted.isna()
                        & column.notna()
                        & (column.astype(str).str.strip() != "")
                    )
                count = int(invalid_mask.sum())
                if count > 0:
                    results.append(
                        RuleResult(
                            field=fr.name,
                            count=count,
                            severity=SeverityLevel.CRITICAL,
                            description=(
                                f"字段 '{fr.name}' 有 {count} 个值 "
                                f"无法转换为 {fr.type.value}"
                            ),
                            sample=self.sample_from_series(column, invalid_mask, count),
                        )
                    )
            except Exception:
                continue
        return results


@register_rule("length")
class LengthRule(Rule):
    """字符串长度约束检查（min_length / max_length）。"""

    dimension = "validity"

    def run(self, data, field_rules, config):
        results: list[RuleResult] = []
        for fr in field_rules:
            if fr.name not in data.columns:
                continue
            if fr.min_length is None and fr.max_length is None:
                continue
            column = data[fr.name]
            str_col = column.astype(str)

            if fr.min_length is not None:
                invalid_mask = (str_col.str.len() < fr.min_length) & column.notna()
                count = int(invalid_mask.sum())
                if count > 0:
                    results.append(
                        RuleResult(
                            field=fr.name,
                            count=count,
                            severity=SeverityLevel.WARNING,
                            description=(
                                f"字段 '{fr.name}' 有 {count} 个值 "
                                f"长度小于最小值 {fr.min_length}"
                            ),
                            sample=self.sample_from_series(column, invalid_mask, count),
                        )
                    )

            if fr.max_length is not None:
                invalid_mask = (str_col.str.len() > fr.max_length) & column.notna()
                count = int(invalid_mask.sum())
                if count > 0:
                    results.append(
                        RuleResult(
                            field=fr.name,
                            count=count,
                            severity=SeverityLevel.WARNING,
                            description=(
                                f"字段 '{fr.name}' 有 {count} 个值 "
                                f"长度超过最大值 {fr.max_length}"
                            ),
                            sample=self.sample_from_series(column, invalid_mask, count),
                        )
                    )
        return results


# ---------------------------------------------------------------------------
# consistency
# ---------------------------------------------------------------------------
@register_rule("enum")
class EnumRule(Rule):
    """枚举值一致性检查（allowed_values）。"""

    dimension = "consistency"

    def run(self, data, field_rules, config):
        results: list[RuleResult] = []
        for fr in field_rules:
            if fr.name not in data.columns or fr.allowed_values is None:
                continue
            non_null = data[fr.name].dropna()
            if len(non_null) == 0:
                continue
            invalid_mask = ~non_null.isin(fr.allowed_values)
            count = int(invalid_mask.sum())
            if count > 0:
                invalid_values = non_null[invalid_mask].unique().tolist()
                indices = invalid_mask[invalid_mask].index.tolist()
                results.append(
                    RuleResult(
                        field=fr.name,
                        count=count,
                        severity=SeverityLevel.WARNING,
                        description=(
                            f"字段 '{fr.name}' 有 {count} 个值不在允许列表中。"
                            f"无效值: {invalid_values[:5]}"
                        ),
                        sample=self.sample_rows(data, indices, count),
                    )
                )
        return results


@register_rule("cross_field")
class CrossFieldRule(Rule):
    """配置驱动的跨字段逻辑规则。

    通过 ``config['consistency_rules']`` 声明，支持内置：
    end_after_start / positive_value / non_negative。
    """

    dimension = "consistency"

    def run(self, data, field_rules, config):
        results: list[RuleResult] = []
        handlers = {
            "end_after_start": self._end_after_start,
            "positive_value": self._positive_value,
            "non_negative": self._non_negative,
        }
        for rule_config in config.get("consistency_rules", []):
            rule_name = rule_config.get("rule")
            params = rule_config.get("params", {})
            handler = handlers.get(rule_name)
            if handler is not None:
                results.extend(handler(data, **params))
        return results

    def _end_after_start(self, data, start_field, end_field):
        if start_field not in data.columns or end_field not in data.columns:
            return []
        start_col = pd.to_datetime(data[start_field], errors="coerce")
        end_col = pd.to_datetime(data[end_field], errors="coerce")
        invalid_mask = (end_col < start_col) & start_col.notna() & end_col.notna()
        count = int(invalid_mask.sum())
        if count == 0:
            return []
        indices = invalid_mask[invalid_mask].index.tolist()
        return [
            RuleResult(
                field=f"{start_field},{end_field}",
                count=count,
                severity=SeverityLevel.WARNING,
                description=f"有 {count} 条记录的 {end_field} 早于 {start_field}",
                sample=self.sample_rows(data, indices, count),
            )
        ]

    def _positive_value(self, data, field):
        if field not in data.columns:
            return []
        col = pd.to_numeric(data[field], errors="coerce")
        invalid_mask = (col <= 0) & col.notna()
        count = int(invalid_mask.sum())
        if count == 0:
            return []
        indices = invalid_mask[invalid_mask].index.tolist()
        return [
            RuleResult(
                field=field,
                count=count,
                severity=SeverityLevel.WARNING,
                description=f"字段 '{field}' 有 {count} 个非正数值",
                sample=self.sample_rows(data, indices, count),
            )
        ]

    def _non_negative(self, data, field):
        if field not in data.columns:
            return []
        col = pd.to_numeric(data[field], errors="coerce")
        invalid_mask = (col < 0) & col.notna()
        count = int(invalid_mask.sum())
        if count == 0:
            return []
        indices = invalid_mask[invalid_mask].index.tolist()
        return [
            RuleResult(
                field=field,
                count=count,
                severity=SeverityLevel.WARNING,
                description=f"字段 '{field}' 有 {count} 个负数值",
                sample=self.sample_rows(data, indices, count),
            )
        ]


# ---------------------------------------------------------------------------
# uniqueness
# ---------------------------------------------------------------------------
@register_rule("unique")
class UniqueFieldRule(Rule):
    """唯一字段重复值检查（unique=True）。"""

    dimension = "uniqueness"

    def run(self, data, field_rules, config):
        results: list[RuleResult] = []
        for fr in field_rules:
            if not getattr(fr, "unique", False) or fr.name not in data.columns:
                continue
            non_null = data[fr.name].dropna()
            if len(non_null) == 0:
                continue
            duplicates = non_null[non_null.duplicated(keep=False)]
            count = len(duplicates)
            if count > 0:
                sample_indices = duplicates.index.tolist()[:5]
                results.append(
                    RuleResult(
                        field=fr.name,
                        count=count,
                        severity=SeverityLevel.CRITICAL,
                        description=f"唯一字段 '{fr.name}' 有 {count} 个重复值",
                        sample=data.iloc[sample_indices].to_dict("records"),
                    )
                )
        return results


@register_rule("duplicate_rows")
class DuplicateRowsRule(Rule):
    """完全重复行检查。"""

    dimension = "uniqueness"

    def run(self, data, field_rules, config):
        full_row_duplicates = data[data.duplicated(keep=False)]
        count = len(full_row_duplicates)
        if count == 0:
            return []
        sample_indices = full_row_duplicates.index.tolist()[:5]
        return [
            RuleResult(
                field=None,
                count=count,
                severity=SeverityLevel.WARNING,
                description=f"有 {count} 行完全重复的记录",
                sample=data.iloc[sample_indices].to_dict("records"),
            )
        ]


# ---------------------------------------------------------------------------
# timeliness
# ---------------------------------------------------------------------------
@register_rule("staleness")
class StalenessRule(Rule):
    """数据新鲜度检查。

    分子只计「过期」记录；无时间戳字段→分数钉 100、无法解析→分数钉 0
    （通过 RuleResult.score_override 承载，与命中数无关）。
    """

    dimension = "timeliness"

    def run(self, data, field_rules, config):
        max_age_days = config.get("max_age_days", 30)
        total_records = len(data)

        timestamp_field = config.get("timestamp_field")
        if not timestamp_field:
            for fr in field_rules:
                if (
                    getattr(fr, "type", None) in (FieldType.DATE, FieldType.DATETIME)
                    and fr.name in data.columns
                ):
                    timestamp_field = fr.name
                    break

        if not timestamp_field or timestamp_field not in data.columns:
            return [
                RuleResult(
                    field=None,
                    count=0,
                    severity=SeverityLevel.INFO,
                    description="未指定时间戳字段，无法评估时效性",
                    counts_toward_score=False,
                    score_override=100.0,
                )
            ]

        try:
            timestamp_col = pd.to_datetime(data[timestamp_field], errors="coerce")
        except Exception:
            return [
                RuleResult(
                    field=timestamp_field,
                    count=0,
                    severity=SeverityLevel.WARNING,
                    description=f"无法解析字段 '{timestamp_field}' 为时间戳",
                    counts_toward_score=False,
                    score_override=0.0,
                )
            ]

        now = datetime.now(timezone.utc)
        cutoff_date = now - timedelta(days=max_age_days)
        if timestamp_col.dt.tz is None:
            cutoff = cutoff_date.replace(tzinfo=None)
        else:
            cutoff = cutoff_date
        stale_mask = (timestamp_col < cutoff) & timestamp_col.notna()
        stale_count = int(stale_mask.sum())

        results: list[RuleResult] = []
        if stale_count > 0:
            oldest_date = timestamp_col.min()
            if pd.notna(oldest_date):
                if oldest_date.tzinfo is not None:
                    oldest_date = oldest_date.tz_localize(None)
                oldest_age_days = (now.replace(tzinfo=None) - oldest_date).days
            else:
                oldest_age_days = 0
            indices = stale_mask[stale_mask].index.tolist()
            results.append(
                RuleResult(
                    field=timestamp_field,
                    count=stale_count,
                    severity=SeverityLevel.WARNING,
                    description=(
                        f"有 {stale_count} 条记录已过期（超过 {max_age_days} 天），"
                        f"最旧数据距今 {oldest_age_days} 天"
                    ),
                    sample=self.sample_rows(data, indices, stale_count),
                )
            )

        null_count = int(timestamp_col.isna().sum())
        if null_count > 0:
            results.append(
                RuleResult(
                    field=timestamp_field,
                    count=null_count,
                    severity=SeverityLevel.INFO,
                    description=f"有 {null_count} 条记录缺少时间戳",
                    counts_toward_score=False,
                )
            )

        return results


__all__ = [
    "MissingScanRule",
    "RangeRule",
    "PatternRule",
    "TypeRule",
    "LengthRule",
    "EnumRule",
    "CrossFieldRule",
    "UniqueFieldRule",
    "DuplicateRowsRule",
    "StalenessRule",
]
