"""文档分块策略模块。"""

from enum import Enum
from typing import Any

from core import get_logger


class ChunkStrategy(Enum):
    """分块策略。"""

    PARAGRAPH = "paragraph"
    SENTENCE = "sentence"
    FIXED_SIZE = "fixed_size"
    SEMANTIC = "semantic"
    RECURSIVE = "recursive"


logger = get_logger(__name__)


# 递归分块的分隔符（按优先级）
_RECURSIVE_SEPARATORS = ["\n\n", "\n", ". ", "。", " ", ""]


def chunk_by_paragraph(
    text: str,
    chunk_size: int = 1000,
    chunk_overlap: int = 0,
    separator: str = "\n\n",
    **kwargs: Any,
) -> list[dict[str, Any]]:
    """按段落分块。

    Args:
        text: 待分块的文本
        chunk_size: 块大小（字符数）
        chunk_overlap: 重叠大小
        separator: 段落分隔符
        **kwargs: 其他选项

    Returns:
        分块结果列表
    """
    # 如果 separator 为空，使用默认值
    if not separator:
        separator = "\n\n"

    chunks = []
    paragraphs = text.split(separator)

    current_chunk = ""
    current_size = 0
    index = 0

    for para in paragraphs:
        para = para.strip()
        if not para:
            continue

        para_size = len(para)

        # 如果单个段落超过 chunk_size，需要分割
        if para_size > chunk_size:
            if current_chunk:
                chunks.append(_create_chunk(current_chunk, index))
                index += 1
                current_chunk = ""
                current_size = 0

            # 分割大段落
            sub_chunks = _split_long_text(para, chunk_size, separator=" ")
            for sub_chunk in sub_chunks:
                chunks.append(_create_chunk(sub_chunk, index))
                index += 1
            continue

        # 检查是否需要开始新块
        if current_size + para_size > chunk_size and current_chunk:
            chunks.append(_create_chunk(current_chunk, index))
            index += 1
            current_chunk = para
            current_size = para_size
        else:
            if current_chunk:
                current_chunk += separator + para
            else:
                current_chunk = para
            current_size += para_size + len(separator)

    # 添加最后一个块
    if current_chunk:
        chunks.append(_create_chunk(current_chunk, index))

    logger.info("按段落分块完成", chunk_count=len(chunks))
    return chunks


def chunk_by_sentence(
    text: str,
    chunk_size: int = 1000,
    chunk_overlap: int = 100,
    **kwargs: Any,
) -> list[dict[str, Any]]:
    """按句子分块。

    Args:
        text: 待分块的文本
        chunk_size: 块大小（字符数）
        chunk_overlap: 重叠大小
        **kwargs: 其他选项

    Returns:
        分块结果列表
    """
    import re

    # 简单的句子分割（中英文）
    sentences = re.split(r'([。！？.!?]+)', text)
    sentences = ["".join(pair) for pair in zip(sentences[::2], sentences[1::2])]

    chunks = []
    current_chunk = ""
    current_size = 0
    index = 0

    for sentence in sentences:
        sentence = sentence.strip()
        if not sentence:
            continue

        sentence_size = len(sentence)

        # 检查是否需要开始新块
        if current_size + sentence_size > chunk_size and current_chunk:
            chunks.append(_create_chunk(current_chunk, index))
            index += 1

            # 添加重叠部分
            if chunk_overlap > 0 and current_chunk:
                overlap_text = current_chunk[-chunk_overlap:]
                current_chunk = overlap_text + " " + sentence
                current_size = len(overlap_text) + 1 + sentence_size
            else:
                current_chunk = sentence
                current_size = sentence_size
        else:
            current_chunk += sentence
            current_size += sentence_size

    # 添加最后一个块
    if current_chunk:
        chunks.append(_create_chunk(current_chunk, index))

    logger.info("按句子分块完成", chunk_count=len(chunks))
    return chunks


