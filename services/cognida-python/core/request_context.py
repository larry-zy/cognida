"""请求上下文透传（request_id）。

跨进程链路追踪：Go 通过 HTTP 头 `X-Request-ID` 或 gRPC metadata `x-request-id`
把请求 ID 传进来，本模块负责在进入点把它绑定到 structlog contextvars，从而让该请求
生命周期内的所有日志自动带上 request_id（logger.py 的处理链首个处理器即
`structlog.contextvars.merge_contextvars`），无需在每处日志手动透传。

提供三种接入点：
- ``RequestIDMiddleware``：FastAPI/Starlette HTTP 中间件
- ``RequestIDServerInterceptor``：gRPC 同步服务器拦截器
- ``bind_request_id`` / ``get_request_id``：手动绑定/读取

另外提供跨服务内部 gRPC 边界的鉴权与租户透传（审计发现 H8）：
- ``TenantServerInterceptor``：从 metadata `x-tenant-id` 绑定租户 ID 到上下文（缺省不拒绝）
- ``AuthServerInterceptor``：校验 metadata `authorization: Bearer <secret>` 共享密钥
"""

from __future__ import annotations

import hmac
import uuid
from typing import Callable

import grpc
import structlog
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import Response

# HTTP 头与 gRPC metadata 的键名（gRPC metadata key 必须小写）。
HTTP_HEADER = "X-Request-ID"
GRPC_METADATA_KEY = "x-request-id"
TENANT_METADATA_KEY = "x-tenant-id"
AUTH_METADATA_KEY = "authorization"

# structlog contextvar / 日志字段名。
_CONTEXT_KEY = "request_id"
_TENANT_CONTEXT_KEY = "tenant_id"


def _new_request_id() -> str:
    """生成回退用的 request_id（上游未传时）。"""
    return uuid.uuid4().hex[:16]


def bind_request_id(request_id: str | None) -> str:
    """把 request_id 绑定到 structlog contextvars，返回最终生效的 ID。

    上游未传时生成一个回退 ID，保证每条请求都有可追踪的 ID。
    """
    rid = request_id or _new_request_id()
    structlog.contextvars.bind_contextvars(**{_CONTEXT_KEY: rid})
    return rid


def get_request_id() -> str | None:
    """读取当前上下文绑定的 request_id（未绑定返回 None）。"""
    return structlog.contextvars.get_contextvars().get(_CONTEXT_KEY)


def bind_tenant_id(tenant_id: str | None) -> str | None:
    """把 tenant_id 绑定到 structlog contextvars，返回最终生效的值。

    与 request_id 不同：租户可缺省（上游未传时返回 None，不绑定），
    因为并非所有内部 RPC 都在租户上下文内。
    """
    if not tenant_id:
        return None
    structlog.contextvars.bind_contextvars(**{_TENANT_CONTEXT_KEY: tenant_id})
    return tenant_id


def get_tenant_id() -> str | None:
    """读取当前上下文绑定的 tenant_id（未绑定返回 None）。"""
    return structlog.contextvars.get_contextvars().get(_TENANT_CONTEXT_KEY)


class RequestIDMiddleware(BaseHTTPMiddleware):
    """HTTP 请求 ID 中间件。

    从 ``X-Request-ID`` 头提取 request_id 绑定到 contextvars，并在响应头回写，
    便于前端/上游把响应与请求关联。请求结束后清理 contextvars 避免线程/协程复用串味。
    """

    async def dispatch(self, request: Request, call_next: Callable) -> Response:
        rid = bind_request_id(request.headers.get(HTTP_HEADER))
        try:
            response = await call_next(request)
        finally:
            structlog.contextvars.unbind_contextvars(_CONTEXT_KEY)
        response.headers[HTTP_HEADER] = rid
        return response


class RequestIDServerInterceptor(grpc.ServerInterceptor):
    """gRPC 同步服务器拦截器：从 metadata 提取 request_id 绑定到 contextvars。

    绑定发生在 handler 执行前；同步服务器基于线程池，contextvars 在被调用线程内可见，
    因此该请求处理期间的日志都会带上 request_id。
    """

    def intercept_service(self, continuation, handler_call_details):
        metadata = dict(handler_call_details.invocation_metadata or ())
        rid = metadata.get(GRPC_METADATA_KEY)

        handler = continuation(handler_call_details)
        if handler is None:
            return handler

        # 仅包装一元-一元（本服务的 gRPC 方法均为一元）；其余原样返回。
        if handler.unary_unary is None:
            return handler

        inner = handler.unary_unary

        def wrapper(request, context):
            bind_request_id(rid)
            try:
                return inner(request, context)
            finally:
                structlog.contextvars.unbind_contextvars(_CONTEXT_KEY)

        return grpc.unary_unary_rpc_method_handler(
            wrapper,
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )


