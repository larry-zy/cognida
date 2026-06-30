"""LLM 客户端基类模块。"""

from abc import ABC, abstractmethod
from typing import Any


class LLMClient(ABC):
    """LLM 客户端基类。"""

    @property
    @abstractmethod
    def name(self) -> str:
        """客户端名称。"""
        pass

    @abstractmethod
    async def generate(
        self,
        prompt: str,
        temperature: float = 0.7,
        max_tokens: int = 2000,
        **kwargs: Any,
    ) -> str:
        """生成文本。

        Args:
            prompt: 提示词
            temperature: 温度参数
            max_tokens: 最大 token 数
            **kwargs: 其他参数

        Returns:
            生成的文本
        """
        pass

    @abstractmethod
    async def generate_json(
        self,
        prompt: str,
        schema: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> dict[str, Any]:
        """生成 JSON 格式输出。

        Args:
            prompt: 提示词
            schema: JSON schema（可选）
            **kwargs: 其他参数

        Returns:
            生成的 JSON 数据
        """
        pass

    async def generate_with_image(
        self,
        prompt: str,
        image_base64: str,
        image_format: str = "png",
        temperature: float = 0.2,
        max_tokens: int = 4000,
        **kwargs: Any,
    ) -> str:
        """基于图片生成文本（多模态/视觉）。

        默认未实现，由支持视觉输入的客户端（如 OpenAI GPT-4V）覆盖。

        Args:
            prompt: 提示词
            image_base64: 图片的 base64 编码（不含 data URI 前缀）
            image_format: 图片格式（png/jpeg/...），用于构造 data URI
            temperature: 温度参数
            max_tokens: 最大 token 数
            **kwargs: 其他参数

        Returns:
            生成的文本

        Raises:
            NotImplementedError: 当前客户端不支持视觉输入
        """
        raise NotImplementedError(
            f"LLM 客户端 {self.name} 不支持图片输入"
        )
