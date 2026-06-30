"""敏感信息脱敏器。

对手机号、邮箱、身份证号等敏感信息进行脱敏。"""

import re
from typing import Any

import pandas as pd

from .base import Cleaner
from ..models import CleaningOperation, CleaningResult
from ..registry import register_cleaner


@register_cleaner("pii_masker")
class PIIMasker(Cleaner):
    """敏感信息脱敏器。

    对手机号、邮箱、身份证号等敏感信息进行脱敏处理。
    """

    cleaner_name = "pii_masker"
    description = "手机号脱敏、邮箱脱敏、身份证号脱敏"

    # 中国大陆手机号正则
    PHONE_PATTERN = re.compile(r"1[3-9]\d{9}")

    # 邮箱正则 - 修复中英文混合文本中的边界问题
    EMAIL_PATTERN = re.compile(r"(?<![a-zA-Z0-9._%+-])[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(?![a-zA-Z0-9._%+-])")

    # 身份证号正则
    ID_CARD_PATTERN = re.compile(
        r"[1-9]\d{5}(18|19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]"
    )

    def clean(
        self,
        data: pd.DataFrame,
        config: dict[str, Any] | None = None,
    ) -> CleaningResult:
        """脱敏处理。

        Args:
            data: 要脱敏的数据
            config: 配置参数

        Returns:
            清洗结果
        """
        config = config or {}
        operations: list[CleaningOperation] = []

        text_column = config.get("text_column")
        if not text_column or text_column not in data.columns:
            # 没有指定文本列，检查所有字符串列
            text_columns = [col for col in data.columns if data[col].dtype == "object"]
        else:
            text_columns = [text_column]

        cleaned_data = data.copy()
        total_masked = 0

        for col in text_columns:
            col_masked = 0
            masked_values = []

            for value in data[col]:
                if pd.notna(value) and isinstance(value, str):
                    masked_value, count = self._mask_text(value, config)
                    masked_values.append(masked_value)
                    col_masked += count
                else:
                    masked_values.append(value)

            cleaned_data[col] = masked_values
            total_masked += col_masked

            if col_masked > 0:
                operations.append(
                    CleaningOperation(
                        type="mask_pii",
                        field=col,
                        count=col_masked,
                        description=f"对 {col} 列中的 {col_masked} 处敏感信息进行了脱敏",
                    )
                )

        return self._create_result(data, cleaned_data, operations)

    def mask_text(self, text: str, config: dict[str, Any] | None = None) -> str:
        """对文本进行脱敏（公共接口）。

        Args:
            text: 文本内容
            config: 配置参数

        Returns:
            脱敏后的文本
        """
        config = config or {}
        masked, _ = self._mask_text(text, config)
        return masked

    def _mask_text(
        self, text: str, config: dict[str, Any]
    ) -> tuple[str, int]:
        """对文本进行脱敏。

        Args:
            text: 文本内容
            config: 配置参数

        Returns:
            (脱敏后的文本, 脱敏数量)
        """
        result = text
        count = 0

        # 手机号脱敏
        if config.get("mask_phone", True):
            result, phone_count = self._mask_phones(result)
            count += phone_count

        # 邮箱脱敏
        if config.get("mask_email", True):
            result, email_count = self._mask_emails(result)
            count += email_count

        # 身份证号脱敏
        if config.get("mask_id_card", True):
            result, id_count = self._mask_id_cards(result)
            count += id_count

        # 自定义模式脱敏
        custom_patterns = config.get("custom_patterns", [])
        for custom in custom_patterns:
            pattern = custom.get("pattern", "")
            replacement = custom.get("replacement", "****")
            if pattern:
                import re as re_module
                compiled = re_module.compile(pattern)
                matches = compiled.findall(result)
                if matches:
                    count += len(matches)
                    result = compiled.sub(replacement, result)

        return result, count

    def _mask_phones(self, text: str) -> tuple[str, int]:
        """脱敏手机号。

        Args:
            text: 文本内容

        Returns:
            (脱敏后的文本, 脱敏数量)
        """
        count = 0

        def mask_func(match: re.Match) -> str:
            nonlocal count
            count += 1
            phone = match.group()
            return phone[:3] + "****" + phone[-4:]

        result = self.PHONE_PATTERN.sub(mask_func, text)
        return result, count

    def _mask_emails(self, text: str) -> tuple[str, int]:
        """脱敏邮箱。

        Args:
            text: 文本内容

        Returns:
            (脱敏后的文本, 脱敏数量)
        """
        count = 0

        def mask_func(match: re.Match) -> str:
            nonlocal count
            count += 1
            email = match.group()
            parts = email.split("@")
            if len(parts) == 2:
                username = parts[0]
                masked_user = username[0] + "***" if len(username) > 1 else "***"
                return masked_user + "@" + parts[1]
            return email

        result = self.EMAIL_PATTERN.sub(mask_func, text)
        return result, count

    def _mask_id_cards(self, text: str) -> tuple[str, int]:
        """脱敏身份证号。

        Args:
            text: 文本内容

        Returns:
            (脱敏后的文本, 脱敏数量)
        """
        count = 0

        def mask_func(match: re.Match) -> str:
            nonlocal count
            count += 1
            id_card = match.group()
            return id_card[:6] + "********" + id_card[-4:]

        result = self.ID_CARD_PATTERN.sub(mask_func, text)
        return result, count

    def mask_phone(self, phone: str) -> str:
        """脱敏单个手机号。

        Args:
            phone: 手机号

        Returns:
            脱敏后的手机号
        """
        if len(phone) == 11:
            return phone[:3] + "****" + phone[-4:]
        return phone

    def mask_email(self, email: str) -> str:
        """脱敏单个邮箱。

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
        """脱敏单个身份证号。

        Args:
            id_card: 身份证号

        Returns:
            脱敏后的身份证号
        """
        if len(id_card) == 18:
            return id_card[:6] + "********" + id_card[-4:]
        return id_card
