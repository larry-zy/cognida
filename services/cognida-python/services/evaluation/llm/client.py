"""评测服务 LLM 客户端（基于 LangChain）—— 规范的多供应商 LLM 抽象（PY-3 canonical）。

支持的 Provider:
- openai: GPT-4, GPT-3.5
- anthropic: Claude 3 Opus/Sonnet/Haiku
- deepseek: DeepSeek-Chat, DeepSeek-Coder

本类是全项目**规范的** LLM provider factory：多供应商 + 同步/异步。
配置来源统一收敛到 `services.llm.provider.resolve_llm_config`（单一配置源），
不再各自散读 os.getenv/settings。
"""

import os
from pathlib import Path
from typing import Any

from dotenv import load_dotenv

from services.llm.provider import resolve_llm_config

# 加载环境变量（支持不同工作目录）
possible_env_paths = [
    Path(__file__).parent.parent.parent.parent / ".env",  # 项目根目录
    Path.cwd() / ".env",                                    # 当前工作目录
    Path("D:/cognida/cognida-python") / ".env",                   # 绝对路径
]
for env_path in possible_env_paths:
    if env_path.exists():
        load_dotenv(env_path)
        break
import httpx
from langchain_openai import ChatOpenAI
from langchain_anthropic import ChatAnthropic
from langchain_core.language_models import BaseChatModel
from langchain_core.messages import HumanMessage, SystemMessage
from langchain_core.output_parsers import JsonOutputParser


def _direct_httpx_clients(timeout: float = 60.0) -> tuple[httpx.Client, httpx.AsyncClient]:
    """构造忽略环境代理的 httpx 客户端（同步 + 异步）。

    评测裁判走的是 DeepSeek 等自有 LLM 接口，不应经过开发机上的 SOCKS/HTTP
    代理（如 Clash 的 all_proxy=socks5://…）。而 httpx 默认 trust_env=True 会在
    构造阶段就为 socks5 代理创建传输，一旦缺少 socksio 依赖便直接抛
    ImportError——这正是历史上 llm_judge 静默失败、评分恒为 null 的根因。
    显式 trust_env=False 让裁判直连、免疫宿主机代理环境，稳定可复现。
    """
    return (
        httpx.Client(trust_env=False, timeout=timeout),
        httpx.AsyncClient(trust_env=False, timeout=timeout),
    )


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
        # 统一配置源（含 DeepSeek 的 ANTHROPIC_* 兼容）：见 services.llm.provider
        config = resolve_llm_config(
            provider=provider,
            model=model,
            api_key=api_key,
            base_url=base_url,
        )
        self.provider = config.provider
        self.model = config.model
        self.api_key = config.api_key
        self.base_url = config.base_url
        self.temperature = temperature

        self._llm = self._create_llm()

    def _create_llm(self) -> BaseChatModel:
        """创建 LangChain LLM 实例。"""
        common_kwargs = {
            "temperature": self.temperature,
            "timeout": 60,
        }

        if self.provider == "openai":
            sync_client, async_client = _direct_httpx_clients()
            # 透传 base_url：provider=openai 走自建网关/兼容端点时，LLM_BASE_URL 必须生效，
            # 否则被静默丢弃、强制打到 api.openai.com，裁判评分恒 None（与 deepseek 分支口径对齐）。
            openai_kwargs = dict(common_kwargs)
            if self.base_url:
                openai_kwargs["base_url"] = self.base_url
            return ChatOpenAI(
                model=self.model or "gpt-4",
                api_key=self.api_key,
                http_client=sync_client,
                http_async_client=async_client,
                **openai_kwargs,
            )
        elif self.provider == "anthropic":
            # ChatAnthropic 不接受 http_client 参数，无法用同款方式注入直连客户端；
            # 评测裁判当前统一走 deepseek/openai，此分支保持原状。
            return ChatAnthropic(
                model=self.model or "claude-3-sonnet-20240229",
                api_key=self.api_key or os.getenv("ANTHROPIC_API_KEY", ""),
                **common_kwargs,
            )
        elif self.provider == "deepseek":
            # DeepSeek 兼容 OpenAI API
            sync_client, async_client = _direct_httpx_clients()
            return ChatOpenAI(
                model=self.model or "deepseek-chat",
                api_key=self.api_key or os.getenv("DEEPSEEK_API_KEY", ""),
                base_url=self.base_url or "https://api.deepseek.com",
                http_client=sync_client,
                http_async_client=async_client,
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
