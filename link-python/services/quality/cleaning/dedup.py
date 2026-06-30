"""去重处理器。

处理精确重复和模糊去重。
"""

from typing import Any

import pandas as pd
from datasketch import MinHash

from .base import Cleaner
from ..models import CleaningOperation, CleaningResult
from ..registry import register_cleaner


@register_cleaner("dedup")
class DedupProcessor(Cleaner):
    """去重处理器。

    支持精确去重和基于相似度的模糊去重。
    """

    cleaner_name = "dedup"
    description = "精确去重和模糊去重"

    def __init__(self) -> None:
        """初始化去重处理器。"""
        super().__init__()

    def clean(
        self,
        data: pd.DataFrame,
        config: dict[str, Any] | None = None,
    ) -> CleaningResult:
        """去重处理。

        Args:
            data: 要去重的数据
            config: 配置参数

        Returns:
            清洗结果
        """
        config = config or {}
        operations: list[CleaningOperation] = []

        original_count = len(data)
        cleaned_data = data.copy()

        # 获取方法参数（兼容不同的参数名）
        method = config.get("method", "exact")
        if method == "fuzzy":
            fuzzy_enabled = True
        elif method == "minhash":
            fuzzy_enabled = True
        else:
            fuzzy_enabled = config.get("fuzzy_dedup", False)

        # 获取列名参数（兼容不同的参数名）
        text_column = config.get("column") or config.get("text_column")

        # 精确去重
        subset = config.get("subset") or (text_column if text_column else None)
        if config.get("exact_dedup", True) and method != "fuzzy" and method != "minhash":
            cleaned_data = self._exact_dedup(cleaned_data, subset)
            duplicates_removed = original_count - len(cleaned_data)

            if duplicates_removed > 0:
                operations.append(
                    CleaningOperation(
                        type="exact_dedup",
                        field=subset,
                        count=duplicates_removed,
                        description=f"精确去重移除了 {duplicates_removed} 条重复记录",
                    )
                )

        # 模糊去重
        if fuzzy_enabled and text_column and text_column in cleaned_data.columns:
            similarity_threshold = config.get("similarity_threshold", config.get("threshold", 0.85))
            cleaned_data, fuzzy_count = self._fuzzy_dedup(
                cleaned_data, text_column, similarity_threshold
            )

            if fuzzy_count > 0:
                operations.append(
                    CleaningOperation(
                        type="fuzzy_dedup",
                        field=text_column,
                        count=fuzzy_count,
                        description=f"模糊去重移除了 {fuzzy_count} 条近似重复记录",
                    )
                )

        return self._create_result(data, cleaned_data, operations)

    def _exact_dedup(
        self, data: pd.DataFrame, subset: list[str] | None = None
    ) -> pd.DataFrame:
        """精确去重。

        Args:
            data: 数据
            subset: 用于去重的列名列表

        Returns:
            去重后的数据
        """
        if subset:
            return data.drop_duplicates(subset=subset, keep="first")
        return data.drop_duplicates(keep="first")

    def _fuzzy_dedup(
        self, data: pd.DataFrame, text_column: str, threshold: float = 0.85
    ) -> tuple[pd.DataFrame, int]:
        """模糊去重。

        Args:
            data: 数据
            text_column: 文本列名
            threshold: 相似度阈值

        Returns:
            (去重后的数据, 移除的记录数)
        """
        # 检测文本长度，对于短文本使用直接Jaccard相似度
        text_lengths = data[text_column].str.len()
        avg_length = text_lengths.mean()

        # 对于短文本（< 50字符），使用直接字符集Jaccard相似度
        if avg_length < 50:
            return self._fuzzy_dedup_jaccard(data, text_column, threshold)
        else:
            return self._fuzzy_dedup_minhash(data, text_column, threshold)

    def _fuzzy_dedup_jaccard(
        self, data: pd.DataFrame, text_column: str, threshold: float
    ) -> tuple[pd.DataFrame, int]:
        """使用直接Jaccard相似度进行模糊去重（适合短文本）。

        Args:
            data: 数据
            text_column: 文本列名
            threshold: 相似度阈值

        Returns:
            (去重后的数据, 移除的记录数)
        """
        # 检测是否包含中文
        has_chinese = data[text_column].str.contains(r'[一-鿿]', regex=True).any()

        to_remove = set()

        for i in range(len(data)):
            if i in to_remove:
                continue

            text1 = str(data[text_column].iloc[i]).lower()

            for j in range(i + 1, len(data)):
                if j in to_remove:
                    continue

                text2 = str(data[text_column].iloc[j]).lower()

                # 计算多种相似度并取最大值
                similarities = []

                # 1. 字符级 Jaccard
                if has_chinese:
                    # 中文：字符 + 2-gram + 3-gram
                    tokens1 = set(text1)
                    for k in range(len(text1) - 1):
                        tokens1.add(text1[k:k+2])
                    for k in range(len(text1) - 2):
                        tokens1.add(text1[k:k+3])

                    tokens2 = set(text2)
                    for k in range(len(text2) - 1):
                        tokens2.add(text2[k:k+2])
                    for k in range(len(text2) - 2):
                        tokens2.add(text2[k:k+3])
                else:
                    tokens1 = set(text1.split())
                    tokens2 = set(text2.split())

                if tokens1 or tokens2:
                    intersection = tokens1 & tokens2
                    union = tokens1 | tokens2
                    similarities.append(len(intersection) / len(union) if union else 0)

                # 2. 最长公共前缀比例
                common_prefix_len = 0
                for k in range(min(len(text1), len(text2))):
                    if text1[k] == text2[k]:
                        common_prefix_len += 1
                    else:
                        break
                lcs_similarity = common_prefix_len / max(len(text1), len(text2)) if text1 or text2 else 0
                similarities.append(lcs_similarity)

                # 3. 简单字符重叠率
                chars1 = set(text1)
                chars2 = set(text2)
                char_overlap = len(chars1 & chars2) / len(chars1 | chars2) if (chars1 | chars2) else 0
                similarities.append(char_overlap)

                # 取最大相似度
                similarity = max(similarities) if similarities else 0

                if similarity >= threshold:
                    to_remove.add(j)

        cleaned_data = data.drop(index=list(to_remove))
        return cleaned_data, len(to_remove)

    def _fuzzy_dedup_minhash(
        self, data: pd.DataFrame, text_column: str, threshold: float
    ) -> tuple[pd.DataFrame, int]:
        """使用 MinHash 进行模糊去重（适合长文本）。

        Args:
            data: 数据
            text_column: 文本列名
            threshold: 相似度阈值

        Returns:
            (去重后的数据, 移除的记录数)
        """
        # 计算每条记录的 MinHash
        minhashes = []
        for text in data[text_column]:
            minhashes.append(self._create_minhash(str(text)))

        # 找出重复组
        to_remove = set()
        for i in range(len(minhashes)):
            if i in to_remove:
                continue

            for j in range(i + 1, len(minhashes)):
                if j in to_remove:
                    continue

                # 计算 Jaccard 相似度
                similarity = minhashes[i].jaccard(minhashes[j])

                if similarity >= threshold:
                    # 标记 j 为重复
                    to_remove.add(j)

        # 创建去重后的数据
        cleaned_data = data.drop(index=list(to_remove))

        return cleaned_data, len(to_remove)

    def _create_minhash(self, text: str, num_perm: int = 128) -> MinHash:
        """创建 MinHash 签名。

        Args:
            text: 文本内容
            num_perm: 排列数

        Returns:
            MinHash 对象
        """
        from datasketch import MinHash

        # 检测是否包含中文
        has_chinese = any('一' <= char <= '鿿' for char in text)

        if has_chinese:
            # 中文使用字符级 shingles（每个字符作为一个 token）
            # 对于短文本，使用较少的排列数
            minhash = MinHash(num_perm=64)
            text_lower = text.lower()
            # 使用单个字符 + 2-gram 组合
            for char in text_lower:
                minhash.update(char.encode("utf-8"))
            for i in range(len(text_lower) - 1):
                bigram = text_lower[i:i+2]
                minhash.update(bigram.encode("utf-8"))
        else:
            # 英文使用单词分词
            minhash = MinHash(num_perm=num_perm)
            words = text.lower().split()
            for word in words:
                minhash.update(word.encode("utf-8"))

        return minhash
