"""基于规则的评分器。"""

import re
import math
from typing import Any, List, Union

from ..base import BaseGrader, GraderMode, GraderScore, MetricType
from ..registry import register_grader


def exact_match(reference: str, output: str) -> float:
    """精确匹配评分。

    去除空格后完全匹配返回 100 分，否则 0 分。

    Args:
        reference: 参考答案
        output: 模型输出

    Returns:
        0-100 分数
    """
    # 去除空白字符后比较
    ref_normalized = re.sub(r"\s+", "", reference.strip())
    out_normalized = re.sub(r"\s+", "", output.strip())

    return 100.0 if ref_normalized == out_normalized else 0.0


def contains_match(reference: str, output: str, keywords: List[str] | None = None) -> float:
    """关键词包含匹配评分。

    根据包含的关键词比例返回 0-100 分。

    Args:
        reference: 参考答案（未使用，保持接口一致）
        output: 模型输出
        keywords: 需要包含的关键词列表。如果为 None，从 reference 提取

    Returns:
        0-100 分数
    """
    if keywords is None:
        # 从 reference 提取关键词（简单分词）
        keywords = re.findall(r"\w+", reference.lower())

    if not keywords:
        return 0.0

    output_lower = output.lower()
    matched = sum(1 for kw in keywords if kw.lower() in output_lower)

    return (matched / len(keywords)) * 100 if keywords else 0.0


def regex_match(reference: str, output: str, pattern: str | None = None) -> float:
    """正则匹配评分。

    匹配成功返回 100 分，否则 0 分。

    Args:
        reference: 参考答案（如果 pattern 为 None，作为正则模式）
        output: 模型输出
        pattern: 正则表达式模式。如果为 None，使用 reference 作为模式

    Returns:
        0-100 分数
    """
    if pattern is None:
        pattern = reference

    try:
        if re.search(pattern, output, re.IGNORECASE | re.DOTALL):
            return 100.0
        return 0.0
    except re.error:
        # 正则表达式无效，返回 0 分
        return 0.0


def numeric_match(
    reference: str,
    output: str,
    tolerance: float = 0.0,
) -> float:
    """数值比较评分。

    根据数值差异和容差返回 0-100 分。

    Args:
        reference: 参考答案（包含数值）
        output: 模型输出（包含数值）
        tolerance: 容差范围

    Returns:
        0-100 分数
    """
    # 从 reference 中提取数值
    ref_numbers = re.findall(r"-?\d+\.?\d*", reference)
    # 从 output 中提取数值
    out_numbers = re.findall(r"-?\d+\.?\d*", output)

    if not ref_numbers or not out_numbers:
        return 0.0

    # 取第一个数值进行比较
    try:
        ref_value = float(ref_numbers[0])
        out_value = float(out_numbers[0])
    except ValueError:
        return 0.0

    # 计算差异
    diff = abs(ref_value - out_value)

    # 在容差范围内
    if diff <= tolerance:
        return 100.0

    # 计算分数（容差外按比例递减）
    if ref_value != 0:
        relative_diff = diff / abs(ref_value)
    else:
        relative_diff = diff if diff > 0 else 0

    # 相对误差超过 100% 则为 0 分
    if relative_diff >= 1.0:
        return 0.0

    return max(0.0, 100.0 * (1.0 - relative_diff))


# 规则评分器类
class RuleBasedGrader:
    """基于规则的评分器。"""

    def __init__(
        self,
        method: str = "exact",
        keywords: List[str] | None = None,
        pattern: str | None = None,
        tolerance: float = 0.0,
    ) -> None:
        """初始化规则评分器。

        Args:
            method: 评分方法 (exact, contains, regex, numeric)
            keywords: 关键词列表（用于 contains 方法）
            pattern: 正则表达式（用于 regex 方法）
            tolerance: 数值容差（用于 numeric 方法）
        """
        self.method = method
        self.keywords = keywords
        self.pattern = pattern
        self.tolerance = tolerance

    def score(self, reference: str, output: str) -> float:
        """计算分数。

        Args:
            reference: 参考答案
            output: 模型输出

        Returns:
            0-100 分数
        """
        if self.method == "exact":
            return exact_match(reference, output)
        elif self.method == "contains":
            return contains_match(reference, output, self.keywords)
        elif self.method == "regex":
            return regex_match(reference, output, self.pattern)
        elif self.method == "numeric":
            return numeric_match(reference, output, self.tolerance)
        else:
            raise ValueError(f"Unknown method: {self.method}")

    def score_batch(
        self,
        references: List[str],
        outputs: List[str],
    ) -> List[float]:
        """批量计算分数。

        Args:
            references: 参考答案列表
            outputs: 模型输出列表

        Returns:
            分数列表
        """
        if len(references) != len(outputs):
            raise ValueError("references 和 outputs 长度必须相同")

        return [
            self.score(ref, out)
            for ref, out in zip(references, outputs)
        ]


