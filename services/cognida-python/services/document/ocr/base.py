"""OCR 基类模块。"""

from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any


@dataclass
class OCRResult:
    """OCR 识别结果。"""

    success: bool
    text: str
    blocks: list[dict[str, Any]] | None = None
    confidence: float = 0.0
    error: str | None = None


class BaseOCR(ABC):
    """OCR 引擎基类。"""

    @property
    @abstractmethod
    def name(self) -> str:
        """引擎名称。"""
        pass

    @abstractmethod
    async def recognize(
        self,
        source: str | bytes,
        language: str = "chi_sim",
        det: bool = True,
        rec: bool = True,
        use_cls: bool = False,
        return_details: bool = False,
        **kwargs: Any,
    ) -> OCRResult:
        """识别图片中的文字。

        Args:
            source: 图片路径或字节内容
            language: 语言 (chi_sim | eng)
            det: 是否检测文字方向
            rec: 是否识别文字
            use_cls: 是否使用方向分类
            return_details: 是否返回详细信息
            **kwargs: 其他选项

        Returns:
            OCR 识别结果
        """
        pass

    async def recognize_file(
        self,
        file_path: str,
        **kwargs: Any,
    ) -> OCRResult:
        """识别文件中的文字。

        Args:
            file_path: 图片文件路径
            **kwargs: 其他选项

        Returns:
            OCR 识别结果
        """
        return await self.recognize(file_path, **kwargs)

    async def recognize_bytes(
        self,
        image_data: bytes,
        **kwargs: Any,
    ) -> OCRResult:
        """识别字节内容中的文字。

        Args:
            image_data: 图片字节内容
            **kwargs: 其他选项

        Returns:
            OCR 识别结果
        """
        return await self.recognize(image_data, **kwargs)
