"""Agent 评测器（轨迹 / 工具 / 任务完成）。

这些指标此前只作为 metrics/agent.py 里的批量算子存在，未注册进 grader 目录，
导致创建 agent 评测任务时前端目录只剩两个通用 LLM 评测器。此处把它们登记为
正式 grader（eval_types=[AGENT], group="agent"），使目录成为唯一事实来源。

注意：批量计算端点（compute_metrics）通过 `_AGENT_GRADERS` 集合优先拦截这些名字，
走 compute_agent_metrics 的整批算子（跨样本聚合工具集合等），不会落到下面的
逐样本 `_aevaluate`。因此这里的 `_aevaluate` 是"单样本可执行"语义的诚实实现，
供 /evaluate 单条接口与目录"每项都对应可执行 grader"的约定使用。
"""

from typing import List, Optional

from ...graders.base import (
    BaseGrader,
    EvalType,
    GraderMode,
    GraderScore,
    MetricType,
)
from ...graders.registry import register_grader
from ...metrics.agent import (
    answer_accuracy,
    step_efficiency,
    tool_order,
    tool_selection,
    trajectory_match,
)


@register_grader("answer_accuracy")
class AnswerAccuracyGrader(BaseGrader):
    """答案准确性评分器（语义相似度阈值判定，0-100 分）。"""

    def __init__(self) -> None:
        super().__init__(
            name="answer_accuracy",
            mode=GraderMode.POINTWISE,
            description="Agent 最终答案与参考答案的准确性（语义相似度 > 0.8 视为正确）",
            label="答案准确性",
            group="agent",
            eval_types=[EvalType.AGENT],
            requires_reference=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        **kwargs,
    ) -> GraderScore:
        acc = answer_accuracy([reference or ""], [response or ""])
        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"答案准确性: {acc:.4f}",
            metrics={"answer_accuracy": acc * 100, "answer_accuracy_raw": acc},
        )


@register_grader("tool_selection")
class ToolSelectionGrader(BaseGrader):
    """工具选择评分器（期望工具与实际工具的 F1，0-100 分）。"""

    def __init__(self) -> None:
        super().__init__(
            name="tool_selection",
            mode=GraderMode.POINTWISE,
            description="Agent 是否选对工具（期望工具集合 vs 实际调用集合的 precision/recall/f1）",
            label="工具选择",
            group="agent",
            eval_types=[EvalType.AGENT],
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        expected_tools: Optional[List[str]] = None,
        tools_used: Optional[List[str]] = None,
        **kwargs,
    ) -> GraderScore:
        result = tool_selection([expected_tools or []], [tools_used or []])
        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=(
                f"工具选择 F1={result['f1']:.4f} "
                f"(P={result['precision']:.4f} R={result['recall']:.4f})"
            ),
            metrics={
                "tool_selection": result["f1"] * 100,
                "tool_precision": result["precision"],
                "tool_recall": result["recall"],
                "tool_f1": result["f1"],
            },
        )


@register_grader("tool_order")
class ToolOrderGrader(BaseGrader):
    """工具调用顺序评分器（期望工具是否按序作为实际序列的子序列，0-100 分）。"""

    def __init__(self) -> None:
        super().__init__(
            name="tool_order",
            mode=GraderMode.POINTWISE,
            description="Agent 工具调用顺序是否正确（期望工具序列为实际调用序列的有序子序列）",
            label="工具调用顺序",
            group="agent",
        )
        # __init__ 里显式声明 eval_types，避免默认全类型泄漏到 llm/rag
        self.eval_types = [EvalType.AGENT]

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        expected_tools: Optional[List[str]] = None,
        tools_used: Optional[List[str]] = None,
        **kwargs,
    ) -> GraderScore:
        result = tool_order([expected_tools or []], [tools_used or []])
        ordered = result["ordered_match"]
        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"工具顺序有序命中: {ordered:.4f}",
            metrics={"tool_order": ordered * 100, "tool_order_raw": ordered},
        )


@register_grader("trajectory_match")
class TrajectoryMatchGrader(BaseGrader):
    """轨迹匹配评分器（实际步骤序列与期望步骤的匹配度，0-100 分）。"""

    def __init__(self) -> None:
        super().__init__(
            name="trajectory_match",
            mode=GraderMode.POINTWISE,
            description="Agent 执行轨迹与期望步骤的匹配度（精确匹配 + 语义相似度）",
            label="轨迹匹配",
            group="agent",
            eval_types=[EvalType.AGENT],
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        expected_steps: Optional[List[str]] = None,
        trajectory: Optional[List[str]] = None,
        **kwargs,
    ) -> GraderScore:
        result = trajectory_match([expected_steps or []], [trajectory or []])
        sim = result["similarity"]
        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=(
                f"轨迹相似度={sim:.4f} 精确匹配={result['exact_match']:.4f}"
            ),
            metrics={
                "trajectory_match": sim * 100,
                "traj_exact_match": result["exact_match"],
                "traj_similarity": sim,
            },
        )


@register_grader("step_efficiency")
class StepEfficiencyGrader(BaseGrader):
    """步骤效率评分器（实际步数与最优步数的接近程度，0-100 分）。"""

    def __init__(self) -> None:
        super().__init__(
            name="step_efficiency",
            mode=GraderMode.POINTWISE,
            description="Agent 完成任务的步数效率（实际步数与最优步数越接近越高）",
            label="步骤效率",
            group="agent",
            eval_types=[EvalType.AGENT],
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        expected_steps: Optional[List[str]] = None,
        trajectory: Optional[List[str]] = None,
        total_steps: Optional[int] = None,
        **kwargs,
    ) -> GraderScore:
        actual = total_steps if total_steps is not None else len(trajectory or [])
        optimal = len(expected_steps) if expected_steps else 1
        result = step_efficiency([actual], [optimal])
        ratio = result["optimal_ratio"]
        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=(
                f"步骤效率={ratio:.4f} (实际 {actual} / 最优 {optimal})"
            ),
            metrics={
                "step_efficiency": ratio * 100,
                "step_optimal_ratio": ratio,
                "step_avg": result["avg_steps"],
            },
        )
