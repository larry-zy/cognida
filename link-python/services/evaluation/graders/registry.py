"""评分器注册表。

提供评分器的注册、发现、获取等功能。
"""

import importlib
import importlib.util
import os
import sys
from pathlib import Path
from typing import Any, Callable, Dict, List, Optional, Type, Union

from core import get_logger

from .base import BaseGrader, GraderMode, MetricType


# ============================================================
# 注册表
# ============================================================

class GraderRegistry:
    """评分器注册表。

    负责：
    - 评分器注册和存储
    - 评分器发现（自动扫描目录）
    - 评分器获取和查询
    - 热重载
    """

    def __init__(self) -> None:
        """初始化注册表。"""
        self._logger = get_logger(__name__)
        self._graders: Dict[str, Type[BaseGrader]] = {}
        self._instances: Dict[str, BaseGrader] = {}
        self._file_timestamps: Dict[str, float] = {}

    @property
    def grader_names(self) -> List[str]:
        """获取所有已注册的评分器名称。"""
        return list(self._graders.keys())

    def register(
        self,
        grader_class: Type[BaseGrader],
        name: Optional[str] = None,
        override: bool = False,
    ) -> None:
        """注册评分器类。

        Args:
            grader_class: 评分器类
            name: 注册名称，默认使用类的 name 属性
            override: 是否覆盖已存在的评分器

        Raises:
            ValueError: 评分器已存在且不允许覆盖
            TypeError: 不是有效的评分器类
        """
        # 验证是 BaseGrader 的子类
        if not issubclass(grader_class, BaseGrader):
            raise TypeError(f"{grader_class} is not a subclass of BaseGrader")

        # 获取名称
        if name is None:
            # 尝试实例化以获取 name
            try:
                temp_instance = grader_class()
                name = temp_instance.name
            except Exception:
                name = grader_class.__name__

        # 检查是否已存在
        if name in self._graders and not override:
            raise ValueError(f"Grader '{name}' already registered")

        self._graders[name] = grader_class
        self._logger.info(f"Registered grader", name=name, class_name=grader_class.__name__)

    def register_instance(
        self,
        instance: BaseGrader,
        override: bool = False,
    ) -> None:
        """注册评分器实例。

        Args:
            instance: 评分器实例
            override: 是否覆盖已存在的评分器

        Raises:
            ValueError: 评分器已存在且不允许覆盖
            TypeError: 不是有效的评分器实例
        """
        if not isinstance(instance, BaseGrader):
            raise TypeError(f"{instance} is not an instance of BaseGrader")

        name = instance.name

        if name in self._instances and not override:
            raise ValueError(f"Grader instance '{name}' already registered")

        self._instances[name] = instance
        self._logger.info(f"Registered grader instance", name=name)

    def register_function(
        self,
        func: Callable,
        name: str,
        mode: GraderMode = GraderMode.POINTWISE,
        description: str = "",
        override: bool = False,
    ) -> None:
        """注册函数为评分器。

        Args:
            func: 评分函数
            name: 评分器名称
            mode: 评分器模式
            description: 描述
            override: 是否覆盖
        """
        from .base import function_grader

        decorator = function_grader(name, mode, description)
        grader_class = decorator(func)

        self.register(grader_class.__class__, name=name, override=override)

    def get(self, name: str, default: Optional[BaseGrader] = None) -> Optional[BaseGrader]:
        """获取评分器实例。

        Args:
            name: 评分器名称
            default: 默认值

        Returns:
            评分器实例，如果不存在返回 default
        """
        # 先检查已注册的实例
        if name in self._instances:
            return self._instances[name]

        # 尝试创建新实例
        if name in self._graders:
            try:
                instance = self._graders[name]()
                self._instances[name] = instance
                return instance
            except Exception as e:
                self._logger.error(f"Failed to instantiate grader", name=name, error=str(e))
                return default

        return default

    def get_metadata(self, name: str) -> Optional[Dict[str, Any]]:
        """获取评分器元数据。

        Args:
            name: 评分器名称

        Returns:
            元数据字典，如果不存在返回 None
        """
        instance = self.get(name)
        if instance is None:
            return None
        return instance.get_metadata()

    def list_graders(self) -> List[Dict[str, Any]]:
        """列出所有评分器的元数据。

        Returns:
            元数据列表
        """
        result = []
        for name in self.grader_names:
            metadata = self.get_metadata(name)
            if metadata:
                result.append(metadata)
        return result

    def exists(self, name: str) -> bool:
        """检查评分器是否存在。

        Args:
            name: 评分器名称

        Returns:
            如果存在返回 True
        """
        return name in self._graders or name in self._instances

    def unregister(self, name: str) -> bool:
        """注销评分器。

        Args:
            name: 评分器名称

        Returns:
            如果成功注销返回 True
        """
        removed = False

        if name in self._graders:
            del self._graders[name]
            removed = True

        if name in self._instances:
            del self._instances[name]
            removed = True

        if name in self._file_timestamps:
            del self._file_timestamps[name]

        if removed:
            self._logger.info(f"Unregistered grader", name=name)

        return removed

    def discover_builtin(self) -> int:
        """发现并注册内置评分器。

        Returns:
            注册的评分器数量
        """
        count = 0
        builtin_dir = Path(__file__).parent / "builtin"

        if not builtin_dir.exists():
            return 0

        for py_file in builtin_dir.glob("*.py"):
            if py_file.name.startswith("_"):
                continue

            module_name = f"services.evaluation.graders.builtin.{py_file.stem}"

            try:
                self._load_module(module_name, str(py_file))
                count += 1
            except Exception as e:
                self._logger.warning(
                    f"Failed to load builtin grader",
                    file=str(py_file),
                    error=str(e)
                )

        self._logger.info(f"Discovered builtin graders", count=count)
        return count

    def discover_custom(self, custom_dir: Optional[str | Path] = None) -> int:
        """发现并注册自定义评分器。

        Args:
            custom_dir: 自定义评分器目录，默认为 graders/custom/

        Returns:
            注册的评分器数量
        """
        if custom_dir is None:
            custom_dir = Path(__file__).parent / "custom"
        else:
            custom_dir = Path(custom_dir)

        if not custom_dir.exists():
            return 0

        count = 0

        for py_file in custom_dir.glob("*.py"):
            if py_file.name.startswith("_"):
                continue

            module_name = f"services.evaluation.graders.custom.{py_file.stem}"

            try:
                self._load_module(module_name, str(py_file))
                count += 1
            except Exception as e:
                self._logger.warning(
                    f"Failed to load custom grader",
                    file=str(py_file),
                    error=str(e)
                )

        self._logger.info(f"Discovered custom graders", count=count)
        return count

    def reload_custom(self, custom_dir: Optional[str | Path] = None) -> int:
        """重新加载自定义评分器。

        Args:
            custom_dir: 自定义评分器目录

        Returns:
            重新加载的评分器数量
        """
        if custom_dir is None:
            custom_dir = Path(__file__).parent / "custom"
        else:
            custom_dir = Path(custom_dir)

        count = 0

        for py_file in custom_dir.glob("*.py"):
            if py_file.name.startswith("_"):
                continue

            # 检查文件是否修改
            mtime = os.path.getmtime(py_file)
            file_key = str(py_file)

            if file_key in self._file_timestamps and mtime <= self._file_timestamps[file_key]:
                continue

            module_name = f"services.evaluation.graders.custom.{py_file.stem}"

            # 清除已加载的模块
            if module_name in sys.modules:
                del sys.modules[module_name]

            try:
                self._load_module(module_name, str(py_file))
                self._file_timestamps[file_key] = mtime
                count += 1
            except Exception as e:
                self._logger.warning(
                    f"Failed to reload custom grader",
                    file=str(py_file),
                    error=str(e)
                )

        return count

    def _load_module(self, module_name: str, file_path: str) -> None:
        """加载模块并注册评分器。

        Args:
            module_name: 模块名称
            file_path: 文件路径
        """
        spec = importlib.util.spec_from_file_location(module_name, file_path)
        if spec is None or spec.loader is None:
            raise ImportError(f"Cannot load module from {file_path}")

        module = importlib.util.module_from_spec(spec)
        sys.modules[module_name] = module
        spec.loader.exec_module(module)

    def initialize(self) -> None:
        """初始化注册表，发现所有评分器。"""
        self._logger.info("Initializing grader registry")
        self.discover_builtin()
        self.discover_custom()
        self._logger.info(
            f"Grader registry initialized",
            count=len(self.grader_names)
        )


