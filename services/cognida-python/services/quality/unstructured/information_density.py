"""信息密度评估器。

评估文本的信息密度，包括文本长度、有效词比例、停用词比例。
"""

import re
from typing import Any

import jieba

from .base import UnstructuredEvaluator
from ..models import SeverityLevel, TextQualityIssue, UnstructuredDimensionScore
from ..registry import register_evaluator
from ..dimension_names import Dimension


# 中文停用词（简化版）
CHINESE_STOP_WORDS = {
    "的", "了", "在", "是", "我", "有", "和", "就", "不", "人",
    "都", "一", "一个", "上", "也", "很", "到", "说", "要", "去",
    "你", "会", "着", "没有", "看", "好", "自己", "这",
}

# 英文停用词（简化版）
ENGLISH_STOP_WORDS = {
    "the", "a", "an", "and", "or", "but", "in", "on", "at", "to",
    "for", "of", "with", "by", "from", "as", "is", "was", "are",
    "were", "be", "been", "being", "have", "has", "had", "do", "does",
    "did", "will", "would", "could", "should", "may", "might", "must",
    "can", "this", "that", "these", "those", "i", "you", "he", "she",
    "it", "we", "they", "what", "which", "who", "when", "where", "why",
    "how", "all", "each", "every", "both", "few", "more", "most", "other",
    "some", "such", "no", "nor", "not", "only", "own", "same", "so",
    "than", "too", "very", "just", "but", "and",
}


@register_evaluator(Dimension.INFORMATION_DENSITY.value)
class InformationDensityEvaluator(UnstructuredEvaluator):
    """信息密度评估器。

    评估文本的信息密度，检测内容是否过于稀疏。
    """

    dimension_name = Dimension.INFORMATION_DENSITY.value
    description = "文本长度、有效词比例、停用词比例"

    def __init__(self) -> None:
        """初始化信息密度评估器。"""
        super().__init__()
        # 首次使用时初始化 jieba
        self._jieba_initialized = False

    def evaluate(
        self,
        text: str,
        config: dict[str, Any] | None = None,
    ) -> UnstructuredDimensionScore:
        """评估文本信息密度。

        Args:
            text: 要评估的文本
            config: 配置参数

        Returns:
            信息密度维度评分
        """
        config = config or {}
        issues: list[TextQualityIssue] = []
        details: dict[str, Any] = {}

        if not text:
            return UnstructuredDimensionScore(
                name=self.dimension_name,
                score=0.0,
                passed=False,
                issues=[
                    TextQualityIssue(
                        type="empty_text",
                        severity=SeverityLevel.CRITICAL,
                        description="文本为空",
                    )
                ],
                details={},
            )

        # 文本长度检查
        text_length = len(text)
        min_length = config.get("min_length", 50)
        details["text_length"] = text_length

        if text_length < min_length:
            issues.append(
                TextQualityIssue(
                    type="short_text",
                    severity=SeverityLevel.WARNING,
                    description=f"文本长度 ({text_length}) 低于最小阈值 ({min_length})",
                )
            )

        # 分词
        words = self._tokenize(text)
        word_count = len(words)
        details["word_count"] = word_count
        details["unique_word_count"] = len(set(words))  # 添加唯一词数量

        if word_count == 0:
            return UnstructuredDimensionScore(
                name=self.dimension_name,
                score=0.0,
                passed=False,
                issues=[
                    TextQualityIssue(
                        type="no_words",
                        severity=SeverityLevel.CRITICAL,
                        description="无法提取有效词汇",
                    )
                ],
                details=details,
            )

        # 有效词比例
        valid_words = [w for w in words if len(w) > 1]
        valid_ratio = len(valid_words) / word_count if word_count > 0 else 0
        details["valid_word_ratio"] = valid_ratio

        if valid_ratio < 0.3:
            issues.append(
                TextQualityIssue(
                    type="low_valid_ratio",
                    severity=SeverityLevel.WARNING,
                    description=f"有效词比例过低: {valid_ratio:.1%}",
                )
            )

        # 停用词比例
        stop_words = self._get_stop_words(text)
        stop_word_count = sum(1 for w in words if w in stop_words)
        stop_ratio = stop_word_count / word_count if word_count > 0 else 0
        details["stopword_ratio"] = stop_ratio  # 使用测试期望的键名

        if stop_ratio > 0.7:
            issues.append(
                TextQualityIssue(
                    type="high_stop_ratio",
                    severity=SeverityLevel.WARNING,
                    description=f"停用词比例过高: {stop_ratio:.1%}",
                )
            )

        # 计算分数 - 综合考虑有效词比例和停用词比例
        base_score = 100.0
        for issue in issues:
            if issue.severity == SeverityLevel.CRITICAL:
                base_score -= 40
            elif issue.severity == SeverityLevel.WARNING:
                base_score -= 20

        # 根据信息密度调整分数
        # 有效词比例越高分数越高，停用词比例越高分数越低
        # 使用更宽松的公式，让高密度文本得分更高
        density_bonus = valid_ratio * 0.7 + (1 - stop_ratio) * 0.3  # 有效词占70%，停用词占30%
        density_score = base_score * max(0.3, density_bonus)  # 最低30%分数
        score = max(0.0, density_score)

        threshold = config.get("threshold", 60)
        passed = self.is_passed(score, threshold)

        return UnstructuredDimensionScore(
            name=self.dimension_name,
            score=score,
            passed=passed,
            issues=issues,
            details=details,
        )

    def _tokenize(self, text: str) -> list[str]:
        """分词。

        Args:
            text: 文本内容

        Returns:
            词汇列表
        """
        # 检测是否包含中文
        has_chinese = bool(re.search(r"[一-鿿]", text))

        if has_chinese:
            # 使用 jieba 分词
            return list(jieba.cut(text))
        else:
            # 英文分词
            return re.findall(r"\b\w+\b", text.lower())

    def _get_stop_words(self, text: str) -> set[str]:
        """获取停用词表。

        Args:
            text: 文本内容（用于判断语言）

        Returns:
            停用词集合
        """
        has_chinese = bool(re.search(r"[一-鿿]", text))
        has_english = bool(re.search(r"[a-zA-Z]", text))

        stop_words = set()
        if has_chinese:
            stop_words.update(CHINESE_STOP_WORDS)
        if has_english:
            stop_words.update(ENGLISH_STOP_WORDS)

        return stop_words
