import asyncio
import base64
import io
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest
from config import get_settings
from fastapi.testclient import TestClient
from PIL import Image
from services.evaluation.fastapi_app import app
from services.lifecycle import enrichment
from services.lifecycle.models import InferenceRequest
from services.lifecycle.router import execute

PREFIX = "/api/v1/lifecycle"


@pytest.fixture
def client(monkeypatch):
    monkeypatch.setattr(get_settings(), "app_env", "test")
    monkeypatch.setattr(get_settings(), "internal_http_auth_token", "")
    with TestClient(app) as client:
        yield client


def test_profile_uses_actual_engine_and_input_digest(client):
    payload = {
        "input": {"kind": "table", "columns": ["name"], "rows": [["Alice"], [None]]},
        "options": {"dimensions": ["completeness"]},
    }
    body = client.post(PREFIX + "/profile", json=payload).json()
    assert body["result"]["overall_score"] == 50
    assert body["result"]["record_count"] == 2
    assert body["result"]["dimensions"][0]["name"] == "completeness"
    assert len(body["input_digest"]) == 64
    clean = client.post(PREFIX + "/clean", json={"input": payload["input"]}).json()
    assert clean["input_digest"] == body["input_digest"]


@pytest.mark.parametrize("options", [{"dimensions": ["unknown"]}, {"dimensions": []}])
def test_profile_rejects_unknown_or_empty_dimensions(client, options):
    response = client.post(
        PREFIX + "/profile", json={"input": {"kind": "text", "text": "hello"}, "options": options}
    )
    assert response.status_code == 422


def test_clean_is_real_ordered_transformation_with_lineage(client):
    source = {
        "kind": "table",
        "columns": ["id", "name"],
        "rows": [[1, " a "], [1, " a "], [2, "b"]],
    }
    response = client.post(
        PREFIX + "/clean",
        json={"input": source, "options": {"operations": ["trim", "dedup"], "keep": "last"}},
    )
    assert response.status_code == 200, response.text
    result = response.json()["result"]
    assert result["data"]["rows"] == [[1, "a"], [2, "b"]]
    assert result["source_row_indices"] == [1, 2]
    assert result["removed_count"] == 1
    assert source["rows"][0][1] == " a "


def test_text_pii_and_duplicate_cleaning(client):
    result = client.post(
        PREFIX + "/clean",
        json={
            "input": {"kind": "text", "text": "联系 13812345678\n联系 13812345678"},
            "options": {"operations": ["pii_masker", "dedup"]},
        },
    ).json()["result"]
    assert "13812345678" not in result["data"]["text"]
    assert result["removed_count"] == 1


def test_numeric_phone_is_masked(client):
    result = client.post(
        PREFIX + "/clean",
        json={
            "input": {"kind": "table", "columns": ["phone"], "rows": [[13812345678]]},
            "options": {"operations": ["pii_masker"]},
        },
    ).json()["result"]
    assert str(result["data"]["rows"][0][0]) != "13812345678"


@pytest.mark.parametrize(
    "source",
    [
        {"kind": "table", "columns": ["x", "x"], "rows": [[1, 2]]},
        {"kind": "table", "columns": ["x"], "rows": [[1, 2]]},
        {"kind": "table", "columns": ["x"], "rows": [[{"nested": 1}]]},
    ],
)
def test_invalid_tables_are_rejected(client, source):
    assert client.post(PREFIX + "/profile", json={"input": source}).status_code == 422


def test_rules_enrichment_has_bounded_source_evidence(client):
    source = "优秀金融服务，联系 test@example.com"
    response = client.post(
        PREFIX + "/enrich",
        json={
            "input": {"kind": "text", "text": source},
            "options": {"labels": [{"name": "finance", "keywords": ["金融"]}], "max_keywords": 3},
        },
    )
    assert response.status_code == 200, response.text
    result = response.json()["result"]
    assert result["category"]["value"] == "finance"
    assert result["sensitivity"]["value"]["level"] == "sensitive"
    assert result["entities"]["value"][0]["text"] != "test@example.com"
    assert result["publication_action"] == "recommendation_only"
    assert len(result["keywords"]["value"]) <= 3


def test_merge_strict_and_union_preserve_provenance(client):
    inputs = [
        {"kind": "table", "columns": ["a"], "rows": [[1]]},
        {"kind": "table", "columns": ["b"], "rows": [[2]]},
    ]
    assert client.post(PREFIX + "/merge", json={"inputs": inputs}).status_code == 422
    result = client.post(
        PREFIX + "/merge", json={"inputs": inputs, "options": {"schema_policy": "union"}}
    ).json()["result"]
    assert result["data"]["columns"] == ["a", "b"]
    assert result["data"]["rows"] == [[1, None], [None, 2]]
    assert result["row_provenance"] == [
        {"input_index": 0, "row_index": 0},
        {"input_index": 1, "row_index": 0},
    ]


