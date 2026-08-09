"""评测策略模块。"""

from .base import BaseStrategy, StrategyConfig
from .zero_shot import ZeroShotStrategy
from .data_driven import DataDrivenStrategy
from .ensemble import EnsembleStrategy, MajorityVotingStrategy
from .conditional import ConditionalStrategy, RouterStrategy

__all__ = [
    "BaseStrategy",
    "StrategyConfig",
    "ZeroShotStrategy",
    "DataDrivenStrategy",
    "EnsembleStrategy",
    "MajorityVotingStrategy",
    "ConditionalStrategy",
    "RouterStrategy",
]

# 策略工厂
_strategy_registry: dict = {
    "zero_shot": ZeroShotStrategy,
    "data_driven": DataDrivenStrategy,
    "ensemble": EnsembleStrategy,
    "majority_voting": MajorityVotingStrategy,
    "conditional": ConditionalStrategy,
    "router": RouterStrategy,
}


def get_strategy(name: str, config=None) -> BaseStrategy:
    """获取策略实例。"""
    strategy_class = _strategy_registry.get(name)
    if strategy_class is None:
        raise ValueError(f"Unknown strategy: {name}")
    return strategy_class(config)


def list_strategies() -> list:
    """列出所有可用策略。"""
    return list(_strategy_registry.keys())
