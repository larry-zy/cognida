import asyncio
from unittest.mock import AsyncMock

import pytest
from config import get_settings
from fastapi.testclient import TestClient
from services.evaluation.fastapi_app import app
from services.lifecycle import analysis
from services.lifecycle.models import AnalyzeRequest


def payload():
    return {"input": {"kind": "table", "columns": ["sales"], "rows": [[10], [20]]},
            "options": {"prompt": "解释销售差异", "source_ref": "asset:1",
                        "statistics": {"columns": [{"name": "sales", "mean": 15}], "row_count": 2}}}


def response():
    return {"narrative": "两条记录的均值为15。", "findings": [
        {"text": "销售均值15", "evidence_ids": ["stat:column:0"]}], "limitations": []}


@pytest.fixture
def client(monkeypatch):
    monkeypatch.setattr(get_settings(), "app_env", "test")
    monkeypatch.setattr(get_settings(), "internal_http_auth_token", "analysis-secret")
    with TestClient(app) as value:
        yield value


@pytest.fixture
def llm(monkeypatch):
    fake = AsyncMock()
    fake.model = "fixture-model"
    fake.generate_json.return_value = response()
    monkeypatch.setattr(analysis, "llm_client", lambda: fake)
    return fake


def test_goal_reaches_model_and_preserves_addressable_evidence(client, llm):
    res = client.post("/api/v1/lifecycle/analyze", json=payload(), headers={"Authorization": "Bearer analysis-secret"})
    assert res.status_code == 200, res.text
    result = res.json()["result"]
    assert result["prompt"] == "解释销售差异"
    assert result["source_ref"] == "asset:1"
    assert result["verification_status"] == "citations_checked_interpretation_unverified"
    assert {x["id"] for x in result["evidence"]} == {"stat:column:0", "stat:row_count", "row:0", "row:1"}
    assert "解释销售差异" in llm.generate_json.call_args.args[0]
    assert "untrusted" in llm.generate_json.call_args.args[0]
    llm.close.assert_awaited_once()


def test_auth_and_invalid_goals_do_not_call_model(client, llm):
    assert client.post("/api/v1/lifecycle/analyze", json=payload()).status_code == 401
    for prompt in ["  ", "x" * 4001]:
        body = payload()
        body["options"]["prompt"] = prompt
        assert client.post("/api/v1/lifecycle/analyze", json=body, headers={"Authorization": "Bearer analysis-secret"}).status_code == 422
    llm.generate_json.assert_not_called()


@pytest.mark.parametrize("bad", [{}, {**response(), "findings": [{"text": "invented", "evidence_ids": ["unknown"]}]}])
def test_invalid_model_evidence_is_failure(client, llm, bad):
    llm.generate_json.return_value = bad
    res = client.post("/api/v1/lifecycle/analyze", json=payload(), headers={"Authorization": "Bearer analysis-secret"})
    assert res.status_code == 503
    llm.close.assert_awaited_once()


def test_evidence_budget_and_sampling_disclosed():
    body = payload()
    body["input"]["rows"] = [["x" * 2000]] * 60
    items, truncated = analysis.source_evidence(AnalyzeRequest.model_validate(body))
    assert truncated
    assert sum(len(x["content"]) for x in items) <= 32000
    assert all(len(x["content"]) <= 1600 for x in items)
    assert all(x["id"] != "row:50" for x in items)


@pytest.mark.asyncio
async def test_cancellation_closes_client(llm):
    started = asyncio.Event()
    async def wait(*args, **kwargs):
        started.set()
        await asyncio.Event().wait()
    llm.generate_json.side_effect = wait
    task = asyncio.create_task(analysis.analyze(AnalyzeRequest.model_validate(payload())))
    await started.wait()
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task
    llm.close.assert_awaited_once()


def test_missing_provider_is_explicit_unavailable(client, monkeypatch):
    def unavailable():
        raise analysis.CapabilityUnavailableError("LLM is not configured")
    monkeypatch.setattr(analysis, "llm_client", unavailable)
    res = client.post("/api/v1/lifecycle/analyze", json=payload(), headers={"Authorization": "Bearer analysis-secret"})
    assert res.status_code == 503
    assert res.json()["detail"]["code"] == "capability_unavailable"
