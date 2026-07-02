"""MCP 请求处理器模块。"""

from typing import Any

import mcp.types as types
from core import get_logger

from tools import get_registry


class MCPHandlers:
    """MCP 请求处理器集合。"""

    def __init__(self) -> None:
        self._logger = get_logger(__name__)
        self._registry = get_registry()

    async def list_tools(self) -> list[types.Tool]:
        """处理 tools/list 请求。

        Returns:
            工具列表
        """
        self._logger.info("处理 tools/list 请求")
        tools = self._registry.to_dict_list()

        return [
            types.Tool(
                name=tool["name"],
                description=tool["description"],
                inputSchema=self._get_input_schema(tool["name"]),
            )
            for tool in tools
        ]

    async def call_tool(
        self, name: str, arguments: dict[str, Any]
    ) -> list[types.TextContent]:
        """处理 tools/call 请求。

        Args:
            name: 工具名称
            arguments: 工具参数

        Returns:
            工具执行结果
        """
        self._logger.info("处理 tools/call 请求", tool=name, args=arguments)

        try:
            result = await self._registry.execute(name, **arguments)

            return [
                types.TextContent(
                    type="text",
                    text=self._format_result(result),
                )
            ]
        except Exception as e:
            self._logger.error("工具执行失败", tool=name, error=str(e))
            return [
                types.TextContent(
                    type="text",
                    text=self._format_error(str(e)),
                )
            ]

    async def list_resources(self) -> list[types.Resource]:
        """处理 resources/list 请求。

        Returns:
            资源列表
        """
        self._logger.info("处理 resources/list 请求")
        return []

    async def list_prompts(self) -> list[types.Prompt]:
        """处理 prompts/list 请求。

        Returns:
            提示词列表
        """
        self._logger.info("处理 prompts/list 请求")
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

    def _format_result(self, result: Any) -> str:
        """格式化工具执行结果。

        Args:
            result: 执行结果

        Returns:
            JSON 字符串
        """
        import json

        return json.dumps(result, ensure_ascii=False, indent=2)

    def _format_error(self, error: str) -> str:
        """格式化错误信息。

        Args:
            error: 错误信息

        Returns:
            JSON 字符串
        """
        import json

        return json.dumps({"error": error}, ensure_ascii=False)


def get_handlers() -> MCPHandlers:
    """获取 MCP 处理器实例。

    Returns:
        MCP 处理器
    """
    return MCPHandlers()
