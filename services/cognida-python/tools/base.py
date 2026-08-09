"""工具基类模块。"""

from abc import ABC, abstractmethod
from typing import Any


class BaseTool(ABC):
    """工具基类，所有工具必须继承此类。

    支持 MCP 协议的工具定义。
    """

    @property
    @abstractmethod
    def name(self) -> str:
        """工具名称，用于工具注册和调用。"""
        pass

    @property
    @abstractmethod
    def description(self) -> str:
        """工具描述，用于让 LLM 理解工具用途。"""
        pass

    @property
    def input_schema(self) -> dict[str, Any]:
        """工具输入参数的 JSON Schema。

        用于 MCP 协议的 tools/list 响应。
        子类可以覆盖此属性以提供详细的参数定义。

        Returns:
            JSON Schema 格式的输入定义
        """
        return {
            "type": "object",
            "properties": {},
        }

    @abstractmethod
    async def execute(self, **kwargs: Any) -> Any:
        """执行工具逻辑。

        Args:
            **kwargs: 工具输入参数

        Returns:
            工具执行结果
        """
        pass

    def to_dict(self) -> dict[str, str]:
        """转换为字典格式，用于工具注册。

        Returns:
            包含 name 和 description 的字典
        """
        return {"name": self.name, "description": self.description}
