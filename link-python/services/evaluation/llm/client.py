"""评测服务 LLM 客户端（基于 LangChain）。

支持的 Provider:
- openai: GPT-4, GPT-3.5
- anthropic: Claude 3 Opus/Sonnet/Haiku
- deepseek: DeepSeek-Chat, DeepSeek-Coder
"""

import os
from pathlib import Path
from typing import Any

from dotenv import load_dotenv
from config import get_settings

# 加载环境变量（支持不同工作目录）
possible_env_paths = [
    Path(__file__).parent.parent.parent.parent / ".env",  # 项目根目录
    Path.cwd() / ".env",                                    # 当前工作目录
    Path("D:/link/link-python") / ".env",                   # 绝对路径
]
for env_path in possible_env_paths:
    if env_path.exists():
        load_dotenv(env_path)
        break
from langchain_openai import ChatOpenAI
from langchain_anthropic import ChatAnthropic
from langchain_core.language_models import BaseChatModel
from langchain_core.messages import HumanMessage, SystemMessage
from langchain_core.output_parsers import JsonOutputParser
from langchain_core.prompts import ChatPromptTemplate


class LLMClient:
    """统一 LLM 客户端。"""

    def __init__(
        self,
        provider: str = "openai",
        model: str | None = None,
        api_key: str | None = None,
        base_url: str | None = None,
        temperature: float = 0.3,
    ) -> None:
        """初始化 LLM 客户端。

        Args:
            provider: 提供商 (openai, anthropic, etc.)
            model: 模型名称
            api_key: API 密钥
            base_url: API 基础 URL
            temperature: 温度参数
        """
        settings = get_settings()

        self.provider = provider
        self.model = model or getattr(settings, "llm_model", "gpt-4")
        self.api_key = api_key or os.getenv("LLM_API_KEY", "")
        self.base_url = base_url or getattr(settings, "llm_base_url", None)
        self.temperature = temperature

        # DeepSeek 特殊配置（支持 ANTHROPIC_* 格式）
        if provider == "deepseek" and not self.api_key:
            self.api_key = os.getenv("ANTHROPIC_AUTH_TOKEN", "")
            if not self.base_url:
                # 提取 base 地址（去掉 /anthropic 后缀）
                anthropic_base = os.getenv("ANTHROPIC_BASE_URL", "")
                self.base_url = anthropic_base.replace("/anthropic", "") if anthropic_base else None

        self._llm = self._create_llm()

    def _create_llm(self) -> BaseChatModel:
        """创建 LangChain LLM 实例。"""
        common_kwargs = {
            "temperature": self.temperature,
            "timeout": 60,
        }

        if self.provider == "openai":
            return ChatOpenAI(
                model=self.model or "gpt-4",
                api_key=self.api_key,
                **common_kwargs,
            )
        elif self.provider == "anthropic":
            return ChatAnthropic(
                model=self.model or "claude-3-sonnet-20240229",
                api_key=self.api_key or os.getenv("ANTHROPIC_API_KEY", ""),
                **common_kwargs,
            )
        elif self.provider == "deepseek":
            # DeepSeek 兼容 OpenAI API
            return ChatOpenAI(
                model=self.model or "deepseek-chat",
                api_key=self.api_key or os.getenv("DEEPSEEK_API_KEY", ""),
                base_url=self.base_url or "https://api.deepseek.com",
                **common_kwargs,
            )
        else:
            raise ValueError(f"Unsupported provider: {self.provider}")

    def generate(self, prompt: str, system_prompt: str | None = None) -> str:
        """生成文本。

        Args:
            prompt: 用户提示词
            system_prompt: 系统提示词（可选）

        Returns:
            生成的文本
        """
        messages = []
        if system_prompt:
            messages.append(SystemMessage(content=system_prompt))
        messages.append(HumanMessage(content=prompt))

        response = self._llm.invoke(messages)
        return response.content

    async def agenerate(self, prompt: str, system_prompt: str | None = None) -> str:
        """异步生成文本。

        Args:
            prompt: 用户提示词
            system_prompt: 系统提示词（可选）

        Returns:
            生成的文本
        """
        messages = []
        if system_prompt:
            messages.append(SystemMessage(content=system_prompt))
        messages.append(HumanMessage(content=prompt))

        response = await self._llm.ainvoke(messages)
        return response.content

    def generate_json(
        self,
        prompt: str,
        system_prompt: str | None = None,
    ) -> dict[str, Any]:
        """生成 JSON 格式输出。

        Args:
            prompt: 用户提示词
            system_prompt: 系统提示词（可选）

        Returns:
            生成的 JSON 数据
        """
        messages = []
        if system_prompt:
            messages.append(SystemMessage(content=system_prompt))
        messages.append(HumanMessage(content=prompt))

        # 使用 JsonOutputParser
        parser = JsonOutputParser()
        chain = self._llm | parser

        result = chain.invoke(messages)
        return result if isinstance(result, dict) else {}

    async def agenerate_json(
        self,
        prompt: str,
        system_prompt: str | None = None,
    ) -> dict[str, Any]:
        """异步生成 JSON 格式输出。

        Args:
            prompt: 用户提示词
            system_prompt: 系统提示词（可选）

        Returns:
            生成的 JSON 数据
        """
        messages = []
        if system_prompt:
            messages.append(SystemMessage(content=system_prompt))
        messages.append(HumanMessage(content=prompt))

        parser = JsonOutputParser()
        chain = self._llm | parser

        result = await chain.ainvoke(messages)
        return result if isinstance(result, dict) else {}


# 默认客户端实例
_default_client: LLMClient | None = None


def get_llm_client(
    provider: str = "openai",
    model: str | None = None,
) -> LLMClient:
    """获取 LLM 客户端单例。

    Args:
        provider: 提供商
        model: 模型名称

    Returns:
        LLM 客户端实例
    """
    global _default_client
    if _default_client is None:
        _default_client = LLMClient(provider=provider, model=model)
    return _default_client
