"""RAG 指标（metrics/rag.py）单元测试。

回归重点：compute_rag_metrics 的检索分支历史上以错误的关键字参数
(relevant_docs=/retrieved_docs=) 调用 compute_retrieval_metrics(逐条布尔序列接口)，
会在任何 retrieval_* 指标下抛 TypeError 导致 /api/evaluate/rag 500。
"""

import pytest

from services.evaluation.metrics.rag import (
    compute_rag_metrics,
    faithfulness,
    context_relevance,
    noise_ratio,
)


def _sample():
    return dict(
        questions=["谁提出相对论", "水的化学式"],
        references=[
            {"answer": "爱因斯坦", "relevant_docs": ["d1", "d2"]},
            {"answer": "H2O", "relevant_docs": ["d3"]},
        ],
        outputs=[
            {"answer": "爱因斯坦提出相对论", "retrieved_docs": ["d1", "d9"]},
            {"answer": "H2O", "retrieved_docs": ["d3", "d7"]},
        ],
    )


def test_retrieval_branch_does_not_crash_and_aggregates():
    """检索分支不再抛异常，并返回 precision/recall/ndcg/mrr/map 全字段。"""
    res = compute_rag_metrics(metrics=["retrieval_precision"], **_sample())
    assert "retrieval" in res
    ret = res["retrieval"]
    for key in ("precision", "recall", "ndcg", "mrr", "map"):
        assert key in ret, f"缺少检索指标 {key}"
        assert 0.0 <= ret[key] <= 1.0

    # q1 命中 1/2、q2 命中 1/1 → 平均召回 0.75；每条 2 取 1 相关 → 平均精确率 0.5
    assert ret["precision"] == pytest.approx(0.5)
    assert ret["recall"] == pytest.approx(0.75)


def test_noise_ratio_from_doc_ids():
    """噪声比例基于文档ID：4 个检索中 2 个不相关 → 0.5。"""
    res = compute_rag_metrics(metrics=["noise_ratio"], **_sample())
    assert res["rag_specific"]["noise_ratio"] == pytest.approx(0.5)


def test_faithfulness_and_context_relevance_present():
    """忠实度/上下文相关性在 [0,1] 范围内且不抛异常。"""
    res = compute_rag_metrics(
        metrics=["faithfulness", "context_relevance"], **_sample()
    )
    rag = res["rag_specific"]
    assert 0.0 <= rag["faithfulness"] <= 1.0
    assert 0.0 <= rag["context_relevance"] <= 1.0


def test_faithfulness_full_overlap():
    """答案词全部出现在上下文中 → 忠实度为 1。"""
    val = faithfulness(
        answers=["爱因斯坦 提出 相对论"],
        retrieved_contexts=[["爱因斯坦 于 1905 年 提出 相对论"]],
    )
    assert val == pytest.approx(1.0)


def test_context_relevance_length_mismatch_raises():
    with pytest.raises(ValueError):
        context_relevance(questions=["a", "b"], retrieved_contexts=[["x"]])


def test_noise_ratio_all_relevant_is_zero():
    val = noise_ratio(
        retrieved_contexts=[["ctx"]],
        relevant_doc_ids=[["d1", "d2"]],
        retrieved_doc_ids=[["d1", "d2"]],
    )
    assert val == pytest.approx(0.0)
