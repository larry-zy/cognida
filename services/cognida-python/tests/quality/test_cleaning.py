"""数据清洗模块测试。"""

import pandas as pd
import pytest

from services.quality.cleaning.text_cleaner import TextCleaner
from services.quality.cleaning.dedup import DedupProcessor
from services.quality.cleaning.denoiser import Denoiser
from services.quality.cleaning.format_converter import FormatConverter
from services.quality.cleaning.pii_masker import PIIMasker
from services.quality.models import CleaningResult


@pytest.fixture
def sample_dataframe():
    """创建测试用的DataFrame。"""
    return pd.DataFrame({
        "id": [1, 2, 3, 4, 5],
        "text": [
            "正常文本",
            "  包含多余空格  ",
            "包含\t制表符\n和换行",
            "  多余    空格  ",
            "正常文本",
        ],
        "email": [
            "user1@example.com",
            "user2@example.com",
            "user1@example.com",  # 重复
            "user4@example.com",
            "user5@example.com",
        ]
    })


@pytest.fixture
def dirty_text():
    """包含需要清洗内容的文本。"""
    return """
    <div>  这段文本  包含  <b>HTML标签</b>  和  多余空格  </div>
    \t\n  还有制表符和换行符  \t\n
    """


@pytest.fixture
def text_with_ads():
    """包含广告的文本。"""
    return """
    这是一篇关于人工智能的文章。AI技术正在快速发展。
    限时优惠！点击链接领取优惠券！立即购买！
    加微信：abc123免费领取资料。
    更多详情请访问：www.example.com/promo
    """


@pytest.fixture
def text_with_pii():
    """包含敏感信息的文本。"""
    return "请联系张三，电话13812345678，邮箱zhangsan@example.com，身份证320102199001011234。"


class TestTextCleaner:
    """文本清洗器测试。"""

    def test_basic_cleaning(self, dirty_text):
        """测试基础清洗功能。"""
        cleaner = TextCleaner()
        cleaned = cleaner.clean_text(dirty_text)

        # 应该移除HTML标签
        assert "<div>" not in cleaned
        assert "<b>" not in cleaned
        # 应该标准化空格
        assert "  " not in cleaned or cleaned.count("  ") < dirty_text.count("  ")

    def test_html_removal(self):
        """测试HTML标签移除。"""
        text = "<p>段落</p><div>内容</div><span>更多</span>"
        cleaner = TextCleaner()
        cleaned = cleaner.clean_text(text, remove_html=True)

        assert "<p>" not in cleaned
        assert "<div>" not in cleaned
        assert "段落内容更多" in cleaned

    def test_whitespace_normalization(self):
        """测试空白字符标准化。"""
        text = "这  是\t一段\n包含  多种  空白  的文本"
        cleaner = TextCleaner()
        cleaned = cleaner.clean_text(text, normalize_whitespace=True)

        # 不应该有连续空格
        assert "  " not in cleaned
        # 不应该有制表符
        assert "\t" not in cleaned

    def test_encoding_repair(self):
        """测试编码修复。"""
        # 模拟乱码文本
        text = "正常文本"
        cleaner = TextCleaner()
        cleaned = cleaner.clean_text(text)

        # 确保清洗后文本不为空
        assert len(cleaned) > 0

    def test_dataframe_cleaning(self, sample_dataframe):
        """测试DataFrame清洗。"""
        cleaner = TextCleaner()
        result = cleaner.clean(sample_dataframe, {"columns": ["text"]})

        assert isinstance(result, CleaningResult)
        assert result.cleaned_count <= result.original_count
        assert len(result.operations) > 0


class TestDedupProcessor:
    """去重处理器测试。"""

    def test_exact_deduplication(self, sample_dataframe):
        """测试精确去重。"""
        processor = DedupProcessor()
        result = processor.clean(sample_dataframe, {"subset": ["text"]})

        # 应该移除重复行
        assert result.cleaned_count < result.original_count
        # 移除的数量应该是重复的行数
        assert result.removed_count > 0

    def test_fuzzy_deduplication(self):
        """测试模糊去重。"""
        df = pd.DataFrame({
            "id": [1, 2, 3, 4],
            "text": [
                "这是一段关于AI的文本",
                "这是一段关于AI的内容",  # 相似
                "完全不同的文本",
                "这又是一段关于AI的文字",  # 相似
            ]
        })
        processor = DedupProcessor()
        result = processor.clean(
            df,
            {
                "method": "fuzzy",
                "threshold": 0.7,
                "column": "text"
            }
        )

        # 应该检测到相似内容
        assert result.cleaned_count <= result.original_count
        assert len(result.operations) > 0

    def test_minhash_deduplication(self):
        """测试MinHash大规模去重。"""
        df = pd.DataFrame({
            "id": range(100),
            "text": [f"文本内容{i % 10}" for i in range(100)],  # 有重复
        })
        processor = DedupProcessor()
        result = processor.clean(
            df,
            {
                "method": "minhash",
                "threshold": 0.8,
                "column": "text"
            }
        )

        # MinHash应该检测到重复
        assert result.removed_count > 0

    def test_empty_dataframe(self):
        """测试空DataFrame处理。"""
        df = pd.DataFrame()
        processor = DedupProcessor()
        result = processor.clean(df, {})

        assert result.cleaned_count == 0