def test_recommend_never_returns_unsupplied_assets(client):
    result = client.post(
        PREFIX + "/recommend",
        json={
            "query": "销售",
            "candidates": [
                {"asset_id": "a", "title": "销售日报"},
                {"asset_id": "b", "title": "天气信息"},
            ],
        },
    ).json()["result"]
    assert [item["asset_id"] for item in result["recommendations"]] == ["a"]
    assert result["recommendations"][0]["evidence"]["matched_terms"]


def test_missing_models_explicitly_unavailable(client, monkeypatch):
    monkeypatch.setattr(
        enrichment,
        "resolve_llm_config",
        lambda: SimpleNamespace(api_key="", model="", provider="openai"),
    )
    monkeypatch.setattr(get_settings(), "image_model_id", "")
    assert (
        client.post(
            PREFIX + "/enrich",
            json={"input": {"kind": "text", "text": "hello"}, "options": {"method": "llm"}},
        ).status_code
        == 503
    )
    assert (
        client.post(
            PREFIX + "/batch-inference", json={"items": [{"id": "a", "prompt": "hello"}]}
        ).status_code
        == 503
    )
    assert (
        client.post(
            PREFIX + "/images/embed", json={"images": [{"id": "a", "image_base64": "abc"}]}
        ).status_code
        == 503
    )


def image_data(color="red"):
    out = io.BytesIO()
    Image.new("RGB", (20, 10), color).save(out, format="PNG")
    return base64.b64encode(out.getvalue()).decode()


def test_resize_actual_pixels_and_no_exif(client):
    response = client.post(
        PREFIX + "/images/resize",
        json={"images": [{"id": "a", "image_base64": image_data()}], "width": 8, "height": 8},
    )
    assert response.status_code == 200, response.text
    result = response.json()["result"]["images"][0]
    with Image.open(io.BytesIO(base64.b64decode(result["image_base64"]))) as image:
        assert image.size == (8, 4)
        assert not image.getexif()


def test_image_search_requires_tenant_and_authorized_candidates(client):
    payload = {"image_base64": image_data(), "candidate_asset_ids": ["a"]}
    assert client.post(PREFIX + "/images/search", json=payload).status_code == 422
    assert (
        client.post(
            PREFIX + "/images/search",
            json={**payload, "candidate_asset_ids": []},
            headers={"X-Tenant-ID": "1"},
        ).status_code
        == 422
    )


def test_auth_and_body_limit(client, monkeypatch):
    monkeypatch.setattr(get_settings(), "internal_http_auth_token", "internal")
    assert client.get(PREFIX + "/capabilities").status_code == 401
    assert (
        client.get(
            PREFIX + "/capabilities", headers={"Authorization": "Bearer internal"}
        ).status_code
        == 200
    )
    monkeypatch.setattr("services.lifecycle.router.MAX_BODY_BYTES", 10)
    assert client.post(
        PREFIX + "/profile", content="x" * 11, headers={"Authorization": "Bearer internal"}
    ).status_code == 413
    assert client.post(PREFIX + "/profile", content="x" * 11).status_code == 401


async def test_batch_inference_partial_failure_and_client_cleanup(monkeypatch):
    client = SimpleNamespace(
        model="test",
        generate=AsyncMock(side_effect=["answer", RuntimeError("private")]),
        close=AsyncMock(),
    )
    monkeypatch.setattr(enrichment, "llm_client", lambda: client)
    result = await enrichment.infer_batch(
        InferenceRequest(
            items=[{"id": "a", "prompt": "one"}, {"id": "b", "prompt": "two"}], concurrency=1
        )
    )
    assert result["result"]["succeeded"] == 1 and result["result"]["failed"] == 1
    assert "private" not in str(result)
    client.close.assert_awaited_once()


async def test_disconnect_cancels_operation():
    cancelled = asyncio.Event()

    async def operation():
        try:
            await asyncio.sleep(10)
        finally:
            cancelled.set()

    from fastapi import HTTPException

    with pytest.raises(HTTPException) as exc:
        await execute(SimpleNamespace(is_disconnected=AsyncMock(return_value=True)), operation())
    assert exc.value.status_code == 499
    assert cancelled.is_set()


