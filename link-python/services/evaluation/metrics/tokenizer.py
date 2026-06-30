"""文本分词工具。

支持中文（jieba）和英文分词，以及混合文本处理。
"""

import re
import string
from typing import List

try:
    import jieba
    JIEBA_AVAILABLE = True
except ImportError:
    JIEBA_AVAILABLE = False


# 中文正则：匹配中文字符
CHINESE_PATTERN = re.compile(r'[一-鿿]+')
# 英文正则：匹配英文单词（包含缩写和连字符）
ENGLISH_PATTERN = re.compile(r'\b[a-zA-Z]+(?:[-\'][a-zA-Z]+)*\b')


def tokenize_chinese(text: str, cut_all: bool = False) -> List[str]:
    """使用 jieba 进行中文分词。

    Args:
        text: 输入文本
        cut_all: 是否使用全模式（返回所有可能的词）

    Returns:
        分词结果列表

    Raises:
        ImportError: jieba 未安装时
    """
    if not JIEBA_AVAILABLE:
        raise ImportError("jieba is required for Chinese tokenization. Install with: pip install jieba")

    # jieba.cut 返回生成器，转换为列表
    return list(jieba.cut(text, cut_all=cut_all))


def tokenize_english(text: str) -> List[str]:
    """英文分词。

    Args:
        text: 输入文本

    Returns:
        小写化后的单词列表
    """
    # 转小写并查找所有英文单词
    words = ENGLISH_PATTERN.findall(text.lower())
    return words


def tokenize_mixed(text: str, cut_all: bool = False) -> List[str]:
    """混合中英文分词。

    分别对中文和英文部分进行分词，然后合并结果。

    Args:
        text: 输入文本
        cut_all: 中文分词是否使用全模式

    Returns:
        分词结果列表
    """
    tokens = []
    pos = 0

    for match in CHINESE_PATTERN.finditer(text):
        # 处理中文之前的英文部分
        if match.start() > pos:
            english_part = text[pos:match.start()]
            if english_part.strip():
                tokens.extend(tokenize_english(english_part))

        # 处理中文部分
        chinese_part = match.group()
        if JIEBA_AVAILABLE:
            tokens.extend(tokenize_chinese(chinese_part, cut_all=cut_all))
        else:
            # 如果 jieba 不可用，按字符分割
            tokens.extend(list(chinese_part))

        pos = match.end()

    # 处理剩余的英文部分
    if pos < len(text):
        english_part = text[pos:]
        if english_part.strip():
            tokens.extend(tokenize_english(english_part))

    return tokens


def tokenize(text: str, language: str = "auto", cut_all: bool = False) -> List[str]:
    """通用分词函数。

    Args:
        text: 输入文本
        language: 语言类型 ("zh", "en", "auto")
        cut_all: 中文分词是否使用全模式

    Returns:
        分词结果列表
    """
    if language == "zh":
        return tokenize_chinese(text, cut_all=cut_all)
    elif language == "en":
        return tokenize_english(text)
    elif language == "auto":
        return tokenize_mixed(text, cut_all=cut_all)
    else:
        raise ValueError(f"Unsupported language: {language}")


def normalize_whitespace(text: str) -> str:
    """规范化空白字符。

    将连续空白字符替换为单个空格。

    Args:
        text: 输入文本

    Returns:
        规范化后的文本
    """
    return re.sub(r'\s+', ' ', text).strip()


def remove_punctuation(text: str, keep_chinese_punct: bool = False) -> str:
    """移除标点符号。

    Args:
        text: 输入文本
        keep_chinese_punct: 是否保留中文标点

    Returns:
        移除标点后的文本
    """
    # 英文标点
    to_remove = string.punctuation

    # 如果不保留中文标点，添加常见中文标点
    if not keep_chinese_punct:
        chinese_punct = '，。！？；：""''（）【】《》、'
        to_remove += chinese_punct

    # 创建翻译表
    trans_table = str.maketrans('', '', to_remove)
    return text.translate(trans_table)


# 预编译的常用词表（用于测试和验证）
COMMON_CHINESE_WORDS = {
    "我", "你", "他", "她", "它", "我们", "你们", "他们",
    "的", "了", "在", "是", "我", "有", "和", "就", "不", "人",
    "都", "一", "一个", "上", "也", "很", "到", "说", "要", "去",
    "你", "会", "着", "没有", "看", "好", "自己", "这",
}

COMMON_ENGLISH_WORDS = {
    "the", "be", "to", "of", "and", "a", "in", "that", "have", "i",
    "it", "for", "not", "on", "with", "he", "as", "you", "do", "at",
    "this", "but", "his", "by", "from", "they", "we", "say", "her", "she",
    "or", "an", "will", "my", "one", "all", "would", "there", "their",
}


if __name__ == "__main__":
    # 测试分词功能
    test_cases = [
        ("这是一个测试文本", "zh"),
        ("This is a test sentence", "en"),
        ("这是一个 mixed 混合 text 文本", "auto"),
    ]

    for text, lang in test_cases:
        tokens = tokenize(text, language=lang)
        print(f"[{lang}] {text}")
        print(f"  -> {tokens}")
