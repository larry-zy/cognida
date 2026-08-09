"""文本清洗器。

处理文本的特殊字符、编码、HTML标签等。
"""

import html
import re
from typing import Any

import chardet

from .base import TextCleaner, Cleaner
from ..models import CleaningOperation, CleaningResult
from ..registry import register_cleaner
import pandas as pd


class TextCleanerImpl(TextCleaner):
    """文本清洗器实现 (仅处理字符串)。

    内部实现类, 不单独注册; 通过 UnifiedTextCleaner("text_cleaner") 对外暴露。
    """

    cleaner_name = "string_text_cleaner"
    description = "特殊字符去除、编码修复、HTML标签去除、空白字符标准化"

    # 特殊字符模式（控制字符等）
    CONTROL_CHARS_PATTERN = re.compile(
        r"[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]"
    )

    # HTML标签模式
    HTML_TAG_PATTERN = re.compile(r"<[^>]+>")

    # 多余空白字符模式
    WHITESPACE_PATTERN = re.compile(r"\s+")

    def clean(
        self,
        text: str,
        config: dict[str, Any] | None = None,
    ) -> tuple[str, list[CleaningOperation]]:
        """清洗文本。

        Args:
            text: 要清洗的文本
            config: 清洗配置参数

        Returns:
            (清洗后的文本, 操作列表)
        """
        config = config or {}
        operations: list[CleaningOperation] = []

        if not text:
            return text, operations

        result = text
        original_length = len(result)

        # 特殊字符去除
        if config.get("remove_special_chars", True):
            result = self._remove_special_chars(result)
            if len(result) != original_length:
                operations.append(
                    CleaningOperation(
                        type="remove_special_chars",
                        field=None,
                        count=original_length - len(result),
                        description="移除了特殊控制字符",
                    )
                )

        # HTML标签去除
        if config.get("remove_html", True):
            result, has_html = self._remove_html(result)
            if has_html:
                operations.append(
                    CleaningOperation(
                        type="remove_html",
                        field=None,
                        count=1,
                        description="移除了HTML标签",
                    )
                )

        # 空白字符标准化
        if config.get("normalize_whitespace", True):
            result = self._normalize_whitespace(result)
            operations.append(
                CleaningOperation(
                    type="normalize_whitespace",
                    field=None,
                    count=1,
                    description="标准化了空白字符",
                )
            )

        # 编码修复
        if config.get("fix_encoding", False):
            result = self._fix_encoding(result)
            operations.append(
                CleaningOperation(
                    type="fix_encoding",
                    field=None,
                    count=1,
                    description="尝试修复编码问题",
                )
            )

        return result, operations

    def clean_text(
        self,
        text: str,
        remove_html: bool = True,
        normalize_whitespace: bool = True,
        remove_special_chars: bool = True,
        fix_encoding: bool = False,
    ) -> str:
        """便捷的文本清洗方法。

        Args:
            text: 要清洗的文本
            remove_html: 是否移除HTML标签
            normalize_whitespace: 是否标准化空白字符
            remove_special_chars: 是否移除特殊字符
            fix_encoding: 是否尝试修复编码

        Returns:
            清洗后的文本
        """
        config = {
            "remove_html": remove_html,
            "normalize_whitespace": normalize_whitespace,
            "remove_special_chars": remove_special_chars,
            "fix_encoding": fix_encoding,
        }
        cleaned, _ = self.clean(text, config)
        return cleaned

    def _remove_special_chars(self, text: str) -> str:
        """移除特殊控制字符。

        Args:
            text: 文本内容

        Returns:
            处理后的文本
        """
        return self.CONTROL_CHARS_PATTERN.sub("", text)

    def _remove_html(self, text: str) -> tuple[str, bool]:
        """移除HTML标签。

        Args:
            text: 文本内容

        Returns:
            (处理后的文本, 是否有HTML)
        """
        # 先检查是否有HTML标签
        has_html = bool(self.HTML_TAG_PATTERN.search(text))

        if has_html:
            # 移除HTML标签
            text = self.HTML_TAG_PATTERN.sub("", text)
            # 解码HTML实体
            text = html.unescape(text)

        return text, has_html

    def _normalize_whitespace(self, text: str) -> str:
        """标准化空白字符。

        Args:
            text: 文本内容

        Returns:
            处理后的文本
        """
        # 将所有空白字符序列替换为单个空格
        text = self.WHITESPACE_PATTERN.sub(" ", text)
        # 去除首尾空白
        text = text.strip()
        return text

    def _fix_encoding(self, text: str) -> str:
        """尝试修复编码问题。

        Args:
            text: 文本内容

        Returns:
            修复后的文本
        """
        try:
            # 尝试检测编码
            raw_bytes = text.encode("latin-1", errors="ignore")
            detected = chardet.detect(raw_bytes)

            if detected["encoding"] and detected["confidence"] > 0.7:
                # 尝试用检测到的编码解码
                try:
                    text = raw_bytes.decode(detected["encoding"], errors="ignore")
                except (UnicodeDecodeError, LookupError):
                    pass
        except Exception:
            pass

        return text


