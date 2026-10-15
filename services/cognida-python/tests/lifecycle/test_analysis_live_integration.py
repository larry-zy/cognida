"""Opt-in real HTTP/provider smoke: no model response fixtures."""
import os
import secrets
import socket
import threading
import time

import httpx
import pytest
import uvicorn
from config import get_settings
from services.evaluation.fastapi_app import app


@pytest.mark.integration
@pytest.mark.skipif(os.getenv("COGNIDA_LIVE_ANALYSIS_TEST") != "1", reason="real provider opt-in required")
def test_goal_analysis_with_real_http_and_configured_model(monkeypatch):
    token = secrets.token_urlsafe(32)
    monkeypatch.setattr(get_settings(), "app_env", "test")
    monkeypatch.setattr(get_settings(), "internal_http_auth_token", token)
    listener = socket.socket()
    listener.bind(("127.0.0.1", 0))
    listener.listen(16)
    port = listener.getsockname()[1]
    server = uvicorn.Server(uvicorn.Config(app, log_level="error", lifespan="on"))
    thread = threading.Thread(target=server.run, kwargs={"sockets": [listener]}, daemon=True)
    thread.start()
    try:
        deadline = time.monotonic() + 15
        while not server.started and thread.is_alive() and time.monotonic() < deadline:
            time.sleep(0.05)
        assert server.started
        body = {"input": {"kind": "table", "columns": ["month", "sales"], "rows": [["Jan", 10], ["Feb", 20]]},
                "options": {"prompt": "比较1月和2月销售额，只报告观察到的差异，不推断因果。", "source_ref": "live-review:1",
                            "statistics": {"row_count": 2, "columns": [{"name": "sales", "mean": 15, "min": 10, "max": 20}]}}}
        url = f"http://127.0.0.1:{port}/api/v1/lifecycle/analyze"
        assert httpx.post(url, json=body, trust_env=False).status_code == 401
        result = httpx.post(url, json=body, headers={"Authorization": f"Bearer {token}"}, timeout=120, trust_env=False)
        assert result.status_code == 200, result.text
        output = result.json()["result"]
        assert output["prompt"] == body["options"]["prompt"]
        assert output["narrative"] and output["findings"] and output["model"]
        ids = {item["id"] for item in output["evidence"]}
        assert all(identifier in ids for item in output["findings"] for identifier in item["evidence_ids"])
        assert output["verification_status"] == "citations_checked_interpretation_unverified"
    finally:
        server.should_exit = True
        thread.join(15)
        listener.close()
        assert not thread.is_alive(), "test-owned server did not stop"
