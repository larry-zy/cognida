"""工具注册引导。

修复 ``ToolRegistry`` 启动时未注册任何工具的缺口：在 MCP server 初始化时
调用 ``register_default_tools`` 一次，把内置工具注册进全局 registry。

设计要点：
- 幂等：重复调用不会重复注册（已存在则跳过）。
- 延迟 import：在函数内 import analytics 工具，避免模块级循环 import。
"""

from __future__ import annotations

from core import get_logger
from tools.registry import ToolRegistry, get_registry

logger = get_logger(__name__)


def register_default_tools(registry: ToolRegistry | None = None) -> ToolRegistry:
    """把内置默认工具注册进 registry（幂等）。

    Args:
        registry: 目标 registry；省略则使用全局单例。

    Returns:
        ToolRegistry: 完成注册的 registry。
    """
    registry = registry or get_registry()

    # 延迟 import，避免与 analytics 工具的模块级循环依赖
    from tools.analytics import ANALYTICS_TOOLS

    registered = 0
    for tool_cls in ANALYTICS_TOOLS:
        tool = tool_cls()
        if registry.get(tool.name) is not None:
            continue  # 幂等：已注册则跳过
        registry.register(tool)
        registered += 1

    logger.info(
        "default tools registered: %d new, %d total",
        registered,
        len(registry.to_dict_list()),
    )
    return registry