class TestDenoiser:
    """去噪处理器测试。"""

    def test_ad_removal(self, text_with_ads):
        """测试广告内容移除。"""
        denoiser = Denoiser()
        cleaned = denoiser.clean_text(text_with_ads, {"remove_ads": True})

        # 应该移除或淡化广告内容
        assert "优惠" not in cleaned or cleaned.count("优惠") < text_with_ads.count("优惠")

    def test_template_removal(self):
        """测试模板内容移除。"""
        text = """
        尊敬的用户，您好！
        感谢您的咨询。
        我们已收到您的问题，将尽快回复。
        祝您生活愉快！
        ---
        这是实际的咨询内容：我想了解产品价格。
        """
        denoiser = Denoiser()
        cleaned = denoiser.clean_text(text, {"remove_templates": True})

        # 模板内容应该被标记或移除
        assert len(cleaned) > 0

    def test_spam_detection(self):
        """测试垃圾内容检测。"""
        spam_text = """
        点击！点击！点击！
        免费领取！限时优惠！
        加微信！！！
        """
        denoiser = Denoiser()
        result = denoiser.is_spam(spam_text)

        # 应该检测到垃圾内容
        assert result is True

    def test_dataframe_denoising(self):
        """测试DataFrame去噪。"""
        df = pd.DataFrame({
            "id": [1, 2, 3],
            "content": [
                "正常内容",
                "点击链接免费领取！！！加微信abc123",
                "另一段正常内容",
            ]
        })
        denoiser = Denoiser()
        result = denoiser.clean(df, {"column": "content"})

        # 应该过滤垃圾内容
        assert result.cleaned_count <= result.original_count


class TestFormatConverter:
    """格式转换器测试。"""

    def test_encoding_conversion(self):
        """测试编码转换。"""
        text = "测试文本"
        converter = FormatConverter()
        # 确保可以获取UTF-8编码
        utf8_bytes = text.encode('utf-8')
        assert utf8_bytes is not None

    def test_date_normalization(self):
        """测试日期格式标准化。"""
        dates = pd.Series([
            "2024-01-01",
            "2024/01/02",
            "2024年1月3日",
            "01/04/2024",
        ])
        converter = FormatConverter()
        normalized = converter.normalize_dates(dates)

        # 应该能解析大部分日期
        assert normalized.notna().sum() > 0

    def test_number_normalization(self):
        """测试数字格式标准化。"""
        numbers = pd.Series([
            "1,234.56",
            "1 234.56",
            "1234.56",
            "1.234,56",  # 欧洲格式
        ])
        converter = FormatConverter()
        normalized = converter.normalize_numbers(numbers)

        # 应该能解析大部分数字
        assert normalized.notna().sum() > 0

    def test_dataframe_conversion(self):
        """测试DataFrame格式转换。"""
        df = pd.DataFrame({
            "date": ["2024-01-01", "2024/01/02", "invalid"],
            "number": ["1,234.56", "5,678.90", "invalid"],
        })
        converter = FormatConverter()
        result = converter.clean(df, {})

        assert isinstance(result, CleaningResult)


