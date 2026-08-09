"""LLM 服务模块（运行时异步 + 视觉客户端）。

角色划分（PY-3）：
- 本包的 `LLMClient`(ABC)/`OpenAIClient` 是**运行时**异步 httpx 客户端，
  支持视觉输入（`generate_with_image`），供 VLM OCR 等生产链路使用。
- 规范的**多供应商 provider factory**（同步/异步、openai/anthropic/deepseek）
  是 `services.evaluation.llm.LLMClient`。
- 两者现共享唯一配置源 `services.llm.provider.resolve_llm_config`。
- 完整合并（把视觉能力并入规范客户端、统一方法签名）涉及不兼容的接口
  语义（同步 langchain vs 异步视觉 httpx），风险较高，故 **deferred**：
  当前仅统一配置源，不强行合并方法面。
"""

from .base import LLMClient
from .openai import OpenAIClient
from .provider import LLMConfig, resolve_llm_config

__all__ = ["LLMClient", "OpenAIClient", "LLMConfig", "resolve_llm_config"]


def get_llm_client() -> LLMClient:
    """获取 LLM 客户端实例。

    Returns:
        LLM 客户端
    """
    return OpenAIClient()
