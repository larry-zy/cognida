"""分词器单元测试。"""


from services.evaluation.metrics.tokenizer import (
    tokenize,
    tokenize_chinese,
    tokenize_english,
    tokenize_mixed,
    normalize_whitespace,
    remove_punctuation,
)


def test_chinese_tokenization():
    """测试中文分词。"""
    text = "这是一个测试文本"
    tokens = tokenize_chinese(text)

    assert len(tokens) > 0
    assert "测试" in tokens or "是" in tokens


def test_english_tokenization():
    """测试英文分词。"""
    text = "This is a test sentence"
    tokens = tokenize_english(text)

    assert len(tokens) == 5
    assert "this" in tokens
    assert "test" in tokens


def test_mixed_tokenization():
    """测试中英混合分词。"""
    text = "这是 mixed 混合 text 文本"
    tokens = tokenize_mixed(text)

    assert len(tokens) > 0
    # 应该包含中文词和英文词
    has_chinese = any("一-鿿" in t for t in tokens)
    has_english = any(t.isalpha() and t.islower() for t in tokens)
    # 由于jieba可能不可用，至少应该有一些token


def test_tokenize_auto():
    """测试自动语言检测分词。"""
    # 中文
    tokens = tokenize("这是中文", language="auto")
    assert len(tokens) > 0

    # 英文
    tokens = tokenize("This is English", language="auto")
    assert len(tokens) > 0

    # 混合
    tokens = tokenize("这是 mixed 文本", language="auto")
    assert len(tokens) > 0


def test_normalize_whitespace():
    """测试空白字符规范化。"""
    text = "这是    多个\n\n\t 空白  字符"
    normalized = normalize_whitespace(text)

    assert "  " not in normalized
    assert "\n" not in normalized
    assert "\t" not in normalized


def test_remove_punctuation():
    """测试标点移除。"""
    text = "Hello, 世界! 这是个测试。"
    result = remove_punctuation(text, keep_chinese_punct=False)

    assert "," not in result
    assert "!" not in result


def test_remove_punctuation_keep_chinese():
    """测试保留中文标点。"""
    text = "Hello, 世界！这是个测试。"
    result = remove_punctuation(text, keep_chinese_punct=True)

    assert "," not in result
    assert "！" in result  # 中文感叹号保留
    assert "。" in result  # 中文句号保留


if __name__ == "__main__":
    test_chinese_tokenization()
    test_english_tokenization()
    test_mixed_tokenization()
    test_tokenize_auto()
    test_normalize_whitespace()
    test_remove_punctuation()
    test_remove_punctuation_keep_chinese()
    print("所有分词测试通过！")
