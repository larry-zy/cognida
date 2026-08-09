"""清洗器基类。"""

from abc import ABC, abstractmethod
from typing import Any

import pandas as pd

from ..models import CleaningResult, CleaningOperation


class Cleaner(ABC):
    """清洗器抽象基类。

    定义数据清洗器的接口。
    """

    cleaner_name: str = ""
    description: str = ""

    @abstractmethod
    def clean(
        self,
        data: pd.DataFrame,
        config: dict[str, Any] | None = None,
    ) -> CleaningResult:
        """清洗数据。

        Args:
            data: 要清洗的数据
            config: 清洗配置参数

        Returns:
            清洗结果
        """

    def _create_operation(
        self,
        operation_type: str,
        field: str | None,
        count: int,
        description: str,
    ) -> CleaningOperation:
        """创建清洗操作记录。

        Args:
            operation_type: 操作类型
            field: 相关字段
            count: 影响的记录数
            description: 操作描述

        Returns:
            清洗操作对象
        """
        return CleaningOperation(
            type=operation_type,
            field=field,
            count=count,
            description=description,
        )

    def _create_result(
        self,
        original_data: pd.DataFrame,
        cleaned_data: pd.DataFrame,
        operations: list[CleaningOperation],
        metadata: dict[str, Any] | None = None,
    ) -> CleaningResult:
        """创建清洗结果。

        Args:
            original_data: 原始数据
            cleaned_data: 清洗后数据
            operations: 清洗操作列表
            metadata: 额外元数据

        Returns:
            清洗结果对象
        """
        original_count = len(original_data)
        cleaned_count = len(cleaned_data)
        removed_count = original_count - cleaned_count

        return CleaningResult(
            original_count=original_count,
            cleaned_count=cleaned_count,
            removed_count=removed_count,
            operations=operations,
            metadata=metadata or {},
            cleaned_data=cleaned_data,
        )


class TextCleaner(ABC):
    """文本清洗器抽象基类。

    用于非结构化文本数据的清洗。
    """

    cleaner_name: str = ""
    description: str = ""

    @abstractmethod
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
