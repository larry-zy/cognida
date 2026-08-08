"""Pipeline 钩子系统。

允许在 Pipeline 执行的关键节点插入自定义逻辑。
"""

from abc import ABC
from typing import Any

from ..models import CleaningResult, Decision, DimensionScore


class PipelineContext:
    """Pipeline 执行上下文。

    Attributes:
        data: 原始数据
        cleaned_data: 清洗后的数据
        quick_score: 快速评估分数
        deep_score: 深度评估分数
        config: 配置参数
        metadata: 元数据
    """

    def __init__(
        self,
        data: Any,
        config: dict[str, Any] | None = None,
    ) -> None:
        """初始化上下文。"""
        self.data = data
        self.cleaned_data: Any | None = None
        self.quick_score: float | None = None
        self.deep_score: float | None = None
        self.config = config or {}
        self.metadata: dict[str, Any] = {}

    def update(self, **kwargs: Any) -> None:
        """更新上下文。"""
        for key, value in kwargs.items():
            if hasattr(self, key):
                setattr(self, key, value)


class PipelineHook(ABC):
    """Pipeline 钩子抽象基类。

    定义了 Pipeline 执行过程中的钩子点。
    """

    hook_name: str = ""

    def before_quick_eval(self, context: PipelineContext) -> None:
        """快速评估前钩子。"""

    def after_quick_eval(
        self, context: PipelineContext, scores: list[DimensionScore]
    ) -> None:
        """快速评估后钩子。"""

    def before_cleaning(self, context: PipelineContext) -> None:
        """清洗前钩子。"""

    def after_cleaning(
        self, context: PipelineContext, result: CleaningResult
    ) -> None:
        """清洗后钩子。"""

    def before_deep_eval(self, context: PipelineContext) -> None:
        """深度评估前钩子。"""

    def after_deep_eval(
        self, context: PipelineContext, scores: list[DimensionScore]
    ) -> None:
        """深度评估后钩子。"""

    def before_decision(
        self, context: PipelineContext, scores: list[DimensionScore]
    ) -> None:
        """决策前钩子。"""

    def after_decision(
        self, context: PipelineContext, decision: Decision
    ) -> None:
        """决策后钩子。"""

    def on_error(self, context: PipelineContext, error: Exception) -> None:
        """错误发生时钩子。"""


class HookManager:
    """钩子管理器。

    管理和执行 Pipeline 钩子。
    """

    def __init__(self) -> None:
        """初始化钩子管理器。"""
        self._hooks: list[PipelineHook] = []

    def register(self, hook: PipelineHook) -> None:
        """注册钩子。

        Args:
            hook: 钩子实例
        """
        if hook not in self._hooks:
            self._hooks.append(hook)

    def unregister(self, hook: PipelineHook) -> None:
        """注销钩子。

        Args:
            hook: 钩子实例
        """
        if hook in self._hooks:
            self._hooks.remove(hook)

    def clear(self) -> None:
        """清空所有钩子。"""
        self._hooks.clear()

    @property
    def hooks(self) -> list[PipelineHook]:
        """获取已注册的钩子列表。"""
        return self._hooks.copy()

    def execute_before_quick_eval(self, context: PipelineContext) -> None:
        """执行快速评估前钩子。"""
        for hook in self._hooks:
            hook.before_quick_eval(context)

    def execute_after_quick_eval(
        self, context: PipelineContext, scores: list[DimensionScore]
    ) -> None:
        """执行快速评估后钩子。"""
        for hook in self._hooks:
            hook.after_quick_eval(context, scores)

    def execute_before_cleaning(self, context: PipelineContext) -> None:
        """执行清洗前钩子。"""
        for hook in self._hooks:
            hook.before_cleaning(context)

    def execute_after_cleaning(
        self, context: PipelineContext, result: CleaningResult
    ) -> None:
        """执行清洗后钩子。"""
        for hook in self._hooks:
            hook.after_cleaning(context, result)

    def execute_before_deep_eval(self, context: PipelineContext) -> None:
        """执行深度评估前钩子。"""
        for hook in self._hooks:
            hook.before_deep_eval(context)

    def execute_after_deep_eval(
        self, context: PipelineContext, scores: list[DimensionScore]
    ) -> None:
        """执行深度评估后钩子。"""
        for hook in self._hooks:
            hook.after_deep_eval(context, scores)

    def execute_before_decision(
        self, context: PipelineContext, scores: list[DimensionScore]
    ) -> None:
        """执行决策前钩子。"""
        for hook in self._hooks:
            hook.before_decision(context, scores)

    def execute_after_decision(
        self, context: PipelineContext, decision: Decision
    ) -> None:
        """执行决策后钩子。"""
        for hook in self._hooks:
            hook.after_decision(context, decision)

    def execute_on_error(self, context: PipelineContext, error: Exception) -> None:
        """执行错误钩子。"""
        for hook in self._hooks:
            hook.on_error(context, error)


