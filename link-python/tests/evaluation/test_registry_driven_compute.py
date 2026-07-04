"""注册表驱动的 compute-metrics 与可用指标目录测试。

覆盖:
- compute-metrics 遍历 request.graders 从注册表解析(而非写死 if 链)
- 通用执行路径可计算无专属批量算子的已注册 grader(如 exact_match/llm_factual)
- 每项输出动态 scores map(name->value),与固定字段并存
- 未注册指标名收集到 unsupported 而非静默忽略
- 零值聚合保留(回归)
- 按 eval_type 过滤目录、共用指标跨类型出现、未知类型报错
"""

import pytest
from fastapi.testclient import TestClient

from services.evaluation.fastapi_app import app
from services.evaluation.graders.registry import get_global_registry, list_graders_for

client = TestClient(app)

ENDPOINT = "/api/v1/evaluation/compute-metrics"


def _items():
    return [
        {
            "question": "2+2 等于几",
            "reference_answer": "4",
            "generated_answer": "4",
            "retrieved_pids": [],
            "relevant_pids": [],
            "retrieved_contexts": [],
        },
        {
            "question": "法国的首都",
            "reference_answer": "巴黎",
            "generated_answer": "伦敦",
            "retrieved_pids": [],
            "relevant_pids": [],
            "retrieved_contexts": [],
        },
    ]


# ============================================================
# compute-metrics 注册表驱动
# ============================================================

def test_generic_grader_path_computes_registered_metric():
    """无专属批量算子的已注册 grader(exact_match)应走通用路径被计算并聚合。"""
    resp = client.post(ENDPOINT, json={"items": _items(), "graders": ["exact_match"]})
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert "exact_match" in body["aggregate"]
    # 第一条完全匹配 -> 100，第二条不匹配 -> 0
    assert body["items"][0]["scores"]["exact_match"] == 100.0
    assert body["items"][1]["scores"]["exact_match"] == 0.0


def test_dynamic_scores_map_mirrors_fixed_fields():
    """固定字段(rouge_*)应同时镜像进动态 scores map。"""
    resp = client.post(ENDPOINT, json={"items": _items(), "graders": ["rouge"]})
    assert resp.status_code == 200, resp.text
    item0 = resp.json()["items"][0]
    assert "rouge_1" in item0["scores"]
    assert item0["scores"]["rouge_1"] == item0["rouge_1"]


def test_unsupported_grader_reported_not_silently_ignored():
    """未注册指标名应出现在 unsupported，而非被静默忽略。"""
    resp = client.post(
        ENDPOINT,
        json={"items": _items(), "graders": ["exact_match", "does_not_exist"]},
    )
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert body["unsupported"] == ["does_not_exist"]
    # 合法的 exact_match 仍被计算
    assert "exact_match" in body["aggregate"]


def test_zero_value_metric_kept_in_generic_path():
    """回归:通用路径下合法 0 分也应保留在聚合中。"""
    only_mismatch = [_items()[1]]  # 仅不匹配项
    resp = client.post(ENDPOINT, json={"items": only_mismatch, "graders": ["exact_match"]})
    assert resp.status_code == 200, resp.text
    agg = resp.json()["aggregate"]
    assert "exact_match" in agg and agg["exact_match"] == 0.0


def test_mixed_specialized_and_generic_graders():
    """专属批量家族(rouge)与通用路径(exact_match)可在同一请求中并存。"""
    resp = client.post(
        ENDPOINT, json={"items": _items(), "graders": ["rouge", "exact_match"]}
    )
    assert resp.status_code == 200, resp.text
    agg = resp.json()["aggregate"]
    assert "rouge_1" in agg  # 专属家族
    assert "exact_match" in agg  # 通用路径


# ============================================================
# 可用指标目录:按 eval_type 过滤
# ============================================================

def _names_for(eval_type):
    return {g["name"] for g in list_graders_for(eval_type)}


def test_registry_initialized_covers_all_catalog_names():
    """目录内每项都应有可执行 grader(注册表即唯一事实来源)。"""
    registry = get_global_registry()
    registry.initialize()
    for eval_type in ("llm", "rag", "agent"):
        for meta in list_graders_for(eval_type):
            assert registry.exists(meta["name"]), f"{meta['name']} 无可执行 grader"


def test_filter_by_eval_type_rag_only():
    """检索类指标(precision 等)只应出现在 rag 目录。"""
    rag = _names_for("rag")
    llm = _names_for("llm")
    assert "precision" in rag
    assert "precision" not in llm
    assert "faithfulness" in rag
    assert "faithfulness" not in llm


def test_shared_metric_appears_across_types():
    """共用指标(rouge)应同时出现在 llm 与 rag 目录。"""
    assert "rouge" in _names_for("llm")
    assert "rouge" in _names_for("rag")


def test_qa_alias_normalized_to_llm():
    """qa 是 llm 的别名,两者目录应一致。"""
    assert _names_for("qa") == _names_for("llm")


def test_unknown_eval_type_raises():
    """未知评测类型应报错,而非无过滤全量返回。"""
    with pytest.raises(ValueError):
        list_graders_for("not_a_type")


def test_metadata_contract_fields_present():
    """每个 grader 元数据应含契约字段。"""
    for meta in list_graders_for("rag"):
        for key in ("name", "label", "group", "eval_types",
                    "requires_reference", "requires_contexts"):
            assert key in meta, f"元数据缺少 {key}"


# ============================================================
# 可用指标目录 HTTP 端点(供 Go 拉取)
# ============================================================

CATALOG_ENDPOINT = "/api/v1/evaluation/graders"


def test_catalog_endpoint_filters_by_eval_type():
    resp = client.get(CATALOG_ENDPOINT, params={"eval_type": "rag"})
    assert resp.status_code == 200, resp.text
    body = resp.json()
    assert body["eval_type"] == "rag"
    names = {g["name"] for g in body["graders"]}
    assert "precision" in names
    assert "faithfulness" in names


def test_catalog_endpoint_qa_alias():
    llm = client.get(CATALOG_ENDPOINT, params={"eval_type": "llm"}).json()
    qa = client.get(CATALOG_ENDPOINT, params={"eval_type": "qa"}).json()
    assert qa["eval_type"] == "llm"
    assert {g["name"] for g in qa["graders"]} == {g["name"] for g in llm["graders"]}


def test_catalog_endpoint_unknown_type_400():
    resp = client.get(CATALOG_ENDPOINT, params={"eval_type": "bogus"})
    assert resp.status_code == 400, resp.text