# ============================================================
# 装饰器
# ============================================================

def register_grader(
    name: Optional[str] = None,
    override: bool = False,
):
    """注册评分器的装饰器。

    Args:
        name: 注册名称，默认使用类的 name 属性
        override: 是否覆盖已存在的评分器

    Returns:
        装饰器函数

    Example:
        @register_grader("my_grader")
        class MyGrader(BaseGrader):
            ...
    """
    def decorator(cls: Type[BaseGrader]) -> Type[BaseGrader]:
        get_global_registry().register(cls, name=name, override=override)
        return cls
    return decorator


# ============================================================
# 全局注册表
# ============================================================

_global_registry: Optional[GraderRegistry] = None


def get_global_registry() -> GraderRegistry:
    """获取全局评分器注册表。"""
    global _global_registry
    if _global_registry is None:
        _global_registry = GraderRegistry()
    return _global_registry


def register_grader_class(
    grader_class: Type[BaseGrader],
    name: Optional[str] = None,
    override: bool = False,
) -> None:
    """注册评分器类的便捷函数。

    Args:
        grader_class: 评分器类
        name: 注册名称
        override: 是否覆盖
    """
    get_global_registry().register(grader_class, name=name, override=override)


def get_grader(name: str, default: Optional[BaseGrader] = None) -> Optional[BaseGrader]:
    """获取评分器的便捷函数。

    Args:
        name: 评分器名称
        default: 默认值

    Returns:
        评分器实例
    """
    return get_global_registry().get(name, default=default)


def list_graders() -> List[Dict[str, Any]]:
    """列出所有评分器的便捷函数。

    Returns:
        元数据列表
    """
    return get_global_registry().list_graders()
