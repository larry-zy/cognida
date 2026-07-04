"""compute-metrics 端点（供 Go Worker 调用）集成测试。

覆盖：检索指标不 500、聚合字段齐全、RAG 专属指标（忠实度/上下文相关性/噪声比）
在传入 retrieved_contexts + 对应 grader 时被计算并聚合。
"""

from fastapi.testclient import TestClient

from services.evaluation.fastapi_app import app

client = TestClient(app)

ENDPOINT = "/api/v1/evaluation/compute-metrics"


def _payload(graders):
    return {
        "items": [
            {
                "question": "谁提出了相对论",
                "reference_answer": "爱因斯坦提出了相对论",
                "generated_answer": "相对论是爱因斯坦提出的",
                "retrieved_pids": ["c1", "c9"],
                "relevant_pids": ["c1"],
                "retrieved_contexts": [
                    "爱因斯坦 于 1905 年 提出 相对论",
                    "一段 无关 的 检索 内容",
                ],
            },
            {
                "question": "水的化学式是什么",
                "reference_answer": "水的化学式是 H2O",
                "generated_answer": "H2O",
                "retrieved_pids": ["c3", "c7"],
                "relevant_pids": ["c3"],
                "retrieved_contexts": ["水 的 化学式 是 H2O"],
            },
        ],
        "graders": graders,
    }


def test_retrieval_metrics_no_500_and_aggregated():
    resp = client.post(ENDPOINT, json=_payload(["precision", "recall", "ndcg", "mrr", "map"]))
    assert resp.status_code == 200, resp.text
    agg = resp.json()["aggregate"]
    for key in ("precision", "recall", "ndcg", "mrr", "map"):
        assert key in agg, f"聚合缺少检索指标 {key}"
        assert 0.0 <= agg[key] <= 1.0
    # 每条 2 取 1 相关 → precision 0.5；两条 recall 均为 1.0 → 0.5 / 1.0
    assert abs(agg["precision"] - 0.5) < 1e-6
    assert abs(agg["recall"] - 1.0) < 1e-6


def test_rag_specific_metrics_computed_when_requested():
    graders = ["faithfulness", "context_relevance", "noise_ratio"]
    resp = client.post(ENDPOINT, json=_payload(graders))
    assert resp.status_code == 200, resp.text
    agg = resp.json()["aggregate"]

    assert "faithfulness" in agg
    assert "context_relevance" in agg
    assert "noise_ratio" in agg
    for key in graders:
        assert 0.0 <= agg[key] <= 1.0
    # 第一条检索 2 命中 1 噪声，第二条检索 2 命中 1 噪声 → 噪声比 0.5
    assert abs(agg["noise_ratio"] - 0.5) < 1e-6


def test_rag_specific_absent_when_grader_not_selected():
    """未选择 RAG 专属 grader 时，聚合不应包含这些字段。"""
    resp = client.post(ENDPOINT, json=_payload(["rouge"]))
    assert resp.status_code == 200, resp.text
    agg = resp.json()["aggregate"]
    assert "faithfulness" not in agg
    assert "context_relevance" not in agg
    assert "noise_ratio" not in agg


def test_semantic_similarity_computed_per_item_and_aggregated():
    """回归：语义相似度此前因 await 同步函数被静默吞掉，现应逐条产出并聚合。"""
    resp = client.post(ENDPOINT, json=_payload(["semantic_similarity"]))
    assert resp.status_code == 200, resp.text
    body = resp.json()
    agg = body["aggregate"]
    assert "semantic_similarity" in agg, "聚合缺少 semantic_similarity（语义指标疑似又被吞掉）"
    assert 0.0 <= agg["semantic_similarity"] <= 1.0
    # 每条结果都应带上逐条语义相似度
    for item in body["items"]:
        assert item.get("semantic_similarity") is not None


def test_rouge_aggregate_kept_when_score_is_zero():
    """回归：ROUGE 全为 0 时，聚合仍应保留字段（此前 sum>0 会误删）。"""
    payload = {
        "items": [
            {
                "question": "问题",
                "reference_answer": "完全对不上的参考答案",
                "generated_answer": "毫不相干输出",
                "retrieved_pids": [],
                "relevant_pids": [],
                "retrieved_contexts": [],
            }
        ],
        "graders": ["rouge"],
    }
    resp = client.post(ENDPOINT, json=payload)
    assert resp.status_code == 200, resp.text
    agg = resp.json()["aggregate"]
    assert "rouge_1" in agg and agg["rouge_1"] == 0.0
    assert "rouge_l" in agg and agg["rouge_l"] == 0.0
