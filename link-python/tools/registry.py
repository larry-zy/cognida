"""工具注册中心模块。"""

from typing import Any, Dict

from core import get_logger
from .base import BaseTool


class ToolRegistry:
    """工具注册中心，管理所有可用工具。"""

    def __init__(self) -> None:
        self._tools: Dict[str, BaseTool] = {}
        self._logger = get_logger(__name__)

    def register(self, tool: BaseTool) -> None:
        """注册工具。

        Args:
            tool: 工具实例
        """
        if tool.name in self._tools:
            self._logger.warning(f"工具 {tool.name} 已存在，将被覆盖")
        self._tools[tool.name] = tool
        self._logger.info(f"工具 {tool.name} 注册成功")

    def get(self, name: str) -> BaseTool | None:
        """获取工具。

        Args:
            name: 工具名称

        Returns:
            工具实例，不存在时返回 None
        """
        return self._tools.get(name)

    async def execute(self, name: str, **kwargs: Any) -> Any:
        """执行工具。

        Args:
            name: 工具名称
            **kwargs: 工具参数

        Returns:
            工具执行结果

        Raises:
            ValueError: 工具不存在
        """
        tool = self.get(name)
        if tool is None:
            raise ValueError(f"工具 {name} 不存在")

        self._logger.info(f"执行工具 {name}", params=kwargs)
        return await tool.execute(**kwargs)

    def list_tools(self) -> Dict[str, str]:
        """列出所有工具。

        Returns:
            工具字典 {name: description}
        """
        return {name: tool.description for name, tool in self._tools.items()}

    def to_dict_list(self) -> list[Dict[str, str]]:
        """转换为字典列表，用于 API 返回。

        Returns:
            工具信息列表
        """
        return [tool.to_dict() for tool in self._tools.values()]


# 全局注册中心实例
_registry: ToolRegistry | None = None


def get_registry() -> ToolRegistry:
    """获取全局工具注册中心。

    Returns:
        工具注册中心单例
    """
    global _registry
    if _registry is None:
        _registry = ToolRegistry()
    return _registry
