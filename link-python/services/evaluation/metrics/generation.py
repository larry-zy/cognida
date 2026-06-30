"""生成评测指标 (ROUGE, BLEU)。"""

import math
import re
from collections import Counter
from typing import List, Sequence

from pydantic import BaseModel


def _normalize(text: str) -> str:
    """标准化文本（小写、去除标点）。"""
    text = text.lower()
    text = re.sub(r"[^\w\s]", " ", text)
    text = re.sub(r"\s+", " ", text).strip()
    return text


def _get_ngrams(tokens: List[str], n: int) -> Counter:
    """获取 n-gram 计数。"""
    if len(tokens) < n:
        return Counter()
    return Counter(tuple(tokens[i:i+n]) for i in range(len(tokens) - n + 1))


def _compute_bleu(
    reference: List[str],
    hypothesis: List[str],
    max_n: int = 4,
) -> tuple[List[float], float]:
    """计算 BLEU 分数。

    Returns:
        (n-gram 精确度列表, 简洁惩罚系数)
    """
    precisions = []

    for n in range(1, max_n + 1):
        ref_ngrams = _get_ngrams(reference, n)
        hyp_ngrams = _get_ngrams(hypothesis, n)

        if not hyp_ngrams:
            precisions.append(0.0)
            continue

        # 计算 matching n-grams
        match_count = sum(min(count, ref_ngrams.get(ngram, 0))
                         for ngram, count in hyp_ngrams.items())
        total_count = sum(hyp_ngrams.values())

        precision = match_count / total_count if total_count > 0 else 0.0
        precisions.append(precision)

    # 计算简洁惩罚
    ref_len = len(reference)
    hyp_len = len(hypothesis)

    if hyp_len > 0 and ref_len > 0:
        bp = 1.0 if hyp_len == ref_len else \
             math.exp(1 - ref_len / hyp_len) if hyp_len < ref_len else 1.0
    else:
        bp = 0.0

    return precisions, bp


def bleu_at_n(
    reference: str,
    hypothesis: str,
    n: int = 4,
) -> float:
    """计算 BLEU-@N 分数。

    Args:
        reference: 参考文本
        hypothesis: 生成文本
        n: n-gram 大小 (1-4)

    Returns:
        BLEU-@N 分数
    """
    ref_tokens = _normalize(reference).split()
    hyp_tokens = _normalize(hypothesis).split()

    if n < 1 or n > 4:
        raise ValueError("n must be between 1 and 4")

    precisions, bp = _compute_bleu(ref_tokens, hyp_tokens, max_n=n)

    if n == 1:
        return precisions[0] * bp

    # 几何平均
    log_precision_sum = sum(math.log(p) if p > 0 else float('-inf')
                           for p in precisions[:n])
    if log_precision_sum == float('-inf'):
        return 0.0

    geo_mean = math.exp(log_precision_sum / n)
    return geo_mean * bp


def rouge_l(reference: str, hypothesis: str) -> float:
    """计算 ROUGE-L 分数 (基于最长公共子序列)。

    Args:
        reference: 参考文本
        hypothesis: 生成文本

    Returns:
        ROUGE-L F1 分数
    """
    def lcs_length(x: List[str], y: List[str]) -> int:
        """计算最长公共子序列长度。"""
        m, n = len(x), len(y)
        dp = [[0] * (n + 1) for _ in range(m + 1)]

        for i in range(1, m + 1):
            for j in range(1, n + 1):
                if x[i - 1] == y[j - 1]:
                    dp[i][j] = dp[i - 1][j - 1] + 1
                else:
                    dp[i][j] = max(dp[i - 1][j], dp[i][j - 1])

        return dp[m][n]

    ref_tokens = _normalize(reference).split()
    hyp_tokens = _normalize(hypothesis).split()

    if not ref_tokens or not hyp_tokens:
        return 0.0

    lcs = lcs_length(ref_tokens, hyp_tokens)

    precision = lcs / len(hyp_tokens) if hyp_tokens else 0.0
    recall = lcs / len(ref_tokens) if ref_tokens else 0.0

    if precision + recall == 0:
        return 0.0

    f1 = 2 * precision * recall / (precision + recall)
    return f1


def rouge_n(reference: str, hypothesis: str, n: int = 2) -> float:
    """计算 ROUGE-N 分数 (基于 n-gram recall)。

    Args:
        reference: 参考文本
        hypothesis: 生成文本
        n: n-gram 大小

    Returns:
        ROUGE-N F1 分数
    """
    ref_tokens = _normalize(reference).split()
    hyp_tokens = _normalize(hypothesis).split()

    ref_ngrams = _get_ngrams(ref_tokens, n)
    hyp_ngrams = _get_ngrams(hyp_tokens, n)

    if not ref_ngrams or not hyp_ngrams:
        return 0.0

    # 计算 overlap
    overlap = sum(min(count, ref_ngrams.get(ngram, 0))
                 for ngram, count in hyp_ngrams.items())

    precision = overlap / len(hyp_ngrams) if hyp_ngrams else 0.0
    recall = overlap / len(ref_ngrams) if ref_ngrams else 0.0

    if precision + recall == 0:
        return 0.0

    f1 = 2 * precision * recall / (precision + recall)
    return f1


class GenerationMetrics(BaseModel):
    """生成评测指标结果。"""

    rouge_1: float = 0.0
    rouge_2: float = 0.0
    rouge_l: float = 0.0
    bleu_1: float = 0.0
    bleu_2: float = 0.0
    bleu_4: float = 0.0

    model_config = {"arbitrary_types_allowed": True}

    @classmethod
    def compute(
        cls,
        reference: str,
        hypothesis: str,
    ) -> "GenerationMetrics":
        """计算生成指标。

        Args:
            reference: 参考文本
            hypothesis: 生成文本

        Returns:
            生成指标结果
        """
        return cls(
            rouge_1=rouge_n(reference, hypothesis, 1),
            rouge_2=rouge_n(reference, hypothesis, 2),
            rouge_l=rouge_l(reference, hypothesis),
            bleu_1=bleu_at_n(reference, hypothesis, 1),
            bleu_2=bleu_at_n(reference, hypothesis, 2),
            bleu_4=bleu_at_n(reference, hypothesis, 4),
        )


# 便捷函数
def compute_generation_metrics(
    reference: str,
    hypothesis: str,
) -> dict:
    """计算生成指标（便捷函数）。"""
    metrics = GenerationMetrics.compute(reference, hypothesis)
    return metrics.model_dump()
