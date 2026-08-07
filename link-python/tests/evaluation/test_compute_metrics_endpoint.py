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


def test_zero_recall_sample_counted_not_dropped():
    """#5 回归：有相关标注但检索结果为空的零召回样本应计入分母（recall=0），而非被静默剔除。

    构造两条：一条正常命中（recall=1），一条 retrieved_pids 为空（零召回 recall=0）。
    若零召回样本被丢弃，平均召回会虚高为 1.0；正确应为 (1.0 + 0.0) / 2 = 0.5。
    """
    payload = {
        "items": [
            {
                "question": "q1",
                "reference_answer": "r1",
                "generated_answer": "g1",
                "retrieved_pids": ["c1"],
                "relevant_pids": ["c1"],
            },
            {
                "question": "q2",
                "reference_answer": "r2",
                "generated_answer": "g2",
                "retrieved_pids": [],  # 什么都没检索到 → 零召回，仍应计入
                "relevant_pids": ["c3"],
            },
        ],
        "graders": ["recall"],
    }
    resp = client.post(ENDPOINT, json=payload)
    assert resp.status_code == 200, resp.text
    agg = resp.json()["aggregate"]
    assert "recall" in agg
    assert abs(agg["recall"] - 0.5) < 1e-6, f"零召回样本疑似被剔除，recall={agg['recall']}（应为 0.5）"


def test_sql_execution_accuracy_excludes_unexecuted_sample():
    """#1 回归：未执行/执行失败（结果集缺省 None）的样本必须剔除出执行准确率分母，
    不能被当成「空==空」误判为满分 1.0。

    第一条：金标准与生成结果集都执行成功且相等（[[1]] == [[1]]）→ 1.0。
    第二条：不带 result_set/gold_result_set（→ None）→ 应剔除，不计 1.0。
    正确聚合应只由第一条决定 = 1.0；若第二条被误判满分，聚合仍为 1.0——因此再补
    一条「执行成功但两侧不等（0.0）」的样本拉开区分：正确为 (1.0+0.0)/2=0.5，
    若未执行样本混入满分则为 (1.0+0.0+1.0)/3≈0.667。
    """
    payload = {
        "items": [
            {  # 执行成功且相等 → 1.0
                "question": "q1",
                "reference_answer": "",
                "generated_answer": "",
                "gold_sql": "SELECT 1",
                "generated_sql": "SELECT 1",
                "gold_result_set": [[1]],
                "result_set": [[1]],
            },
            {  # 未执行（结果集缺省 → None）→ 应剔除出分母
                "question": "q2",
                "reference_answer": "",
                "generated_answer": "",
                "gold_sql": "SELECT 2",
                "generated_sql": "SELECT x FROM broken",
            },
            {  # 执行成功但不等 → 0.0
                "question": "q3",
                "reference_answer": "",
                "generated_answer": "",
                "gold_sql": "SELECT 3",
                "generated_sql": "SELECT 4",
                "gold_result_set": [[3]],
                "result_set": [[4]],
            },
        ],
        "graders": ["sql_execution_accuracy"],
    }
    resp = client.post(ENDPOINT, json=payload)
    assert resp.status_code == 200, resp.text
    body = resp.json()
    agg = body["aggregate"]
    assert "sql_execution_accuracy" in agg
    assert abs(agg["sql_execution_accuracy"] - 0.5) < 1e-6, (
        f"未执行样本疑似被误判满分，agg={agg['sql_execution_accuracy']}（应为 0.5，仅两条已执行样本参与）"
    )
    # 第二条（未执行）逐条不应带 sql_execution_accuracy 键
    assert "sql_execution_accuracy" not in body["items"][1]["scores"]


def test_sql_execution_accuracy_empty_result_sets_are_equal():
    """#1 边界：金标准与生成都「执行成功但零行」([] == []) 是合法正确答案 → 1.0，
    与「未执行」(None) 区分开——两条都空且相等应记满分而非剔除。
    """
    payload = {
        "items": [
            {
                "question": "q1",
                "reference_answer": "",
                "generated_answer": "",
                "gold_sql": "SELECT 1 WHERE 1=0",
                "generated_sql": "SELECT 1 WHERE 1=0",
                "gold_result_set": [],  # 执行成功，零行
                "result_set": [],
            }
        ],
        "graders": ["sql_execution_accuracy"],
    }
    resp = client.post(ENDPOINT, json=payload)
    assert resp.status_code == 200, resp.text
    agg = resp.json()["aggregate"]
    assert abs(agg["sql_execution_accuracy"] - 1.0) < 1e-6, (
        "两侧执行成功且都零行应视为相等（1.0）"
    )


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
