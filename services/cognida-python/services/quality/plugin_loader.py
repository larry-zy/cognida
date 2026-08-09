"""插件加载器。

支持从外部目录动态加载评估器和清洗器插件。
"""

import importlib
import importlib.util
import sys
from pathlib import Path
from typing import Any

from .registry import CleanerRegistry, EvaluatorRegistry


class PluginLoadError(Exception):
    """插件加载错误。"""


class PluginLoader:
    """插件加载器。

    从指定目录动态发现和加载评估器、清洗器插件。
    """

    def __init__(self) -> None:
        """初始化插件加载器。"""
        self._loaded_plugins: dict[str, Any] = {}

    def load_from_directory(
        self,
        plugin_dir: str | Path,
        recursive: bool = False,
    ) -> dict[str, Any]:
        """从目录加载插件。

        Args:
            plugin_dir: 插件目录路径
            recursive: 是否递归加载子目录

        Returns:
            加载的插件信息字典

        Raises:
            PluginLoadError: 加载失败时
        """
        plugin_path = Path(plugin_dir)
        if not plugin_path.exists():
            raise PluginLoadError(f"Plugin directory not found: {plugin_dir}")

        if not plugin_path.is_dir():
            raise PluginLoadError(f"Not a directory: {plugin_dir}")

        loaded = {}

        # 查找 Python 文件
        pattern = "**/*.py" if recursive else "*.py"
        for py_file in plugin_path.glob(pattern):
            if py_file.name.startswith("_"):
                continue

            try:
                plugin_info = self._load_module(py_file)
                if plugin_info:
                    loaded[py_file.stem] = plugin_info
            except Exception as e:
                raise PluginLoadError(f"Failed to load {py_file}: {e}") from e

        self._loaded_plugins.update(loaded)
        return loaded

    def _load_module(self, file_path: Path) -> dict[str, Any] | None:
        """加载单个 Python 模块。

        Args:
            file_path: Python 文件路径

        Returns:
            插件信息字典
        """
        module_name = f"quality_plugin_{file_path.stem}"

        # 创建模块规范并加载
        spec = importlib.util.spec_from_file_location(module_name, file_path)
        if spec is None or spec.loader is None:
            return None

        module = importlib.util.module_from_spec(spec)
        sys.modules[module_name] = module

        try:
            spec.loader.exec_module(module)
        except Exception as e:
            raise PluginLoadError(f"Failed to execute module {module_name}: {e}")

        # 收集插件信息
        info = {
            "path": str(file_path),
            "module": module_name,
            "evaluators": [],
            "cleaners": [],
        }

        # 检查模块中定义的类
        for attr_name in dir(module):
            if attr_name.startswith("_"):
                continue

            attr = getattr(module, attr_name)

            # 检查是否是评估器类
            if isinstance(attr, type):
                from .dimensions.base import DimensionEvaluator
                from .unstructured.base import UnstructuredEvaluator

                if issubclass(attr, (DimensionEvaluator, UnstructuredEvaluator)):
                    if attr is not DimensionEvaluator and attr is not UnstructuredEvaluator:
                        info["evaluators"].append(attr_name)
                        # 自动注册
                        if hasattr(attr, "dimension_name") and attr.dimension_name:
                            EvaluatorRegistry.register(attr.dimension_name)(attr)

                from .cleaning.base import Cleaner

                if issubclass(attr, Cleaner) and attr is not Cleaner:
                    info["cleaners"].append(attr_name)
                    # 自动注册
                    if hasattr(attr, "cleaner_name") and attr.cleaner_name:
                        CleanerRegistry.register(attr.cleaner_name)(attr)

        return info if info["evaluators"] or info["cleaners"] else None

    def load_from_file(self, file_path: str | Path) -> dict[str, Any] | None:
        """从单个文件加载插件。

        Args:
            file_path: Python 文件路径

        Returns:
            插件信息字典
        """
        path = Path(file_path)
        if not path.exists() or not path.is_file() or path.suffix != ".py":
            return None

        return self._load_module(path)

    def get_loaded_plugins(self) -> dict[str, Any]:
        """获取已加载的插件列表。

        Returns:
            插件信息字典
        """
        return self._loaded_plugins.copy()

    def unload_plugin(self, plugin_name: str) -> bool:
        """卸载插件。

        Args:
            plugin_name: 插件名称

        Returns:
            是否成功卸载
        """
        if plugin_name in self._loaded_plugins:
            del self._loaded_plugins[plugin_name]
            return True
        return False

    def reload_plugin(self, file_path: str | Path) -> dict[str, Any] | None:
        """重新加载插件。

        Args:
            file_path: Python 文件路径

        Returns:
            插件信息字典
        """
        path = Path(file_path)
        self.unload_plugin(path.stem)
        return self._load_from_file(path)

    def discover_plugins(self, search_dirs: list[str | Path]) -> list[str]:
        """发现可用插件。

        Args:
            search_dirs: 搜索目录列表

        Returns:
            发现的插件文件路径列表
        """
        discovered = []

        for search_dir in search_dirs:
            path = Path(search_dir)
            if path.exists() and path.is_dir():
                for py_file in path.glob("**/*.py"):
                    if not py_file.name.startswith("_"):
                        discovered.append(str(py_file))

        return discovered


# 全局插件加载器实例
_global_loader: PluginLoader | None = None


def get_plugin_loader() -> PluginLoader:
    """获取全局插件加载器实例。"""
    global _global_loader
    if _global_loader is None:
        _global_loader = PluginLoader()
    return _global_loader
