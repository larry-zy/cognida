"""请求上下文透传（request_id）。

跨进程链路追踪：Go 通过 HTTP 头 `X-Request-ID` 或 gRPC metadata `x-request-id`
把请求 ID 传进来，本模块负责在进入点把它绑定到 structlog contextvars，从而让该请求
生命周期内的所有日志自动带上 request_id（logger.py 的处理链首个处理器即
`structlog.contextvars.merge_contextvars`），无需在每处日志手动透传。

提供三种接入点：
- ``RequestIDMiddleware``：FastAPI/Starlette HTTP 中间件
- ``RequestIDServerInterceptor``：gRPC 同步服务器拦截器
- ``bind_request_id`` / ``get_request_id``：手动绑定/读取
"""

from __future__ import annotations

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

# structlog contextvar / 日志字段名。
_CONTEXT_KEY = "request_id"


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