def chunk_by_fixed_size(
    text: str,
    chunk_size: int = 1000,
    chunk_overlap: int = 200,
    **kwargs: Any,
) -> list[dict[str, Any]]:
    """按固定大小分块。

    Args:
        text: 待分块的文本
        chunk_size: 块大小（字符数）
        chunk_overlap: 重叠大小
        **kwargs: 其他选项

    Returns:
        分块结果列表
    """
    chunks = []
    index = 0
    pos = 0
    text_len = len(text)

    while pos < text_len:
        end_pos = min(pos + chunk_size, text_len)
        chunk_text = text[pos:end_pos]

        chunks.append(
            _create_chunk(chunk_text, index, start_pos=pos, end_pos=end_pos)
        )
        index += 1

        # 如果到达末尾，结束循环；否则应用重叠
        if end_pos >= text_len:
            break
        pos = end_pos - chunk_overlap if chunk_overlap > 0 else end_pos

    logger.info("按固定大小分块完成", chunk_count=len(chunks))
    return chunks


def chunk_semantic(
    text: str,
    chunk_size: int = 1000,
    chunk_overlap: int = 200,
    **kwargs: Any,
) -> list[dict[str, Any]]:
    """语义分块（基于句子向量相似度）。

    Args:
        text: 待分块的文本
        chunk_size: 块大小（字符数）
        chunk_overlap: 重叠大小
        **kwargs: 其他选项

    Returns:
        分块结果列表
    """
    model_name = kwargs.get("embedding_model", "paraphrase-multilingual-MiniLM-L12-v2")
    # 相似度低于该阈值视为语义边界（句向量归一化后取相对百分位）
    breakpoint_percentile = kwargs.get("breakpoint_percentile", 0.25)

    try:
        from sentence_transformers import SentenceTransformer
        import numpy as np
    except ImportError:
        # 依赖缺失时优雅降级到段落分块，保证功能可用
        logger.warning("sentence-transformers 未安装，语义分块回退到段落分块")
        return chunk_by_paragraph(text, chunk_size, chunk_overlap)

    # 切分为句子（复用句子分割逻辑）
    import re

    raw = re.split(r'(?<=[。！？.!?])\s*', text)
    sentences = [s.strip() for s in raw if s and s.strip()]

    if len(sentences) <= 1:
        return chunk_by_paragraph(text, chunk_size, chunk_overlap)

    try:
        model = SentenceTransformer(model_name)
        embeddings = model.encode(sentences, normalize_embeddings=True)
    except Exception as e:  # 模型下载/加载失败时降级
        logger.warning("语义模型加载失败，回退到段落分块", error=str(e))
        return chunk_by_paragraph(text, chunk_size, chunk_overlap)

    embeddings = np.asarray(embeddings)

    # 相邻句子余弦相似度（已归一化，点积即余弦）
    similarities = [
        float(np.dot(embeddings[i], embeddings[i + 1]))
        for i in range(len(sentences) - 1)
    ]

    # 以相似度的低百分位作为语义边界阈值
    if similarities:
        threshold = float(np.quantile(similarities, breakpoint_percentile))
    else:
        threshold = 0.0

    chunks: list[dict[str, Any]] = []
    current_sentences: list[str] = []
    current_size = 0
    index = 0

    for i, sentence in enumerate(sentences):
        sentence_size = len(sentence)

        # 超过 chunk_size，或在语义边界处（相似度低于阈值）则切分
        is_boundary = i > 0 and similarities[i - 1] < threshold
        over_size = current_size + sentence_size > chunk_size and current_sentences

        if over_size or (is_boundary and current_size >= chunk_overlap and current_sentences):
            chunks.append(_create_chunk("".join(current_sentences), index))
            index += 1
            current_sentences = [sentence]
            current_size = sentence_size
        else:
            current_sentences.append(sentence)
            current_size += sentence_size

    if current_sentences:
        chunks.append(_create_chunk("".join(current_sentences), index))

    logger.info("语义分块完成", chunk_count=len(chunks))
    return chunks


