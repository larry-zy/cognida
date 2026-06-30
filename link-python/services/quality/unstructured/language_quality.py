"""语言质量评估器。

评估文本的语言质量，包括语法、标点符号检查。
"""

import re
from typing import Any

from .base import UnstructuredEvaluator
from ..models import SeverityLevel, TextQualityIssue, UnstructuredDimensionScore
from ..registry import register_evaluator


@register_evaluator("language_quality")
class LanguageQualityEvaluator(UnstructuredEvaluator):
    """语言质量评估器。

    评估文本的语言质量，检测语法和标点符号问题。
    """

    dimension_name = "language_quality"
    description = "基础语法检查、标点符号检查"

    # 标点符号模式
    CHINESE_PUNCTUATION = r"。！？，、；：""''（）【】《》"
    ENGLISH_PUNCTUATION = r".!?,;:\"'()[]{}<>"

    def evaluate(
        self,
        text: str,
        config: dict[str, Any] | None = None,
    ) -> UnstructuredDimensionScore:
        """评估文本语言质量。

        Args:
            text: 要评估的文本
            config: 配置参数

        Returns:
            语言质量维度评分
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

        # 检测语言
        has_chinese = bool(re.search(r"[一-鿿]", text))
        has_english = bool(re.search(r"[a-zA-Z]", text))
        details["has_chinese"] = has_chinese
        details["has_english"] = has_english

        # 标点符号检查
        punctuation_issues = self._check_punctuation(text, has_chinese, has_english)
        issues.extend(punctuation_issues)

        # 基础语法检查
        grammar_issues = self._check_grammar(text, has_chinese)
        issues.extend(grammar_issues)

        # 计算分数
        base_score = 100.0
        for issue in issues:
            if issue.severity == SeverityLevel.CRITICAL:
                base_score -= 30
            elif issue.severity == SeverityLevel.WARNING:
                base_score -= 15

        score = max(0.0, base_score)

        threshold = config.get("threshold", 60)
        passed = self.is_passed(score, threshold)

        return UnstructuredDimensionScore(
            name=self.dimension_name,
            score=score,
            passed=passed,
            issues=issues,
            details=details,
        )

    def _check_punctuation(
        self, text: str, has_chinese: bool, has_english: bool
    ) -> list[TextQualityIssue]:
        """检查标点符号。

        Args:
            text: 文本内容
            has_chinese: 是否包含中文
            has_english: 是否包含英文

        Returns:
            标点符号问题列表
        """
        issues = []

        # 检查缺失句末标点
        sentences = re.split(r"[。！？.!?]+", text)
        for sentence in sentences:
            sentence = sentence.strip()
            if len(sentence) > 20 and not any(
                p in sentence for p in "。！？.!?"
            ):
                issues.append(
                    TextQualityIssue(
                        type="missing_ending_punctuation",
                        severity=SeverityLevel.INFO,
                        description="长句子缺少结束标点符号",
                        snippet=sentence[:50] + "..." if len(sentence) > 50 else sentence,
                    )
                )

        # 检查标点符号混用
        if has_chinese and has_english:
            # 检查中文句子中使用英文标点
            for match in re.finditer(
                r"[一-鿿]+[a-zA-Z,.!?]+[一-鿿]*", text
            ):
                issues.append(
                    TextQualityIssue(
                        type="mixed_punctuation",
                        severity=SeverityLevel.INFO,
                        description="中英文混用时标点符号使用不规范",
                        snippet=match.group()[:50],
                    )
                )
                break  # 只报告一次

        # 检查连续标点
        consecutive_pattern = r"([。！？，、.!?]){2,}"
        for match in re.finditer(consecutive_pattern, text):
            issues.append(
                TextQualityIssue(
                    type="consecutive_punctuation",
                    severity=SeverityLevel.WARNING,
                    description="连续使用相同标点符号",
                    snippet=match.group()[:10],
                    position=(match.start(), match.end()),
                )
            )
            break  # 只报告一次

        return issues

    def _check_grammar(self, text: str, has_chinese: bool) -> list[TextQualityIssue]:
        """检查基础语法。

        Args:
            text: 文本内容
            has_chinese: 是否包含中文

        Returns:
            语法问题列表
        """
        issues = []

        # 检查重复词（中文）
        if has_chinese:
            # 检查连续重复的2-4字词
            repeat_word_pattern = r"([一-鿿]{2,4})\1"
            for match in re.finditer(repeat_word_pattern, text):
                issues.append(
                    TextQualityIssue(
                        type="repeated_word",
                        severity=SeverityLevel.WARNING,
                        description=f"检测到重复词: {match.group()[:10]}",
                        snippet=match.group()[:20],
                    )
                )
                break  # 只报告一次

        # 检查重复字符
        repeat_pattern = r"(.)\1{4,}"  # 同一字符重复5次以上
        for match in re.finditer(repeat_pattern, text):
            char = match.group()[0]
            if char.isalnum():
                issues.append(
                    TextQualityIssue(
                        type="excessive_repetition",
                        severity=SeverityLevel.WARNING,
                        description=f"字符 '{char}' 过度重复",
                        snippet=match.group()[:10],
                    )
                )
                break  # 只报告一次

        # 检查空格使用
        if has_chinese:
            # 中文之间不应有空格
            chinese_space_pattern = r"[一-鿿]\s+[一-鿿]"
            if re.search(chinese_space_pattern, text):
                issues.append(
                    TextQualityIssue(
                        type="chinese_spacing",
                        severity=SeverityLevel.INFO,
                        description="中文字符之间不应有空格",
                    )
                )

        # 检查英文单词空格
        english_words = re.findall(r"[a-zA-Z]{4,}", text)
        for word in english_words[:10]:  # 只检查前10个
            # 检查是否有连续大写（可能是拼写错误）
            if re.match(r"^[A-Z]{2,}", word):
                issues.append(
                    TextQualityIssue(
                        type="suspicious_caps",
                        severity=SeverityLevel.INFO,
                        description=f"单词 '{word}' 连续大写可能存在拼写错误",
                    )
                )
                break

        return issues
