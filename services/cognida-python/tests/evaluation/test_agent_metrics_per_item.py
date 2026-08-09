"""Agent 评测「逐条落库 + 聚合自洽」回归测试。

覆盖两层：
1. metrics/agent.py：新增的 *_items 逐样本算子与其聚合口径自洽（聚合 == 逐条均值），
   无期望标注的样本不参与统计。
2. compute-metrics 端点：请求 Agent 家族指标时，每条 item.scores 带上该样本的
   逐条分值（工具/轨迹/准确率/步骤效率），供前端逐条 debug；聚合仍是逐条均值。
"""

from fastapi.testclient import TestClient

from services.evaluation.fastapi_app import app
import services.evaluation.metrics.agent as agent_mod
from services.evaluation.metrics.agent import (
    _numeric_coverage,
    answer_accuracy,
    answer_accuracy_items,
    compute_agent_metrics,
    step_efficiency,
    step_efficiency_items,
    tool_order,
    tool_order_items,
    tool_selection,
    tool_selection_items,
    trajectory_match,
    trajectory_match_items,
)

client = TestClient(app)
ENDPOINT = "/api/v1/evaluation/compute-metrics"


# ============================================================
# 单元层：*_items 与聚合口径自洽
# ============================================================

def test_answer_accuracy_aggregate_equals_mean_of_items():
    refs = ["爱因斯坦提出了相对论", "水的化学式是 H2O"]
    outs = ["相对论是爱因斯坦提出的", "H2O"]
    items = answer_accuracy_items(refs, outs)
    assert len(items) == 2
    assert abs(answer_accuracy(refs, outs) - sum(items) / len(items)) < 1e-9


def test_tool_selection_skips_unlabeled_and_macro_averages():
    # 第二条无期望工具标注 → None，不参与统计
    expected = [["get_schema", "sql_execute"], []]
    used = [["get_schema", "sql_execute"], ["render_ui"]]
    items = tool_selection_items(expected, used)
    assert items[1] is None
    assert items[0]["f1"] == 1.0
    agg = tool_selection(expected, used)
    # 仅第一条参与 → 宏平均即其本身
    assert abs(agg["f1"] - 1.0) < 1e-9
    assert abs(agg["precision"] - 1.0) < 1e-9


def test_tool_order_ordered_subsequence_and_skip_none():
    expected = [["get_schema", "sql_execute"], ["a", "b"], []]
    used = [["get_schema", "x", "sql_execute"], ["b", "a"], ["z"]]
    items = tool_order_items(expected, used)
    assert items == [1.0, 0.0, None]
    agg = tool_order(expected, used)
    assert abs(agg["ordered_match"] - 0.5) < 1e-9  # 两条参与，1 命中


def test_trajectory_match_aggregate_equals_mean_of_items():
    expected = [["get_schema", "sql_execute"], ["a"]]
    actual = [["get_schema", "sql_execute"], ["b"]]
    items = trajectory_match_items(expected, actual)
    assert items[0]["exact_match"] == 1.0
    assert items[1]["exact_match"] == 0.0
    agg = trajectory_match(expected, actual)
    assert abs(agg["exact_match"] - 0.5) < 1e-9
    assert abs(agg["similarity"] - sum(x["similarity"] for x in items) / 2) < 1e-9


# ============================================================
# A5：中文数量单位（万/亿）纳入数值覆盖率
# ============================================================

def test_numeric_coverage_matches_chinese_units():
    """「6万」应与参考「60,000」命中；「3亿」应与「300000000」命中（A5）。"""
    assert _numeric_coverage("共 60,000 笔订单", "大约 6 万单") == 1.0
    assert _numeric_coverage("销售额 3 亿元", "约 300000000 元") == 1.0
    # 数字部分不因中文单位而被重复计入：1.2万=12000，唯一 token
    assert _numeric_coverage("1.2 万", "12000") == 1.0
    # 数值不符仍应漏配（守卫：单位换算不能把不同量级凑成命中）
    assert _numeric_coverage("60,000 笔", "6 千笔") == 0.0


# ============================================================
# A3：数值覆盖率语义地板（sim 太低时数字命中不得抬分）
# ============================================================

