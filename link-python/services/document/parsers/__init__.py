"""文档解析器模块。"""

from .base import BaseParser
from .pdf_parser import PDFParser
from .docx_parser import DocxParser
from .excel_parser import ExcelParser
from .csv_parser import CSVParser
from .markdown_parser import MarkdownParser
from .text_parser import TextParser

__all__ = [
    "BaseParser",
    "PDFParser",
    "DocxParser",
    "ExcelParser",
    "CSVParser",
    "MarkdownParser",
    "TextParser",
]


def get_parser(format: str) -> BaseParser:
    """根据格式获取解析器。

    Args:
        format: 文档格式

    Returns:
        解析器实例

    Raises:
        ValueError: 不支持的格式
    """
    parsers = {
        "pdf": PDFParser,
        "docx": DocxParser,
        "doc": DocxParser,
        "xlsx": ExcelParser,
        "xls": ExcelParser,
        "csv": CSVParser,
        "md": MarkdownParser,
        "markdown": MarkdownParser,
        "txt": TextParser,
        "text": TextParser,
    }

    parser_class = parsers.get(format.lower())
    if parser_class is None:
        raise ValueError(f"不支持的文档格式: {format}")

    return parser_class()
