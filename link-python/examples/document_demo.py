"""文档处理功能演示。

展示如何使用文档解析、OCR、分块等功能。
"""

import asyncio
from pathlib import Path

from services.document import (
    parse_document,
    ocr_image,
    chunk_text,
    fetch_url,
    ChunkStrategy,
)


async def demo_text_parsing():
    """演示文本解析。"""
    print("=" * 50)
    print("1. 文本解析演示")
    print("=" * 50)

    text = "Python 是一种高级编程语言。\n\n它具有简洁的语法和强大的功能。\n\nPython 广泛应用于 Web 开发、数据分析、人工智能等领域。".encode("utf-8")

    result = await parse_document(
        source=text,
        format="txt",
        include_metadata=True,
    )

    print(f"成功: {result['success']}")
    print(f"内容:\n{result['text']}")
    print(f"元数据: {result['metadata']}")
    print()


async def demo_chunking():
    """演示文本分块。"""
    print("=" * 50)
    print("2. 文本分块演示")
    print("=" * 50)

    text = """第一段：这是关于Python的介绍。

第二段：Python具有简洁的语法。

第三段：Python拥有丰富的生态系统。

第四段：Python广泛应用于各个领域。

第五段：学习Python是一个很好的选择。"""

    # 按段落分块
    result = await chunk_text(
        text=text,
        strategy=ChunkStrategy.PARAGRAPH,
        chunk_size=50,
        chunk_overlap=0,
    )

    print(f"分块策略: paragraph")
    print(f"分块数量: {result['total_count']}")
    print("分块内容:")
    for chunk in result["chunks"]:
        print(f"  [{chunk['index']}] {chunk['text'][:30]}...")
    print()


async def demo_sentence_chunking():
    """演示句子分块。"""
    print("=" * 50)
    print("3. 句子分块演示")
    print("=" * 50)

    text = "这是第一句。这是第二句。这是第三句。这是第四句。这是第五句。"

    result = await chunk_text(
        text=text,
        strategy=ChunkStrategy.SENTENCE,
        chunk_size=30,
        chunk_overlap=5,
    )

    print(f"分块策略: sentence")
    print(f"分块数量: {result['total_count']}")
    for chunk in result["chunks"]:
        print(f"  [{chunk['index']}] {chunk['text']}")
    print()


async def demo_fixed_size_chunking():
    """演示固定大小分块。"""
    print("=" * 50)
    print("4. 固定大小分块演示")
    print("=" * 50)

    text = "A" * 100

    result = await chunk_text(
        text=text,
        strategy=ChunkStrategy.FIXED_SIZE,
        chunk_size=30,
        chunk_overlap=5,
    )

    print(f"分块策略: fixed_size")
    print(f"分块数量: {result['total_count']}")
    for chunk in result["chunks"]:
        print(f"  [{chunk['index']}] 长度={len(chunk['text'])}, 内容={chunk['text'][:20]}...")
    print()


async def demo_csv_parsing():
    """演示CSV解析。"""
    print("=" * 50)
    print("5. CSV 解析演示")
    print("=" * 50)

    csv_content = b"name,age,city\nAlice,30,Beijing\nBob,25,Shanghai\nCharlie,35,Guangzhou"

    result = await parse_document(
        source=csv_content,
        format="csv",
    )

    print(f"成功: {result['success']}")
    print(f"内容:\n{result['text']}")
    print()


async def demo_markdown_parsing():
    """演示Markdown解析。"""
    print("=" * 50)
    print("6. Markdown 解析演示")
    print("=" * 50)

    md_content = "# Title\n\nThis is some text.\n\n- Item 1\n- Item 2\n\n[Link](https://example.com)".encode("utf-8")

    result = await parse_document(
        source=md_content,
        format="md",
    )

    print(f"成功: {result['success']}")
    print(f"内容:\n{result['text']}")
    print()


# async def demo_ocr():
#     """演示OCR识别。"""
#     print("=" * 50)
#     print("7. OCR 识别演示")
#     print("=" * 50)
#
#     # 需要实际的测试图片
#     image_path = "path/to/test/image.png"
#
#     result = await ocr_image(
#         source=image_path,
#         engine="paddleocr",
#         language="chi_sim",
#     )
#
#     print(f"成功: {result['success']}")
#     print(f"识别文本: {result['text']}")
#     print(f"置信度: {result['confidence']}")
#     print()


# async def demo_url_fetch():
#     """演示URL获取。"""
#     print("=" * 50)
#     print("8. URL 获取演示")
#     print("=" * 50)
#
#     result = await fetch_url(
#         url="https://example.com",
#         timeout=10,
#     )
#
#     print(f"成功: {result['success']}")
#     print(f"标题: {result['title']}")
#     print(f"链接数: {len(result['links'])}")
#     print(f"文本预览: {result['text'][:100]}...")
#     print()


async def main():
    """主函数。"""
    print("\n文档处理功能演示\n")

    await demo_text_parsing()
    await demo_chunking()
    await demo_sentence_chunking()
    await demo_fixed_size_chunking()
    await demo_csv_parsing()
    await demo_markdown_parsing()
    # await demo_ocr()
    # await demo_url_fetch()

    print("=" * 50)
    print("演示完成!")
    print("=" * 50)


if __name__ == "__main__":
    asyncio.run(main())
