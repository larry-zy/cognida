"""轻量级 DataFrame 结构清洗器。

对应前端「数据清洗」页的基础操作，语义单一、可组合：
- trim: 去除文本单元格首尾空白
- normalize_ws: 规整文本单元格内部连续空白为单个空格并去首尾空白
- drop_empty: 删除全为空/缺失的行

去重（dedup）由 DedupProcessor 提供，不在此处重复实现。
"""

import re
from typing import Any

import pandas as pd

from .base import Cleaner
from ..models import CleaningOperation, CleaningResult
from ..registry import register_cleaner

_WHITESPACE_RUN = re.compile(r"\s+")


def _string_columns(data: pd.DataFrame) -> list[str]:
    """返回可作文本处理的列（object / StringDtype 均覆盖）。"""
    return [col for col in data.columns if pd.api.types.is_string_dtype(data[col])]


@register_cleaner("trim")
class TrimCleaner(Cleaner):
    """去除文本单元格首尾空白。"""

    cleaner_name = "trim"
    description = "去除文本列每个单元格的首尾空白"

    def clean(
        self,
        data: pd.DataFrame,
        config: dict[str, Any] | None = None,
    ) -> CleaningResult:
        """执行首尾空白清理。"""
        cleaned_data = data.copy()
        operations: list[CleaningOperation] = []

        for col in _string_columns(cleaned_data):
            original = cleaned_data[col]
            stripped = original.map(
                lambda v: v.strip() if isinstance(v, str) else v
            )
            changed = int((original.fillna("") != stripped.fillna("")).sum())
            cleaned_data[col] = stripped
            if changed:
                operations.append(
                    self._create_operation(
                        "trim", col, changed, f"列「{col}」去除了 {changed} 处首尾空白"
                    )
                )

        return self._create_result(data, cleaned_data, operations)


@register_cleaner("normalize_ws")
class NormalizeWhitespaceCleaner(Cleaner):
    """规整文本单元格内部空白。"""

    cleaner_name = "normalize_ws"
    description = "将文本列内部连续空白压缩为单个空格并去首尾空白"

    def clean(
        self,
        data: pd.DataFrame,
        config: dict[str, Any] | None = None,
    ) -> CleaningResult:
        """执行空白规整。"""
        cleaned_data = data.copy()
        operations: list[CleaningOperation] = []

        for col in _string_columns(cleaned_data):
            original = cleaned_data[col]
            normalized = original.map(
                lambda v: _WHITESPACE_RUN.sub(" ", v).strip()
                if isinstance(v, str)
                else v
            )
            changed = int((original.fillna("") != normalized.fillna("")).sum())
            cleaned_data[col] = normalized
            if changed:
                operations.append(
                    self._create_operation(
                        "normalize_ws",
                        col,
                        changed,
                        f"列「{col}」规整了 {changed} 处空白",
                    )
                )

        return self._create_result(data, cleaned_data, operations)


@register_cleaner("drop_empty")
class DropEmptyRowsCleaner(Cleaner):
    """删除全为空/缺失的行。"""

    cleaner_name = "drop_empty"
    description = "删除所有列均为空值或空白字符串的行"

    def clean(
        self,
        data: pd.DataFrame,
        config: dict[str, Any] | None = None,
    ) -> CleaningResult:
        """执行空行删除。"""
        operations: list[CleaningOperation] = []

        # 逐行判断：全部单元格为 NA 或去空白后为空串即视为空行
        def _is_blank_row(row: "pd.Series") -> bool:
            for value in row:
                if pd.isna(value):
                    continue
                if isinstance(value, str) and value.strip() == "":
                    continue
                return False
            return True

        if len(data.columns) == 0:
            return self._create_result(data, data.copy(), operations)

        blank_mask = data.apply(_is_blank_row, axis=1)
        removed = int(blank_mask.sum())
        cleaned_data = data.loc[~blank_mask]

        if removed:
            operations.append(
                self._create_operation(
                    "drop_empty", None, removed, f"删除了 {removed} 个空行"
                )
            )

        return self._create_result(data, cleaned_data, operations)
