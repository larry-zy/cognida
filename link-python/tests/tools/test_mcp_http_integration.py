"""MCP HTTP 传输集成测试：tools/list 与 tools/call 走真实 JSON-RPC。

启动一个 in-process 的 aiohttp MCP server（http 模式），用 aiohttp 客户端
对 `/mcp` 发送 JSON-RPC 请求，校验 analytics 工具可被发现与调用。
"""

import json

import pytest
from aiohttp import web
from aiohttp.test_utils import TestClient, TestServer

from mcp.server import LinkPythonMCPServer

EXPECTED_TOOLS = {
    "data_describe",
    "data_trend",
    "data_anomaly",
    "data_correlation",
    "data_insight",
}

SAMPLE_ROWSET = {
    "columns": ["month", "sales", "cost"],
    "rows": [
        ["2024-01", 100, 80],
        ["2024-02", 120, 85],
        ["2024-03", 150, 90],
        ["2024-04", 170, 95],
        ["2024-05", 999, 100],
        ["2024-06", 210, 110],
    ],
}


@pytest.fixture
async def client() -> TestClient:
    """启动 http 模式 MCP server 并返回测试客户端。"""
    server = LinkPythonMCPServer(mode="http")
    app: web.Application = server._app
    test_client = TestClient(TestServer(app))
    await test_client.start_server()
    yield test_client
    await test_client.close()


async def _rpc(client: TestClient, method: str, params: dict | None = None) -> dict:
    """发送一次 JSON-RPC 请求并返回解析后的响应。"""
    payload = {"jsonrpc": "2.0", "id": 1, "method": method}
    if params is not None:
        payload["params"] = params
    resp = await client.post("/mcp", json=payload)
    return await resp.json()


@pytest.mark.integration
class TestMCPHttpTransport:
    async def test_health(self, client) -> None:
        resp = await client.get("/health")
        body = await resp.json()
        assert body["status"] == "ok"

    async def test_tools_list_returns_analytics_tools(self, client) -> None:
        body = await _rpc(client, "tools/list")
        tools = body["result"]
        names = {t["name"] for t in tools}
        assert EXPECTED_TOOLS.issubset(names)
        # 每个工具都带 inputSchema
        for t in tools:
            if t["name"] in EXPECTED_TOOLS:
                assert t["inputSchema"]["type"] == "object"
                assert "data" in t["inputSchema"]["properties"]

    @pytest.mark.parametrize(
        "name,arguments",
        [
            ("data_describe", {"data": SAMPLE_ROWSET}),
            ("data_trend", {"data": SAMPLE_ROWSET, "value_col": "sales"}),
            ("data_anomaly", {"data": SAMPLE_ROWSET, "value_col": "sales"}),
            ("data_correlation", {"data": SAMPLE_ROWSET}),
            ("data_insight", {"data": SAMPLE_ROWSET}),
        ],
    )
    async def test_tools_call(self, client, name, arguments) -> None:
        body = await _rpc(
            client, "tools/call", {"name": name, "arguments": arguments}
        )
        content = body["result"]
        # MCP TextContent 列表
        assert isinstance(content, list) and content[0]["type"] == "text"
        parsed = json.loads(content[0]["text"])
        # {success, data} 信封：成功且内层带 row_count
        assert parsed["success"] is True
        assert parsed["data"]["row_count"] >= 1

    async def test_tools_call_structured_error(self, client) -> None:
        # 缺 value_col → 工具返回结构化错误信封（而非 JSON-RPC error）
        body = await _rpc(
            client,
            "tools/call",
            {"name": "data_trend", "arguments": {"data": SAMPLE_ROWSET}},
        )
        parsed = json.loads(body["result"][0]["text"])
        assert parsed["success"] is False
        assert parsed["error_type"] == "missing_param"
