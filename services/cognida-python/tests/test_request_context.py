"""request_id 透传（HTTP 中间件 / gRPC 拦截器）单元测试。"""

import structlog
from fastapi import FastAPI
from fastapi.testclient import TestClient

from core.request_context import (
    HTTP_HEADER,
    GRPC_METADATA_KEY,
    RequestIDMiddleware,
    RequestIDServerInterceptor,
    bind_request_id,
    get_request_id,
)


def _make_app() -> FastAPI:
    app = FastAPI()
    app.add_middleware(RequestIDMiddleware)

    @app.get("/ping")
    def ping():
        # 处理期间应能读到已绑定的 request_id
        return {"rid": get_request_id()}

    return app


def test_http_uses_incoming_header():
    """上游传入 X-Request-ID 时，处理期间可读且响应头回写相同值。"""
    client = TestClient(_make_app())
    resp = client.get("/ping", headers={HTTP_HEADER: "rid-from-go"})
    assert resp.status_code == 200
    assert resp.json()["rid"] == "rid-from-go"
    assert resp.headers[HTTP_HEADER] == "rid-from-go"


def test_http_generates_fallback_when_absent():
    """上游未传时生成回退 ID，请求内可读且响应头带回。"""
    client = TestClient(_make_app())
    resp = client.get("/ping")
    assert resp.status_code == 200
    rid = resp.json()["rid"]
    assert rid
    assert resp.headers[HTTP_HEADER] == rid


def test_http_unbinds_after_request():
    """请求结束后 contextvar 被清理，不残留串味。"""
    client = TestClient(_make_app())
    client.get("/ping", headers={HTTP_HEADER: "rid-x"})
    assert get_request_id() is None


def test_bind_and_get_request_id():
    """手动绑定/读取一致。"""
    structlog.contextvars.clear_contextvars()
    rid = bind_request_id("manual-rid")
    assert rid == "manual-rid"
    assert get_request_id() == "manual-rid"
    structlog.contextvars.clear_contextvars()


def test_grpc_interceptor_binds_from_metadata():
    """gRPC 拦截器从 metadata 提取 request_id 绑定到上下文，handler 内可读。"""
    structlog.contextvars.clear_contextvars()
    interceptor = RequestIDServerInterceptor()

    seen = {}

    def handler(request, context):
        seen["rid"] = get_request_id()
        return "resp"

    class FakeHandler:
        unary_unary = staticmethod(handler)
        request_streaming = False
        response_streaming = False
        request_deserializer = None
        response_serializer = None

    class FakeDetails:
        invocation_metadata = ((GRPC_METADATA_KEY, "grpc-rid"),)

    wrapped = interceptor.intercept_service(lambda d: FakeHandler(), FakeDetails())
    result = wrapped.unary_unary("req", object())

    assert result == "resp"
    assert seen["rid"] == "grpc-rid"
    # 调用后已解绑
    assert get_request_id() is None
