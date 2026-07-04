"""RAG 专属评分器。

将忠实度 / 上下文相关性 / 噪声比封装为注册表可发现的 grader,
使其出现在 RAG 类型的可用指标目录中并可被单独执行。

注意:compute-metrics 主流程为效率仍走批量函数(metrics.rag);
这里的 _aevaluate 提供单条可执行实现,供目录"每项均可执行"校验与通用执行路径使用。
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
from ...metrics.rag import context_relevance, faithfulness, noise_ratio


@register_grader("faithfulness")
class FaithfulnessGrader(BaseGrader):
    """忠实度评分器(答案是否基于检索内容)。"""

    def __init__(self) -> None:
        super().__init__(
            name="faithfulness",
            mode=GraderMode.POINTWISE,
            description="忠实度评分器 (0-100 分)",
            label="忠实度",
            group="rag",
            eval_types=[EvalType.RAG],
            requires_contexts=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        **kwargs,
    ) -> GraderScore:
        if not response or not context:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少答案或检索上下文",
                metrics={"faithfulness": 0.0},
            )

        value = faithfulness([response], [context])
        score = value * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"忠实度: {value:.4f}",
            metrics={
                "faithfulness": score,
                "faithfulness_raw": value,
            },
        )


@register_grader("context_relevance")
class ContextRelevanceGrader(BaseGrader):
    """上下文相关性评分器(检索内容与问题的相关性)。"""

    def __init__(self) -> None:
        super().__init__(
            name="context_relevance",
            mode=GraderMode.POINTWISE,
            description="上下文相关性评分器 (0-100 分)",
            label="上下文相关性",
            group="rag",
            eval_types=[EvalType.RAG],
            requires_contexts=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        **kwargs,
    ) -> GraderScore:
        if not query or not context:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少问题或检索上下文",
                metrics={"context_relevance": 0.0},
            )

        value = context_relevance([query], [context])
        score = value * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"上下文相关性: {value:.4f}",
            metrics={
                "context_relevance": score,
                "context_relevance_raw": value,
            },
        )


@register_grader("noise_ratio")
class NoiseRatioGrader(BaseGrader):
    """噪声比评分器(检索到的不相关文档比例)。"""

    def __init__(self) -> None:
        super().__init__(
            name="noise_ratio",
            mode=GraderMode.POINTWISE,
            description="噪声比评分器 (0-100 分,越低越好)",
            label="噪声比",
            group="rag",
            eval_types=[EvalType.RAG],
            requires_contexts=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        relevant_pids: Optional[List[str]] = None,
        retrieved_pids: Optional[List[str]] = None,
        **kwargs,
    ) -> GraderScore:
        if not retrieved_pids:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少检索文档 ID",
                metrics={"noise_ratio": 0.0},
            )

        value = noise_ratio(
            retrieved_contexts=[context or []],
            relevant_doc_ids=[relevant_pids or []],
            retrieved_doc_ids=[retrieved_pids],
        )
        score = value * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"噪声比: {value:.4f}",
            metrics={
                "noise_ratio": score,
                "noise_ratio_raw": value,
            },
        )
