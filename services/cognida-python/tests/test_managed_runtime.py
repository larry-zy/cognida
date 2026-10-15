"""Managed listeners: real socket/auth/cleanup checks on isolated ephemeral ports."""

import asyncio
import json
import socket
from concurrent.futures import ThreadPoolExecutor
from types import SimpleNamespace
from unittest.mock import AsyncMock

import grpc
import httpx
import pytest
from config import get_settings
from core import main as runtime
from core.app import create_app
from core.readiness import DependencyStatus
from fastapi.testclient import TestClient
from mcp_service.server import LinkPythonMCPServer
from services.evaluation.fastapi_app import app as compute_app


@pytest.fixture
def runtime_settings(monkeypatch, unused_tcp_port_factory):
    settings = get_settings()
    values = {
        "app_env": "test", "api_host": "127.0.0.1", "mcp_host": "127.0.0.1",
        "api_port": unused_tcp_port_factory(), "evaluation_http_port": unused_tcp_port_factory(),
        "mcp_port": unused_tcp_port_factory(), "grpc_port": unused_tcp_port_factory(),
        "grpc_enabled": True, "internal_http_auth_token": "runtime-http-secret",
        "grpc_auth_token": "runtime-grpc-secret", "log_level": "ERROR",
    }
    for key, value in values.items():
        monkeypatch.setattr(settings, key, value)
    # The test has no storage dependencies and must not probe the developer's DB.
    for key in ("REDIS_URL", "DATABASE_URL", "MILVUS_HOST", "NEO4J_URI"):
        monkeypatch.delenv(key, raising=False)
    monkeypatch.setattr(runtime, "_load_evaluation_app", lambda: compute_app)
    return settings


async def wait_ready(client, url, task):
    async with asyncio.timeout(20):
        while True:
            if task.done():
                await task
                pytest.fail("Runtime exited before readiness")
            try:
                response = await client.get(url + "/ready")
                if response.status_code == 200:
                    return response.json()
            except httpx.TransportError:
                pass
            await asyncio.sleep(0.02)


async def assert_ports_closed(ports):
    for port in ports:
        with pytest.raises(OSError):
            reader, writer = await asyncio.open_connection("127.0.0.1", port)
            writer.close()
            await writer.wait_closed()


def test_readiness_requires_listeners_and_storage(monkeypatch):
    from core import readiness

    app = create_app()
    state = {"http": True, "compute": True, "grpc": True, "mcp": False}
    app.state.runtime_readiness = lambda: dict(state)
    monkeypatch.setattr(readiness, "check_readiness", AsyncMock(return_value=(True, [])))
    with TestClient(app) as client:
        pending = client.get("/ready")
        assert pending.status_code == 503
        assert pending.json()["services"]["mcp"] is False
        state["mcp"] = True
        assert client.get("/ready").status_code == 200
        monkeypatch.setattr(readiness, "check_readiness", AsyncMock(return_value=(
            False, [DependencyStatus("redis", "error", "unavailable")],
        )))
        blocked = client.get("/ready")
        assert blocked.status_code == 503
        assert blocked.json()["dependencies"]["redis"]["status"] == "error"
        assert client.get("/health").status_code == 200


@pytest.mark.parametrize("token_field", ["internal_http_auth_token", "grpc_auth_token"])
def test_production_missing_credentials_fail_before_start(runtime_settings, monkeypatch, token_field):
    monkeypatch.setattr(runtime_settings, "app_env", "prod")
    monkeypatch.setattr(runtime_settings, token_field, "")
    with pytest.raises(ValueError, match="required in production"):
        runtime._validate_runtime(runtime_settings)


def test_duplicate_listener_ports_fail_before_start(runtime_settings, monkeypatch):
    monkeypatch.setattr(runtime_settings, "evaluation_http_port", runtime_settings.mcp_port)
    with pytest.raises(ValueError, match="distinct"):
        runtime._validate_runtime(runtime_settings)


async def test_required_compute_import_failure_is_not_silenced(runtime_settings, monkeypatch):
    def missing_compute():
        raise ImportError("required dependency unavailable")
    monkeypatch.setattr(runtime, "_load_evaluation_app", missing_compute)
    with pytest.raises(ImportError, match="required dependency"):
        await runtime._run()