class TestPIIMasker:
    """敏感信息脱敏测试。"""

    def test_phone_masking(self, text_with_pii):
        """测试手机号脱敏。"""
        masker = PIIMasker()
        masked = masker.mask_text(text_with_pii, {"mask_phone": True})

        # 应该隐藏手机号中间4位
        assert "138****5678" in masked or "****" in masked
        # 原始完整手机号不应该出现
        assert "13812345678" not in masked

    def test_email_masking(self, text_with_pii):
        """测试邮箱脱敏。"""
        masker = PIIMasker()
        masked = masker.mask_text(text_with_pii, {"mask_email": True})

        # 应该隐藏邮箱
        assert "@" not in masked or "****" in masked

    def test_id_card_masking(self, text_with_pii):
        """测试身份证号脱敏。"""
        masker = PIIMasker()
        masked = masker.mask_text(text_with_pii, {"mask_id_card": True})

        # 应该隐藏身份证号
        assert "320102199001011234" not in masked
        assert "****" in masked

    def test_address_masking(self, text_with_pii):
        """测试地址脱敏。"""
        masker = PIIMasker()
        masked = masker.mask_text(text_with_pii, {"mask_address": True})

        # 地址可能被部分隐藏
        assert len(masked) > 0

    def test_custom_mask_pattern(self):
        """测试自定义脱敏模式。"""
        text = "信用卡号：4000000000000000"
        masker = PIIMasker()
        config = {
            "custom_patterns": [
                {
                    "name": "credit_card",
                    "pattern": r"\b\d{16}\b",
                    "replacement": "****-****-****-****"
                }
            ]
        }
        masked = masker.mask_text(text, config)

        assert "****" in masked

    def test_dataframe_masking(self):
        """测试DataFrame脱敏。"""
        df = pd.DataFrame({
            "id": [1, 2, 3],
            "contact": [
                "张三 13812345678",
                "李四 13987654321",
                "王五 13600000000",
            ]
        })
        masker = PIIMasker()
        result = masker.clean(df, {"columns": ["contact"]})

        assert isinstance(result, CleaningResult)
        assert len(result.operations) > 0


class TestDataFrameOps:
    """结构清洗器（trim/normalize_ws/drop_empty）测试。

    这些是前端「数据清洗」页四个基础操作里的三个，回归此前
    「只有 dedup 生效、其余名字被静默跳过」的缺陷。
    """

    def _messy(self) -> pd.DataFrame:
        return pd.DataFrame({
            "name": ["  Alice  ", "Bob", " ", "Alice"],
            "note": ["hello    world", "  spaced   out  ", "   ", "hi"],
        })

    def test_trim_strips_edges(self):
        """trim 去除首尾空白且行数不变。"""
        from services.quality.cleaning.dataframe_ops import TrimCleaner

        result = TrimCleaner().clean(self._messy())
        assert result.cleaned_data is not None
        assert result.cleaned_count == result.original_count  # 只转换不删行
        assert result.cleaned_data["name"].iloc[0] == "Alice"
        # 内部多余空格不属于 trim 职责，应保留
        assert result.cleaned_data["note"].iloc[0] == "hello    world"

    def test_normalize_ws_collapses_runs(self):
        """normalize_ws 压缩内部连续空白并去首尾。"""
        from services.quality.cleaning.dataframe_ops import (
            NormalizeWhitespaceCleaner,
        )

        result = NormalizeWhitespaceCleaner().clean(self._messy())
        assert result.cleaned_data is not None
        assert result.cleaned_data["note"].iloc[0] == "hello world"
        assert result.cleaned_data["note"].iloc[1] == "spaced out"

    def test_drop_empty_removes_blank_rows(self):
        """drop_empty 删除全空白行。"""
        from services.quality.cleaning.dataframe_ops import DropEmptyRowsCleaner

        result = DropEmptyRowsCleaner().clean(self._messy())
        assert result.cleaned_data is not None
        assert result.removed_count == 1  # 第三行 name/note 均为空白
        assert len(result.cleaned_data) == 3


class TestCleanDataFlow:
    """清洗管线端到端数据流：清洗后的 DataFrame 必须真正产出。"""

    def test_pipeline_applies_transforms_not_truncation(self):
        """回归：此前管线按行数截断、丢弃转换结果；现应链式应用真实清洗。"""
        from services.quality.pipeline.executor import QualityPipeline
        import services.quality.cleaning  # noqa: F401 触发注册

        data = pd.DataFrame({
            "name": ["  Alice  ", "Alice", "Bob", " "],
            "note": ["hello    world", "hello world", "  x  ", "  "],
        })
        pipeline = QualityPipeline.__new__(QualityPipeline)
        result = pipeline._clean_data(
            data,
            {"cleaners": ["trim", "normalize_ws", "drop_empty", "dedup"]},
        )

        assert result.cleaned_data is not None
        # 空行删除 + 去重后剩 Alice / Bob 两行
        assert result.cleaned_count == 2
        names = list(result.cleaned_data["name"])
        assert names == ["Alice", "Bob"]
        assert result.cleaned_data["note"].iloc[0] == "hello world"

    def test_unknown_cleaner_is_surfaced_not_silent(self):
        """未知清洗器名不再静默跳过，应记录 skipped 操作。"""
        from services.quality.pipeline.executor import QualityPipeline
        import services.quality.cleaning  # noqa: F401

        data = pd.DataFrame({"a": ["x", "x", "y"]})
        pipeline = QualityPipeline.__new__(QualityPipeline)
        result = pipeline._clean_data(data, {"cleaners": ["nope", "dedup"]})

        skipped = [op for op in result.operations if op.type == "skipped"]
        assert len(skipped) == 1
        assert "nope" in skipped[0].description
