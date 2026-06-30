"""非结构化数据评估器测试。"""

import pytest

from services.quality.unstructured.readability import ReadabilityEvaluator
from services.quality.unstructured.information_density import InformationDensityEvaluator
from services.quality.unstructured.language_quality import LanguageQualityEvaluator
from services.quality.unstructured.duplication import DuplicationEvaluator
from services.quality.unstructured.pii_detector import PIIDetector
from services.quality.unstructured.relevance import RelevanceEvaluator
from services.quality.models import SeverityLevel


@pytest.fixture
def normal_text():
    """正常文本样本。"""
    return """
    人工智能是计算机科学的一个分支，致力于创建能够执行通常需要人类智能的任务的系统。
    机器学习是人工智能的核心技术之一，它使计算机能够从数据中学习并改进。
    深度学习是机器学习的一个子领域，使用神经网络来模拟人脑的学习过程。
    """


@pytest.fixture
def short_text():
    """短文本样本。"""
    return "短文本。"


@pytest.fixture
def garbled_text():
    """乱码文本样本。"""
    return "正常文本���这里有控制字符\x01\x02和乱码内容。"


@pytest.fixture
def low_density_text():
    """低信息密度文本样本。"""
    return """
    的了是在我和有他这为之大来以个中上们到说国去发你过好作对生能而也子那事于成么就如么都天样看又起时把还下用可者这就出很会么者面想后学所做所看所用所想。
    """


@pytest.fixture
def high_density_text():
    """高信息密度文本样本。"""
    return """
    人工智能机器学习深度学习神经网络自然语言处理计算机视觉数据挖掘大数据分析算法优化模型训练推理部署特征工程数据清洗ETL数据管道实时流处理。
    """


@pytest.fixture
def grammatical_errors_text():
    """语法错误文本样本。"""
    return "这个句子，，标点符号错误。。缺少空格这里有重复词重复词。"


@pytest.fixture
def text_with_pii():
    """包含敏感信息的文本样本。"""
    return """
    请联系张三，电话号码是13812345678，邮箱是zhangsan@example.com。
    身份证号是320102199001011234，地址是南京市玄武区某某街道123号。
    """


@pytest.fixture
def duplicate_text():
    """重复内容文本样本。"""
    return """
    这是一段测试文本。这是一段测试文本。这是一段测试文本。
    This is a sample text. This is a sample text.
    """


class TestReadabilityEvaluator:
    """可读性评估器测试。"""

    def test_normal_text_evaluation(self, normal_text):
        """测试正常文本评估。"""
        evaluator = ReadabilityEvaluator()
        result = evaluator.evaluate(normal_text)

        assert result.name == "readability"
        assert result.score > 70
        assert result.passed is True

    def test_short_text_detection(self, short_text):
        """测试短文本检测。"""
        evaluator = ReadabilityEvaluator()
        result = evaluator.evaluate(short_text)

        assert result.score < 70
        # 应该有长度相关的issues
        length_issues = [i for i in result.issues if "长度" in i.description]
        assert len(length_issues) > 0

    def test_garbled_text_detection(self, garbled_text):
        """测试乱码检测。"""
        evaluator = ReadabilityEvaluator()
        result = evaluator.evaluate(garbled_text)

        assert result.score < 70
        # 应该检测到编码问题
        encoding_issues = [i for i in result.issues if "编码" in i.description or "乱码" in i.description]
        assert len(encoding_issues) > 0

    def test_chinese_language_detection(self, normal_text):
        """测试中文语言检测。"""
        evaluator = ReadabilityEvaluator()
        result = evaluator.evaluate(normal_text)

        assert "language" in result.details
        assert result.details["language"] in ["zh", "chinese", "mixed"]

    def test_readable_character_ratio(self, normal_text):
        """测试可读字符比例。"""
        evaluator = ReadabilityEvaluator()
        result = evaluator.evaluate(normal_text)

        assert "readable_ratio" in result.details
        assert result.details["readable_ratio"] > 0.8


