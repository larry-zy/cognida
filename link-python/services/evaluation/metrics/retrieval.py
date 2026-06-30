"""检索评测指标。"""

import math
from typing import List
from collections.abc import Sequence


def precision_at_k(retrieved: Sequence[bool], k: int) -> float:
    """计算 Precision@K。

    Args:
        retrieved: 布尔列表，True 表示检索到相关文档
        k: 前k个结果

    Returns:
        Precision@K 分数
    """
    if k == 0:
        return 0.0
    relevant_at_k = sum(1 for i, is_relevant in enumerate(retrieved[:k]) if is_relevant)
    return relevant_at_k / min(k, len(retrieved))


def recall_at_k(retrieved: Sequence[bool], total_relevant: int, k: int) -> float:
    """计算 Recall@K。

    Args:
        retrieved: 布尔列表，True 表示检索到相关文档
        total_relevant: 总相关文档数
        k: 前k个结果

    Returns:
        Recall@K 分数
    """
    if total_relevant == 0:
        return 0.0
    relevant_at_k = sum(1 for i, is_relevant in enumerate(retrieved[:k]) if is_relevant)
    return relevant_at_k / total_relevant


def ndcg_at_k(retrieved: Sequence[bool | float], k: int) -> float:
    """计算 NDCG@K (Normalized Discounted Cumulative Gain)。

    Args:
        retrieved: 分数列表，可以是布尔值（1/0）或实数相关性分数
        k: 前k个结果

    Returns:
        NDCG@K 分数
    """
    if k == 0 or not retrieved:
        return 0.0

    def dcg(rels: Sequence[float | bool]) -> float:
        """计算 DCG。"""
        return sum(
            (float(r) if r else 0) / math.log2(i + 2)
            for i, r in enumerate(rels[:k])
        )

    # 计算 DCG
    dcg_score = dcg(retrieved)

    # 计算 IDCG (理想情况下的 DCG)
    ideal_rels = sorted([float(r) if r else 0 for r in retrieved], reverse=True)
    idcg_score = dcg(ideal_rels)

    if idcg_score == 0:
        return 0.0
    return dcg_score / idcg_score


def mrr(retrieved: Sequence[bool]) -> float:
    """计算 MRR (Mean Reciprocal Rank)。

    Args:
        retrieved: 布尔列表，True 表示检索到相关文档

    Returns:
        MRR 分数
    """
    for i, is_relevant in enumerate(retrieved):
        if is_relevant:
            return 1.0 / (i + 1)
    return 0.0


def ap_at_k(retrieved: Sequence[bool], k: int) -> float:
    """计算 AP@K (Average Precision at K)。

    Args:
        retrieved: 布尔列表，True 表示检索到相关文档
        k: 前k个结果

    Returns:
        AP@K 分数
    """
    if k == 0:
        return 0.0

    retrieved_at_k = retrieved[:k]
    if not any(retrieved_at_k):
        return 0.0

    precision_sum = 0.0
    relevant_count = 0

    for i, is_relevant in enumerate(retrieved_at_k, 1):
        if is_relevant:
            relevant_count += 1
            precision_sum += relevant_count / i

    return precision_sum / min(sum(retrieved_at_k), k)


def map_at_k(query_results: List[Sequence[bool]], k: int) -> float:
    """计算 MAP@K (Mean Average Precision at K)。

    Args:
        query_results: 多个查询的检索结果列表
        k: 前k个结果

    Returns:
        MAP@K 分数
    """
    if not query_results:
        return 0.0
    return sum(ap_at_k(r, k) for r in query_results) / len(query_results)


from pydantic import BaseModel


class RetrievalMetrics(BaseModel):
    """检索评测指标结果。"""

    precision: float
    recall: float
    ndcg: float
    mrr: float
    map_score: float

    model_config = {"arbitrary_types_allowed": True}

    @classmethod
    def compute(
        cls,
        retrieved: Sequence[bool | float],
        total_relevant: int | None = None,
        k: int = 10,
    ) -> "RetrievalMetrics":
        """计算检索指标。

        Args:
            retrieved: 检索结果相关性列表
            total_relevant: 总相关文档数（用于计算 Recall）
            k: 计算前k个结果

        Returns:
            检索指标结果
        """
        # 转换为布尔列表
        retrieved_bool = [bool(r) for r in retrieved]

        return cls(
            precision=precision_at_k(retrieved_bool, k),
            recall=recall_at_k(
                retrieved_bool,
                total_relevant if total_relevant is not None else sum(retrieved_bool),
                k
            ),
            ndcg=ndcg_at_k(retrieved, k),
            mrr=mrr(retrieved_bool),
            map_score=ap_at_k(retrieved_bool, k),
        )


# 便捷函数
def compute_retrieval_metrics(
    retrieved: Sequence[bool | float],
    total_relevant: int | None = None,
    k: int = 10,
) -> dict:
    """计算检索指标（便捷函数）。"""
    metrics = RetrievalMetrics.compute(retrieved, total_relevant, k)
    return metrics.model_dump()
