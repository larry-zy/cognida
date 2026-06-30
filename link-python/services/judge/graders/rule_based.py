"""基于规则的评分器。"""

import re
from typing import Any

from .base import Grader


class ExactMatchGrader(Grader):
    """精确匹配评分器。"""

    name = "exact"

    async def score(
        self,
        model_output: str,
        reference: str | None = None,
        **kwargs: Any,
    ) -> tuple[float, str, bool]:
        """精确匹配评分。"""
        if reference is None:
            return 0.0, "未提供参考答案", False

        # 去除空格和换行后比较
        output_normalized = re.sub(r"\s+", "", model_output.strip())
        reference_normalized = re.sub(r"\s+", "", reference.strip())

        if output_normalized == reference_normalized:
            return 100.0, "完全匹配", True
        return 0.0, f"不匹配: {output_normalized!r} != {reference_normalized!r}", False


class ContainsGrader(Grader):
    """关键词包含评分器。"""

    name = "contains"

    async def score(
        self,
        model_output: str,
        reference: str | None = None,
        **kwargs: Any,
    ) -> tuple[float, str, bool]:
        """关键词包含评分。"""
        if reference is None:
            return 0.0, "未提供参考答案", False

        # reference 可以是多个关键词，用 | 分隔
        keywords = [k.strip() for k in reference.split("|")]

        output_lower = model_output.lower()
        found_keywords = [k for k in keywords if k.lower() in output_lower]

        if found_keywords:
            score = len(found_keywords) / len(keywords) * 100
            return score, f"包含关键词: {', '.join(found_keywords)}", True
        return 0.0, f"不包含任何关键词: {keywords}", False


class RegexGrader(Grader):
    """正则表达式评分器。"""

    name = "regex"

    async def score(
        self,
        model_output: str,
        reference: str | None = None,
        **kwargs: Any,
    ) -> tuple[float, str, bool]:
        """正则表达式匹配评分。"""
        if reference is None:
            return 0.0, "未提供正则表达式", False

        try:
            pattern = re.compile(reference, re.MULTILINE | re.DOTALL)
            match = pattern.search(model_output)

            if match:
                return 100.0, f"匹配成功: {match.group(0)!r}", True
            return 0.0, "不匹配正则表达式", False
        except re.error as e:
            return 0.0, f"正则表达式错误: {e}", False


class NumericGrader(Grader):
    """数值比较评分器。"""

    name = "numeric"

    async def score(
        self,
        model_output: str,
        reference: str | None = None,
        tolerance: float = 0.01,
        **kwargs: Any,
    ) -> tuple[float, str, bool]:
        """数值比较评分。"""
        if reference is None:
            return 0.0, "未提供参考答案", False

        # 从输出中提取数值
        import math

        def extract_number(text: str) -> float | None:
            # 尝试直接转换
            try:
                return float(text)
            except ValueError:
                pass
            # 尝试从文本中提取数字
            numbers = re.findall(r"-?\d+\.?\d*", text)
            if numbers:
                return float(numbers[-1])  # 取最后一个数字
            return None

        output_value = extract_number(model_output)
        reference_value = extract_number(reference)

        if output_value is None:
            return 0.0, "无法从输出中提取数值", False
        if reference_value is None:
            return 0.0, "无法从参考答案中提取数值", False

        diff = abs(output_value - reference_value)
        relative_diff = diff / (abs(reference_value) + 1e-9)

        if diff <= tolerance or relative_diff <= tolerance:
            return 100.0, f"数值匹配: {output_value} ≈ {reference_value}", True

        return (
            max(0, 100 * (1 - relative_diff)),
            f"数值差异: {output_value} vs {reference_value} (差异: {diff:.2f})",
            False,
        )


# 预定义的评分器实例
EXACT_MATCH = ExactMatchGrader()
CONTAINS = ContainsGrader()
REGEX = RegexGrader()
NUMERIC = NumericGrader()

_RULE_GRADERS = {
    "exact": EXACT_MATCH,
    "contains": CONTAINS,
    "regex": REGEX,
    "numeric": NUMERIC,
}


def get_rule_grader(name: str) -> Grader:
    """获取规则评分器。"""
    if name not in _RULE_GRADERS:
        raise ValueError(f"Unknown rule grader: {name}. Available: {list(_RULE_GRADERS.keys())}")
    return _RULE_GRADERS[name]
