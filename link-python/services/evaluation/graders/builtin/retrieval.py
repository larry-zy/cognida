"""检索评分器。

提供检索质量的评分功能。
"""

from typing import List, Optional

from ...graders.base import (
    BaseGrader,
    GraderMode,
    GraderScore,
    MetricType,
)
from ...graders.registry import register_grader
from ...metrics.retrieval import (
    ndcg_at_k,
    map_at_k,
    mrr,
    precision_at_k,
    recall_at_k,
)


@register_grader("precision")
class PrecisionGrader(BaseGrader):
    """精确率评分器。

    衡量检索结果中相关文档的比例。
    """

    def __init__(self):
        super().__init__(
            name="precision",
            mode=GraderMode.POINTWISE,
            description="精确率评分器 (0-1 分)",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        retrieved_relevant: Optional[List[int]] = None,
        k: int = 5,
        **kwargs
    ) -> GraderScore:
        """评估精确率。

        Args:
            retrieved_relevant: 检索结果的相关性标签列表 (1=相关, 0=不相关)
            k: top-k 值
        """
        if retrieved_relevant is None:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="没有提供检索结果",
                metrics={"precision": 0.0},
            )

        precision = precision_at_k([retrieved_relevant], k=k)
        score = precision * 100  # 转换为百分比

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"Top-{k} 精确率: {precision:.4f}",
            metrics={
                "precision": score,
                "precision_raw": precision,
            },
        )


@register_grader("recall")
class RecallGrader(BaseGrader):
    """召回率评分器。

    衡量相关文档被检索到的比例。
    """

    def __init__(self):
        super().__init__(
            name="recall",
            mode=GraderMode.POINTWISE,
            description="召回率评分器 (0-100 分)",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        retrieved_relevant: Optional[List[int]] = None,
        total_relevant: Optional[int] = None,
        k: int = 5,
        **kwargs
    ) -> GraderScore:
        """评估召回率。

        Args:
            retrieved_relevant: 检索结果的相关性标签列表
            total_relevant: 相关文档总数（如果为 None，从 retrieved_relevant 计算）
            k: top-k 值
        """
        if retrieved_relevant is None:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="没有提供检索结果",
                metrics={"recall": 0.0},
            )

        if total_relevant is None:
            total_relevant = sum(retrieved_relevant)

        if total_relevant == 0:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="没有相关文档",
                metrics={"recall": 100.0},  # 没有相关文档时算 100%
            )

        recall = sum(retrieved_relevant[:k]) / total_relevant
        score = recall * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"召回 {sum(retrieved_relevant[:k])}/{total_relevant} 个相关文档",
            metrics={
                "recall": score,
                "recall_raw": recall,
            },
        )


@register_grader("ndcg")
class NDCGGrader(BaseGrader):
    """NDCG 评分器。

    衡量检索结果的排序质量。
    """

    def __init__(self):
        super().__init__(
            name="ndcg",
            mode=GraderMode.POINTWISE,
            description="NDCG 评分器 (0-100 分)",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        retrieved_relevant: Optional[List[int]] = None,
        retrieved_gains: Optional[List[float]] = None,
        k: int = 5,
        **kwargs
    ) -> GraderScore:
        """评估 NDCG。

        Args:
            retrieved_relevant: 检索结果的相关性标签 (0/1)
            retrieved_gains: 检索结果的增益值 (优先使用)
            k: top-k 值
        """
        if retrieved_gains:
            ndcg = ndcg_at_k([retrieved_gains], k=k)
        elif retrieved_relevant:
            ndcg = ndcg_at_k([retrieved_relevant], k=k)
        else:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="没有提供检索结果",
                metrics={"ndcg": 0.0},
            )

        score = ndcg * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"Top-{k} NDCG: {ndcg:.4f}",
            metrics={
                "ndcg": score,
                "ndcg_raw": ndcg,
            },
        )


@register_grader("mrr")
class MRRGrader(BaseGrader):
    """MRR (Mean Reciprocal Rank) 评分器。

    衡量第一个相关文档的排名倒数。
    """

    def __init__(self):
        super().__init__(
            name="mrr",
            mode=GraderMode.POINTWISE,
            description="MRR 评分器 (0-100 分)",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        retrieved_relevant: Optional[List[int]] = None,
        **kwargs
    ) -> GraderScore:
        """评估 MRR。

        Args:
            retrieved_relevant: 检索结果的相关性标签
        """
        if retrieved_relevant is None:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="没有提供检索结果",
                metrics={"mrr": 0.0},
            )

        mrr_value = mrr([retrieved_relevant])
        score = mrr_value * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"第一个相关文档排名倒数: {mrr_value:.4f}",
            metrics={
                "mrr": score,
                "mrr_raw": mrr_value,
            },
        )


@register_grader("map")
class MAPGrader(BaseGrader):
    """MAP (Mean Average Precision) 评分器。

    衡量平均精确率。
    """

    def __init__(self):
        super().__init__(
            name="map",
            mode=GraderMode.POINTWISE,
            description="MAP 评分器 (0-100 分)",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        retrieved_relevant: Optional[List[int]] = None,
        k: int = 5,
        **kwargs
    ) -> GraderScore:
        """评估 MAP。

        Args:
            retrieved_relevant: 检索结果的相关性标签
            k: top-k 值
        """
        if retrieved_relevant is None:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="没有提供检索结果",
                metrics={"map": 0.0},
            )

        ap = ap_at_k([retrieved_relevant], k=k)
        score = ap * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"Top-{k} 平均精确率: {ap:.4f}",
            metrics={
                "map": score,
                "ap_raw": ap,
            },
        )
