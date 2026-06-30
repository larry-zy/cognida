"""评分器基类。"""

from abc import ABC, abstractmethod
from typing import Any


class Grader(ABC):
    """评分器基类。"""

    @property
    @abstractmethod
    def name(self) -> str:
        """评分器名称。"""
        pass

    @abstractmethod
    async def score(
        self,
        model_output: str,
        reference: str | None = None,
        **kwargs: Any,
    ) -> tuple[float, str, bool]:
        """评分。

        Args:
            model_output: 模型输出
            reference: 参考答案（可选）
            **kwargs: 其他参数

        Returns:
            (分数, 理由, 是否通过) 分数范围 0-100
        """
        pass
