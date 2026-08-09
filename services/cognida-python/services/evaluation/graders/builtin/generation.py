"""生成评分器。

提供文本生成质量的评分功能。
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
from ...metrics.generation import bleu_at_n, rouge_l, rouge_n


@register_grader("rouge")
class RougeGrader(BaseGrader):
    """ROUGE 评分器。

    支持 ROUGE-1、ROUGE-2、ROUGE-L。
    """

    def __init__(self) -> None:
        super().__init__(
            name="rouge",
            mode=GraderMode.POINTWISE,
            description="ROUGE 评分器 (0-100 分)",
            label="ROUGE",
            group="generation",
            eval_types=[EvalType.LLM, EvalType.RAG],
            requires_reference=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        rouge_type: str = "l",
        **kwargs
    ) -> GraderScore:
        """评估 ROUGE 分数。

        Args:
            response: 生成的文本
            reference: 参考文本
            rouge_type: ROUGE 类型 ("1", "2", "l")
        """
        if not reference or not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少参考答案或生成答案",
                metrics={"rouge": 0.0},
            )

        if rouge_type == "1":
            rouge_result = rouge_n(reference, response, n=1)
            metric_name = "rouge_1"
        elif rouge_type == "2":
            rouge_result = rouge_n(reference, response, n=2)
            metric_name = "rouge_2"
        else:  # rouge_l
            rouge_result = rouge_l(reference, response)
            metric_name = "rouge_l"

        score = rouge_result * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"ROUGE-{rouge_type.upper()}: {rouge_result:.4f}",
            metrics={
                "rouge": score,
                f"{metric_name}_raw": rouge_result,
            },
        )


@register_grader("rouge_1")
class Rouge1Grader(BaseGrader):
    """ROUGE-1 评分器（单词重叠）。"""

    def __init__(self) -> None:
        super().__init__(
            name="rouge_1",
            mode=GraderMode.POINTWISE,
            description="ROUGE-1 评分器 (0-100 分)",
            label="ROUGE-1",
            group="generation",
            eval_types=[EvalType.LLM, EvalType.RAG],
            requires_reference=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        **kwargs
    ) -> GraderScore:
        """评估 ROUGE-1 分数。"""
        if not reference or not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少参考答案或生成答案",
                metrics={"rouge_1": 0.0},
            )

        rouge_result = rouge_n(reference, response, n=1)
        score = rouge_result * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"ROUGE-1: {rouge_result:.4f}",
            metrics={
                "rouge_1": score,
                "rouge_1_raw": rouge_result,
            },
        )


@register_grader("rouge_2")
class Rouge2Grader(BaseGrader):
    """ROUGE-2 评分器（双词组重叠）。"""

    def __init__(self) -> None:
        super().__init__(
            name="rouge_2",
            mode=GraderMode.POINTWISE,
            description="ROUGE-2 评分器 (0-100 分)",
            label="ROUGE-2",
            group="generation",
            eval_types=[EvalType.LLM, EvalType.RAG],
            requires_reference=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        **kwargs
    ) -> GraderScore:
        """评估 ROUGE-2 分数。"""
        if not reference or not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少参考答案或生成答案",
                metrics={"rouge_2": 0.0},
            )

        rouge_result = rouge_n(reference, response, n=2)
        score = rouge_result * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"ROUGE-2: {rouge_result:.4f}",
            metrics={
                "rouge_2": score,
                "rouge_2_raw": rouge_result,
            },
        )


@register_grader("rouge_l")
class RougeLGrader(BaseGrader):
    """ROUGE-L 评分器（最长公共子序列）。"""

    def __init__(self) -> None:
        super().__init__(
            name="rouge_l",
            mode=GraderMode.POINTWISE,
            description="ROUGE-L 评分器 (0-100 分)",
            label="ROUGE-L",
            group="generation",
            eval_types=[EvalType.LLM, EvalType.RAG],
            requires_reference=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        **kwargs
    ) -> GraderScore:
        """评估 ROUGE-L 分数。"""
        if not reference or not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少参考答案或生成答案",
                metrics={"rouge_l": 0.0},
            )

        rouge_result = rouge_l(reference, response)
        score = rouge_result * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"ROUGE-L: {rouge_result:.4f}",
            metrics={
                "rouge_l": score,
                "rouge_l_raw": rouge_result,
            },
        )


@register_grader("bleu")
class BleuGrader(BaseGrader):
    """BLEU 评分器。

    支持 BLEU-1、BLEU-2、BLEU-4。
    """

    def __init__(self) -> None:
        super().__init__(
            name="bleu",
            mode=GraderMode.POINTWISE,
            description="BLEU 评分器 (0-100 分)",
            label="BLEU",
            group="generation",
            eval_types=[EvalType.LLM, EvalType.RAG],
            requires_reference=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        n: int = 4,
        **kwargs
    ) -> GraderScore:
        """评估 BLEU 分数。

        Args:
            response: 生成的文本
            reference: 参考文本
            n: n-gram 大小 (1, 2, 4)
        """
        if not reference or not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少参考答案或生成答案",
                metrics={"bleu": 0.0},
            )

        bleu_result = bleu_at_n(reference, response, n=n)
        score = bleu_result * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"BLEU-{n}: {bleu_result:.4f}",
            metrics={
                "bleu": score,
                f"bleu_{n}_raw": bleu_result,
            },
        )


@register_grader("bleu_1")
class Bleu1Grader(BaseGrader):
    """BLEU-1 评分器。"""

    def __init__(self) -> None:
        super().__init__(
            name="bleu_1",
            mode=GraderMode.POINTWISE,
            description="BLEU-1 评分器 (0-100 分)",
            label="BLEU-1",
            group="generation",
            eval_types=[EvalType.LLM, EvalType.RAG],
            requires_reference=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        **kwargs
    ) -> GraderScore:
        """评估 BLEU-1 分数。"""
        if not reference or not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少参考答案或生成答案",
                metrics={"bleu_1": 0.0},
            )

        bleu_result = bleu_at_n(reference, response, n=1)
        score = bleu_result * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"BLEU-1: {bleu_result:.4f}",
            metrics={
                "bleu_1": score,
                "bleu_1_raw": bleu_result,
            },
        )


@register_grader("bleu_4")
class Bleu4Grader(BaseGrader):
    """BLEU-4 评分器。"""

    def __init__(self) -> None:
        super().__init__(
            name="bleu_4",
            mode=GraderMode.POINTWISE,
            description="BLEU-4 评分器 (0-100 分)",
            label="BLEU-4",
            group="generation",
            eval_types=[EvalType.LLM, EvalType.RAG],
            requires_reference=True,
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        **kwargs
    ) -> GraderScore:
        """评估 BLEU-4 分数。"""
        if not reference or not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少参考答案或生成答案",
                metrics={"bleu_4": 0.0},
            )

        bleu_result = bleu_at_n(reference, response, n=4)
        score = bleu_result * 100

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"BLEU-4: {bleu_result:.4f}",
            metrics={
                "bleu_4": score,
                "bleu_4_raw": bleu_result,
            },
        )