def test_answer_accuracy_numeric_floor_gates_low_semantic(monkeypatch):
    """语义严重跑偏（sim<地板）时，纵然数字全对也不得被 max(cov) 救成高分（A3）。"""
    # 语义低于地板：答案跑偏
    monkeypatch.setattr(agent_mod, "compute_semantic_metrics", lambda r, o: {"similarity": 0.1})
    low = answer_accuracy_items(["订单总数 60000 笔"], ["今天天气不错 60000"])
    assert low[0] == 0.0, "低语义下数字命中不应抬分"

    # 语义过线：数字覆盖率允许抬分
    monkeypatch.setattr(agent_mod, "compute_semantic_metrics", lambda r, o: {"similarity": 0.5})
    high = answer_accuracy_items(["订单总数 60000 笔"], ["共有 60000 笔订单"])
    assert high[0] == 1.0, "语义过线时数字全命中应抬到满分"


# ============================================================
# A2：轨迹匹配比对工具序列而非自然语言步骤
# ============================================================

def test_trajectory_match_compares_tool_sequences_not_nl_steps():
    """expected_steps 为自然语言注释；轨迹匹配应看 tools_used 序列（A2）。"""
    references = [
        {"final_answer": "x", "tools_used": ["get_schema", "sql_execute"],
         "expected_steps": ["理解问题", "调用 sql_execute 查询", "整理回复"]},
    ]
    # 工具序列完全一致 → exact_match=1.0，尽管 expected_steps 是异构的自然语言
    same = compute_agent_metrics(
        references,
        [{"final_answer": "x", "tools_used": ["get_schema", "sql_execute"],
          "trajectory": ["随便什么描述"], "total_steps": 2}],
        metrics=["trajectory_match"],
    )
    assert same["trajectory_match"]["exact_match"] == 1.0

    # 工具序列不同 → exact_match=0.0
    diff = compute_agent_metrics(
        references,
        [{"final_answer": "x", "tools_used": ["render_ui"],
          "trajectory": ["理解问题", "调用 sql_execute 查询", "整理回复"], "total_steps": 3}],
        metrics=["trajectory_match"],
    )
    assert diff["trajectory_match"]["exact_match"] == 0.0


# ============================================================
# A4：无标注样本在 trajectory_match / step_efficiency 中不参与统计
# ============================================================

def test_trajectory_match_skips_unlabeled_samples():
    """期望工具为空的样本返回 None、不进分母（A4）。"""
    items = trajectory_match_items([["a", "b"], []], [["a", "b"], ["z"]])
    assert items[0]["exact_match"] == 1.0
    assert items[1] is None
    agg = trajectory_match([["a", "b"], []], [["a", "b"], ["z"]])
    assert agg["exact_match"] == 1.0  # 仅第一条参与


def test_step_efficiency_skips_unlabeled_samples():
    """optimal<=0（无 expected_steps）的样本返回 None、不进分母（A4，旧实现默认 1 虚构分）。"""
    items = step_efficiency_items([3, 2], [0, 2])
    assert items[0] is None
    assert abs(items[1] - 1.0) < 1e-9
    agg = step_efficiency([3, 2], [0, 2])
    assert abs(agg["optimal_ratio"] - 1.0) < 1e-9  # 仅第二条参与
    assert abs(agg["optimal_steps"] - 2.0) < 1e-9  # 仅统计有标注的最优步数


def test_step_efficiency_symmetric_ratio_and_mean_of_items():
    actual = [4, 2]
    optimal = [2, 2]
    items = step_efficiency_items(actual, optimal)
    assert abs(items[0] - 0.5) < 1e-9   # min(2/4, 4/2)=0.5
    assert abs(items[1] - 1.0) < 1e-9
    agg = step_efficiency(actual, optimal)
    # 聚合恒等于逐条均值（而非旧的先平均步数再取比）
    assert abs(agg["optimal_ratio"] - 0.75) < 1e-9
    assert abs(agg["avg_steps"] - 3.0) < 1e-9
    assert abs(agg["optimal_steps"] - 2.0) < 1e-9


def test_step_efficiency_ratio_of_averages_would_differ():
    """守卫：旧的 ratio-of-averages 与逐条均值在绕路场景下不同，确认已改为后者。"""
    actual = [4, 2]
    optimal = [2, 2]
    # ratio-of-averages: min(2/3, 3/2)=0.666...；逐条均值=0.75。二者必须不同
    ratio_of_avgs = min(2.0 / 3.0, 3.0 / 2.0)
    agg = step_efficiency(actual, optimal)
    assert abs(agg["optimal_ratio"] - ratio_of_avgs) > 1e-3


# ============================================================
# compute_agent_metrics：return_items 逐条产出且键名与聚合平铺一致
# ============================================================

