"""LLM 裁判正确性测试。

覆盖：
- grader 调 agenerate_json 后分数按预期计算（mock 真实返回）
- 调用/解析失败时不再静默返回固定分（3.0/零分），而是显式错误
- 分数量纲统一 1-5：维度、total_score 一致
"""

from unittest.mock import AsyncMock, patch

import pytest

from services.evaluation.graders.base import GraderError, GraderScore
from services.evaluation.graders.builtin.llm import LLMJudgeGrader
from services.evaluation.metrics.llm_judge import (
    LLMJudgeMetrics,
    compute_llm_judge_metrics_async,
)


# ============================================================
# LLMJudgeGrader（graders/builtin/llm.py）
# ============================================================

@pytest.mark.asyncio
async def test_grader_scores_from_generate_json():
    """mock generate_json 真实返回，断言分数按预期计算（1-5 口径）。"""
    grader = LLMJudgeGrader(dimensions=["accuracy", "relevance"])
    mock_client = AsyncMock()
    mock_client.agenerate_json = AsyncMock(
        return_value={"accuracy": 4.0, "relevance": 5.0, "reason": "答得不错"}
    )
    grader._llm_client = mock_client

    result = await grader.aevaluate(query="什么是 RAG?", response="检索增强生成……")

    assert isinstance(result, GraderScore)
    assert result.metrics["accuracy"] == 4.0
    assert result.metrics["relevance"] == 5.0
    assert result.metrics["total_score"] == pytest.approx(4.5)
    # 确认走的是结构化 JSON 接口
    mock_client.agenerate_json.assert_awaited_once()


@pytest.mark.asyncio
async def test_grader_clamps_out_of_range_scores():
    """越界分数被截断到 [1, 5]。"""
    grader = LLMJudgeGrader(dimensions=["accuracy"])
    mock_client = AsyncMock()
    mock_client.agenerate_json = AsyncMock(return_value={"accuracy": 99})
    grader._llm_client = mock_client

    result = await grader.aevaluate(query="q", response="r")

    assert isinstance(result, GraderScore)
    assert result.metrics["accuracy"] == 5.0


@pytest.mark.asyncio
async def test_grader_llm_failure_is_explicit_error_not_3():
    """LLM 调用抛异常时返回 GraderError，绝不返回 total_score=3.0。"""
    grader = LLMJudgeGrader(dimensions=["accuracy"])
    mock_client = AsyncMock()
    mock_client.agenerate_json = AsyncMock(side_effect=RuntimeError("LLM 挂了"))
    grader._llm_client = mock_client

    result = await grader.aevaluate(query="q", response="r")

    assert isinstance(result, GraderError)
    assert "LLM 挂了" in result.error


@pytest.mark.asyncio
async def test_grader_missing_dimension_is_explicit_error():
    """LLM 返回缺维度时报错，不用默认分填充。"""
    grader = LLMJudgeGrader(dimensions=["accuracy", "relevance"])
    mock_client = AsyncMock()
    mock_client.agenerate_json = AsyncMock(return_value={"accuracy": 4.0})  # 缺 relevance
    grader._llm_client = mock_client

    result = await grader.aevaluate(query="q", response="r")

    assert isinstance(result, GraderError)
    assert "relevance" in result.error


@pytest.mark.asyncio
async def test_grader_missing_input_is_explicit_error():
    """缺问题/答案时返回显式错误，不伪造中间分。"""
    grader = LLMJudgeGrader()

    result = await grader.aevaluate(query="", response="r")

    assert isinstance(result, GraderError)


# ============================================================
# LLMJudgeMetrics（metrics/llm_judge.py）
# ============================================================

class _FakeClient:
    """同步/异步 generate_json 假客户端。"""

    def __init__(self, result=None, error=None):
        self._result = result
        self._error = error
        self.json_calls = 0

    def generate_json(self, prompt, system_prompt=None):
        self.json_calls += 1
        if self._error:
            raise self._error
        return self._result

    async def agenerate_json(self, prompt, system_prompt=None):
        return self.generate_json(prompt, system_prompt)


def test_metrics_unified_scale_1_to_5():
    """维度与 total_score 同为 1-5 口径，total 为维度均分。"""
    client = _FakeClient(result={"accuracy": 4, "clarity": 2, "reason": "ok"})

    m = LLMJudgeMetrics.compute(
        reference="ref", hypothesis="hyp", question="q",
        llm_client=client, dimensions=["accuracy", "clarity"],
    )

    assert m.dimension_scores == {"accuracy": 4.0, "clarity": 2.0}
    assert m.total_score == pytest.approx(3.0)
    assert 1.0 <= m.total_score <= 5.0
    assert m.reasoning == "ok"


def test_metrics_failure_raises_not_zero():
    """LLM 失败时抛出，不再静默返回零分。"""
    client = _FakeClient(error=RuntimeError("boom"))

    with pytest.raises(RuntimeError, match="boom"):
        LLMJudgeMetrics.compute(
            reference="ref", hypothesis="hyp",
            llm_client=client, dimensions=["accuracy"],
        )


def test_metrics_missing_dimension_raises():
    """返回缺维度时抛 ValueError，不填零分。"""
    client = _FakeClient(result={"accuracy": 4})

    with pytest.raises(ValueError, match="clarity"):
        LLMJudgeMetrics.compute(
            reference="ref", hypothesis="hyp",
            llm_client=client, dimensions=["accuracy", "clarity"],
        )


# ============================================================
# 无状态 compute 入口（compute_llm_judge_metrics_async）
# ============================================================

@pytest.mark.asyncio
async def test_async_metrics_returns_dimension_scores_dict():
    """compute_llm_judge_metrics_async 按 (reference, hypothesis, question, dimensions)
    签名调用，返回 dict 且下游读 result["dimension_scores"]。"""
    fake = _FakeClient(result={"accuracy": 5, "relevance": 3})

    with patch(
        "services.evaluation.metrics.llm_judge.LLMJudgeMetrics._default_client",
        return_value=fake,
    ):
        result = await compute_llm_judge_metrics_async(
            reference="ref",
            hypothesis="hyp",
            question="q",
            dimensions=["accuracy", "relevance"],
        )

    assert result["dimension_scores"] == {"accuracy": 5.0, "relevance": 3.0}
    assert result["total_score"] == pytest.approx(4.0)
