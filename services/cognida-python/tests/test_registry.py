"""工具注册中心测试。"""

import pytest

from tools import BaseTool, get_registry


class DummyTool(BaseTool):
    """测试用工具。"""

    name = "dummy_tool"
    description = "一个测试工具"

    async def execute(self, **kwargs):
        return {"result": "ok"}


class TestToolRegistry:
    """工具注册中心测试。"""

    @pytest.fixture
    def registry(self):
        """获取新的注册中心实例。"""
        from tools.registry import ToolRegistry
        return ToolRegistry()

    def test_register_tool(self, registry) -> None:
        """测试工具注册。"""
        tool = DummyTool()
        registry.register(tool)

        assert registry.get("dummy_tool") is tool
        assert "dummy_tool" in registry.list_tools()

    def test_execute_tool(self, registry) -> None:
        """测试工具执行。"""
        registry.register(DummyTool())

        import asyncio

        result = asyncio.run(registry.execute("dummy_tool"))
        assert result == {"result": "ok"}

    def test_execute_nonexistent_tool(self, registry) -> None:
        """测试执行不存在的工具。"""
        import asyncio

        with pytest.raises(ValueError, match="不存在"):
            asyncio.run(registry.execute("nonexistent"))

    def test_global_registry(self) -> None:
        """测试全局注册中心。"""
        registry = get_registry()
        assert isinstance(registry, get_registry().__class__)