@pytest.mark.integration
async def test_managed_runtime_real_http_mcp_grpc_auth_and_shutdown(runtime_settings):
    from proto import docreader_pb2, docreader_pb2_grpc

    settings = runtime_settings
    stop = asyncio.Event()
    task = asyncio.create_task(runtime._run(stop))
    api_url = f"http://127.0.0.1:{settings.api_port}"
    headers = {"Authorization": "Bearer runtime-http-secret", "X-Request-ID": "runtime-test"}
    try:
        async with httpx.AsyncClient(trust_env=False, timeout=5) as client:
            ready = await wait_ready(client, api_url, task)
            assert ready["ready"] is True
            assert all(ready["services"].values())
            for port in (settings.api_port, settings.evaluation_http_port):
                url = f"http://127.0.0.1:{port}/api/v1/lifecycle/profile"
                payload = {"input": {"kind": "table", "columns": ["name"], "rows": [["Ada"], [None]]},
                           "options": {"dimensions": ["completeness"]}}
                for auth in ({}, {"Authorization": "Bearer wrong"}):
                    assert (await client.post(url, json=payload, headers=auth)).status_code == 401
                response = await client.post(url, json=payload, headers=headers)
                assert response.status_code == 200, response.text
                assert response.json()["result"]["overall_score"] == 50
                assert response.headers["x-request-id"] == "runtime-test"
            mcp_url = f"http://127.0.0.1:{settings.mcp_port}/mcp"
            payload = {"jsonrpc": "2.0", "id": 1, "method": "tools/list"}
            for auth in ({}, {"Authorization": "Bearer wrong"}):
                assert (await client.post(mcp_url, json=payload, headers=auth)).status_code == 401
            response = await client.post(mcp_url, json=payload, headers=headers)
            assert "data_describe" in {t["name"] for t in response.json()["result"]}
            payload.update(method="tools/call", params={"name": "data_describe", "arguments": {
                "data": {"columns": ["score"], "rows": [[2], [4]]},
            }})
            result = (await client.post(mcp_url, json=payload, headers=headers)).json()
            assert json.loads(result["result"][0]["text"])["data"]["row_count"] == 2
        async with grpc.aio.insecure_channel(f"127.0.0.1:{settings.grpc_port}") as channel:
            stub = docreader_pb2_grpc.DocumentReaderServiceStub(channel)
            request = docreader_pb2.ChunkRequest(text="Runtime integration test paragraph.")
            for metadata in ((), (("authorization", "Bearer wrong"),)):
                with pytest.raises(grpc.aio.AioRpcError) as error:
                    await stub.ChunkDocument(request, metadata=metadata, timeout=5)
                assert error.value.code() == grpc.StatusCode.UNAUTHENTICATED
            result = await stub.ChunkDocument(request, metadata=(
                ("authorization", "Bearer runtime-grpc-secret"), ("x-tenant-id", "runtime-test"),
            ), timeout=5)
            assert result.total_count >= 1
    finally:
        stop.set()
        await asyncio.wait_for(task, timeout=15)
    await assert_ports_closed([settings.api_port, settings.evaluation_http_port, settings.mcp_port, settings.grpc_port])


@pytest.mark.integration
@pytest.mark.parametrize("occupied_field", ["api_port", "mcp_port", "grpc_port"])
async def test_bind_failure_stops_all_sibling_listeners(runtime_settings, occupied_field):
    settings = runtime_settings
    if occupied_field == "grpc_port":
        executor = ThreadPoolExecutor(max_workers=1)
        occupied = grpc.server(executor, options=[("grpc.so_reuseport", 0)])
        occupied.add_insecure_port(f"0.0.0.0:{settings.grpc_port}")
        occupied.start()
    else:
        occupied = socket.socket()
        # macOS allows wildcard and specific-address listeners to coexist.
        # Occupy the exact address used by the managed HTTP/MCP listener.
        occupied.bind(("127.0.0.1", getattr(settings, occupied_field)))
        occupied.listen()
    try:
        with pytest.raises((RuntimeError, OSError)) as error:
            await asyncio.wait_for(runtime._run(), timeout=20)
        assert not isinstance(error.value, TimeoutError), "Startup failure must be raised by the runtime"
        ports = [getattr(settings, name) for name in (
            "api_port", "evaluation_http_port", "mcp_port", "grpc_port",
        ) if name != occupied_field]
        await assert_ports_closed(ports)
    finally:
        if occupied_field == "grpc_port":
            occupied.stop(0).wait(5)
            executor.shutdown(wait=True)
        else:
            occupied.close()


@pytest.mark.integration
async def test_mcp_cancellation_cancels_inflight_tool_and_closes_listener(runtime_settings, monkeypatch):
    entered, cancelled, ready = asyncio.Event(), asyncio.Event(), asyncio.Event()

    async def hanging_tool(*args, **kwargs):
        entered.set()
        try:
            await asyncio.Event().wait()
        finally:
            cancelled.set()

    server = LinkPythonMCPServer(mode="http")
    monkeypatch.setattr(server, "_registry", SimpleNamespace(execute=hanging_tool))
    task = asyncio.create_task(server.run("127.0.0.1", runtime_settings.mcp_port, on_ready=ready.set))
    call = None
    try:
        await asyncio.wait_for(ready.wait(), timeout=5)
        async with httpx.AsyncClient(trust_env=False, timeout=15) as client:
            call = asyncio.create_task(client.post(
                f"http://127.0.0.1:{runtime_settings.mcp_port}/mcp",
                headers={"Authorization": "Bearer runtime-http-secret"},
                json={"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "hanging"}},
            ))
            await asyncio.wait_for(entered.wait(), timeout=5)
            task.cancel()
            with pytest.raises(asyncio.CancelledError):
                await asyncio.wait_for(task, timeout=8)
            assert cancelled.is_set()
            await asyncio.gather(call, return_exceptions=True)
    finally:
        if call is not None:
            call.cancel()
            await asyncio.gather(call, return_exceptions=True)
        task.cancel()
        await asyncio.gather(task, return_exceptions=True)
    await assert_ports_closed([runtime_settings.mcp_port])