@register_cleaner("dataframe_text_cleaner")
class DataFrameTextCleaner(Cleaner):
    """DataFrame文本清洗器。

    对DataFrame中的文本列进行清洗。
    """

    cleaner_name = "dataframe_text_cleaner"
    description = "对DataFrame中的文本列进行批量清洗"

    def __init__(self) -> None:
        """初始化清洗器。"""
        super().__init__()
        self._text_cleaner = TextCleanerImpl()

    def clean(
        self,
        data: pd.DataFrame,
        config: dict[str, Any] | None = None,
    ) -> CleaningResult:
        """清洗DataFrame数据。

        Args:
            data: 要清洗的数据
            config: 清洗配置参数

        Returns:
            清洗结果
        """
        config = config or {}
        operations: list[CleaningOperation] = []

        # 获取要清洗的列; 未指定时自动检测字符串列。
        # 注: pandas 3.x 字符串列默认 dtype 为 'str'(StringDtype) 而非 'object',
        # 故用 is_string_dtype 同时覆盖两者。
        text_columns = config.get(
            "columns",
            [col for col in data.columns if pd.api.types.is_string_dtype(data[col])],
        )

        cleaned_data = data.copy()

        for col in text_columns:
            if col not in data.columns:
                continue

            # 对每条记录进行清洗
            cleaned_values = []
            for value in data[col]:
                if pd.isna(value):
                    cleaned_values.append(value)
                else:
                    text = str(value)
                    cleaned_text, ops = self._text_cleaner.clean(text, config)
                    cleaned_values.append(cleaned_text)
                    operations.extend(ops)

            cleaned_data[col] = cleaned_values

        return self._create_result(data, cleaned_data, operations)


# 统一的文本清洗器 - 同时支持字符串和DataFrame清洗
# 注册为 "text_cleaner": 对外的默认文本清洗器, 按输入类型分派
# (字符串 -> 返回 (str, ops); DataFrame -> 返回 CleaningResult)。
@register_cleaner("text_cleaner")
class UnifiedTextCleaner(Cleaner):
    """统一的文本清洗器。

    支持字符串清洗和DataFrame清洗。
    """

    cleaner_name = "text_cleaner"
    description = "统一的文本清洗器，支持字符串和DataFrame"

    def __init__(self) -> None:
        """初始化清洗器。"""
        super().__init__()
        self._text_cleaner = TextCleanerImpl()
        self._df_cleaner = DataFrameTextCleaner()

    def clean_text(
        self,
        text: str,
        remove_html: bool = True,
        normalize_whitespace: bool = True,
        remove_special_chars: bool = True,
        fix_encoding: bool = False,
    ) -> str:
        """清洗单个文本。

        Args:
            text: 要清洗的文本
            remove_html: 是否移除HTML标签
            normalize_whitespace: 是否标准化空白字符
            remove_special_chars: 是否移除特殊字符
            fix_encoding: 是否尝试修复编码

        Returns:
            清洗后的文本
        """
        return self._text_cleaner.clean_text(
            text, remove_html, normalize_whitespace, remove_special_chars, fix_encoding
        )

    def clean(
        self,
        data: pd.DataFrame | str,
        config: dict[str, Any] | None = None,
    ) -> CleaningResult | tuple[str, list[CleaningOperation]]:
        """清洗数据。

        Args:
            data: 要清洗的数据（DataFrame或字符串）
            config: 清洗配置参数

        Returns:
            清洗结果
        """
        if isinstance(data, str):
            return self._text_cleaner.clean(data, config)
        else:
            return self._df_cleaner.clean(data, config)


# 对外别名: 默认文本清洗器统一为 UnifiedTextCleaner
TextCleaner = UnifiedTextCleaner
