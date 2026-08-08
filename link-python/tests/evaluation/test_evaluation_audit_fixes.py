"""定向对抗审计（wf_8b7c35e5-80d）确认缺陷的回归测试。

本轮修复涵盖计分正确性与分词三类缺陷，此文件锁死修复后的行为：
- Rank 3：工具选择精确率不再被展示类辅助工具（render_ui）惩罚解题正确的 Agent。
- Rank 2：步骤效率的「最优步数」取期望工具调用数，与实际工具调用次数同口径，
  而非旧的 expected_steps 自然语言步骤数（与调用次数负相关）。
- Rank 4：分词保留数字/百分比 token（如 "2024"、"3.14"、"95%"），不再被英文词模式吞掉。
"""

from services.evaluation.metrics.agent import (
    compute_agent_metrics,
    step_efficiency,
    tool_selection,
    tool_selection_items,
)
from services.evaluation.metrics.tokenizer import tokenize, tokenize_english


# ============================================================
# Rank 3：展示类辅助工具（render_ui）不惩罚精确率
# ============================================================

def test_tool_selection_precision_excludes_unexpected_render_ui():
    """期望集里没有 render_ui，Agent 额外调了它渲染结果——精确率不应被拉低。"""
    expected = [["query_sql"]]
    used = [["query_sql", "render_ui"]]
    item = tool_selection_items(expected, used)[0]
    assert item is not None
    # 命中 query_sql；render_ui 属期望之外的展示类工具，被排除出分母 → 精确率满分。
    assert abs(item["precision"] - 1.0) < 1e-6
    assert abs(item["recall"] - 1.0) < 1e-6


def test_tool_selection_render_ui_counts_when_expected():
    """若数据集把 render_ui 标进期望集（确需渲染），则照常计入、不豁免。"""
    expected = [["query_sql", "render_ui"]]
    used = [["query_sql", "render_ui"]]
    item = tool_selection_items(expected, used)[0]
    assert item is not None
    assert abs(item["precision"] - 1.0) < 1e-6
    assert abs(item["recall"] - 1.0) < 1e-6


def test_tool_selection_still_penalizes_wrong_non_auxiliary_tool():
    """调用了非展示类的错误工具，精确率仍应被惩罚（豁免仅限展示类辅助工具）。"""
    expected = [["query_sql"]]
    used = [["query_sql", "delete_table"]]
    item = tool_selection_items(expected, used)[0]
    assert item is not None
    # 2 个调用命中 1 个 → 精确率 0.5。
    assert abs(item["precision"] - 0.5) < 1e-6


def test_tool_selection_aggregate_render_ui_not_penalized():
    expected = [["query_sql"], ["list_tables"]]
    used = [["query_sql", "render_ui"], ["list_tables", "render_ui"]]
    agg = tool_selection(expected, used)
    assert abs(agg["precision"] - 1.0) < 1e-6


# ============================================================
# Rank 2：步骤效率最优步数取工具调用数，与实际同口径
# ============================================================

def test_step_efficiency_optimal_ratio_from_tool_count():
    """高效 Agent（1 次工具调用，最优也 1 次）应得满分效率比。

    旧实现用 expected_steps（自然语言步骤，恒 ~3 条）当最优值：1 次调用被算成 0.33，
    与实际负相关。改后最优步数取期望工具调用数（此处 1），效率比应为 1.0。
    """
    references = [{"final_answer": "x", "tools_used": ["query_sql"]}]
    outputs = [{"final_answer": "x", "tools_used": ["query_sql"], "total_steps": 1}]
    result = compute_agent_metrics(references, outputs, metrics=["step_efficiency"])
    assert abs(result["step_efficiency"]["optimal_ratio"] - 1.0) < 1e-6


def test_step_efficiency_detects_detour():
    """实际 3 次调用、最优 1 次 → 效率比 1/3，绕路被如实反映。"""
    references = [{"final_answer": "x", "tools_used": ["query_sql"]}]
    outputs = [
        {"final_answer": "x", "tools_used": ["a", "b", "query_sql"], "total_steps": 3}
    ]
    result = compute_agent_metrics(references, outputs, metrics=["step_efficiency"])
    assert abs(result["step_efficiency"]["optimal_ratio"] - (1 / 3)) < 1e-6


def test_step_efficiency_no_expected_tools_excluded():
    """无期望工具标注（最优步数=0）的样本不参与统计、不虚构分数。"""
    references = [{"final_answer": "x", "tools_used": []}]
    outputs = [{"final_answer": "x", "tools_used": ["query_sql"], "total_steps": 1}]
    result = compute_agent_metrics(references, outputs, metrics=["step_efficiency"])
    # 唯一样本无最优步数 → 无有效样本 → optimal_ratio 记 0（分母为空）。
    assert result["step_efficiency"]["optimal_ratio"] == 0.0


def test_step_efficiency_symmetric_ratio_helper():
    """对称比：min(o/a, a/o)，超步与欠步都被扣分。"""
    assert abs(step_efficiency([2], [1])["optimal_ratio"] - 0.5) < 1e-6
    assert abs(step_efficiency([1], [2])["optimal_ratio"] - 0.5) < 1e-6


# ============================================================
# Rank 4：分词保留数字/百分比 token
# ============================================================

def test_tokenize_english_preserves_numbers():
    tokens = tokenize_english("revenue grew 25% in 2024")
    assert "25%" in tokens
    assert "2024" in tokens
    assert "revenue" in tokens


def test_tokenize_english_preserves_decimal():
    tokens = tokenize_english("pi is about 3.14 and e is 2.71")
    assert "3.14" in tokens
    assert "2.71" in tokens


def test_tokenize_mixed_cn_number():
    """中英混排下数字 token 不丢失。"""
    tokens = tokenize("2024 年营收增长 25%")
    assert "2024" in tokens
    assert "25%" in tokens


def test_tokenize_number_order_preserved():
    """数字与英文词交错时保持原始顺序（BLEU 依赖 n-gram 顺序）。"""
    tokens = tokenize_english("sales 2024 grew 10")
    assert tokens == ["sales", "2024", "grew", "10"]
