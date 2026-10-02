"""文档解析器模块。"""

from .base import BaseParser
from .pdf_parser import PDFParser
from .docx_parser import DocxParser
from .excel_parser import ExcelParser
from .csv_parser import CSVParser
from .markdown_parser import MarkdownParser
from .text_parser import TextParser
from .html_parser import HTMLParser
from .pptx_parser import PPTXParser
from ..formats import DocumentFormat

__all__ = [
    "BaseParser",
    "PDFParser",
    "DocxParser",
    "ExcelParser",
    "CSVParser",
    "MarkdownParser",
    "TextParser",
    "HTMLParser",
    "PPTXParser",
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
    # key 引用 DocumentFormat.value（format wire 契约〔M13-P4〕），字面结果不变。
    # 别名 doc/xls/markdown 用字面量归入对应解析器；"text" 为 Python 侧内部别名，
    # 不属跨服务 wire 契约。旧二进制 Office 格式不能仅改后缀冒充 OOXML。
    parsers = {
        DocumentFormat.PDF.value: PDFParser,
        DocumentFormat.DOCX.value: DocxParser,
        "doc": DocxParser,
        DocumentFormat.XLSX.value: ExcelParser,
        "xls": ExcelParser,
        DocumentFormat.CSV.value: CSVParser,
        DocumentFormat.MD.value: MarkdownParser,
        "markdown": MarkdownParser,
        DocumentFormat.TXT.value: TextParser,
        "text": TextParser,
        DocumentFormat.HTML.value: HTMLParser,
        "htm": HTMLParser,
        DocumentFormat.PPTX.value: PPTXParser,
    }

    parser_class = parsers.get(format.lower())
    if parser_class is None:
        raise ValueError(f"不支持的文档格式: {format}")

    return parser_class()
