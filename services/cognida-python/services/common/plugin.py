"""插件加载 / 注册的统一契约（PY-5）。

本模块收敛全项目分散的注册表/插件机制的两个关键面：

1. ``RegistryProtocol`` —— 注册表的统一最小契约（结构化 Protocol）。
   现有注册表（``EvaluatorRegistry`` / ``CleanerRegistry`` / ``GraderRegistry``
   / ``ToolRegistry``）无需继承即天然符合 ``register`` + ``get`` 的约定，
   这里给出单一可引用的契约，避免"四套各异且互不相干"的认知负担。

2. ``load_plugin_class`` —— **受控白名单**的动态导入。
   历史安全隐患：quality gRPC 服务的 ``RegisterCustomEvaluator`` /
   ``RegisterCustomCleaner`` 直接 ``importlib.import_module(request.module_path)``，
   由**请求方任意控制**模块路径。import 期副作用 == 任意代码执行（RCE）。
   本函数在 import **之前**先按 ``allowed_prefixes`` 校验模块路径，
   并可选校验加载到的对象是期望基类的子类，杜绝任意模块导入。
"""

from __future__ import annotations

import importlib
from typing import Any, Protocol, runtime_checkable


class PluginSecurityError(Exception):
    """插件加载安全校验失败（模块不在允许列表内、目标非类等）。"""


@runtime_checkable
class RegistryProtocol(Protocol):
    """注册表统一契约（结构化 Protocol，现有注册表无需改动即符合）。"""

    def register(self, *args: Any, **kwargs: Any) -> Any:
        """注册一个条目（类/实例/装饰器，语义由具体注册表定义）。"""
        ...

    def get(self, name: str, *args: Any, **kwargs: Any) -> Any:
        """按名称获取已注册条目，不存在返回 None/默认值。"""
        ...


def _module_allowed(module_path: str, allowed_prefixes: tuple[str, ...]) -> bool:
    """判断 module_path 是否落在允许前缀内（精确匹配或作为子模块）。"""
    return any(
        module_path == prefix or module_path.startswith(prefix + ".")
        for prefix in allowed_prefixes
    )


def load_plugin_class(
    module_path: str,
    class_name: str,
    *,
    allowed_prefixes: tuple[str, ...],
    expected_base: type | tuple[type, ...] | None = None,
) -> type:
    """在白名单约束下动态加载插件类。

    安全要点：**先校验白名单再 import**，避免请求驱动的任意模块导入。

    Args:
        module_path: 模块路径（点分），必须落在 ``allowed_prefixes`` 内
        class_name: 模块内的类名
        allowed_prefixes: 允许的模块路径前缀（不可为空）
        expected_base: 可选，要求加载到的类是其子类（防止注册无关对象）

    Returns:
        加载到的类对象

    Raises:
        PluginSecurityError: 模块不在白名单 / 目标非类 / 非期望基类子类
        ImportError, AttributeError: 白名单内模块导入或取属性失败（沿用原语义）
    """
    if not module_path or not class_name:
        raise PluginSecurityError("module_path 与 class_name 均不能为空")

    if not allowed_prefixes:
        raise PluginSecurityError("未配置允许的模块前缀，拒绝加载")

    if not _module_allowed(module_path, allowed_prefixes):
        raise PluginSecurityError(
            f"模块 {module_path!r} 不在允许列表内（允许前缀: {allowed_prefixes}），拒绝加载"
        )

    module = importlib.import_module(module_path)
    obj = getattr(module, class_name)

    if not isinstance(obj, type):
        raise PluginSecurityError(f"{module_path}.{class_name} 不是可注册的类")

    if expected_base is not None and not issubclass(obj, expected_base):
        raise PluginSecurityError(
            f"{class_name} 不是 {expected_base} 的子类，拒绝注册"
        )

    return obj
