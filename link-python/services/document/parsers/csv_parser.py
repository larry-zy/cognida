"""CSV 文档解析器。"""

from typing import Any

from .base import BaseParser, ParseResult


class CSVParser(BaseParser):
    """CSV 文档解析器。"""

    @property
    def supported_formats(self) -> list[str]:
        return ["csv"]

    async def parse(
        self,
        source: str | bytes,
        include_metadata: bool = True,
        extract_tables: bool = True,
        extract_images: bool = False,
        **kwargs: Any,
    ) -> ParseResult:
        """解析 CSV 文档。

        Args:
            source: 文件路径或内容
            include_metadata: 是否包含元数据
            extract_tables: 是否提取表格
            extract_images: 是否提取图片
            **kwargs: 其他选项
                - encoding: 文件编码（默认 utf-8）
                - delimiter: 分隔符（默认 ,）

        Returns:
            解析结果
        """
        try:
            import csv
            from io import StringIO

            encoding = kwargs.get("encoding", "utf-8")
            delimiter = kwargs.get("delimiter", ",")

            if isinstance(source, str):
                with open(source, "r", encoding=encoding, newline="") as f:
                    reader = csv.reader(f, delimiter=delimiter)
                    rows = list(reader)
            else:
                text = source.decode(encoding)
                reader = csv.reader(StringIO(text), delimiter=delimiter)
                rows = list(reader)

            # 提取文本
            text_lines = ["\t".join(row) for row in rows]
            text = "\n".join(text_lines)

            # 构建元数据
            metadata = {}
            if include_metadata:
                metadata = {
                    "format": "csv",
                    "row_count": len(rows),
                    "column_count": len(rows[0]) if rows else 0,
                    "char_count": len(text),
                    "encoding": encoding,
                    "delimiter": delimiter,
                }

            # 提取表格
            tables = None
            if extract_tables and rows:
                tables = [{
                    "table_index": 0,
                    "rows": rows,
                    "markdown": self._table_to_markdown(rows),
                }]

            return ParseResult(
                success=True,
                text=text,
                metadata=metadata,
                tables=tables,
            )

        except Exception as e:
            return ParseResult(
                success=False,
                text="",
                metadata={},
                error=str(e),
            )

    def _table_to_markdown(self, rows: list[list[str]]) -> str:
        """将表格转换为 Markdown 格式。

        Args:
            rows: 表格行数据

        Returns:
            Markdown 字符串
        """
        if not rows:
            return ""

        lines = []
        for i, row in enumerate(rows):
            cells = [cell.replace("|", "\\|").strip() for cell in row]
            lines.append("| " + " | ".join(cells) + " |")
            if i == 0:
                lines.append("|" + "|".join(["---"] * len(cells)) + "|")

        return "\n".join(lines)
