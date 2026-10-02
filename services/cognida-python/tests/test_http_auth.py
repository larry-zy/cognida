"""MCP/evaluation HTTP shared-token checks and production startup configuration."""

import pytest
from config import get_settings
from core.http_auth import authorized_internal_request, validate_http_auth_config
from fastapi.testclient import TestClient
from services.evaluation.fastapi_app import app


@pytest.mark.parametrize(
    "header", [None, "", "secret", "Basic secret", "Bearer wrong", "Bearer 秘密"]
)
def test_invalid_internal_credentials_rejected(monkeypatch, header):
    monkeypatch.setattr(get_settings(), "internal_http_auth_token", "secret")
    assert not authorized_internal_request(header)


def test_production_requires_configured_token(monkeypatch):
    monkeypatch.setattr(get_settings(), "app_env", "prod")
    monkeypatch.setattr(get_settings(), "internal_http_auth_token", "")
    with pytest.raises(ValueError, match="required in production"):
        validate_http_auth_config()
    assert not authorized_internal_request(None)


def test_evaluation_routes_require_shared_token_but_health_is_public(monkeypatch):
    monkeypatch.setattr(get_settings(), "internal_http_auth_token", "test-shared-token")
    with TestClient(app) as client:
        assert client.get("/health").status_code == 200
        assert client.get("/api/v1/evaluation/graders?eval_type=llm").status_code == 401
        response = client.get(
            "/api/v1/evaluation/graders?eval_type=llm",
            headers={"Authorization": "Bearer test-shared-token"},
        )
        assert response.status_code == 200
