"""端到端流程执行器。

整合评估、清洗、决策的完整流程。"""

import time
from typing import Any

import pandas as pd

from ..cleaning.base import Cleaner
from ..dimensions.base import DimensionEvaluator
from ..models import (
    CleaningResult,
    Decision,
    DimensionScore,
    PipelineResult,
    QualityReport,
    UnstructuredQualityReport,
)
from ..registry import CleanerRegistry, EvaluatorRegistry
from .hooks import HookManager, PipelineContext


class QualityPipeline:
    """质量评估流程。

    执行快速评估 → 清洗 → 深度评估 → 决策的完整流程。
    """

    def __init__(self) -> None:
        """初始化流程。"""
        self.hook_manager = HookManager()
        self._max_iterations = 3

    def set_max_iterations(self, max_iterations: int) -> None:
        """设置最大迭代次数。

        Args:
            max_iterations: 最大迭代次数
        """
        self._max_iterations = max_iterations

    def register_hook(self, hook) -> None:
        """注册钩子。"""
        self.hook_manager.register(hook)

    def process_structured(
        self,
        data: pd.DataFrame,
        config: dict[str, Any] | None = None,
    ) -> PipelineResult:
        """处理结构化数据。

        Args:
            data: 要处理的数据
            config: 配置参数

        Returns:
            流程执行结果
        """
        config = config or {}
        stage_times = {}
        context = PipelineContext(data, config)

        # 快速评估
        stage_start = time.time()
        self.hook_manager.execute_before_quick_eval(context)
        quick_score = self._quick_evaluate_structured(data, config)
        context.quick_score = quick_score
        self.hook_manager.execute_after_quick_eval(context, [])
        stage_times["quick_eval"] = (time.time() - stage_start) * 1000

        # 决定是否需要清洗
        quick_threshold = config.get("quick_threshold", 90)
        if quick_score < quick_threshold:
            # 清洗
            stage_start = time.time()
            self.hook_manager.execute_before_cleaning(context)
            cleaning_result = self._clean_data(data, config)
            context.cleaned_data = data  # 实际应使用清洗后的数据
            self.hook_manager.execute_after_cleaning(context, cleaning_result)
            stage_times["cleaning"] = (time.time() - stage_start) * 1000
        else:
            cleaning_result = None
            stage_times["cleaning"] = 0

        # 深度评估
        stage_start = time.time()
        self.hook_manager.execute_before_deep_eval(context)
        deep_report = self._deep_evaluate_structured(data, config)
        deep_score = deep_report.overall_score
        context.deep_score = deep_score
        self.hook_manager.execute_after_deep_eval(context, deep_report.dimensions)
        stage_times["deep_eval"] = (time.time() - stage_start) * 1000

        # 决策
        stage_start = time.time()
        self.hook_manager.execute_before_decision(context, deep_report.dimensions)
        decision = self._make_decision(deep_score, config)
        self.hook_manager.execute_after_decision(context, decision)
        stage_times["decision"] = (time.time() - stage_start) * 1000

        return PipelineResult(
            decision=decision,
            quick_score=quick_score,
            deep_score=deep_score,
            cleaning_result=cleaning_result,
            quality_report=deep_report,
            stage_times=stage_times,
        )

    def process_unstructured(
        self,
        text: str,
        config: dict[str, Any] | None = None,
    ) -> PipelineResult:
        """处理非结构化数据。

        Args:
            text: 要处理的文本
            config: 配置参数

        Returns:
            流程执行结果
        """
        config = config or {}
        stage_times = {}
        context = PipelineContext(text, config)

        # 快速评估
        stage_start = time.time()
        self.hook_manager.execute_before_quick_eval(context)
        quick_score = self._quick_evaluate_unstructured(text, config)
        context.quick_score = quick_score
        self.hook_manager.execute_after_quick_eval(context, [])
        stage_times["quick_eval"] = (time.time() - stage_start) * 1000

        # 深度评估
        stage_start = time.time()
        self.hook_manager.execute_before_deep_eval(context)
        deep_report = self._deep_evaluate_unstructured(text, config)
        deep_score = deep_report.overall_score
        context.deep_score = deep_score
        self.hook_manager.execute_after_deep_eval(context, deep_report.dimensions)
        stage_times["deep_eval"] = (time.time() - stage_start) * 1000

        # 决策
        stage_start = time.time()
        self.hook_manager.execute_before_decision(context, deep_report.dimensions)
        decision = self._make_decision(deep_score, config)
        self.hook_manager.execute_after_decision(context, decision)
        stage_times["decision"] = (time.time() - stage_start) * 1000

        return PipelineResult(
            decision=decision,
            quick_score=quick_score,
            deep_score=deep_score,
            cleaning_result=None,
            quality_report=deep_report,
            stage_times=stage_times,
        )

    def _quick_evaluate_structured(
        self, data: pd.DataFrame, config: dict[str, Any]
    ) -> float:
        """快速评估结构化数据。

        Args:
            data: 数据
            config: 配置

        Returns:
            快速评估分数
        """
        # 只做基础检查：空值比例、明显异常值
        total_cells = len(data) * len(data.columns)
        null_cells = data.isna().sum().sum()

        if total_cells == 0:
            return 100.0

        completeness = (1 - null_cells / total_cells) * 100
        return completeness

    def _deep_evaluate_structured(
        self, data: pd.DataFrame, config: dict[str, Any]
    ) -> QualityReport:
        """深度评估结构化数据。

        Args:
            data: 数据
            config: 配置

        Returns:
            质量报告
        """
        from ..rules import RuleEngine

        # 加载规则
        rule_engine = RuleEngine()
        rules = rule_engine.load_rules(config.get("rule_file"))

        # 执行各维度评估
        dimensions = []
        all_issues = []

        for dim_rule in rules.dimension_rules:
            if not dim_rule.enabled:
                continue

            evaluator = EvaluatorRegistry.get_instance(dim_rule.name)
            if evaluator:
                result = evaluator.evaluate(
                    data, rules.field_rules, {**config, **dim_rule.config}
                )
                dimensions.append(result)
                all_issues.extend(result.issues)

        # 计算总体分数
        if dimensions:
            total_weight = sum(dimension.weight for dimension in rules.dimension_rules if dimension.enabled)
            weighted_score = sum(
                d.score * next(
                    r.weight for r in rules.dimension_rules if r.name == d.name
                )
                for d in dimensions
            )
            overall_score = weighted_score / total_weight if total_weight > 0 else 0
        else:
            overall_score = 0.0

        # 排序问题
        all_issues.sort(key=lambda i: (i.severity.value, -i.count), reverse=True)

        return QualityReport(
            overall_score=overall_score,
            dimensions=dimensions,
            issues=all_issues,
            record_count=len(data),
        )

    def _quick_evaluate_unstructured(
        self, text: str, config: dict[str, Any]
    ) -> float:
        """快速评估非结构化数据。

        Args:
            text: 文本
            config: 配置

        Returns:
            快速评估分数
        """
        if not text:
            return 0.0

        # 基础检查：长度、编码
        score = 100.0

        if len(text) < 50:
            score -= 30

        # 检查乱码
        has_control_chars = any(ord(c) < 32 and c not in "\n\t\r" for c in text)
        if has_control_chars:
            score -= 20

        return max(0.0, score)

    def _deep_evaluate_unstructured(
        self, text: str, config: dict[str, Any]
    ) -> UnstructuredQualityReport:
        """深度评估非结构化数据。

        Args:
            text: 文本
            config: 配置

        Returns:
            质量报告
        """
        # 要执行的维度列表
        dimension_names = config.get(
            "dimensions",
            ["readability", "information_density", "language_quality", "duplication", "pii_detector"],
        )

        dimensions = []
        all_issues = []
        text_stats = {"length": len(text), "word_count": len(text.split())}

        for dim_name in dimension_names:
            evaluator = EvaluatorRegistry.get_instance(dim_name)
            if evaluator:
                result = evaluator.evaluate(text, config)
                dimensions.append(result)
                all_issues.extend(result.issues)

        # 计算总体分数
        overall_score = (
            sum(d.score for d in dimensions) / len(dimensions) if dimensions else 0
        )

        return UnstructuredQualityReport(
            overall_score=overall_score,
            dimensions=dimensions,
            text_stats=text_stats,
            issues=all_issues,
        )

    def _clean_data(
        self, data: pd.DataFrame, config: dict[str, Any]
    ) -> CleaningResult:
        """清洗数据。

        Args:
            data: 数据
            config: 配置

        Returns:
            清洗结果
        """
        cleaners = config.get("cleaners", ["dataframe_text_cleaner", "dedup"])

        cleaned_data = data.copy()
        all_operations = []

        for cleaner_name in cleaners:
            cleaner = CleanerRegistry.get_instance(cleaner_name)
            if cleaner:
                result = cleaner.clean(cleaned_data, config)
                cleaned_data = cleaned_data.iloc[: result.cleaned_count]
                all_operations.extend(result.operations)

        from ..models import CleaningResult

        return CleaningResult(
            original_count=len(data),
            cleaned_count=len(cleaned_data),
            removed_count=len(data) - len(cleaned_data),
            operations=all_operations,
        )

    def _make_decision(self, score: float, config: dict[str, Any]) -> Decision:
        """做出质量决策。

        Args:
            score: 质量分数
            config: 配置

        Returns:
            决策结果
        """
        # config 经 proto map<string,string> 传入时阈值可能为字符串, 统一转 float
        try:
            accept_threshold = float(config.get("accept_threshold", 80))
        except (TypeError, ValueError):
            accept_threshold = 80.0
        try:
            reject_threshold = float(config.get("reject_threshold", 50))
        except (TypeError, ValueError):
            reject_threshold = 50.0

        if score >= accept_threshold:
            return Decision.ACCEPT
        elif score <= reject_threshold:
            return Decision.REJECT
        else:
            return Decision.REVIEW
