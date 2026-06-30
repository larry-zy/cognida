"""格式转换器。

处理编码转换、日期格式标准化、数字格式标准化。"""

import re
from datetime import datetime
from typing import Any

import pandas as pd

from .base import Cleaner, TextCleaner
from ..models import CleaningOperation, CleaningResult
from ..registry import register_cleaner


@register_cleaner("format_converter")
class FormatConverter(Cleaner):
    """格式转换器。

    处理编码转换、日期格式标准化、数字格式标准化。
    """

    cleaner_name = "format_converter"
    description = "编码转换、日期格式标准化、数字格式标准化"

    # 日期格式模式
    DATE_PATTERNS = [
        (r"\d{4}-\d{2}-\d{2}", "%Y-%m-%d"),
        (r"\d{4}/\d{2}/\d{2}", "%Y/%m/%d"),
        (r"\d{2}-\d{2}-\d{4}", "%d-%m-%Y"),
        (r"\d{2}/\d{2}/\d{4}", "%d/%m/%Y"),
        (r"\d{4}\d{2}\d{2}", "%Y%m%d"),
    ]

    def clean(
        self,
        data: pd.DataFrame,
        config: dict[str, Any] | None = None,
    ) -> CleaningResult:
        """格式转换。

        Args:
            data: 要转换的数据
            config: 配置参数

        Returns:
            清洗结果
        """
        config = config or {}
        operations: list[CleaningOperation] = []

        cleaned_data = data.copy()

        # 编码转换
        if config.get("convert_encoding"):
            target_encoding = config["convert_encoding"]
            text_columns = config.get("text_columns", [])

            for col in text_columns:
                if col in cleaned_data.columns:
                    cleaned_data[col], count = self._convert_column_encoding(
                        cleaned_data[col], target_encoding
                    )
                    if count > 0:
                        operations.append(
                            CleaningOperation(
                                type="convert_encoding",
                                field=col,
                                count=count,
                                description=f"将 {col} 列转换为 {target_encoding} 编码",
                            )
                        )

        # 日期格式标准化
        if config.get("normalize_dates"):
            date_columns = config.get("date_columns", [])

            for col in date_columns:
                if col in cleaned_data.columns:
                    cleaned_data[col], count = self._normalize_date_column(
                        cleaned_data[col], config.get("date_format", "%Y-%m-%d")
                    )
                    if count > 0:
                        operations.append(
                            CleaningOperation(
                                type="normalize_date",
                                field=col,
                                count=count,
                                description=f"标准化 {col} 列的日期格式",
                            )
                        )

        # 数字格式标准化
        if config.get("normalize_numbers"):
            number_columns = config.get("number_columns", [])

            for col in number_columns:
                if col in cleaned_data.columns:
                    cleaned_data[col], count = self._normalize_number_column(cleaned_data[col])
                    if count > 0:
                        operations.append(
                            CleaningOperation(
                                type="normalize_number",
                                field=col,
                                count=count,
                                description=f"标准化 {col} 列的数字格式",
                            )
                        )

        return self._create_result(data, cleaned_data, operations)

    def _convert_column_encoding(
        self, series: pd.Series, target_encoding: str = "utf-8"
    ) -> tuple[pd.Series, int]:
        """转换列编码。

        Args:
            series: 数据列
            target_encoding: 目标编码

        Returns:
            (转换后的列, 转换的记录数)
        """
        converted = series.copy()
        count = 0

        for idx, value in series.items():
            if pd.notna(value) and isinstance(value, str):
                try:
                    # 尝试转换编码
                    converted_bytes = value.encode("utf-8", errors="ignore")
                    decoded = converted_bytes.decode(target_encoding, errors="ignore")
                    if decoded != value:
                        converted.iloc[idx] = decoded
                        count += 1
                except (UnicodeDecodeError, UnicodeEncodeError):
                    pass

        return converted, count

    def normalize_dates(
        self,
        dates: list[str] | pd.Series,
        target_format: str = "%Y-%m-%d",
    ) -> list[str] | pd.Series:
        """标准化日期格式。

        Args:
            dates: 日期字符串列表或Series
            target_format: 目标日期格式

        Returns:
            标准化后的日期列表或Series
        """
        is_series = isinstance(dates, pd.Series)
        if is_series:
            return self._normalize_date_column(dates, target_format)[0]
        else:
            result = []
            for date_str in dates:
                parsed = self._parse_date(str(date_str))
                if parsed:
                    result.append(parsed.strftime(target_format))
                else:
                    result.append(date_str)
            return result

    def normalize_numbers(self, numbers: list[str | float | int] | pd.Series) -> list[float] | pd.Series:
        """标准化数字格式。

        Args:
            numbers: 数字列表或Series（可以是字符串或数字）

        Returns:
            标准化后的浮点数列表或Series
        """
        is_series = isinstance(numbers, pd.Series)
        if is_series:
            return self._normalize_number_column(numbers)[0]
        else:
            result = []
            for value in numbers:
                if isinstance(value, (int, float)):
                    result.append(float(value))
                elif isinstance(value, str):
                    # 移除数字中的非数字字符（保留小数点和负号）
                    cleaned = re.sub(r"[^\d.-]", "", value)
                    try:
                        if cleaned:
                            result.append(float(cleaned))
                        else:
                            result.append(0.0)
                    except ValueError:
                        result.append(0.0)
                else:
                    result.append(0.0)
            return result

    def _normalize_date_column(
        self, series: pd.Series, target_format: str = "%Y-%m-%d"
    ) -> tuple[pd.Series, int]:
        """标准化日期列。

        Args:
            series: 数据列
            target_format: 目标日期格式

        Returns:
            (标准化后的列, 标准化的记录数)
        """
        normalized = series.copy()
        count = 0

        for idx, value in series.items():
            if pd.notna(value):
                # 尝试解析日期
                parsed_date = self._parse_date(str(value))
                if parsed_date:
                    normalized.iloc[idx] = parsed_date.strftime(target_format)
                    count += 1

        return normalized, count

    def _parse_date(self, date_str: str) -> datetime | None:
        """尝试解析日期字符串。

        Args:
            date_str: 日期字符串

        Returns:
            解析后的日期对象
        """
        for pattern, fmt in self.DATE_PATTERNS:
            if re.match(pattern, date_str):
                try:
                    return datetime.strptime(date_str, fmt)
                except ValueError:
                    continue

        return None

    def _normalize_number_column(self, series: pd.Series) -> tuple[pd.Series, int]:
        """标准化数字列。

        Args:
            series: 数据列

        Returns:
            (标准化后的列, 标准化的记录数)
        """
        normalized = series.copy()
        count = 0

        for idx, value in series.items():
            if pd.notna(value) and isinstance(value, str):
                # 移除数字中的非数字字符（保留小数点和负号）
                cleaned = re.sub(r"[^\d.-]", "", value)
                try:
                    if cleaned:
                        normalized.iloc[idx] = float(cleaned)
                        count += 1
                except ValueError:
                    pass

        return normalized, count
