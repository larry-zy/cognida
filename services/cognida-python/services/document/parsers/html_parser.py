"""Offline HTML extraction; never executes scripts or downloads linked resources."""

from pathlib import Path
from typing import Any

from bs4 import BeautifulSoup

from .base import BaseParser, ParseResult


class HTMLParser(BaseParser):
    @property
    def supported_formats(self):
        return ["html", "htm"]

    async def parse(
        self,
        source: str | bytes,
        include_metadata: bool = True,
        extract_tables: bool = False,
        extract_images: bool = False,
        **kwargs: Any,
    ) -> ParseResult:
        try:
            content = Path(source).read_bytes() if isinstance(source, str) else source
            if len(content) > 20 * 1024 * 1024:
                raise ValueError("HTML exceeds 20 MiB")
            soup = BeautifulSoup(content, "html.parser")
            title = soup.title.get_text(" ", strip=True) if soup.title else ""
            for element in soup(["script", "style", "noscript", "template"]):
                element.decompose()
            text = soup.get_text("\n", strip=True)
            tables = []
            if extract_tables:
                for index, table in enumerate(soup.find_all("table")[:100]):
                    rows = [
                        [
                            cell.get_text(" ", strip=True)
                            for cell in row.find_all(["td", "th"], recursive=False)
                        ]
                        for row in table.find_all("tr")
                    ]
                    rows = [row for row in rows if row]
                    tables.append({"table_index": index, "rows": rows})
            images = (
                [
                    {
                        "image_index": index,
                        "path": image.get("src", ""),
                        "alt": image.get("alt", ""),
                    }
                    for index, image in enumerate(soup.find_all("img")[:100])
                ]
                if extract_images
                else []
            )
            return ParseResult(
                True,
                text,
                {
                    "format": "html",
                    "char_count": len(text),
                    "page_count": 1,
                    "properties": {"title": title},
                }
                if include_metadata
                else {},
                tables=tables,
                images=images,
            )
        except Exception as exc:
            return ParseResult(False, "", {}, error=str(exc))
