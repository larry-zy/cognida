"""可读性评估器。

评估文本的可读性，包括编码检测、乱码识别、语言检测。
"""

import re
from typing import Any

import chardet

from .base import UnstructuredEvaluator
from ..models import SeverityLevel, TextQualityIssue, UnstructuredDimensionScore
from ..registry import register_evaluator
from ..dimension_names import Dimension


@register_evaluator(Dimension.READABILITY.value)
class ReadabilityEvaluator(UnstructuredEvaluator):
    """可读性评估器。

    评估文本的可读性，检测编码问题、乱码等。
    """

    dimension_name = Dimension.READABILITY.value
    description = "编码检测、乱码识别、语言检测"

    # 可读字符模式（可打印ASCII + 常用Unicode字符）
    READABLE_PATTERN = re.compile(
        r"^[\x20-\x7E一-鿿぀-ゟ゠-ヿЀ-ӿͰ-Ͽ]*$"
    )

    def evaluate(
        self,
        text: str,
        config: dict[str, Any] | None = None,
    ) -> UnstructuredDimensionScore:
        """评估文本可读性。

        Args:
            text: 要评估的文本
            config: 配置参数

        Returns:
            可读性维度评分
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
                details={"encoding": None, "language": None},
            )

        # 编码检测
        encoding_info = self._detect_encoding(text)
        details["encoding"] = encoding_info["encoding"]
        details["encoding_confidence"] = encoding_info["confidence"]

        # 检测乱码
        mojibake_result = self._detect_mojibake(text, encoding_info)
        if mojibake_result["has_mojibake"]:
            issues.append(
                TextQualityIssue(
                    type="mojibake",
                    severity=SeverityLevel.CRITICAL,
                    description="检测到乱码（字符编码损坏）",
                    snippet=mojibake_result["sample"],
                )
            )

        # 语言检测
        language_info = self._detect_language(text)
        details["language"] = language_info["language"]

        # 可读字符比例
        readable_ratio = self._calculate_readable_ratio(text)
        details["readable_ratio"] = readable_ratio

        # 检查文本长度
        text_length = len(text)
        min_text_length = config.get("min_text_length", 20)
        if text_length < min_text_length:
            # 如果文本非常短（小于最小长度的一半），则为严重问题
            severity = SeverityLevel.CRITICAL if text_length < min_text_length / 2 else SeverityLevel.WARNING
            issues.append(
                TextQualityIssue(
                    type="short_text",
                    severity=severity,
                    description=f"文本长度 ({text_length}) 低于最小阈值 ({min_text_length})",
                )
            )

        if readable_ratio < 0.5:
            issues.append(
                TextQualityIssue(
                    type="low_readable_ratio",
                    severity=SeverityLevel.WARNING,
                    description=f"可读字符比例过低: {readable_ratio:.1%}",
                )
            )

        # 计算分数
        base_score = 100.0
        for issue in issues:
            if issue.severity == SeverityLevel.CRITICAL:
                base_score -= 40
            elif issue.severity == SeverityLevel.WARNING:
                base_score -= 20

        score = max(0.0, base_score)

        threshold = config.get("threshold", 70)
        passed = self.is_passed(score, threshold)

        return UnstructuredDimensionScore(
            name=self.dimension_name,
            score=score,
            passed=passed,
            issues=issues,
            details=details,
        )

    def _detect_encoding(self, text: str) -> dict[str, Any]:
        """检测文本编码。

        Args:
            text: 文本内容

        Returns:
            编码信息字典
        """
        try:
            # 对于已经是 str 的文本，检测其字节表示的编码
            raw_bytes = text.encode("utf-8", errors="ignore")
            result = chardet.detect(raw_bytes)
            return {
                "encoding": result.get("encoding", "utf-8"),
                "confidence": result.get("confidence", 1.0),
            }
        except Exception:
            return {"encoding": "utf-8", "confidence": 0.0}

    def _detect_mojibake(
        self, text: str, encoding_info: dict[str, Any]
    ) -> dict[str, Any]:
        """检测乱码。

        Args:
            text: 文本内容
            encoding_info: 编码信息

        Returns:
            乱码检测结果
        """
        # 常见乱码模式
        mojibake_patterns = [
            r"[�]+",  # 替换字符
            r"[\xc2-\xf4][\x80-\xbf]{1,3}[ \n]",  # 无效UTF-8序列
        ]

        for pattern in mojibake_patterns:
            matches = re.findall(pattern, text)
            if matches:
                return {
                    "has_mojibake": True,
                    "sample": matches[0][:50],
                }

        # 检查编码置信度
        if encoding_info["confidence"] < 0.7:
            return {
                "has_mojibake": True,
                "sample": text[:50] if len(text) > 50 else text,
            }

        return {"has_mojibake": False, "sample": None}

    def _detect_language(self, text: str) -> dict[str, Any]:
        """检测文本语言。

        Args:
            text: 文本内容

        Returns:
            语言信息字典
        """
        # 简单的语言检测基于字符范围
        chinese_chars = len(re.findall(r"[一-鿿]", text))
        japanese_hiragana = len(re.findall(r"[぀-ゟ]", text))
        japanese_katakana = len(re.findall(r"[゠-ヿ]", text))
        cyrillic = len(re.findall(r"[Ѐ-ӿ]", text))
        latin = len(re.findall(r"[a-zA-Z]", text))

        total_chars = len(text.strip())
        if total_chars == 0:
            return {"language": "unknown"}

        # 计算各语言字符比例
        ratios = {
            "chinese": chinese_chars / total_chars,
            "japanese": (japanese_hiragana + japanese_katakana) / total_chars,
            "russian": cyrillic / total_chars,
            "english": latin / total_chars,
        }

        # 找出比例最高的语言
        max_lang = max(ratios, key=ratios.get)
        max_ratio = ratios[max_lang]

        if max_ratio > 0.3:
            return {"language": max_lang, "confidence": max_ratio}
        else:
            return {"language": "unknown", "confidence": 0.0}

    def _calculate_readable_ratio(self, text: str) -> float:
        """计算可读字符比例。

        Args:
            text: 文本内容

        Returns:
            可读字符比例
        """
        if not text:
            return 0.0

        # 计算可打印字符数量
        printable_chars = sum(1 for c in text if c.isprintable() or c.isspace())
        return printable_chars / len(text)
