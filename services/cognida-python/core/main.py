"""Managed HTTP, lifecycle, gRPC and MCP runtime with truthful readiness.

Required services fail startup as a unit. /health remains a liveness probe;
/ready succeeds only after every configured listener has actually started.
"""

from __future__ import annotations

import asyncio
import signal
from contextlib import contextmanager
from typing import Any

import uvicorn
from config import get_settings

from core import get_logger, setup_logging
from core.http_auth import InternalHTTPAuthMiddleware, validate_http_auth_config


def _load_evaluation_app() -> Any:
    # Lifecycle computation and evaluation are required in this topology.
    # Configuration/dependency failures must not become a healthy empty server.
    from services.evaluation.fastapi_app import app

    return app


class ManagedServer(uvicorn.Server):
    async def serve(self, sockets=None):
        try:
            await super().serve(sockets=sockets)
        except SystemExit as exc:
            # Uvicorn uses sys.exit on bind errors. Keep it inside the runtime
            # supervisor so sibling listeners still get their cleanup phase.
            raise RuntimeError("Required HTTP listener failed to start") from exc

    @contextmanager
    def capture_signals(self):
        # The outer runtime owns signals for all listeners.
        yield

    def install_signal_handlers(self):
        # Compatibility with earlier uvicorn releases.
        pass


def _build_http_server(app: Any, host: str, port: int, log_level: str) -> uvicorn.Server:
    return ManagedServer(uvicorn.Config(app, host=host, port=port, log_level=log_level))


async def _serve_grpc(port: int, stop_event: asyncio.Event, on_ready=None) -> None:
    from grpc_service.servicer import create_grpc_server

    server = create_grpc_server(port)
    try:
        server.start()
        if on_ready is not None:
            on_ready()
        get_logger(__name__).info("gRPC server started", port=port)
        await stop_event.wait()
    finally:
        shutdown = server.stop(grace=5)
        await asyncio.to_thread(shutdown.wait, 6)


def _validate_runtime(settings) -> None:
    validate_http_auth_config()
    if settings.is_prod and settings.grpc_enabled and not settings.grpc_auth_token:
        raise ValueError("GRPC_AUTH_TOKEN is required in production")
    ports = [settings.api_port, settings.evaluation_http_port, settings.mcp_port]
    if settings.grpc_enabled:
        ports.append(settings.grpc_port)
    if len(set(ports)) != len(ports):
        raise ValueError("HTTP, evaluation, MCP and gRPC ports must be distinct")


async def _run(stop_event: asyncio.Event | None = None) -> None:
    settings = get_settings()
    setup_logging()
    _validate_runtime(settings)
    from mcp_service.server import LinkPythonMCPServer

    from core.app import create_app

    evaluation_app = _load_evaluation_app()
    mcp = LinkPythonMCPServer(mode="http")
    main_app = create_app()
    main_app.add_middleware(InternalHTTPAuthMiddleware)
    stop_event = stop_event or asyncio.Event()
    state = {"grpc": not settings.grpc_enabled, "mcp": False}
    servers = [
        _build_http_server(main_app, settings.api_host, settings.api_port, settings.log_level.lower()),
        _build_http_server(evaluation_app, settings.api_host, settings.evaluation_http_port, settings.log_level.lower()),
    ]

    # Extend the existing /ready endpoint, retaining its storage dependency
    # probes. Adding a second route would be shadowed by the original route.
    main_app.state.runtime_readiness = lambda: {
        **state,
        "http": servers[0].started,
        "compute": servers[1].started,
        "accepting": not stop_event.is_set(),
    }

    main_app.mount("/", evaluation_app)
    coros = [server.serve() for server in servers]
    coros.append(mcp.run(settings.mcp_host, settings.mcp_port, stop_event=stop_event,
                         on_ready=lambda: state.update(mcp=True)))
    if settings.grpc_enabled:
        coros.append(_serve_grpc(settings.grpc_port, stop_event,
                                on_ready=lambda: state.update(grpc=True)))

    loop = asyncio.get_running_loop()
    installed = []
    for sig in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(sig, stop_event.set)
            installed.append(sig)
        except (NotImplementedError, RuntimeError):
            pass

    tasks = [asyncio.create_task(coro) for coro in coros]
    stopping = asyncio.create_task(stop_event.wait())
    try:
        done, _ = await asyncio.wait([*tasks, stopping], return_when=asyncio.FIRST_COMPLETED)
        if stopping not in done:
            # A server that returns early (even without an exception) is not a
            # functioning listener; propagate failure and stop its siblings.
            for task in done:
                task.result()
            raise RuntimeError("A required service stopped unexpectedly")
    finally:
        stop_event.set()
        state.update(grpc=False, mcp=False)
        for server in servers:
            server.should_exit = True
        _, pending = await asyncio.wait(tasks, timeout=10)
        for task in pending:
            task.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        stopping.cancel()
        await asyncio.gather(stopping, return_exceptions=True)
        for sig in installed:
            loop.remove_signal_handler(sig)


def main() -> None:
    asyncio.run(_run())


if __name__ == "__main__":
    main()
