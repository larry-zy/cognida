"""去噪处理器。

去除广告、垃圾内容、模板内容等噪声。"""

import re
from typing import Any

import pandas as pd

from .base import Cleaner
from ..models import CleaningOperation, CleaningResult
from ..registry import register_cleaner


@register_cleaner("denoiser")
class Denoiser(Cleaner):
    """去噪处理器。

    识别和去除广告、垃圾内容、模板内容等噪声。
    """

    cleaner_name = "denoiser"
    description = "广告内容识别和去除、垃圾内容检测、模板内容去除"

    # 广告关键词模式
    AD_PATTERNS = [
        r"点击.{0,5}领取",
        r"扫码.{0,5}关注",
        r"加微信.{0,5}咨询",
        r"限时.{0,5}优惠",
        r"立即.{0,5}购买",
        r"免费.{0,5}试用",
        r"vip.{0,5}会员",
        r"代理.{0,5}加盟",
    ]

    # 垃圾内容模式（重复字符等）- 不额外分组，直接使用
    JUNK_PATTERNS = [
        r"(.)\1{10,}",  # 同一字符重复10次以上
        r"[!！]{5,}",  # 多个感叹号
        r"[?？]{5,}",  # 多个问号
    ]

    # 模板内容模式
    BOILERPLATE_PATTERNS = [
        r"^转载.*来源",
        r"^版权所有",
        r"^免责声明",
        r"^本文版权归",
    ]

    def __init__(self) -> None:
        """初始化去噪处理器。"""
        super().__init__()
        self._ad_pattern = re.compile("|".join(f"({p})" for p in self.AD_PATTERNS), re.IGNORECASE)
        # 不额外包装分组，直接连接模式
        self._junk_pattern = re.compile("|".join(self.JUNK_PATTERNS))
        self._boilerplate_pattern = re.compile(
            "|".join(self.BOILERPLATE_PATTERNS), re.MULTILINE
        )

    def clean(
        self,
        data: pd.DataFrame,
        config: dict[str, Any] | None = None,
    ) -> CleaningResult:
        """去噪处理。

        Args:
            data: 要去噪的数据
            config: 配置参数

        Returns:
            清洗结果
        """
        config = config or {}
        operations: list[CleaningOperation] = []

        text_column = config.get("text_column")
        if not text_column or text_column not in data.columns:
            # 没有指定文本列，返回原数据
            return self._create_result(data, data, operations)

        cleaned_data = data.copy()
        noise_mask = pd.Series(False, index=data.index)

        # 检测广告内容
        if config.get("remove_ads", True):
            ad_mask = data[text_column].apply(
                lambda x: bool(self._ad_pattern.search(str(x)))
            )
            noise_mask |= ad_mask

        # 检测垃圾内容
        if config.get("remove_junk", True):
            junk_mask = data[text_column].apply(
                lambda x: bool(self._junk_pattern.search(str(x)))
            )
            noise_mask |= junk_mask

        # 检测模板内容
        if config.get("remove_boilerplate", False):
            boilerplate_mask = data[text_column].apply(
                lambda x: bool(self._boilerplate_pattern.search(str(x)))
            )
            noise_mask |= boilerplate_mask

        noise_count = noise_mask.sum()

        if noise_count > 0:
            cleaned_data = cleaned_data[~noise_mask].reset_index(drop=True)

            operations.append(
                CleaningOperation(
                    type="remove_noise",
                    field=text_column,
                    count=noise_count,
                    description=f"移除了 {noise_count} 条噪声记录",
                )
            )

        return self._create_result(data, cleaned_data, operations)

    def detect_ad(self, text: str) -> bool:
        """检测是否为广告内容。

        Args:
            text: 文本内容

        Returns:
            是否包含广告
        """
        return bool(self._ad_pattern.search(text))

    def detect_junk(self, text: str) -> bool:
        """检测是否为垃圾内容。

        Args:
            text: 文本内容

        Returns:
            是否为垃圾内容
        """
        return bool(self._junk_pattern.search(text))

    def detect_boilerplate(self, text: str) -> bool:
        """检测是否包含模板内容。

        Args:
            text: 文本内容

        Returns:
            是否包含模板内容
        """
        return bool(self._boilerplate_pattern.search(text))

    def remove_ad_text(self, text: str) -> str:
        """从文本中移除广告内容。

        Args:
            text: 文本内容

        Returns:
            移除广告后的文本
        """
        return self._ad_pattern.sub("", text)

    def remove_junk_chars(self, text: str) -> str:
        """从文本中移除垃圾字符。

        Args:
            text: 文本内容

        Returns:
            移除垃圾字符后的文本
        """
        return self._junk_pattern.sub("", text)

    def clean_text(self, text: str, config: dict[str, Any] | None = None) -> str:
        """清洗文本（便捷方法）。

        Args:
            text: 文本内容
            config: 配置参数

        Returns:
            清洗后的文本
        """
        config = config or {}
        result = text

        # 移除广告内容
        if config.get("remove_ads", True):
            result = self.remove_ad_text(result)

        # 移除垃圾内容
        if config.get("remove_junk", True):
            result = self.remove_junk_chars(result)

        # 移除模板内容
        if config.get("remove_templates", False):
            result = self._boilerplate_pattern.sub("", result)

        return result

    def is_spam(self, text: str) -> bool:
        """检测是否为垃圾内容。

        Args:
            text: 文本内容

        Returns:
            是否包含垃圾内容
        """
        return self.detect_junk(text) or self.detect_ad(text)