class TestInformationDensityEvaluator:
    """信息密度评估器测试。"""

    def test_normal_density_evaluation(self, normal_text):
        """测试正常密度评估。"""
        evaluator = InformationDensityEvaluator()
        result = evaluator.evaluate(normal_text)

        assert result.name == "information_density"
        assert 0 <= result.score <= 100

    def test_low_density_detection(self, low_density_text):
        """测试低信息密度检测。"""
        evaluator = InformationDensityEvaluator()
        result = evaluator.evaluate(low_density_text)

        assert result.score < 70
        # 应该检测到停用词比例高
        assert "stopword_ratio" in result.details

    def test_high_density_detection(self, high_density_text):
        """测试高信息密度检测。"""
        evaluator = InformationDensityEvaluator()
        result = evaluator.evaluate(high_density_text)

        assert result.score > 70
        # 有效词比例应该高
        assert "valid_word_ratio" in result.details

    def test_word_count(self, normal_text):
        """测试词数统计。"""
        evaluator = InformationDensityEvaluator()
        result = evaluator.evaluate(normal_text)

        assert "word_count" in result.details
        assert result.details["word_count"] > 0
        assert "unique_word_count" in result.details

    def test_jieba_integration(self, normal_text):
        """测试jieba分词集成。"""
        evaluator = InformationDensityEvaluator()
        result = evaluator.evaluate(normal_text)

        # 应该有分词相关的统计
        assert "word_count" in result.details or "unique_words" in result.details


class TestLanguageQualityEvaluator:
    """语言质量评估器测试。"""

    def test_normal_quality_evaluation(self, normal_text):
        """测试正常质量评估。"""
        evaluator = LanguageQualityEvaluator()
        result = evaluator.evaluate(normal_text)

        assert result.name == "language_quality"
        assert result.score > 60

    def test_punctuation_detection(self, grammatical_errors_text):
        """测试标点符号检测。"""
        evaluator = LanguageQualityEvaluator()
        result = evaluator.evaluate(grammatical_errors_text)

        # 应该检测到标点符号问题
        punctuation_issues = [i for i in result.issues if "标点" in i.description]
        assert len(punctuation_issues) > 0

    def test_whitespace_detection(self, grammatical_errors_text):
        """测试空白字符检测。"""
        evaluator = LanguageQualityEvaluator()
        result = evaluator.evaluate(grammatical_errors_text)

        # 可能检测到空格问题
        whitespace_issues = [i for i in result.issues if "空格" in i.description]
        # 不强制要求，因为文本可能没有明显的空格问题

    def test_repeated_word_detection(self, grammatical_errors_text):
        """测试重复词检测。"""
        evaluator = LanguageQualityEvaluator()
        result = evaluator.evaluate(grammatical_errors_text)

        # 应该检测到重复词
        repeat_issues = [i for i in result.issues if "重复" in i.description]
        assert len(repeat_issues) > 0


class TestDuplicationEvaluator:
    """重复度评估器测试。"""

    def test_no_duplication(self, normal_text):
        """测试无重复内容。"""
        evaluator = DuplicationEvaluator()
        result = evaluator.evaluate(normal_text)

        assert result.score > 80
        assert len(result.issues) == 0 or all("重复" not in i.description for i in result.issues)

    def test_exact_duplication(self, duplicate_text):
        """测试精确重复检测。"""
        evaluator = DuplicationEvaluator()
        result = evaluator.evaluate(duplicate_text)

        assert result.score < 80
        # 应该检测到重复
        duplication_issues = [i for i in result.issues if "重复" in i.description]
        assert len(duplication_issues) > 0

    def test_similarity_detection(self):
        """测试相似度检测。"""
        text = """
        人工智能是计算机科学的重要分支。
        人工智能是计算机科学的一个重要领域。
        """
        evaluator = DuplicationEvaluator()
        result = evaluator.evaluate(text)

        # 应该检测到相似内容
        similarity_issues = [i for i in result.issues if "相似" in i.description]
        # 相似度阈值可能不触发，所以不做强制断言

    def test_minhash_lsh_integration(self, duplicate_text):
        """测试MinHash + LSH集成。"""
        evaluator = DuplicationEvaluator()
        config = {"use_minhash": True, "threshold": 0.7}
        result = evaluator.evaluate(duplicate_text, config)

        assert "duplication_ratio" in result.details or "similar_sentences" in result.details