# 内置钩子


class LoggingHook(PipelineHook):
    """日志钩子。

    记录 Pipeline 执行过程中的关键信息。
    """

    hook_name = "logging"

    def __init__(self, log_scores: bool = True) -> None:
        """初始化日志钩子。

        Args:
            log_scores: 是否记录分数详情
        """
        self.log_scores = log_scores

    def after_quick_eval(
        self, context: PipelineContext, scores: list[DimensionScore]
    ) -> None:
        """记录快速评估结果。"""
        if self.log_scores and scores:
            avg_score = sum(s.score for s in scores) / len(scores)
            context.metadata["quick_eval_summary"] = {
                "avg_score": avg_score,
                "dimension_count": len(scores),
            }

    def after_cleaning(
        self, context: PipelineContext, result: CleaningResult
    ) -> None:
        """记录清洗结果。"""
        if result is not None:
            context.metadata["cleaning_summary"] = {
                "original_count": result.original_count,
                "cleaned_count": result.cleaned_count,
                "removed_count": result.removed_count,
                "operations_count": len(result.operations),
            }

    def after_decision(
        self, context: PipelineContext, decision: Decision
    ) -> None:
        """记录决策结果。"""
        context.metadata["final_decision"] = decision.value


class ThresholdOverrideHook(PipelineHook):
    """阈值覆盖钩子。

    允许根据上下文动态调整决策阈值。
    """

    hook_name = "threshold_override"

    def __init__(
        self,
        dataset_thresholds: dict[str, dict[str, float]] | None = None,
    ) -> None:
        """初始化阈值覆盖钩子。

        Args:
            dataset_thresholds: 数据集类型特定的阈值配置
                {"critical": {"accept": 90, "reject": 60}, ...}
        """
        self.dataset_thresholds = dataset_thresholds or {}

    def before_decision(
        self, context: PipelineContext, scores: list[DimensionScore]
    ) -> None:
        """调整决策阈值。"""
        dataset_type = context.config.get("dataset_type")
        if dataset_type and dataset_type in self.dataset_thresholds:
            thresholds = self.dataset_thresholds[dataset_type]
            context.config.update(
                {
                    "accept_threshold": thresholds.get("accept", 80),
                    "reject_threshold": thresholds.get("reject", 50),
                }
            )


class MetricsHook(PipelineHook):
    """指标收集钩子。

    收集 Pipeline 执行过程中的各项指标。
    """

    hook_name = "metrics"

    def __init__(self) -> None:
        """初始化指标钩子。"""
        self.metrics: dict[str, Any] = {}

    def before_quick_eval(self, context: PipelineContext) -> None:
        """记录开始时间。"""
        import time

        self.metrics["quick_eval_start"] = time.time()

    def after_quick_eval(
        self, context: PipelineContext, scores: list[DimensionScore]
    ) -> None:
        """记录快速评估耗时。"""
        import time

        if "quick_eval_start" in self.metrics:
            duration_ms = (time.time() - self.metrics["quick_eval_start"]) * 1000
            self.metrics["quick_eval_duration_ms"] = duration_ms
            self.metrics["quick_eval_duration"] = duration_ms  # 兼容测试期望

    def before_cleaning(self, context: PipelineContext) -> None:
        """记录清洗开始时间。"""
        import time

        self.metrics["cleaning_start"] = time.time()

    def after_cleaning(
        self, context: PipelineContext, result: CleaningResult
    ) -> None:
        """记录清洗耗时。"""
        import time

        if "cleaning_start" in self.metrics:
            self.metrics["cleaning_duration_ms"] = (
                time.time() - self.metrics["cleaning_start"]
            ) * 1000

    def before_deep_eval(self, context: PipelineContext) -> None:
        """记录深度评估开始时间。"""
        import time

        self.metrics["deep_eval_start"] = time.time()

    def after_deep_eval(
        self, context: PipelineContext, scores: list[DimensionScore]
    ) -> None:
        """记录深度评估耗时。"""
        import time

        if "deep_eval_start" in self.metrics:
            self.metrics["deep_eval_duration_ms"] = (
                time.time() - self.metrics["deep_eval_start"]
            ) * 1000

    def get_metrics(self) -> dict[str, Any]:
        """获取收集的指标。"""
        return self.metrics.copy()

    def reset_metrics(self) -> None:
        """重置指标。"""
        self.metrics.clear()
