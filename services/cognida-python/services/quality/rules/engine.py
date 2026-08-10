"""声明式可插拔规则引擎。

把「规则」抽象为一等公民：每条规则 = 断言(check) + 产出(RuleResult:命中数/样本/严重级别/描述)，
并声明自己归属的维度(dimension)。规则通过 ``@register_rule`` 注册进 :class:`RuleRegistry`，
引擎按维度分桶执行、汇总打分。新增一条规则 = 一个类 + 一行装饰器，无需改动引擎/维度核心。

维度评估器由此退化为「薄壳」：只保留 name/threshold/weight 元数据 + 委托 ``evaluate_dimension``，
命令式检查逻辑全部下沉到 :mod:`services.quality.rules.builtins` 的规则里。

打分口径与历史各维度保持一致（等价替换）：

- 通用（accuracy/validity/consistency）：``score = max(0, 100 * (1 - Σ命中/记录数))``
- uniqueness：同上但**不截断**为 0（沿用旧实现 ``(unique/total)*100`` 可为负的口径）
- completeness：``score = max(0, 100 * (1 - 缺失单元格/总单元格))``，全表扫描、与规则命中解耦
- timeliness：只把「过期」计入分子；无时间戳字段→100、无法解析→0（由 RuleResult.score_override 承载）
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any, Callable

import pandas as pd

from ..dimension_names import Dimension

from ..models import (
    DimensionScore,
    FieldRule,
    QualityIssue,
    QualityReport,
    SeverityLevel,
)
from ..registry import EvaluatorRegistry

# 各维度默认阈值：沿用旧维度评估器代码里的默认值（均为 80，YAML dimension_rules.threshold
# 在历史评估路径中并未被透传到维度，故这里同样以代码默认值为准，保持等价）。
_DEFAULT_THRESHOLD = 80.0


# ---------------------------------------------------------------------------
# 规则契约
# ---------------------------------------------------------------------------
@dataclass
class RuleResult:
    """一条规则命中后的产出（对应一个 QualityIssue 的原料）。

    Attributes:
        field: 相关字段名（跨字段/表级问题可为 None）
        count: 影响的记录数
        severity: 严重级别
        description: 问题描述
        sample: 样本行
        counts_toward_score: 是否计入该维度扣分（部分 INFO 级注解不影响分数）
        score_override: 若非 None，直接把维度分数钉死为该值（用于 timeliness 的
            「无时间戳字段=100 / 无法解析=0」这类与命中数无关的特判）
    """

    field: str | None
    count: int
    severity: SeverityLevel
    description: str
    sample: list[dict[str, Any]] = field(default_factory=list)
    counts_toward_score: bool = True
    score_override: float | None = None


class Rule(ABC):
    """规则抽象基类。

    子类声明 :attr:`dimension`（归属维度），实现 :meth:`run` 返回命中产出。
    ``run`` 接收整张表 + 字段规则 + 配置，自行决定遍历哪些字段/做何种断言，
    以便忠实承载历史各维度的逐字段/表级检查逻辑。
    """

    #: 规则注册名（由 @register_rule 赋值）
    name: str = ""
    #: 归属维度
    dimension: str = ""

    @abstractmethod
    def run(
        self,
        data: pd.DataFrame,
        field_rules: list[FieldRule],
        config: dict[str, Any],
    ) -> list[RuleResult]:
        """对数据执行本规则，返回命中产出列表（无命中则返回空列表）。"""

    # -- 供子类复用的样本/取值工具（与旧 DimensionEvaluator 行为一致）------------
    @staticmethod
    def sample_rows(
        data: pd.DataFrame, indices: list[int], max_samples: int
    ) -> list[dict[str, Any]]:
        """按行索引取样本（沿用旧 ``_get_sample_rows``：以 iloc 定位）。"""
        if not indices:
            return []
        return data.iloc[indices[:max_samples]].to_dict("records")

    @staticmethod
    def sample_from_series(
        series: pd.Series, mask: pd.Series, max_samples: int
    ) -> list[dict[str, Any]]:
        """从单列 + 掩码取样本（沿用旧 ``_get_sample_rows_from_series``）。"""
        indices = mask[mask].index.tolist()[:max_samples]
        return [{series.name: series.iloc[i]} for i in indices]


# ---------------------------------------------------------------------------
# 注册表
# ---------------------------------------------------------------------------
class RuleRegistry:
    """规则注册表：按注册名与维度双向索引，维度内按注册顺序执行。"""

    _rules: dict[str, type[Rule]] = {}
    _by_dimension: dict[str, list[str]] = {}
    _instances: dict[str, Rule] = {}

    @classmethod
    def register(cls, name: str) -> Callable[[type[Rule]], type[Rule]]:
        """注册规则装饰器。

        Args:
            name: 规则注册名

        Example:
            @register_rule("range")
            class RangeRule(Rule):
                dimension = "accuracy"
                ...
        """

        def decorator(rule_class: type[Rule]) -> type[Rule]:
            rule_class.name = name
            cls._rules[name] = rule_class
            cls._by_dimension.setdefault(rule_class.dimension, [])
            if name not in cls._by_dimension[rule_class.dimension]:
                cls._by_dimension[rule_class.dimension].append(name)
            return rule_class

        return decorator

    @classmethod
    def get_instance(cls, name: str) -> Rule | None:
        """获取规则实例（单例）。"""
        if name not in cls._instances:
            rule_class = cls._rules.get(name)
            if rule_class is None:
                return None
            cls._instances[name] = rule_class()
        return cls._instances[name]

    @classmethod
    def rules_for_dimension(cls, dimension: str) -> list[Rule]:
        """返回某维度下的规则实例（按注册顺序）。"""
        names = cls._by_dimension.get(dimension, [])
        return [inst for name in names if (inst := cls.get_instance(name)) is not None]

    @classmethod
    def list_rules(cls) -> list[str]:
        """列出所有已注册规则名。"""
        return list(cls._rules.keys())

    @classmethod
    def list_dimensions(cls) -> list[str]:
        """列出所有出现过的维度名。"""
        return list(cls._by_dimension.keys())

    @classmethod
    def clear(cls) -> None:
        """清空注册表（主要用于测试）。"""
        cls._rules.clear()
        cls._by_dimension.clear()
        cls._instances.clear()


def register_rule(name: str) -> Callable[[type[Rule]], type[Rule]]:
    """注册规则的便捷装饰器。"""
    return RuleRegistry.register(name)


# ---------------------------------------------------------------------------
# 引擎
# ---------------------------------------------------------------------------
def _to_issue(dimension: str, result: RuleResult) -> QualityIssue:
    """把规则产出转换为质量问题。"""
    return QualityIssue(
        severity=result.severity,
        dimension=dimension,
        field=result.field,
        description=result.description,
        count=result.count,
        sample=result.sample,
    )


def _resolve_threshold(config: dict[str, Any]) -> float:
    """解析阈值（proto map 传入可能为字符串，做兜底）。"""
    try:
        return float(config.get("threshold", _DEFAULT_THRESHOLD))
    except (TypeError, ValueError):
        return _DEFAULT_THRESHOLD


class RuleEvaluationEngine:
    """规则评估引擎：唯一的结构化评估核心。

    - :meth:`evaluate_dimension` 执行单维度（供「薄壳」维度评估器委托）
    - :meth:`assemble_report` 编排多维度、加权汇总为一份 :class:`QualityReport`
      （供两条评估入口——EvaluateQuality 与 ProcessPipeline——共用，消除重复实现）
    """

    def evaluate_dimension(
        self,
        dimension: str,
        data: pd.DataFrame,
        field_rules: list[FieldRule] | None = None,
        config: dict[str, Any] | None = None,
    ) -> DimensionScore:
        """执行单个维度的全部规则并打分。"""
        field_rules = field_rules or []
        config = config or {}

        results: list[RuleResult] = []
        for rule in RuleRegistry.rules_for_dimension(dimension):
            results.extend(rule.run(data, field_rules, config))

        score = self._score_dimension(dimension, data, results, config)
        passed = score >= _resolve_threshold(config)
        issues = [_to_issue(dimension, r) for r in results]

        return DimensionScore(
            name=dimension,
            score=score,
            passed=passed,
            issues=issues,
        )

    def _score_dimension(
        self,
        dimension: str,
        data: pd.DataFrame,
        results: list[RuleResult],
        config: dict[str, Any],
    ) -> float:
        """按维度口径计算分数（与历史实现逐一对齐）。"""
        # 特判：某些结果直接钉死分数（timeliness 无字段=100 / 无法解析=0）
        for result in results:
            if result.score_override is not None:
                return result.score_override

        total_records = len(data)

        # completeness：全表缺失单元格 / 总单元格，与规则命中解耦
        if dimension == Dimension.COMPLETENESS:
            total_cells = len(data) * len(data.columns)
            if total_cells == 0:
                return 0.0
            missing_cells = int(data.isna().sum().sum())
            return max(0.0, 100.0 * (1 - missing_cells / total_cells))

        if total_records == 0:
            return 0.0

        scored = sum(r.count for r in results if r.counts_toward_score)
        raw = 100.0 * (1 - scored / total_records)
        # uniqueness 沿用旧口径不截断；其余维度截断到 [0, 100]
        if dimension == Dimension.UNIQUENESS:
            return raw
        return max(0.0, raw)

    def assemble_report(
        self,
        data: pd.DataFrame,
        field_rules: list[FieldRule],
        dimension_metas: list[Any],
        config: dict[str, Any] | None = None,
        dimensions: list[str] | None = None,
    ) -> QualityReport:
        """编排多维度评估、加权汇总为质量报告。

        Args:
            data: 数据
            field_rules: 字段规则
            dimension_metas: 维度元数据列表（需含 ``name``/``enabled``/``weight``/``config``）
            config: 额外配置（覆盖维度自身 config）
            dimensions: 指定要评估的维度名（有序）；None 时取 metas 中 enabled 的维度

        Returns:
            质量报告
        """
        config = config or {}
        meta_by_name = {m.name: m for m in dimension_metas}

        if dimensions is None:
            dimensions = [m.name for m in dimension_metas if m.enabled]

        all_dimensions: list[DimensionScore] = []
        all_issues: list[QualityIssue] = []

        for dim_name in dimensions:
            evaluator = EvaluatorRegistry.get_instance(dim_name)
            if evaluator is None:
                continue
            meta = meta_by_name.get(dim_name)
            dim_config = getattr(meta, "config", {}) if meta is not None else {}
            result = evaluator.evaluate(data, field_rules, {**dim_config, **config})
            all_dimensions.append(result)
            all_issues.extend(result.issues)

        overall_score = self._weighted_overall(all_dimensions, meta_by_name)

        # 按严重级别排序：critical → warning → info，同级按命中数降序
        severity_priority = {"critical": 1, "warning": 2, "info": 3}
        all_issues.sort(
            key=lambda i: (severity_priority.get(i.severity.value, 99), -i.count)
        )

        return QualityReport(
            overall_score=overall_score,
            dimensions=all_dimensions,
            issues=all_issues,
            record_count=len(data),
            metadata={"dimensions_evaluated": len(all_dimensions)},
        )

    @staticmethod
    def _weighted_overall(
        dimensions: list[DimensionScore], meta_by_name: dict[str, Any]
    ) -> float:
        """按维度权重加权求总体分数。"""
        if not dimensions:
            return 0.0

        def weight(name: str) -> float:
            meta = meta_by_name.get(name)
            return getattr(meta, "weight", 1.0) if meta is not None else 1.0

        total_weight = sum(weight(d.name) for d in dimensions)
        if total_weight <= 0:
            return 0.0
        weighted = sum(d.score * weight(d.name) for d in dimensions)
        return weighted / total_weight


#: 进程内共享的引擎单例
_ENGINE = RuleEvaluationEngine()


def get_engine() -> RuleEvaluationEngine:
    """获取共享的规则评估引擎实例。"""
    return _ENGINE


__all__ = [
    "Rule",
    "RuleResult",
    "RuleRegistry",
    "register_rule",
    "RuleEvaluationEngine",
    "get_engine",
]