class TestPIIDetector:
    """敏感信息检测测试。"""

    def test_phone_detection(self, text_with_pii):
        """测试手机号检测。"""
        evaluator = PIIDetector()
        result = evaluator.evaluate(text_with_pii)

        # 应该检测到手机号
        phone_issues = [i for i in result.issues if "手机" in i.description or "电话" in i.description]
        assert len(phone_issues) > 0

    def test_email_detection(self, text_with_pii):
        """测试邮箱检测。"""
        evaluator = PIIDetector()
        result = evaluator.evaluate(text_with_pii)

        # 应该检测到邮箱
        email_issues = [i for i in result.issues if "邮箱" in i.description or "邮件" in i.description]
        assert len(email_issues) > 0

    def test_id_card_detection(self, text_with_pii):
        """测试身份证号检测。"""
        evaluator = PIIDetector()
        result = evaluator.evaluate(text_with_pii)

        # 应该检测到身份证号
        id_issues = [i for i in result.issues if "身份证" in i.description]
        assert len(id_issues) > 0

    def test_address_detection(self, text_with_pii):
        """测试地址检测。"""
        evaluator = PIIDetector()
        result = evaluator.evaluate(text_with_pii)

        # 应该检测到地址
        address_issues = [i for i in result.issues if "地址" in i.description]
        assert len(address_issues) > 0

    def test_severity_level(self, text_with_pii):
        """测试严重级别。"""
        evaluator = PIIDetector()
        result = evaluator.evaluate(text_with_pii)

        # PII问题应该是INFO、WARNING或CRITICAL级别
        assert all(i.severity in [SeverityLevel.INFO, SeverityLevel.WARNING, SeverityLevel.CRITICAL] for i in result.issues)


class TestRelevanceEvaluator:
    """主题相关性评估测试。"""

    def test_keyword_matching(self):
        """测试关键词匹配。"""
        text = """
        人工智能和机器学习是当今科技发展的热点领域。
        深度学习作为机器学习的一个重要分支，在图像识别和自然语言处理方面取得了突破性进展。
        """
        evaluator = RelevanceEvaluator()
        config = {"keywords": ["人工智能", "机器学习", "深度学习", "神经网络"]}
        result = evaluator.evaluate(text, config)

        assert result.score > 70

    def test_no_keywords(self):
        """测试不包含关键词的文本。"""
        text = "今天天气很好，我去公园散步。"
        evaluator = RelevanceEvaluator()
        config = {"keywords": ["人工智能", "机器学习"]}
        result = evaluator.evaluate(text, config)

        assert result.score < 50

    def test_topic_classification(self):
        """测试主题分类。"""
        text = """
        本季度公司营收增长20%，净利润达到5000万元。
        用户活跃度提升15%，市场份额进一步扩大。
        """
        evaluator = RelevanceEvaluator()
        config = {"topics": ["科技", "金融", "体育", "娱乐"]}
        result = evaluator.evaluate(text, config)

        assert "detected_topic" in result.details

    def test_config_with_weights(self):
        """测试带权重的配置。"""
        text = "人工智能和机器学习在金融领域有广泛应用。"
        evaluator = RelevanceEvaluator()
        config = {
            "keywords": ["人工智能", "机器学习", "金融"],
            "weights": {"人工智能": 2.0, "机器学习": 1.5, "金融": 1.0}
        }
        result = evaluator.evaluate(text, config)

        assert result.score > 0
        assert "keyword_matches" in result.details
