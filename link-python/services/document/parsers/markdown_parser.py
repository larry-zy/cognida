"""Markdown 文档解析器。"""

from typing import Any

from .base import BaseParser, ParseResult


class MarkdownParser(BaseParser):
    """Markdown 文档解析器。"""

    @property
    def supported_formats(self) -> list[str]:
        return ["md", "markdown"]

    async def parse(
        self,
        source: str | bytes,
        include_metadata: bool = True,
        extract_tables: bool = False,
        extract_images: bool = False,
        **kwargs: Any,
    ) -> ParseResult:
        """解析 Markdown 文档。

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

            if isinstance(source, str):
                # 假设是文件路径
                try:
                    with open(source, "r", encoding="utf-8") as f:
                        text = f.read()
                except (FileNotFoundError, UnicodeDecodeError):
                    # 可能是直接文本
                    text = source
            else:
                text = source.decode("utf-8")

            # 提取图片链接
            images = None
            if extract_images:
                images = self._extract_images(text)

            # 提取表格
            tables = None
            if extract_tables:
                tables = self._extract_tables(text)

            # 构建元数据
            metadata = {}
            if include_metadata:
                metadata = {
                    "format": "markdown",
                    "char_count": len(text),
                    "line_count": len(text.splitlines()),
                }

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

    def _extract_images(self, text: str) -> list[dict[str, Any]]:
        """提取图片链接。

        Args:
            text: Markdown 文本

        Returns:
            图片信息列表
        """
        import re

        # 匹配 Markdown 图片语法 ![alt](url)
        pattern = r"!\[([^\]]*)\]\(([^)]+)\)"
        matches = re.findall(pattern, text)

        images = []
        for alt, url in matches:
            images.append({
                "alt": alt,
                "path": url,
            })

        return images

    def _extract_tables(self, text: str) -> list[dict[str, Any]]:
        """提取表格。

        Args:
            text: Markdown 文本

        Returns:
            表格列表
        """
        import re

        tables = []
        # 匹配 Markdown 表格
        table_pattern = r"""
            (?:^\|[^\n]+\|\n)+
        """
        matches = re.findall(table_pattern, text, re.MULTILINE)

        for idx, match in enumerate(matches):
            rows = []
            lines = match.strip().split("\n")
            for line in lines:
                if line.startswith("|"):
                    cells = [cell.strip() for cell in line.split("|")[1:-1]]
                    if cells:  # 跳过空行（分隔符行）
                        rows.append(cells)

            if rows:
                tables.append({
                    "table_index": idx,
                    "rows": rows,
                    "markdown": match.strip(),
                })

        return tables