def chunk_recursive(
    text: str,
    chunk_size: int = 1000,
    chunk_overlap: int = 200,
    min_chunk_size: int = 100,
    **kwargs: Any,
) -> list[dict[str, Any]]:
    """递归分块（LangChain 风格）。

    按优先级尝试不同分隔符，优先保持段落完整。

    Args:
        text: 待分块的文本
        chunk_size: 块大小（字符数）
        chunk_overlap: 重叠大小
        min_chunk_size: 最小块大小
        **kwargs: 其他选项

    Returns:
        分块结果列表
    """
    chunks = _recursive_split(text, _RECURSIVE_SEPARATORS, chunk_size, chunk_overlap)

    # 过滤太小的块（但保留第一个）
    filtered = []
    for i, chunk in enumerate(chunks):
        if len(chunk["text"]) >= min_chunk_size or i == 0:
            filtered.append(chunk)

    logger.info("按递归策略分块完成", chunk_count=len(filtered))
    return filtered


def _recursive_split(
    text: str,
    separators: list[str],
    chunk_size: int,
    chunk_overlap: int,
) -> list[dict[str, Any]]:
    """递归分割文本。

    Args:
        text: 待分割文本
        separators: 分隔符列表（按优先级）
        chunk_size: 块大小
        chunk_overlap: 重叠大小

    Returns:
        分块列表
    """
    if not text:
        return []

    if not separators:
        # 没有更多分隔符，直接按大小分割
        return _split_by_size(text, chunk_size, chunk_overlap)

    separator = separators[0]
    remaining_separators = separators[1:]

    # 用当前分隔符分割
    splits = text.split(separator)

    if len(splits) == 1:
        # 分隔符无效，尝试下一个
        return _recursive_split(text, remaining_separators, chunk_size, chunk_overlap)

    chunks = []
    current_chunk = ""
    current_size = 0
    index = 0

    for split in splits:
        split_size = len(split)

        if current_size + split_size + len(separator) <= chunk_size:
            # 可以加入当前块
            if current_chunk:
                current_chunk += separator + split
            else:
                current_chunk = split
            current_size += split_size + len(separator)
        else:
            # 当前块已满
            if current_chunk:
                chunks.append(_create_chunk(current_chunk, index))
                index += 1

            if split_size > chunk_size:
                # 单个分割结果太大，递归处理
                sub_chunks = _recursive_split(split, remaining_separators, chunk_size, chunk_overlap)
                chunks.extend(sub_chunks)
                current_chunk = ""
                current_size = 0
            else:
                current_chunk = split
                current_size = split_size

    # 添加最后一个块
    if current_chunk:
        chunks.append(_create_chunk(current_chunk, index))

    return chunks


def _split_by_size(
    text: str,
    chunk_size: int,
    chunk_overlap: int,
) -> list[dict[str, Any]]:
    """按固定大小分割文本（无重叠）。

    Args:
        text: 待分割文本
        chunk_size: 块大小
        chunk_overlap: 重叠大小

    Returns:
        分块列表
    """
    chunks = []
    index = 0
    pos = 0
    text_len = len(text)

    while pos < text_len:
        end_pos = min(pos + chunk_size, text_len)
        chunk_text = text[pos:end_pos]

        chunks.append(_create_chunk(chunk_text, index))
        index += 1

        if end_pos >= text_len:
            break
        pos = end_pos - chunk_overlap if chunk_overlap > 0 else end_pos

    return chunks


def _create_chunk(
    text: str,
    index: int,
    start_pos: int = 0,
    end_pos: int = 0,
) -> dict[str, Any]:
    """创建分块。

    Args:
        text: 分块文本
        index: 分块索引
        start_pos: 起始位置
        end_pos: 结束位置

    Returns:
        分块字典
    """
    return {
        "text": text,
        "index": index,
        "start_pos": start_pos,
        "end_pos": end_pos or start_pos + len(text),
        "char_count": len(text),
    }


def _split_long_text(
    text: str,
    max_size: int,
    separator: str = " ",
) -> list[str]:
    """分割长文本。

    Args:
        text: 长文本
        max_size: 最大大小
        separator: 分隔符

    Returns:
        分割后的文本列表
    """
    if len(text) <= max_size:
        return [text]

    parts = text.split(separator)
    chunks = []
    current = ""

    for part in parts:
        if len(current) + len(part) + len(separator) <= max_size:
            if current:
                current += separator + part
            else:
                current = part
        else:
            if current:
                chunks.append(current)
            current = part

    if current:
        chunks.append(current)

    return chunks