def test_clean_preserves_large_integer_and_float_precision(client):
    rows = [[9223372036854775807, 0.12345678901234567], [None, None]]
    response = client.post(
        PREFIX + "/clean",
        json={
            "input": {"kind": "table", "columns": ["id", "measure"], "rows": rows},
            "options": {"operations": ["dedup"]},
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["result"]["data"]["rows"] == rows


async def test_timeout_cancels_operation(monkeypatch):
    monkeypatch.setattr(get_settings(), "lifecycle_timeout_seconds", 0.01)
    cancelled = asyncio.Event()

    async def operation():
        try:
            await asyncio.sleep(10)
        finally:
            cancelled.set()

    from fastapi import HTTPException

    with pytest.raises(HTTPException) as exc:
        await execute(SimpleNamespace(is_disconnected=AsyncMock(return_value=False)), operation())
    assert exc.value.status_code == 504 and cancelled.is_set()


@pytest.mark.parametrize(
    "reply", [{"confidence": 1, "evidence": ["invented"]}, {"invalid": "schema"}]
)
async def test_llm_requires_valid_schema_and_literal_evidence(monkeypatch, reply):
    client = SimpleNamespace(generate_json=AsyncMock(return_value=reply), close=AsyncMock())
    monkeypatch.setattr(enrichment, "llm_client", lambda: client)
    from services.lifecycle.models import EnrichRequest

    with pytest.raises(enrichment.CapabilityUnavailableError):
        await enrichment.enrich(
            EnrichRequest(input={"kind": "text", "text": "source"}, options={"method": "llm"})
        )
    client.close.assert_awaited_once()


def test_table_enrichment_preserves_schema_and_cell_evidence(client):
    response = client.post(
        PREFIX + "/enrich",
        json={
            "input": {
                "kind": "table",
                "columns": ["email", "category"],
                "rows": [["test@example.com", "金融"], [None, "金融"]],
            },
            "options": {"labels": [{"name": "finance", "keywords": ["金融"]}]},
        },
    )
    assert response.status_code == 200, response.text
    result = response.json()["result"]
    assert result["input_schema"] == {"kind": "table", "columns": ["email", "category"]}
    assert result["record_count"] == 2
    assert result["column_metadata"][0]["non_null_count"] == 1
    assert result["category"]["value"] == "finance"
    assert result["sensitivity"]["evidence"][0]["source_cells"] == [
        {"row_index": 0, "column": "email"}
    ]
    assert not result["evidence_truncated"]


def test_table_enrichment_preprocessing_respects_shared_cpu_capacity(client, monkeypatch):
    from services.lifecycle import compute

    def unexpected_preprocessing(_):
        pytest.fail("table preprocessing ran despite exhausted CPU capacity")

    monkeypatch.setattr(enrichment, "table_enrichment_source", unexpected_preprocessing)
    acquired = 0
    try:
        while compute._CPU_CAPACITY.acquire(blocking=False):
            acquired += 1
        response = client.post(
            PREFIX + "/enrich",
            json={"input": {"kind": "table", "columns": ["text"], "rows": [["sales"]]}},
        )
        assert response.status_code == 503, response.text
        assert response.json()["detail"]["code"] == "capability_unavailable"
    finally:
        for _ in range(acquired):
            compute._CPU_CAPACITY.release()


async def test_body_deadline_releases_capacity(monkeypatch):
    from services.lifecycle.router import LifecycleBodyLimitMiddleware

    monkeypatch.setattr("services.lifecycle.router.BODY_TIMEOUT_SECONDS", 0.01)
    monkeypatch.setattr("services.lifecycle.router.MAX_ACTIVE_REQUESTS", 1)
    downstream = AsyncMock()
    middleware = LifecycleBodyLimitMiddleware(downstream)
    messages = []

    async def slow_body():
        await asyncio.Event().wait()

    async def send(message):
        messages.append(message)

    await middleware({"type": "http", "path": PREFIX + "/profile"}, slow_body, send)
    assert messages[0]["status"] == 408
    assert b"request_body_timeout" in messages[1]["body"]
    downstream.assert_not_awaited()
    assert middleware.capacity.acquire(blocking=False)
    middleware.capacity.release()


async def test_request_capacity_rejects_before_reading_and_recovers(monkeypatch):
    from services.lifecycle.router import LifecycleBodyLimitMiddleware

    monkeypatch.setattr("services.lifecycle.router.MAX_ACTIVE_REQUESTS", 1)
    started = asyncio.Event()
    release = asyncio.Event()
    middleware = LifecycleBodyLimitMiddleware(AsyncMock())
    scope = {"type": "http", "path": PREFIX + "/profile"}

    async def pending_body():
        started.set()
        await release.wait()
        return {"type": "http.disconnect"}

    first = asyncio.create_task(middleware(scope, pending_body, AsyncMock()))
    await started.wait()
    response = []
    never_read = AsyncMock(side_effect=AssertionError("overloaded request read body"))

    async def send(message):
        response.append(message)

    try:
        await middleware(scope, never_read, send)
        assert response[0]["status"] == 503
        assert b"compute_capacity_busy" in response[1]["body"]
        never_read.assert_not_awaited()
    finally:
        release.set()
        await first
    assert middleware.capacity.acquire(blocking=False)
    middleware.capacity.release()
