"""文档分块模块。"""

from typing import Any

from .strategies import (
    ChunkStrategy,
    chunk_by_paragraph,
    chunk_by_sentence,
    chunk_by_fixed_size,
    chunk_semantic,
    chunk_recursive,
)

__all__ = [
    "ChunkStrategy",
    "chunk_by_paragraph",
    "chunk_by_sentence",
    "chunk_by_fixed_size",
    "chunk_semantic",
    "chunk_recursive",
]


def chunk_document(
    text: str,
    strategy: ChunkStrategy = ChunkStrategy.PARAGRAPH,
    chunk_size: int = 1000,
    chunk_overlap: int = 200,
    **kwargs: Any,
) -> list[dict[str, Any]]:
    """分块文档。

    Args:
        text: 待分块的文本
        strategy: 分块策略
        chunk_size: 块大小
        chunk_overlap: 重叠大小
        **kwargs: 其他选项

    Returns:
        分块结果列表
    """
    chunkers = {
        ChunkStrategy.PARAGRAPH: chunk_by_paragraph,
        ChunkStrategy.SENTENCE: chunk_by_sentence,
        ChunkStrategy.FIXED_SIZE: chunk_by_fixed_size,
        ChunkStrategy.SEMANTIC: chunk_semantic,
        ChunkStrategy.RECURSIVE: chunk_recursive,
    }

    chunker = chunkers.get(strategy, chunk_by_paragraph)
    return chunker(text, chunk_size=chunk_size, chunk_overlap=chunk_overlap, **kwargs)
