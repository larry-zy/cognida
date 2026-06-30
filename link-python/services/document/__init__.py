"""文档处理工具模块。

提供文档解析、OCR识别、文本分块、URL获取等功能。
"""

from typing import Any

from core import get_logger
from .parsers import get_parser
from .parsers.base import BaseParser, ParseResult
from .ocr import get_ocr_engine
from .ocr.base import BaseOCR, OCRResult
from .chunking import ChunkStrategy, chunk_document


__all__ = [
    "BaseParser",
    "ParseResult",
    "get_parser",
    "BaseOCR",
    "OCRResult",
    "get_ocr_engine",
    "ChunkStrategy",
    "chunk_document",
    "parse_document",
    "ocr_image",
    "chunk_text",
    "fetch_url",
]

logger = get_logger(__name__)


async def parse_document(
    source: str | bytes,
    format: str = "",
    include_metadata: bool = True,
    extract_tables: bool = False,
    extract_images: bool = False,
    **kwargs: Any,
) -> dict[str, Any]:
    """解析文档。

    Args:
        source: 文件路径或字节内容
        format: 文档格式（不指定则自动检测）
        include_metadata: 是否包含元数据
        extract_tables: 是否提取表格
        extract_images: 是否提取图片
        **kwargs: 其他选项

    Returns:
        解析结果字典
    """
    try:
        # 自动检测格式
        if not format:
            if isinstance(source, str):
                # 从文件扩展名检测
                ext = source.rsplit(".", 1)[-1].lower()
                format = ext
            else:
                format = "txt"

        parser = get_parser(format)

        # 解析文档
        if isinstance(source, str):
            result: ParseResult = await parser.parse_from_file(
                source,
                extract_tables=extract_tables,
                extract_images=extract_images,
            )
        else:
            result = await parser.parse_from_bytes(
                source,
                format=format,
                extract_tables=extract_tables,
                extract_images=extract_images,
            )

        response = {
            "success": True,
            "text": result.text,
            "metadata": result.metadata if include_metadata else None,
            "tables": result.tables or [],
            "images": result.images or [],
            "error": None,
        }

        logger.info(
            "文档解析完成",
            format=format,
            char_count=len(result.text),
        )
        return response

    except Exception as e:
        logger.error("文档解析失败", error=str(e))
        return {
            "success": False,
            "text": "",
            "metadata": None,
            "tables": [],
            "images": [],
            "error": str(e),
        }


async def ocr_image(
    source: str | bytes,
    engine: str = "paddleocr",
    language: str = "chi_sim",
    det: bool = True,
    rec: bool = True,
    use_cls: bool = False,
    return_details: bool = False,
    **kwargs: Any,
) -> dict[str, Any]:
    """OCR识别图片。

    Args:
        source: 图片路径或字节内容
        engine: OCR引擎 (paddleocr | vlm)
        language: 语言 (chi_sim | eng)
        det: 是否检测
        rec: 是否识别
        use_cls: 是否使用方向分类
        return_details: 是否返回详细信息
        **kwargs: 其他选项

    Returns:
        OCR识别结果字典
    """
    try:
        ocr_engine = get_ocr_engine(engine)

        if isinstance(source, str):
            result: OCRResult = await ocr_engine.recognize_file(
                source,
                language=language,
                det=det,
                rec=rec,
                use_cls=use_cls,
                return_details=return_details,
            )
        else:
            result = await ocr_engine.recognize_bytes(
                source,
                language=language,
                det=det,
                rec=rec,
                use_cls=use_cls,
                return_details=return_details,
            )

        return {
            "success": result.success,
            "text": result.text,
            "blocks": result.blocks or [],
            "confidence": result.confidence,
            "error": result.error,
        }

    except Exception as e:
        logger.error("OCR识别失败", error=str(e))
        return {
            "success": False,
            "text": "",
            "blocks": [],
            "confidence": 0.0,
            "error": str(e),
        }


async def chunk_text(
    text: str,
    strategy: str | ChunkStrategy = ChunkStrategy.PARAGRAPH,
    chunk_size: int = 1000,
    chunk_overlap: int = 200,
    **kwargs: Any,
) -> dict[str, Any]:
    """文本分块。

    Args:
        text: 待分块的文本
        strategy: 分块策略
        chunk_size: 块大小
        chunk_overlap: 重叠大小
        **kwargs: 其他选项

    Returns:
        分块结果字典
    """
    try:
        if isinstance(strategy, str):
            strategy = ChunkStrategy(strategy)

        chunks = chunk_document(
            text,
            strategy=strategy,
            chunk_size=chunk_size,
            chunk_overlap=chunk_overlap,
            **kwargs,
        )

        return {
            "chunks": chunks,
            "total_count": len(chunks),
        }

    except Exception as e:
        logger.error("文本分块失败", error=str(e))
        return {
            "chunks": [],
            "total_count": 0,
            "error": str(e),
        }


async def fetch_url(
    url: str,
    timeout: int = 30,
    wait_for_javascript: bool = False,
    wait_time: int = 1000,
    screenshot: bool = False,
    **kwargs: Any,
) -> dict[str, Any]:
    """获取URL内容。

    Args:
        url: 目标URL
        timeout: 超时时间（秒）
        wait_for_javascript: 是否等待JS执行
        wait_time: 等待时间（毫秒）
        screenshot: 是否截图
        **kwargs: 其他选项

    Returns:
        获取结果字典
    """
    try:
        from .fetcher import URLFetcher

        fetcher = URLFetcher()
        result = await fetcher.fetch(
            url,
            timeout=timeout,
            wait_for_javascript=wait_for_javascript,
            wait_time=wait_time,
            screenshot=screenshot,
        )

        return {
            "success": True,
            "text": result.get("text", ""),
            "html": result.get("html", ""),
            "title": result.get("title", ""),
            "links": result.get("links", []),
            "screenshot": result.get("screenshot", b""),
            "error": None,
        }

    except Exception as e:
        logger.error("URL获取失败", url=url, error=str(e))
        return {
            "success": False,
            "text": "",
            "html": "",
            "title": "",
            "links": [],
            "screenshot": b"",
            "error": str(e),
        }
