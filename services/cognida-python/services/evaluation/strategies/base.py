"""评测策略基类。

定义评测策略的接口。
"""

from abc import ABC, abstractmethod
from typing import Any, Dict, List, Optional

from pydantic import BaseModel, Field


class StrategyConfig(BaseModel):
    """策略配置。"""
    name: str = Field(..., description="策略名称")
    params: Dict[str, Any] = Field(default_factory=dict, description="策略参数")


class BaseStrategy(ABC):
    """评测策略基类。

    定义如何组合和执行评分器。
    """

    def __init__(
        self,
        name: str,
        description: str = "",
        config: Optional[StrategyConfig] = None,
    ):
        """初始化策略。

        Args:
            name: 策略名称
            description: 策略描述
            config: 策略配置
        """
        self.name = name
        self.description = description
        self.config = config or StrategyConfig(name=name)

    @abstractmethod
    async def execute(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        graders: Optional[List[str]] = None,
        **kwargs
    ) -> Dict[str, Any]:
        """执行策略。

        Args:
            query: 问题
            response: 回答
            reference: 参考答案
            context: 上下文
            graders: 评分器名称列表

        Returns:
            策略执行结果
        """
        pass

    def get_metadata(self) -> Dict[str, Any]:
        """获取策略元数据。"""
        return {
            "name": self.name,
            "description": self.description,
            "config": self.config.model_dump(),
        }
