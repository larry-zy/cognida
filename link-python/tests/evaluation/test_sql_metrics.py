"""Text2SQL / SQL 生成评测「结构匹配 + 执行准确率 + 逐条落库」测试。

覆盖两层：
1. metrics/sql.py：归一化/精确匹配/结构组件 F1/执行准确率算子，及 compute_sql_metrics
   聚合口径自洽（聚合 == 参与样本均值），缺金标准/缺结果集的样本不参与统计。
2. compute-metrics 端点：请求 SQL 家族指标时，每条 item.scores 带上该样本分值，
   聚合平铺进 aggregate。
"""

from fastapi.testclient import TestClient

from services.evaluation.fastapi_app import app
from services.evaluation.metrics.sql import (
    compute_sql_metrics,
    normalize_sql,
    sql_component_match,
    sql_exact_match,
    sql_execution_accuracy,
)

client = TestClient(app)
ENDPOINT = "/api/v1/evaluation/compute-metrics"


# ============================================================
# 单元层：算子行为
# ============================================================

def test_normalize_sql_ignores_case_whitespace_quotes_semicolon():
    a = normalize_sql("SELECT `a`,  b  FROM  t ;")
    b = normalize_sql('select a, b from t')
    assert a == b == "select a, b from t"


def test_exact_match_equivalent_writings():
    assert sql_exact_match("SELECT a FROM t;", "select a from t") == 1.0
    assert sql_exact_match("SELECT a FROM t", "SELECT b FROM t") == 0.0


def test_component_match_tolerates_spacing_but_penalizes_wrong_table():
    # 仅空白/大小写差异 → 结构组件完全一致
    assert sql_component_match(
        "SELECT a FROM t WHERE x=1", "SELECT a FROM t WHERE x = 1"
    ) == 1.0
    # 错表 → F1 < 1
    assert sql_component_match(
        "SELECT a FROM orders", "SELECT a FROM customers"
    ) < 1.0


def test_execution_accuracy_order_insensitive_and_numeric_normalized():
    # 行顺序不同但多重集合相等 → 1.0
    assert sql_execution_accuracy([[1, "a"], [2, "b"]], [[2, "b"], [1, "a"]]) == 1.0
    # 12 与 12.0 视为相等
    assert sql_execution_accuracy([[12]], [[12.0]]) == 1.0
    # 值不同 → 0.0
    assert sql_execution_accuracy([[1]], [[2]]) == 0.0
    # 都空 → 1.0（无满足条件行的正确答案）
    assert sql_execution_accuracy([], []) == 1.0


def test_execution_accuracy_asymmetric_on_missing_result_set():
    # 金标准结果集缺失（未执行/执行失败 → 无正确性基准）→ None，剔除出分母
    assert sql_execution_accuracy(None, [[1]]) is None
    # 金标准执行成功、但生成结果集缺失（生成 SQL 为空/执行失败）→ 0.0，答案错误，计入分母
    assert sql_execution_accuracy([[1]], None) == 0.0
    # 两侧都缺失 → 仍按金标准缺失剔除
    assert sql_execution_accuracy(None, None) is None


def test_compute_sql_metrics_aggregate_equals_mean_and_skips_unevaluable():
    references = [
        {"gold_sql": "SELECT a FROM t", "gold_result_set": [[1]]},
        {"gold_sql": "SELECT b FROM t", "gold_result_set": None},  # 无金标准结果集
        {"gold_sql": "", "gold_result_set": [[9]]},                # 无金标准 SQL
    ]
    outputs = [
        {"generated_sql": "select a from t", "result_set": [[1]]},
        {"generated_sql": "SELECT b FROM t", "result_set": [[2]]},
        {"generated_sql": "SELECT x FROM t", "result_set": [[9]]},
    ]
    res = compute_sql_metrics(references, outputs, return_items=True)
    items = res["_items"]
    assert len(items) == 3

    # 精确匹配：仅前两条有金标准 SQL 参与（第三条 gold_sql 空 → 缺席）
    assert "sql_exact_match" not in items[2]
    em_vals = [items[0]["sql_exact_match"], items[1]["sql_exact_match"]]
    assert abs(res["sql_exact_match"] - sum(em_vals) / len(em_vals)) < 1e-9

    # 执行准确率：第二条缺金标准结果集 → 缺席，仅第 0/2 条参与
    assert "sql_execution_accuracy" not in items[1]
    ea_vals = [items[0]["sql_execution_accuracy"], items[2]["sql_execution_accuracy"]]
    assert abs(res["sql_execution_accuracy"] - sum(ea_vals) / len(ea_vals)) < 1e-9


def test_compute_sql_metrics_default_no_items_key():
    res = compute_sql_metrics(
        [{"gold_sql": "SELECT a FROM t"}], [{"generated_sql": "SELECT a FROM t"}]
    )
    assert "_items" not in res
    assert res["sql_exact_match"] == 1.0


# ============================================================
# 端点层：SQL 指标逐条落进 item.scores，聚合平铺
# ============================================================

def _sql_payload():
    return {
        "items": [
            {
                "question": "一月订单总额",
                "reference_answer": "",
                "generated_answer": "",
                "gold_sql": "SELECT SUM(amount) FROM orders WHERE month=1",
                "generated_sql": "select sum(amount) from orders where month=1",
                "gold_result_set": [[1000]],
                "result_set": [[1000]],
            },
            {
                "question": "客户数",
                "reference_answer": "",
                "generated_answer": "",
                "gold_sql": "SELECT COUNT(*) FROM customers",
                "generated_sql": "SELECT COUNT(id) FROM users",
                "gold_result_set": [[50]],
                "result_set": [[70]],
            },
        ],
        "graders": ["sql_exact_match", "sql_component_match", "sql_execution_accuracy"],
    }


def test_sql_metrics_per_item_scores_and_aggregate():
    resp = client.post(ENDPOINT, json=_sql_payload())
    assert resp.status_code == 200, resp.text
    body = resp.json()
    agg = body["aggregate"]
    items = body["items"]

    for key in ("sql_exact_match", "sql_component_match", "sql_execution_accuracy"):
        assert key in agg, f"聚合缺 {key}"

    assert len(items) == 2
    for it in items:
        for key in ("sql_exact_match", "sql_component_match", "sql_execution_accuracy"):
            assert key in it["scores"], f"逐条 scores 缺 {key}"

    # 第一条：归一化后精确相等、结果集一致 → 全 1
    assert items[0]["scores"]["sql_exact_match"] == 1.0
    assert items[0]["scores"]["sql_execution_accuracy"] == 1.0
    # 第二条：错表、结果集不同 → 精确匹配与执行准确率为 0
    assert items[1]["scores"]["sql_exact_match"] == 0.0
    assert items[1]["scores"]["sql_execution_accuracy"] == 0.0
