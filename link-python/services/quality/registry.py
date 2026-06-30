"""评估器和清洗器注册表。"""

from typing import Any, Callable, TypeVar

T = TypeVar("T")


class EvaluatorRegistry:
    """评估器注册表。

    管理所有结构化和非结构化评估器的注册和获取。
    """

    _evaluators: dict[str, type[Any]] = {}
    _instances: dict[str, Any] = {}

    @classmethod
    def register(cls, name: str) -> Callable[[T], T]:
        """注册评估器装饰器。

        Args:
            name: 评估器名称

        Returns:
            装饰器函数

        Example:
            @evaluator_registry.register("completeness")
            class CompletenessEvaluator(DimensionEvaluator):
                ...
        """

        def decorator(evaluator_class: T) -> T:
            cls._evaluators[name] = evaluator_class
            return evaluator_class

        return decorator

    @classmethod
    def get(cls, name: str) -> type[Any] | None:
        """获取评估器类。

        Args:
            name: 评估器名称

        Returns:
            评估器类，如果不存在返回 None
        """
        return cls._evaluators.get(name)

    @classmethod
    def get_instance(cls, name: str) -> Any | None:
        """获取评估器实例（单例）。

        Args:
            name: 评估器名称

        Returns:
            评估器实例，如果不存在返回 None
        """
        if name not in cls._instances:
            evaluator_class = cls.get(name)
            if evaluator_class is None:
                return None
            cls._instances[name] = evaluator_class()
        return cls._instances[name]

    @classmethod
    def list_evaluators(cls) -> list[str]:
        """列出所有已注册的评估器名称。

        Returns:
            评估器名称列表
        """
        return list(cls._evaluators.keys())

    @classmethod
    def clear(cls) -> None:
        """清空注册表（主要用于测试）。"""
        cls._evaluators.clear()
        cls._instances.clear()


class CleanerRegistry:
    """清洗器注册表。

    管理所有数据清洗器的注册和获取。
    """

    _cleaners: dict[str, type[Any]] = {}
    _instances: dict[str, Any] = {}

    @classmethod
    def register(cls, name: str) -> Callable[[T], T]:
        """注册清洗器装饰器。

        Args:
            name: 清洗器名称

        Returns:
            装饰器函数

        Example:
            @cleaner_registry.register("text_cleaner")
            class TextCleaner(Cleaner):
                ...
        """

        def decorator(cleaner_class: T) -> T:
            cls._cleaners[name] = cleaner_class
            return cleaner_class

        return decorator

    @classmethod
    def get(cls, name: str) -> type[Any] | None:
        """获取清洗器类。

        Args:
            name: 清洗器名称

        Returns:
            清洗器类，如果不存在返回 None
        """
        return cls._cleaners.get(name)

    @classmethod
    def get_instance(cls, name: str) -> Any | None:
        """获取清洗器实例（单例）。

        Args:
            name: 清洗器名称

        Returns:
            清洗器实例，如果不存在返回 None
        """
        if name not in cls._instances:
            cleaner_class = cls.get(name)
            if cleaner_class is None:
                return None
            cls._instances[name] = cleaner_class()
        return cls._instances[name]

    @classmethod
    def list_cleaners(cls) -> list[str]:
        """列出所有已注册的清洗器名称。

        Returns:
            清洗器名称列表
        """
        return list(cls._cleaners.keys())

    @classmethod
    def clear(cls) -> None:
        """清空注册表（主要用于测试）。"""
        cls._cleaners.clear()
        cls._instances.clear()


# 便捷装饰器
def register_evaluator(name: str) -> Callable[[T], T]:
    """注册评估器的便捷装饰器。

    Args:
        name: 评估器名称

    Returns:
        装饰器函数
    """
    return EvaluatorRegistry.register(name)


def register_cleaner(name: str) -> Callable[[T], T]:
    """注册清洗器的便捷装饰器。

    Args:
        name: 清洗器名称

    Returns:
        装饰器函数
    """
    return CleanerRegistry.register(name)