class TenantServerInterceptor(grpc.ServerInterceptor):
    """gRPC 同步服务器拦截器：从 metadata 提取 tenant_id 绑定到 contextvars。

    与 ``RequestIDServerInterceptor`` 同样的机制；租户缺省时不拒绝，仅不绑定。
    """

    def intercept_service(self, continuation, handler_call_details):
        metadata = dict(handler_call_details.invocation_metadata or ())
        tenant_id = metadata.get(TENANT_METADATA_KEY)

        handler = continuation(handler_call_details)
        if handler is None:
            return handler

        # 仅包装一元-一元（本服务的 gRPC 方法均为一元）；其余原样返回。
        if handler.unary_unary is None:
            return handler

        inner = handler.unary_unary

        def wrapper(request, context):
            bind_tenant_id(tenant_id)
            try:
                return inner(request, context)
            finally:
                structlog.contextvars.unbind_contextvars(_TENANT_CONTEXT_KEY)

        return grpc.unary_unary_rpc_method_handler(
            wrapper,
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )


def _deny_handler(request_streaming: bool, response_streaming: bool):
    """构造与 RPC 流式 arity 匹配的拒绝 handler，abort 为 UNAUTHENTICATED。

    对 unary/stream 四种组合均适用；用对应构造器保证 gRPC 能正确分发到该 handler。
    """

    def abort(request, context):
        context.abort(
            grpc.StatusCode.UNAUTHENTICATED, "missing or invalid credentials"
        )

    def abort_stream(request, context):
        context.abort(
            grpc.StatusCode.UNAUTHENTICATED, "missing or invalid credentials"
        )
        # abort 抛出后不会到达此处，yield 仅为把函数声明成生成器以匹配流式响应。
        yield  # pragma: no cover

    if request_streaming and response_streaming:
        return grpc.stream_stream_rpc_method_handler(abort_stream)
    if request_streaming and not response_streaming:
        return grpc.stream_unary_rpc_method_handler(abort)
    if not request_streaming and response_streaming:
        return grpc.unary_stream_rpc_method_handler(abort_stream)
    return grpc.unary_unary_rpc_method_handler(abort)


class AuthServerInterceptor(grpc.ServerInterceptor):
    """gRPC 同步服务器拦截器：校验跨服务共享密钥（审计发现 H8）。

    约定：每个 RPC 必须携带 metadata ``authorization: Bearer <secret>``，
    ``<secret>`` 与 ``GRPC_AUTH_TOKEN`` 环境变量一致。缺失/格式错误/不匹配时
    以 ``UNAUTHENTICATED`` 中止（不回显所提供的 token），使用常量时间比较。

    ``expected_token`` 为空时退化为透传（不做任何强制），保留本地/开发默认行为；
    ``create_grpc_server`` 在 token 为空时不会加入本拦截器，此处再做一层内部兜底。
    """

    def __init__(self, expected_token: str) -> None:
        self._expected_token = expected_token or ""

    def _is_authorized(self, metadata: dict) -> bool:
        header = metadata.get(AUTH_METADATA_KEY)
        if not header:
            return False
        prefix = "Bearer "
        if not header.startswith(prefix):
            return False
        provided = header[len(prefix):]
        return hmac.compare_digest(provided, self._expected_token)

    def intercept_service(self, continuation, handler_call_details):
        # 未配置密钥 → 透传（内部兜底，正常路径由 create_grpc_server 决定是否装载）。
        if not self._expected_token:
            return continuation(handler_call_details)

        metadata = dict(handler_call_details.invocation_metadata or ())
        if self._is_authorized(metadata):
            return continuation(handler_call_details)

        # 鉴权失败：返回与该 RPC arity 匹配的拒绝 handler（覆盖一元/流式四种组合）。
        handler = continuation(handler_call_details)
        request_streaming = getattr(handler, "request_streaming", False)
        response_streaming = getattr(handler, "response_streaming", False)
        return _deny_handler(request_streaming, response_streaming)
