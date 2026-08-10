"""敏感信息检测器。

检测文本中的个人隐私信息（PII）。
"""

import re
from typing import Any

from .base import UnstructuredEvaluator
from ..models import SeverityLevel, TextQualityIssue, UnstructuredDimensionScore
from ..registry import register_evaluator
from ..dimension_names import Dimension


@register_evaluator(Dimension.PII_DETECTOR.value)
class PIIDetector(UnstructuredEvaluator):
    """敏感信息检测器。

    检测文本中的手机号、邮箱、身份证号等敏感信息。
    """

    dimension_name = Dimension.PII_DETECTOR.value
    description = "检测手机号、邮箱、身份证号、地址等敏感信息"

    # 中国大陆手机号正则
    PHONE_PATTERN = re.compile(
        r"(?<!\d)"
        r"1[3-9]\d{9}"
        r"(?!\d)"
    )

    # 邮箱正则
    EMAIL_PATTERN = re.compile(
        r"(?<![a-zA-Z0-9._%+-])[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(?![a-zA-Z0-9._%+-])"
    )

    # 身份证号正则（18位） - 使用非捕获分组
    ID_CARD_PATTERN = re.compile(
        r"(?<!\d)"
        r"[1-9]\d{5}(?:18|19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]"
        r"(?!\d)"
    )

    # 地址关键词
    ADDRESS_KEYWORDS = [
        "省", "市", "区", "县", "镇", "乡", "街道", "路", "号",
        "province", "city", "district", "street", "road",
    ]

    def evaluate(
        self,
        text: str,
        config: dict[str, Any] | None = None,
    ) -> UnstructuredDimensionScore:
        """评估文本中的敏感信息。

        Args:
            text: 要评估的文本
            config: 配置参数

        Returns:
            敏感信息检测结果
        """
        config = config or {}
        issues: list[TextQualityIssue] = []
        details: dict[str, Any] = {}

        if not text:
            return UnstructuredDimensionScore(
                name=self.dimension_name,
                score=100.0,
                passed=True,
                issues=[],
                details={},
            )

        total_detections = 0

        # 手机号检测
        phones = self._detect_phones(text)
        if phones:
            total_detections += len(phones)
            details["phone_count"] = len(phones)
            issues.append(
                TextQualityIssue(
                    type="phone_number",
                    severity=SeverityLevel.WARNING,
                    description=f"检测到 {len(phones)} 个手机号",
                    snippet=", ".join(p[:3] for p in phones) + (
                        "..." if len(phones) > 3 else ""
                    ),
                )
            )

        # 邮箱检测
        emails = self._detect_emails(text)
        if emails:
            total_detections += len(emails)
            details["email_count"] = len(emails)
            issues.append(
                TextQualityIssue(
                    type="email",
                    severity=SeverityLevel.INFO,
                    description=f"检测到 {len(emails)} 个邮箱地址",
                    snippet=", ".join(e[:3] for e in emails) + (
                        "..." if len(emails) > 3 else ""
                    ),
                )
            )

        # 身份证号检测
        id_cards = self._detect_id_cards(text)
        if id_cards:
            total_detections += len(id_cards)
            details["id_card_count"] = len(id_cards)
            issues.append(
                TextQualityIssue(
                    type="id_card",
                    severity=SeverityLevel.CRITICAL,
                    description=f"检测到 {len(id_cards)} 个身份证号",
                    snippet=", ".join(
                        id_card[:6] + "****" + id_card[-4:]
                        for id_card in id_cards[:3]
                    ),
                )
            )

        # 地址检测
        addresses = self._detect_addresses(text)
        if addresses:
            total_detections += len(addresses)
            details["address_count"] = len(addresses)
            issues.append(
                TextQualityIssue(
                    type="address",
                    severity=SeverityLevel.WARNING,
                    description=f"检测到 {len(addresses)} 个可能的地址",
                    snippet=addresses[0][:50] + "..." if len(addresses[0]) > 50 else addresses[0],
                )
            )

        details["total_detections"] = total_detections

        # 计算分数
        if total_detections == 0:
            score = 100.0
        else:
            # 每个敏感信息扣分，身份证扣更多
            penalty = 0
            for issue in issues:
                if issue.type == "id_card":
                    penalty += 40 * details.get(f"{issue.type}_count", 1)
                elif issue.type == "phone_number":
                    penalty += 20 * details.get(f"{issue.type}_count", 1)
                else:
                    penalty += 10 * details.get(f"{issue.type}_count", 1)
            score = max(0.0, 100 - penalty)

        threshold = config.get("threshold", 80)
        passed = self.is_passed(score, threshold)

        return UnstructuredDimensionScore(
            name=self.dimension_name,
            score=score,
            passed=passed,
            issues=issues,
            details=details,
        )

    def _detect_phones(self, text: str) -> list[str]:
        """检测手机号。

        Args:
            text: 文本内容

        Returns:
            手机号列表
        """
        return self.PHONE_PATTERN.findall(text)

    def _detect_emails(self, text: str) -> list[str]:
        """检测邮箱地址。

        Args:
            text: 文本内容

        Returns:
            邮箱列表
        """
        return self.EMAIL_PATTERN.findall(text)

    def _detect_id_cards(self, text: str) -> list[str]:
        """检测身份证号。

        Args:
            text: 文本内容

        Returns:
            身份证号列表
        """
        return self.ID_CARD_PATTERN.findall(text)

    def _detect_addresses(self, text: str) -> list[str]:
        """检测地址。

        Args:
            text: 文本内容

        Returns:
            地址列表
        """
        addresses = []

        # 查找包含地址关键词的句子
        sentences = re.split(r"[。！？.!?\\n]", text)
        for sentence in sentences:
            if any(keyword in sentence for keyword in self.ADDRESS_KEYWORDS):
                # 检查是否包含数字（门牌号）
                if re.search(r"\d+", sentence):
                    addresses.append(sentence.strip())

        return addresses

    def mask_phone(self, phone: str) -> str:
        """脱敏手机号。

        Args:
            phone: 手机号

        Returns:
            脱敏后的手机号
        """
        if len(phone) == 11:
            return phone[:3] + "****" + phone[-4:]
        return phone

    def mask_email(self, email: str) -> str:
        """脱敏邮箱。

        Args:
            email: 邮箱地址

        Returns:
            脱敏后的邮箱
        """
        parts = email.split("@")
        if len(parts) == 2:
            username = parts[0]
            masked_user = username[0] + "***" if len(username) > 1 else "***"
            return masked_user + "@" + parts[1]
        return email

    def mask_id_card(self, id_card: str) -> str:
        """脱敏身份证号。

        Args:
            id_card: 身份证号

        Returns:
            脱敏后的身份证号
        """
        if len(id_card) == 18:
            return id_card[:6] + "********" + id_card[-4:]
        return id_card