def compute_rule_metrics(
    references: List[str],
    outputs: List[str],
    method: str = "exact",
    **kwargs,
) -> dict[str, float]:
    """计算规则指标（便捷函数）。

    Args:
        references: 参考答案列表
        outputs: 模型输出列表
        method: 评分方法
        **kwargs: 其他参数（keywords, pattern, tolerance）

    Returns:
        包含平均分数和每个样本分数的字典
    """
    grader = RuleBasedGrader(method=method, **kwargs)
    scores = grader.score_batch(references, outputs)

    return {
        "avg_score": sum(scores) / len(scores) if scores else 0.0,
        "min_score": min(scores) if scores else 0.0,
        "max_score": max(scores) if scores else 0.0,
        "scores": scores,
    }


# ============================================================
# Grader 类封装
# ============================================================

@register_grader("exact_match")
class ExactMatchGrader(BaseGrader):
    """精确匹配评分器。"""

    def __init__(self) -> None:
        super().__init__(
            name="exact_match",
            mode=GraderMode.POINTWISE,
            description="精确匹配评分器 (0-100 分)",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: str | None = None,
        context: List[str] | None = None,
        **kwargs
    ) -> GraderScore:
        """评估精确匹配分数。"""
        if not reference or not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少参考答案或生成答案",
                metrics={"score": 0.0},
            )

        score = exact_match(reference, response)

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"精确匹配: {'是' if score > 0 else '否'}",
            metrics={"score": score},
        )


@register_grader("contains_match")
class ContainsMatchGrader(BaseGrader):
    """关键词包含匹配评分器。"""

    def __init__(self) -> None:
        super().__init__(
            name="contains_match",
            mode=GraderMode.POINTWISE,
            description="关键词包含匹配评分器 (0-100 分)",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: str | None = None,
        context: List[str] | None = None,
        keywords: List[str] | None = None,
        **kwargs
    ) -> GraderScore:
        """评估关键词包含分数。"""
        if not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少生成答案",
                metrics={"score": 0.0},
            )

        score = contains_match(reference or "", response, keywords)

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"关键词匹配: {score:.1f}%",
            metrics={"score": score},
        )


@register_grader("regex_match")
class RegexMatchGrader(BaseGrader):
    """正则匹配评分器。"""

    def __init__(self) -> None:
        super().__init__(
            name="regex_match",
            mode=GraderMode.POINTWISE,
            description="正则匹配评分器 (0-100 分)",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: str | None = None,
        context: List[str] | None = None,
        pattern: str | None = None,
        **kwargs
    ) -> GraderScore:
        """评估正则匹配分数。"""
        if not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少生成答案",
                metrics={"score": 0.0},
            )

        score = regex_match(reference or "", response, pattern)

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"正则匹配: {'是' if score > 0 else '否'}",
            metrics={"score": score},
        )


@register_grader("numeric_match")
class NumericMatchGrader(BaseGrader):
    """数值比较评分器。"""

    def __init__(self) -> None:
        super().__init__(
            name="numeric_match",
            mode=GraderMode.POINTWISE,
            description="数值比较评分器 (0-100 分)",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: str | None = None,
        context: List[str] | None = None,
        tolerance: float = 0.0,
        **kwargs
    ) -> GraderScore:
        """评估数值比较分数。"""
        if not reference or not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少参考答案或生成答案",
                metrics={"score": 0.0},
            )

        score = numeric_match(reference, response, tolerance)

        return GraderScore(
            name=self.name,
            metric_type=MetricType.PERCENTAGE,
            reason=f"数值匹配: {score:.1f}%",
            metrics={"score": score},
        )
