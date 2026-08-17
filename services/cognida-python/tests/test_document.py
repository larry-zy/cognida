"""文档处理功能测试。"""

import asyncio

import pytest
import services.document as document_service
from services.document import (
    ChunkStrategy,
    chunk_text,
    parse_document,
)
from services.document.parsers.base import ParseResult


@pytest.mark.asyncio
async def test_chunk_by_paragraph():
    """测试按段落分块。"""
    text = """第一段内容。

第二段内容。

第三段内容。"""

    result = await chunk_text(
        text=text,
        strategy=ChunkStrategy.PARAGRAPH,
        chunk_size=50,
        chunk_overlap=0,
    )

    assert result["total_count"] > 0
    assert len(result["chunks"]) > 0
    assert result["chunks"][0]["text"]


@pytest.mark.asyncio
async def test_chunk_by_sentence():
    """测试按句子分块。"""
    text = "这是第一句。这是第二句。这是第三句。"

    result = await chunk_text(
        text=text,
        strategy=ChunkStrategy.SENTENCE,
        chunk_size=20,
        chunk_overlap=5,
    )

    assert result["total_count"] > 0


@pytest.mark.asyncio
async def test_chunk_by_fixed_size():
    """测试固定大小分块。"""
    text = "A" * 100

    result = await chunk_text(
        text=text,
        strategy=ChunkStrategy.FIXED_SIZE,
        chunk_size=30,
        chunk_overlap=5,
    )

    # 应该分成至少3块
    assert result["total_count"] >= 3


@pytest.mark.asyncio
async def test_parse_text():
    """测试纯文本解析。"""
    text = b"Hello, world!\n\nThis is a test."

    result = await parse_document(
        source=text,
        format="txt",
    )

    assert result["success"] is True
    assert result["text"] == "Hello, world!\n\nThis is a test."
    # 解析器统一以规范格式名 "text" 标注 (txt 仅为文件扩展名别名)
    assert result["metadata"]["format"] == "text"


@pytest.mark.asyncio
async def test_parse_csv():
    """测试CSV解析。"""
    csv_content = b"name,age\nAlice,30\nBob,25"

    result = await parse_document(
        source=csv_content,
        format="csv",
    )

    assert result["success"] is True
    assert "Alice" in result["text"]
    assert "Bob" in result["text"]


@pytest.mark.asyncio
async def test_parse_pdf_from_file_path(tmp_path):
    """PDF 文件句柄在延迟读取页面时必须保持可用。"""
    pypdf = pytest.importorskip("pypdf")

    pdf_path = tmp_path / "sample.pdf"
    writer = pypdf.PdfWriter()
    writer.add_blank_page(width=72, height=72)
    with pdf_path.open("wb") as pdf_file:
        writer.write(pdf_file)

    result = await parse_document(source=str(pdf_path), format="pdf")

    assert result["success"] is True
    assert result["metadata"]["page_count"] == 1


@pytest.mark.asyncio
async def test_parse_document_preserves_parser_failure(monkeypatch):
    """底层解析失败时，不应被包装成空文档成功。"""

    class FailingParser:
        async def parse_from_bytes(self, source, **kwargs):
            return ParseResult(
                success=False,
                text="",
                metadata={},
                error="模拟解析失败",
            )

    monkeypatch.setattr(document_service, "get_parser", lambda _format: FailingParser())

    result = await document_service.parse_document(source=b"invalid", format="pdf")

    assert result["success"] is False
    assert result["text"] == ""
    assert result["error"] == "模拟解析失败"


# @pytest.mark.asyncio
# async def test_ocr_paddle():
#     """测试PaddleOCR识别。"""
#     # 需要实际的测试图片
#     pass


if __name__ == "__main__":
    # 简单运行测试
    asyncio.run(test_chunk_by_paragraph())
    asyncio.run(test_chunk_by_sentence())
    asyncio.run(test_chunk_by_fixed_size())
    asyncio.run(test_parse_text())
    asyncio.run(test_parse_csv())
    print("所有测试通过!")
