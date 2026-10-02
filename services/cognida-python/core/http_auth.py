"""Shared-service HTTP authentication for MCP and evaluation compute."""

import hmac

from config import get_settings
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.responses import JSONResponse


def validate_http_auth_config() -> None:
    settings = get_settings()
    if settings.is_prod and not settings.internal_http_auth_token:
        raise ValueError("INTERNAL_HTTP_AUTH_TOKEN is required in production")


def authorized_internal_request(header: str | None) -> bool:
    settings = get_settings()
    expected = settings.internal_http_auth_token
    if not expected:
        return not settings.is_prod
    if not header or not header.startswith("Bearer "):
        return False
    return hmac.compare_digest(header[7:].encode("utf-8"), expected.encode("utf-8"))


class InternalHTTPAuthMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request, call_next):
        if request.url.path.startswith("/api/") and not authorized_internal_request(
            request.headers.get("authorization")
        ):
            return JSONResponse(
                {"detail": "missing or invalid credentials"},
                status_code=401,
                headers={"WWW-Authenticate": "Bearer"},
            )
        return await call_next(request)
