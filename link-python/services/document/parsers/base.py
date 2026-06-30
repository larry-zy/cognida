"""文档解析器基类模块。"""

from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any


@dataclass
class ParseResult:
    """解析结果。"""

    success: bool
    text: str
    metadata: dict[str, Any]
    tables: list[dict[str, Any]] | None = None
    images: list[dict[str, Any]] | None = None
    error: str | None = None


class BaseParser(ABC):
    """文档解析器基类。"""

    @property
    @abstractmethod
    def supported_formats(self) -> list[str]:
        """支持的格式列表。"""
        pass

    @property
    def name(self) -> str:
        """解析器名称。"""
        return self.__class__.__name__

    @abstractmethod
    async def parse(
        self,
        source: str | bytes,
        include_metadata: bool = True,
        extract_tables: bool = False,
        extract_images: bool = False,
        **kwargs: Any,
    ) -> ParseResult:
        """解析文档。

        Args:
            source: 文件路径或内容
            include_metadata: 是否包含元数据
            extract_tables: 是否提取表格
            extract_images: 是否提取图片
            **kwargs: 其他选项

        Returns:
            解析结果
        """
        pass

    def _detect_format(self, file_path: str) -> str:
        """检测文件格式。

        Args:
            file_path: 文件路径

        Returns:
            格式字符串
        """
        import os

        ext = os.path.splitext(file_path)[1].lower().lstrip(".")
        return ext

    async def parse_from_file(
        self,
        file_path: str,
        **kwargs: Any,
    ) -> ParseResult:
        """从文件解析。

        Args:
            file_path: 文件路径
            **kwargs: 解析选项

        Returns:
            解析结果
        """
        return await self.parse(file_path, **kwargs)

    async def parse_from_bytes(
        self,
        content: bytes,
        format: str,
        **kwargs: Any,
    ) -> ParseResult:
        """从字节内容解析。

        Args:
            content: 文件内容
            format: 文档格式
            **kwargs: 解析选项

        Returns:
            解析结果
        """
        return await self.parse(content, **kwargs)
