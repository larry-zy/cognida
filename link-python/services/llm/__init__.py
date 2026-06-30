"""LLM 服务模块。"""

from .base import LLMClient
from .openai import OpenAIClient

__all__ = ["LLMClient", "OpenAIClient"]


def get_llm_client() -> LLMClient:
    """获取 LLM 客户端实例。

    Returns:
        LLM 客户端
    """
    return OpenAIClient()
