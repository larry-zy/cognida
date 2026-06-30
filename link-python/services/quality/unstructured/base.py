"""非结构化数据评估器基类。"""

from abc import ABC, abstractmethod
from typing import Any

from ..models import (
    SeverityLevel,
    TextQualityIssue,
    UnstructuredDimensionScore,
)


class UnstructuredEvaluator(ABC):
    """非结构化数据维度评估器抽象基类。

    每个非结构化评估器负责评估文本质量的某一个维度。
    """

    dimension_name: str = ""
    description: str = ""

    @abstractmethod
    def evaluate(
        self,
        text: str,
        config: dict[str, Any] | None = None,
    ) -> UnstructuredDimensionScore:
        """评估文本质量。

        Args:
            text: 要评估的文本
            config: 额外配置参数

        Returns:
            维度评分结果
        """

    def _create_issue(
        self,
        issue_type: str,
        severity: SeverityLevel,
        description: str,
        position: tuple[int, int] | None = None,
        snippet: str | None = None,
        suggestion: str | None = None,
    ) -> TextQualityIssue:
        """创建文本质量问题。

        Args:
            issue_type: 问题类型
            severity: 严重级别
            description: 问题描述
            position: 问题位置 (start, end)
            snippet: 问题文本片段
            suggestion: 修复建议

        Returns:
            文本质量问题对象
        """
        return TextQualityIssue(
            type=issue_type,
            severity=severity,
            description=description,
            position=position,
            snippet=snippet,
            suggestion=suggestion,
        )

    def _calculate_score(
        self, total_issues: int, total_chars: int, base_score: float = 100.0
    ) -> float:
        """计算维度分数。

        Args:
            total_issues: 问题总数
            total_chars: 总字符数
            base_score: 基础分数

        Returns:
            维度分数 (0-100)
        """
        if total_chars == 0:
            return 0.0
        # 根据问题密度计算分数
        issue_ratio = min(1.0, total_issues / max(1, total_chars / 100))
        return max(0.0, base_score * (1 - issue_ratio))

    def is_passed(self, score: float, threshold: float) -> bool:
        """判断是否通过阈值。

        Args:
            score: 维度分数
            threshold: 阈值

        Returns:
            是否通过
        """
        return score >= threshold
