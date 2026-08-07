"""电商 Agent 种子数据集「金标准标注不变量」回归测试（DB-free）。

背景：scenario_ecommerce_agent.jsonl 曾从其生成器 convert_eval_datasets.py 悄悄漂移——
每题被硬挂 get_schema+render_ui 全集（纯数值问答本不产 UI，render_ui 拉低 tool_selection
recall），Q10「客单价」题面未提状态、金标准却偷偷按 completed 过滤（answer_accuracy 判错）。
根因是「构建期写死快照、无 CI 对拍」。本测试对**已提交的**种子 JSONL 施加结构不变量，
不依赖 DB、不重跑生成器，专抓这一类错标注回归。

若确需调整某题工具/口径，改生成器 convert_eval_datasets.py 后 --only 重生成再同步本文件断言。
"""

import json
from pathlib import Path

import pytest

# 种子数据在 Go 服务目录；从本测试文件相对定位（link-python/tests/evaluation → link-go/...）。
_SEED = (
    Path(__file__).resolve().parents[3]
    / "link-go"
    / "cmd"
    / "seed-eval-datasets"
    / "data"
    / "scenario_ecommerce_agent.jsonl"
)

# data-agent 真实工具词表：expected_tools 只能取自此集合，杜绝编造工具名（旧缺陷之一）。
_KNOWN_TOOLS = {
    "get_schema",
    "sql_execute",
    "data_analysis",
    "render_ui",
    "semantic_query",
    "semantic_models",
    "ground_terms",
    "graph_query",
}

# 只有题面明确要求可视化的题才允许 render_ui（否则纯文本问答期望 render_ui 会冤压 recall）。
_VIZ_HINTS = ("图", "可视化", "柱状", "chart", "画出", "绘制")


def _records() -> list[dict]:
    assert _SEED.exists(), f"种子数据集缺失: {_SEED}"
    return [json.loads(line) for line in _SEED.read_text(encoding="utf-8").splitlines() if line.strip()]


def test_seed_dataset_present_and_nonempty():
    recs = _records()
    assert len(recs) == 17, f"电商 Agent 集应有 17 条, got {len(recs)}"


@pytest.mark.parametrize("rec", _records())
def test_expected_tools_within_known_vocabulary(rec):
    """expected_tools 非空且全部落在真实工具词表内。"""
    tools = rec.get("expected_tools") or []
    assert tools, f"expected_tools 不应为空: {rec['question']}"
    unknown = set(tools) - _KNOWN_TOOLS
    assert not unknown, f"expected_tools 含未知工具 {unknown}: {rec['question']}"


@pytest.mark.parametrize("rec", _records())
def test_render_ui_only_for_visualization_questions(rec):
    """render_ui 只能出现在题面明确要可视化的题上（防「全集硬挂」回归）。"""
    if "render_ui" not in (rec.get("expected_tools") or []):
        return
    q = rec["question"]
    assert any(h in q for h in _VIZ_HINTS), (
        f"题面未要求可视化却期望 render_ui，属错标注: {q!r}"
    )


@pytest.mark.parametrize("rec", _records())
def test_completed_scope_consistent_between_question_and_answer(rec):
    """金标准答案若限定「已完成订单」，题面必须同样限定——防 Q10 式隐藏过滤口径回归。"""
    ans = rec.get("reference_answer") or ""
    if "已完成订单" not in ans:
        return
    q = rec["question"]
    assert ("已完成" in q) or ("completed" in q.lower()), (
        f"答案按已完成订单口径、题面却未限定，属隐藏过滤错标注: {q!r}"
    )


@pytest.mark.parametrize("rec", _records())
def test_expected_steps_are_descriptive_not_bare_tool_names(rec):
    """expected_steps 应为描述性步骤而非裸工具名（裸名是旧漂移快照的特征）。"""
    steps = rec.get("expected_steps") or []
    assert steps, f"expected_steps 不应为空: {rec['question']}"
    bare = [s for s in steps if s in _KNOWN_TOOLS]
    assert not bare, f"expected_steps 含裸工具名 {bare}（应为描述性步骤）: {rec['question']}"
