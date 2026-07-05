"""示例自定义评分器。

演示如何创建自定义评分器。
"""

from typing import Optional

from ...graders.base import (
    BaseGrader,
    GraderMode,
    GraderScore,
    MetricType,
)
from ...graders.registry import register_grader, register_grader_class


# ============================================================
# 方法 1: 使用类继承 BaseGrader
# ============================================================

@register_grader("answer_length")
class AnswerLengthGrader(BaseGrader):
    """答案长度评分器。

    根据答案长度给予 1-5 分：
    - 5 分: > 200 字符
    - 4 分: 100-200 字符
    - 3 分: 50-100 字符
    - 2 分: 20-50 字符
    - 1 分: < 20 字符
    """

    def __init__(self) -> None:
        super().__init__(
            name="answer_length",
            mode=GraderMode.POINTWISE,
            description="根据答案长度评分 (1-5 分)",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[list] = None,
        **kwargs
    ) -> GraderScore:
        """评估答案长度。"""
        length = len(response)

        if length > 200:
            score = 5.0
        elif length > 100:
            score = 4.0
        elif length > 50:
            score = 3.0
        elif length > 20:
            score = 2.0
        else:
            score = 1.0

        return GraderScore(
            name=self.name,
            metric_type=MetricType.SCORE,
            reason=f"答案长度为 {length} 字符",
            metrics={"length_score": score, "length": float(length)},
        )


# ============================================================
# 方法 2: 使用函数装饰器
# ============================================================

from ...graders.base import function_grader


@function_grader(
    name="keyword_match",
    mode=GraderMode.POINTWISE,
    description="关键词匹配评分器",
)
async def keyword_match_grader(
    query: str,
    response: str,
    reference: Optional[str] = None,
    keywords: Optional[list] = None,
    **kwargs
) -> GraderScore:
    """关键词匹配评分器。

    检查回答中是否包含问题中的关键词。
    """
    if keywords is None:
        # 从问题中提取关键词（简单示例：提取中文词组）
        import re
        keywords = re.findall(r'[一-龥]{2,}', query)

    if not keywords:
        return GraderScore(
            name="keyword_match",
            metric_type=MetricType.SCORE,
            reason="没有找到关键词",
            metrics={"match_score": 3.0, "matched_count": 0.0},
        )

    matched_count = sum(1 for kw in keywords if kw in response)
    match_ratio = matched_count / len(keywords) if keywords else 0

    # 转换为 1-5 分
    score = 1.0 + match_ratio * 4.0

    return GraderScore(
        name="keyword_match",
        metric_type=MetricType.SCORE,
        reason=f"匹配了 {matched_count}/{len(keywords)} 个关键词",
        metrics={
            "match_score": round(score, 2),
            "matched_count": float(matched_count),
            "total_keywords": float(len(keywords)),
        },
    )


# ============================================================
# 方法 3: 使用注册表直接注册
# ============================================================

class ConcisenessGrader(BaseGrader):
    """简洁性评分器。

    根据答案相对于参考答案的长度比例评分。
    """

    def __init__(self) -> None:
        super().__init__(
            name="conciseness",
            mode=GraderMode.POINTWISE,
            description="答案简洁性评分 (1-5 分)",
        )

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[list] = None,
        **kwargs
    ) -> GraderScore:
        """评估简洁性。"""
        if not reference:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.SCORE,
                reason="没有参考答案",
                metrics={"conciseness_score": 3.0},
            )

        ref_length = len(reference)
        resp_length = len(response)

        if ref_length == 0:
            ratio = 1.0
        else:
            ratio = resp_length / ref_length

        # 理想比例是 0.7-1.3
        if 0.7 <= ratio <= 1.3:
            score = 5.0
        elif 0.5 <= ratio <= 1.5:
            score = 4.0
        elif 0.3 <= ratio <= 2.0:
            score = 3.0
        elif 0.2 <= ratio <= 3.0:
            score = 2.0
        else:
            score = 1.0

        return GraderScore(
            name=self.name,
            metric_type=MetricType.SCORE,
            reason=f"答案长度与参考答案比例: {ratio:.2f}",
            metrics={"conciseness_score": score, "ratio": ratio},
        )


# 注册评分器
register_grader_class(ConcisenessGrader, name="conciseness")
