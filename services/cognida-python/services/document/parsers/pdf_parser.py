"""PDF 文档解析器。"""

from typing import Any

from .base import BaseParser, ParseResult


class PDFParser(BaseParser):
    """PDF 文档解析器。"""

    @property
    def supported_formats(self) -> list[str]:
        return ["pdf"]

    async def parse(
        self,
        source: str | bytes,
        include_metadata: bool = True,
        extract_tables: bool = False,
        extract_images: bool = False,
        **kwargs: Any,
    ) -> ParseResult:
        """解析 PDF 文档。

        Args:
            source: 文件路径或内容
            include_metadata: 是否包含元数据
            extract_tables: 是否提取表格
            extract_images: 是否提取图片
            **kwargs: 其他选项

        Returns:
            解析结果
        """
        try:
            from io import BytesIO

            import pypdf

            if isinstance(source, str):
                # 直接传入路径，让 pypdf 管理延迟读取所需的文件生命周期。
                pdf_reader = pypdf.PdfReader(source)
            else:
                # 从字节内容读取
                pdf_reader = pypdf.PdfReader(BytesIO(source))
            pages = pdf_reader.pages

            # 提取文本
            text_parts = []
            for page in pages:
                text_parts.append(page.extract_text())
            text = "\n\n".join(text_parts)

            # 构建元数据
            metadata = {}
            if include_metadata:
                metadata = {
                    "format": "pdf",
                    "page_count": len(pages),
                    "char_count": len(text),
                }
                if hasattr(pdf_reader, "metadata"):
                    metadata.update({k: str(v) for k, v in pdf_reader.metadata.items()})

            # TODO: 实现表格提取
            tables = None
            if extract_tables:
                tables = await self._extract_tables(source, **kwargs)

            # TODO: 实现图片提取
            images = None
            if extract_images:
                images = await self._extract_images(source, **kwargs)

            return ParseResult(
                success=True,
                text=text,
                metadata=metadata,
                tables=tables,
                images=images,
            )

        except Exception as e:
            return ParseResult(
                success=False,
                text="",
                metadata={},
                error=str(e),
            )

    async def _extract_tables(self, source: str | bytes, **kwargs: Any) -> list[dict[str, Any]]:
        """使用 pdfplumber 提取表格。

        Args:
            source: 文件路径或内容
            **kwargs: 其他选项

        Returns:
            表格列表，每个元素包含页码、行列数与单元格数据
        """
        try:
            import pdfplumber
        except ImportError:
            # 未安装 pdfplumber 时优雅降级，不阻断文本解析
            return []

        from io import BytesIO

        pdf_source: Any = source if isinstance(source, str) else BytesIO(source)
        tables: list[dict[str, Any]] = []

        with pdfplumber.open(pdf_source) as pdf:
            for page_index, page in enumerate(pdf.pages):
                for table_index, table in enumerate(page.extract_tables() or []):
                    if not table:
                        continue
                    # 规整单元格：None -> ""，并去除首尾空白
                    rows = [
                        [("" if cell is None else str(cell).strip()) for cell in row]
                        for row in table
                    ]
                    header = rows[0] if rows else []
                    tables.append(
                        {
                            "page": page_index + 1,
                            "index": table_index,
                            "row_count": len(rows),
                            "col_count": len(header),
                            "header": header,
                            "rows": rows,
                            "markdown": self._table_to_markdown(rows),
                        }
                    )

        return tables

    async def _extract_images(self, source: str | bytes, **kwargs: Any) -> list[dict[str, Any]]:
        """使用 PyMuPDF(fitz) 提取图片。

        Args:
            source: 文件路径或内容
            **kwargs: 其他选项

        Returns:
            图片信息列表，每个元素包含页码、尺寸、格式与 base64 数据
        """
        try:
            import fitz  # PyMuPDF
        except ImportError:
            # 未安装 PyMuPDF 时优雅降级
            return []

        import base64

        if isinstance(source, str):
            doc = fitz.open(source)
        else:
            doc = fitz.open(stream=source, filetype="pdf")

        images: list[dict[str, Any]] = []
        try:
            for page_index in range(doc.page_count):
                page = doc.load_page(page_index)
                for image_index, img in enumerate(page.get_images(full=True)):
                    xref = img[0]
                    base_image = doc.extract_image(xref)
                    image_bytes = base_image.get("image")
                    if not image_bytes:
                        continue
                    images.append(
                        {
                            "page": page_index + 1,
                            "index": image_index,
                            "xref": xref,
                            "ext": base_image.get("ext", "png"),
                            "width": base_image.get("width", 0),
                            "height": base_image.get("height", 0),
                            "data": base64.b64encode(image_bytes).decode("utf-8"),
                        }
                    )
        finally:
            doc.close()

        return images

    @staticmethod
    def _table_to_markdown(rows: list[list[str]]) -> str:
        """将二维表格数据转换为 Markdown 表格。"""
        if not rows:
            return ""

        col_count = max(len(row) for row in rows)

        def _pad(row: list[str]) -> list[str]:
            return row + [""] * (col_count - len(row))

        header = _pad(rows[0])
        lines = ["| " + " | ".join(header) + " |"]
        lines.append("| " + " | ".join(["---"] * col_count) + " |")
        for row in rows[1:]:
            lines.append("| " + " | ".join(_pad(row)) + " |")
        return "\n".join(lines)
