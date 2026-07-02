"""MCP Server 实现。"""

import asyncio
import json
from typing import Any

from aiohttp import web
from config import get_settings
from core import get_logger, setup_logging
from tools.bootstrap import register_default_tools

from tools import get_registry


class LinkPythonMCPServer:
    """Link Python MCP Server。

    通过 MCP 协议暴露工具给 Go 服务调用。
    支持 stdio 和 HTTP 两种传输模式。
    """

    def __init__(self, mode: str = "stdio") -> None:
        """初始化 MCP Server。

        Args:
            mode: 传输模式，"stdio" 或 "http"
        """
        self._logger = get_logger(__name__)
        self._registry = get_registry()
        # 注册内置默认工具（含 analytics MCP 工具），幂等
        register_default_tools(self._registry)
        self._mode = mode

        if mode == "http":
            self._server = None  # HTTP 模式不使用 stdio server
            self._app = web.Application()
            self._setup_http_handlers()
        else:
            # 延迟 import 官方 mcp SDK：仅 stdio 模式需要，避免 http 模式强依赖。
            # 本地包已由 mcp/ 重命名为 mcp_service/，不再遮蔽官方 mcp SDK。
            from mcp.server import Server

            self._server = Server(get_settings().app_name)
            self._app = None
            self._setup_handlers()

    def _setup_http_handlers(self) -> None:
        """设置 HTTP MCP 请求处理器。"""

        async def handle_mcp_request(request: web.Request) -> web.Response:
            """处理 MCP JSON-RPC 请求。"""
            try:
                req_data = await request.json()

                # 验证 JSON-RPC 请求
                if not isinstance(req_data, dict):
                    return web.json_response(
                        {
                            "jsonrpc": "2.0",
                            "id": req_data.get("id"),
                            "error": {"code": -32600, "message": "Invalid Request"},
                        },
                        status=400,
                    )

                method = req_data.get("method")
                req_id = req_data.get("id")
                params = req_data.get("params", {})

                self._logger.info("收到 HTTP MCP 请求", method=method, id=req_id)

                # 处理 tools/list
                if method == "tools/list":
                    tools_list = await self._handle_list_tools()
                    return web.json_response(
                        {"jsonrpc": "2.0", "id": req_id, "result": tools_list}
                    )

                # 处理 tools/call
                elif method == "tools/call":
                    name = params.get("name")
                    arguments = params.get("arguments", {})
                    result = await self._handle_call_tool(name, arguments)
                    return web.json_response(
                        {"jsonrpc": "2.0", "id": req_id, "result": result}
                    )

                # 处理 resources/list
                elif method == "resources/list":
                    resources = await self._handle_list_resources()
                    return web.json_response(
                        {"jsonrpc": "2.0", "id": req_id, "result": resources}
                    )

                # 处理 prompts/list
                elif method == "prompts/list":
                    prompts = await self._handle_list_prompts()
                    return web.json_response(
                        {"jsonrpc": "2.0", "id": req_id, "result": prompts}
                    )

                # 未知方法
                else:
                    return web.json_response(
                        {
                            "jsonrpc": "2.0",
                            "id": req_id,
                            "error": {"code": -32601, "message": "Method not found"},
                        },
                        status=404,
                    )

            except json.JSONDecodeError:
                return web.json_response(
                    {"jsonrpc": "2.0", "id": None, "error": {"code": -32700, "message": "Parse error"}},
                    status=400,
                )
            except Exception as e:
                self._logger.error("处理 MCP 请求失败", error=str(e))
                return web.json_response(
                    {
                        "jsonrpc": "2.0",
                        "id": req_data.get("id") if 'req_data' in locals() else None,
                        "error": {"code": -32603, "message": f"Internal error: {str(e)}"},
                    },
                    status=500,
                )

        # 注册路由
        self._app.router.add_post("/mcp", handle_mcp_request)
        self._app.router.add_get("/health", self._handle_health)

    async def _handle_health(self, request: web.Request) -> web.Response:
        """健康检查。"""
        return web.json_response({"status": "ok", "mode": "http"})

    async def _handle_list_tools(self) -> list[dict[str, Any]]:
        """处理 tools/list 请求。"""
        self._logger.info("处理 tools/list 请求")
        tools = self._registry.to_dict_list()

        return [
            {
                "name": tool["name"],
                "description": tool["description"],
                "inputSchema": self._get_input_schema(tool["name"]),
            }
            for tool in tools
        ]

    async def _handle_call_tool(
        self, name: str, arguments: dict[str, Any]
    ) -> list[dict[str, Any]]:
        """处理 tools/call 请求。

        Args:
            name: 工具名称
            arguments: 工具参数

        Returns:
            MCP TextContent 列表
        """
        self._logger.info("处理 tools/call 请求", tool=name, args=arguments)

        try:
            result = await self._registry.execute(name, **arguments)

            return [
                {
                    "type": "text",
                    "text": json.dumps(result, ensure_ascii=False, indent=2),
                }
            ]
        except Exception as e:
            self._logger.error("工具执行失败", tool=name, error=str(e))
            return [
                {
                    "type": "text",
                    "text": json.dumps({"error": str(e)}, ensure_ascii=False),
                }
            ]

    async def _handle_list_resources(self) -> list[dict[str, Any]]:
        """处理 resources/list 请求。"""
        self._logger.info("处理 resources/list 请求")
        return []

    async def _handle_list_prompts(self) -> list[dict[str, Any]]:
        """处理 prompts/list 请求。"""
        self._logger.info("处理 prompts/list 请求")
        return []

    def _setup_handlers(self) -> None:
        """设置 MCP 请求处理器（stdio 模式）。"""
        # 延迟 import 官方 mcp SDK 类型：仅 stdio 模式需要
        import mcp.types as types

        @self._server.list_tools()
        async def handle_list_tools() -> list[types.Tool]:
            """处理 tools/list 请求。"""
            self._logger.info("收到 tools/list 请求")
            tools = self._registry.to_dict_list()

            return [
                types.Tool(
                    name=tool["name"],
                    description=tool["description"],
                    inputSchema=self._get_input_schema(tool["name"]),
                )
                for tool in tools
            ]

        @self._server.call_tool()
        async def handle_call_tool(
            name: str, arguments: dict[str, Any]
        ) -> list[types.TextContent]:
            """处理 tools/call 请求。"""
            self._logger.info("收到 tools/call 请求", tool=name, args=arguments)

            try:
                result = await self._registry.execute(name, **arguments)

                return [
                    types.TextContent(
                        type="text",
                        text=json.dumps(result, ensure_ascii=False, indent=2),
                    )
                ]
            except Exception as e:
                self._logger.error("工具执行失败", tool=name, error=str(e))
                return [
                    types.TextContent(
                        type="text",
                        text=json.dumps({"error": str(e)}, ensure_ascii=False),
                    )
                ]

        @self._server.list_resources()
        async def handle_list_resources() -> list[types.Resource]:
            """处理 resources/list 请求。"""
            self._logger.info("收到 resources/list 请求")
            return []

        @self._server.list_prompts()
        async def handle_list_prompts() -> list[types.Prompt]:
            """处理 prompts/list 请求。"""
            self._logger.info("收到 prompts/list 请求")
            return []

    def _get_input_schema(self, tool_name: str) -> dict[str, Any]:
        """获取工具的输入 schema。

        Args:
            tool_name: 工具名称

        Returns:
            JSON Schema 格式的输入定义
        """
        tool = self._registry.get(tool_name)
        if tool and hasattr(tool, "input_schema"):
            return tool.input_schema

        # 默认 schema
        return {
            "type": "object",
            "properties": {},
        }

    async def run(self, host: str = "0.0.0.0", port: int = 3000) -> None:
        """运行 MCP Server。

        Args:
            host: HTTP 模式下的监听地址
            port: HTTP 模式下的监听端口
        """
        settings = get_settings()
        self._logger.info(
            "启动 MCP Server",
            app_name=settings.app_name,
            env=settings.app_env,
            mode=self._mode,
        )

        if self._mode == "http":
            # HTTP 模式
            self._logger.info(f"启动 HTTP MCP Server on {host}:{port}")
            runner = web.AppRunner(self._app)
            await runner.setup()
            site = web.TCPSite(runner, host, port)
            await site.start()

            self._logger.info(f"HTTP MCP Server 已启动: http://{host}:{port}/mcp")

            # 保持运行
            try:
                while True:
                    await asyncio.sleep(3600)
            except asyncio.CancelledError:
                self._logger.info("HTTP MCP Server 已停止")
                await runner.cleanup()
        else:
            # stdio 模式：官方 SDK 用 stdio_server() 提供读写流，
            # 再交给 Server.run() 处理 JSON-RPC 循环。
            from mcp.server.stdio import stdio_server

            self._logger.info("启动 stdio MCP Server")
            async with stdio_server() as (read_stream, write_stream):
                await self._server.run(
                    read_stream,
                    write_stream,
                    self._server.create_initialization_options(),
                )


async def main() -> None:
    """主函数。"""
    # 设置日志
    setup_logging()

    # 从配置读取运行参数（settings 已支持 .env 与环境变量覆盖）
    settings = get_settings()
    mode = getattr(settings, "mcp_mode", "stdio")  # 默认 stdio 模式
    host = getattr(settings, "mcp_host", "0.0.0.0")
    port = getattr(settings, "mcp_port", 3000)

    # 启动服务器（工具通过 get_registry() 自动获取）
    server = LinkPythonMCPServer(mode=mode)
    await server.run(host=host, port=port)


if __name__ == "__main__":
    asyncio.run(main())
