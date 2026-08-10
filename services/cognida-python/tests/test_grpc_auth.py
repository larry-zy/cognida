"""跨服务内部 gRPC 边界鉴权 / 租户透传拦截器单元测试（审计发现 H8）。

全部为进程内 hermetic 测试，不建立真实网络连接：直接驱动
``intercept_service``，用假的 ``handler_call_details`` / continuation /
``ServicerContext`` 模拟 gRPC 运行时行为。
"""

import grpc
import pytest
import structlog

from core.request_context import (
    AUTH_METADATA_KEY,
    TENANT_METADATA_KEY,
    AuthServerInterceptor,
    TenantServerInterceptor,
    get_tenant_id,
)


class _FakeDetails:
    """假的 handler_call_details，仅携带 invocation_metadata。"""

    def __init__(self, metadata):
        self.invocation_metadata = metadata


class _AbortError(Exception):
    """模拟 context.abort 抛出（gRPC 运行时 abort 会中断 handler）。"""

    def __init__(self, code, details):
        self.code = code
        self.details = details
        super().__init__(details)


class _FakeContext:
    """假的 ServicerContext：abort 抛出 _AbortError 以断言拒绝。"""

    def abort(self, code, details):
        raise _AbortError(code, details)


def _ok_handler(request, context):
    return "resp"


def _continuation(handler_call_details):
    """返回一个一元-一元 handler，模拟正常放行路径。"""
    return grpc.unary_unary_rpc_method_handler(_ok_handler)


TOKEN = "s3cr3t-shared-token"


def test_auth_allows_correct_bearer():
    """携带正确的 Bearer 密钥 → 放行，handler 正常返回。"""
    interceptor = AuthServerInterceptor(TOKEN)
    details = _FakeDetails(((AUTH_METADATA_KEY, f"Bearer {TOKEN}"),))

    handler = interceptor.intercept_service(_continuation, details)
    result = handler.unary_unary("req", _FakeContext())

    assert result == "resp"


def test_auth_rejects_missing_credentials():
    """缺失 authorization metadata → UNAUTHENTICATED 中止。"""
    interceptor = AuthServerInterceptor(TOKEN)
    details = _FakeDetails(())

    handler = interceptor.intercept_service(_continuation, details)
    with pytest.raises(_AbortError) as exc:
        handler.unary_unary("req", _FakeContext())

    assert exc.value.code == grpc.StatusCode.UNAUTHENTICATED
    # 不回显所提供的 token
    assert TOKEN not in exc.value.details


def test_auth_rejects_wrong_token():
    """Bearer 密钥不匹配 → UNAUTHENTICATED 中止。"""
    interceptor = AuthServerInterceptor(TOKEN)
    details = _FakeDetails(((AUTH_METADATA_KEY, "Bearer wrong-token"),))

    handler = interceptor.intercept_service(_continuation, details)
    with pytest.raises(_AbortError) as exc:
        handler.unary_unary("req", _FakeContext())

    assert exc.value.code == grpc.StatusCode.UNAUTHENTICATED


def test_auth_rejects_malformed_header():
    """authorization 缺少 'Bearer ' 前缀 → UNAUTHENTICATED 中止。"""
    interceptor = AuthServerInterceptor(TOKEN)
    details = _FakeDetails(((AUTH_METADATA_KEY, TOKEN),))

    handler = interceptor.intercept_service(_continuation, details)
    with pytest.raises(_AbortError) as exc:
        handler.unary_unary("req", _FakeContext())

    assert exc.value.code == grpc.StatusCode.UNAUTHENTICATED


def test_auth_passthrough_when_token_empty():
    """未配置密钥（空）→ 透传，不做任何强制（保留本地/开发行为）。"""
    interceptor = AuthServerInterceptor("")
    details = _FakeDetails(())  # 无 metadata 也应放行

    handler = interceptor.intercept_service(_continuation, details)
    result = handler.unary_unary("req", _FakeContext())

    assert result == "resp"


def test_auth_deny_handler_matches_streaming_arity():
    """流式 RPC 鉴权失败时返回的拒绝 handler arity 与原 RPC 匹配。"""
    interceptor = AuthServerInterceptor(TOKEN)
    details = _FakeDetails(())

    def stream_continuation(hcd):
        return grpc.stream_stream_rpc_method_handler(lambda req, ctx: iter(()))

    handler = interceptor.intercept_service(stream_continuation, details)
    assert handler.request_streaming is True
    assert handler.response_streaming is True
    # 驱动流式拒绝 handler：abort 应在迭代前抛出
    with pytest.raises(_AbortError) as exc:
        for _ in handler.stream_stream(iter(()), _FakeContext()):
            pass
    assert exc.value.code == grpc.StatusCode.UNAUTHENTICATED


def test_tenant_binds_from_metadata():
    """TenantServerInterceptor 从 metadata 绑定 tenant_id，handler 内可读，调用后解绑。"""
    structlog.contextvars.clear_contextvars()
    interceptor = TenantServerInterceptor()

    seen = {}

    def handler(request, context):
        seen["tenant"] = get_tenant_id()
        return "resp"

    def continuation(hcd):
        return grpc.unary_unary_rpc_method_handler(handler)

    details = _FakeDetails(((TENANT_METADATA_KEY, "42"),))
    wrapped = interceptor.intercept_service(continuation, details)
    result = wrapped.unary_unary("req", _FakeContext())

    assert result == "resp"
    assert seen["tenant"] == "42"
    # 调用后已解绑
    assert get_tenant_id() is None


def test_tenant_absent_does_not_reject():
    """租户缺省时不拒绝，handler 内读到 None。"""
    structlog.contextvars.clear_contextvars()
    interceptor = TenantServerInterceptor()

    seen = {}

    def handler(request, context):
        seen["tenant"] = get_tenant_id()
        return "resp"

    def continuation(hcd):
        return grpc.unary_unary_rpc_method_handler(handler)

    details = _FakeDetails(())
    wrapped = interceptor.intercept_service(continuation, details)
    result = wrapped.unary_unary("req", _FakeContext())

    assert result == "resp"
    assert seen["tenant"] is None
