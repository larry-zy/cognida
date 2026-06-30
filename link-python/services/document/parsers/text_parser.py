"""纯文本文档解析器。"""

from typing import Any

from .base import BaseParser, ParseResult


class TextParser(BaseParser):
    """纯文本文档解析器。"""

    @property
    def supported_formats(self) -> list[str]:
        return ["txt", "text"]

    async def parse(
        self,
        source: str | bytes,
        include_metadata: bool = True,
        extract_tables: bool = False,
        extract_images: bool = False,
        **kwargs: Any,
    ) -> ParseResult:
        """解析纯文本文档。

        Args:
            source: 文件路径或内容
            include_metadata: 是否包含元数据
            extract_tables: 是否提取表格
            extract_images: 是否提取图片
            **kwargs: 其他选项
                - encoding: 文件编码（默认 utf-8）

        Returns:
            解析结果
        """
        try:
            from io import BytesIO

            encoding = kwargs.get("encoding", "utf-8")

            if isinstance(source, str):
                # 尝试作为文件路径打开
                try:
                    with open(source, "r", encoding=encoding) as f:
                        text = f.read()
                except (FileNotFoundError, OSError):
                    # 作为直接文本
                    text = source
            else:
                text = source.decode(encoding)

            # 构建元数据
            metadata = {}
            if include_metadata:
                metadata = {
                    "format": "text",
                    "char_count": len(text),
                    "line_count": len(text.splitlines()),
                    "encoding": encoding,
                }

            return ParseResult(
                success=True,
                text=text,
                metadata=metadata,
            )

        except Exception as e:
            return ParseResult(
                success=False,
                text="",
                metadata={},
                error=str(e),
            )
