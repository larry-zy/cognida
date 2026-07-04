"""tools.bootstrap 注册引导测试。"""

import pytest

from tools.analytics import ANALYTICS_TOOLS
from tools.bootstrap import register_default_tools
from tools.registry import ToolRegistry

EXPECTED_TOOLS = {
    "data_describe",
    "data_trend",
    "data_anomaly",
    "data_correlation",
    "data_insight",
    "data_attribution",
    "data_comparison",
}


@pytest.mark.unit
class TestRegisterDefaultTools:
    """register_default_tools 行为测试。"""

    def test_registers_all_analytics_tools(self) -> None:
        """注册后 registry 含全部 analytics 工具。"""
        registry = ToolRegistry()
        register_default_tools(registry)

        names = set(registry.list_tools().keys())
        assert EXPECTED_TOOLS.issubset(names)
        assert len(EXPECTED_TOOLS) == len(ANALYTICS_TOOLS)

    def test_idempotent_no_duplicates(self) -> None:
        """重复调用不报错且不产生重复。"""
        registry = ToolRegistry()
        register_default_tools(registry)
        first = dict(registry.list_tools())

        register_default_tools(registry)  # 第二次
        second = dict(registry.list_tools())

        assert first.keys() == second.keys()
        assert len(second) == len(EXPECTED_TOOLS)

    def test_returns_registry(self) -> None:
        """返回传入的 registry 实例。"""
        registry = ToolRegistry()
        assert register_default_tools(registry) is registry

    def test_defaults_to_global_registry(self) -> None:
        """省略参数时使用全局单例。"""
        result = register_default_tools()
        names = set(result.list_tools().keys())
        assert EXPECTED_TOOLS.issubset(names)