def test_compute_agent_metrics_return_items_shape():
    references = [
        {"final_answer": "爱因斯坦提出了相对论", "tools_used": ["get_schema", "sql_execute"], "expected_steps": ["get_schema", "sql_execute"]},
        {"final_answer": "H2O", "tools_used": ["get_schema"], "expected_steps": ["get_schema"]},
    ]
    outputs = [
        {"final_answer": "相对论是爱因斯坦提出的", "tools_used": ["get_schema", "sql_execute"], "trajectory": ["get_schema", "sql_execute"], "total_steps": 2},
        {"final_answer": "H2O", "tools_used": ["get_schema", "sql_execute", "render_ui"], "trajectory": ["get_schema", "sql_execute", "render_ui"], "total_steps": 3},
    ]
    metrics = ["answer_accuracy", "tool_selection", "tool_order", "trajectory_match", "step_efficiency"]
    result = compute_agent_metrics(references, outputs, metrics=metrics, return_items=True)

    items = result["_items"]
    assert len(items) == 2
    # 逐条键名与聚合平铺后一致
    for it in items:
        for k in ("answer_accuracy", "tool_precision", "tool_recall", "tool_f1",
                  "tool_order", "traj_exact_match", "traj_similarity", "step_optimal_ratio"):
            assert k in it, f"逐条分值缺 {k}"

    # 聚合恒等于逐条均值
    assert abs(result["answer_accuracy"] - sum(x["answer_accuracy"] for x in items) / 2) < 1e-9
    assert abs(result["tool_selection"]["f1"] - sum(x["tool_f1"] for x in items) / 2) < 1e-9
    assert abs(result["step_efficiency"]["optimal_ratio"] - sum(x["step_optimal_ratio"] for x in items) / 2) < 1e-9


def test_compute_agent_metrics_default_no_items_key():
    """默认 return_items=False 不应泄漏 _items（保持旧调用方契约）。"""
    result = compute_agent_metrics(
        [{"final_answer": "a", "tools_used": [], "expected_steps": []}],
        [{"final_answer": "a", "tools_used": [], "trajectory": [], "total_steps": 0}],
        metrics=["answer_accuracy"],
    )
    assert "_items" not in result


# ============================================================
# 端点层：Agent 指标逐条落进 item.scores，聚合仍为逐条均值
# ============================================================

def _agent_payload():
    return {
        "items": [
            {
                "question": "一月渠道客单价",
                "reference_answer": "线上渠道客单价为 328 元",
                "generated_answer": "线上渠道客单价约 328 元",
                "expected_tools": ["get_schema", "sql_execute"],
                "expected_steps": ["get_schema", "sql_execute"],
                "tools_used": ["get_schema", "sql_execute"],
                "trajectory": ["get_schema", "sql_execute"],
                "total_steps": 2,
            },
            {
                "question": "库里有哪些订单相关表",
                "reference_answer": "orders / order_items / order_payments",
                "generated_answer": "有 orders、order_items 等表",
                "expected_tools": ["get_schema"],
                "expected_steps": ["get_schema"],
                "tools_used": ["get_schema", "sql_execute", "render_ui"],
                "trajectory": ["get_schema", "sql_execute", "render_ui"],
                "total_steps": 3,
            },
        ],
        "graders": ["answer_accuracy", "tool_selection", "tool_order", "trajectory_match", "step_efficiency"],
    }


def test_agent_metrics_per_item_scores_and_aggregate_consistency():
    resp = client.post(ENDPOINT, json=_agent_payload())
    assert resp.status_code == 200, resp.text
    body = resp.json()
    agg = body["aggregate"]
    items = body["items"]

    # 聚合仍齐全
    for key in ("answer_accuracy", "tool_precision", "tool_recall", "tool_f1",
                "tool_order", "traj_exact_match", "traj_similarity", "step_optimal_ratio"):
        assert key in agg, f"聚合缺 {key}"

    # 每条 item.scores 带上逐条分值（供前端逐条 debug）
    assert len(items) == 2
    for it in items:
        s = it["scores"]
        for key in ("answer_accuracy", "tool_f1", "tool_order",
                    "traj_exact_match", "step_optimal_ratio"):
            assert key in s, f"逐条 scores 缺 {key}"

    # 聚合 == 逐条均值（自洽）
    mean_f1 = sum(it["scores"]["tool_f1"] for it in items) / 2
    assert abs(agg["tool_f1"] - mean_f1) < 1e-9
    mean_ratio = sum(it["scores"]["step_optimal_ratio"] for it in items) / 2
    assert abs(agg["step_optimal_ratio"] - mean_ratio) < 1e-9

    # 第二条工具过度调用（3 调用 vs 期望 1）→ tool_f1 应低于第一条
    assert items[1]["scores"]["tool_f1"] < items[0]["scores"]["tool_f1"]
