"""analytics 工具名跨服务契约锁定测试〔M13 P1〕。

守护 ToolName 单一真源与各 *_tool.py / registry 注册键一致，防止 wire 值漂移。

跨语言对照锚点：下方 EXPECTED_TOOL_NAMES 的 7 个字面量必须与 Go 侧锁定测试
services/cognida-go/internal/service/agent/tools/contract_test.go 的
expectedMCPToolNames 逐字一致——两侧各自写死同一份字面量，任一侧改 wire 值都会被打红。
"""

import pytest

from tools.analytics import ANALYTICS_TOOLS, ToolName
from tools.bootstrap import register_default_tools
from tools.registry import ToolRegistry

# 与 Go 侧 expectedMCPToolNames 对照的 7 个工具名字面量。
EXPECTED_TOOL_NAMES = {
    "data_describe",
    "data_trend",
    "data_anomaly",
    "data_correlation",
    "data_insight",
    "data_comparison",
    "data_attribution",
}


@pytest.mark.unit
class TestToolNameContract:
    """ToolName 枚举与工具/注册键的一致性。"""

    def test_enum_values_match_expected(self) -> None:
        """ToolName 取值集合 == 期望 7 个字面量。"""
        assert {member.value for member in ToolName} == EXPECTED_TOOL_NAMES

    def test_each_tool_name_in_enum(self) -> None:
        """各 *_tool.py 的 name 属性 ∈ ToolName 取值集合。"""
        enum_values = {member.value for member in ToolName}
        for tool_cls in ANALYTICS_TOOLS:
            name = tool_cls().name
            assert name in enum_values, f"{tool_cls.__name__}.name={name!r} 不在 ToolName 内"

    def test_tool_names_cover_enum(self) -> None:
        """7 个工具的 name 恰好覆盖 ToolName 全集（无遗漏、无多余）。"""
        tool_names = {tool_cls().name for tool_cls in ANALYTICS_TOOLS}
        assert tool_names == {member.value for member in ToolName}

    def test_registry_keys_match_enum(self) -> None:
        """registry 注册键集合 == ToolName 取值集合（key 字面值不变）。"""
        registry = ToolRegistry()
        register_default_tools(registry)
        assert set(registry.list_tools().keys()) == {member.value for member in ToolName}
