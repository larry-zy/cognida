"""LLM 供应商配置解析 —— 全项目唯一的 LLM 配置来源（PY-3）。

历史问题：`services/llm` 与 `services/evaluation/llm` 两个同名 `LLMClient`
各自从 settings / os.getenv 读取 provider/api_key/base_url/model，来源分裂、
口径不一致（"split provider/key sources"）。

本模块集中解析上述配置，作为**单一配置源**：两侧客户端均通过
`resolve_llm_config()` 取配置，不再各自散读环境变量。

注意：`get_settings()` 基于 pydantic-settings，字段大小写不敏感，
`settings.llm_api_key`/`llm_base_url`/`llm_model`/`llm_provider` 分别读取
环境变量 `LLM_API_KEY`/`LLM_BASE_URL`/`LLM_MODEL`/`LLM_PROVIDER`，
与历史 `os.getenv("LLM_API_KEY")` 等取到的是同一份来源，故切换后行为等价。
"""

from __future__ import annotations

import os
from dataclasses import dataclass

from config import get_settings


@dataclass
class LLMConfig:
    """解析后的 LLM 连接配置。"""

    provider: str
    api_key: str
    base_url: str | None
    model: str


def resolve_llm_config(
    provider: str | None = None,
    model: str | None = None,
    api_key: str | None = None,
    base_url: str | None = None,
) -> LLMConfig:
    """从单一来源（settings + 约定环境变量）解析 LLM 配置。

    参数优先级：显式入参 > settings（即环境变量/.env）。行为与历史两处
    `LLMClient.__init__` 保持等价，仅把"来源"收敛到此处。

    Args:
        provider: 供应商（openai/anthropic/deepseek）；缺省回落 settings.llm_provider
        model: 模型名；缺省回落 settings.llm_model
        api_key: API 密钥；缺省回落 settings.llm_api_key
        base_url: API 基础地址；缺省回落 settings.llm_base_url

    Returns:
        解析后的 LLMConfig
    """
    settings = get_settings()

    provider = provider or getattr(settings, "llm_provider", "openai") or "openai"
    model = model or getattr(settings, "llm_model", "gpt-4")
    api_key = api_key or getattr(settings, "llm_api_key", "") or ""
    base_url = base_url or getattr(settings, "llm_base_url", None)

    # DeepSeek 兼容 ANTHROPIC_* 格式（与历史 evaluation/LLMClient 行为一致）。
    if provider == "deepseek" and not api_key:
        api_key = os.getenv("ANTHROPIC_AUTH_TOKEN", "")
        if not base_url:
            # 提取 base 地址（去掉 /anthropic 后缀）
            anthropic_base = os.getenv("ANTHROPIC_BASE_URL", "")
            base_url = anthropic_base.replace("/anthropic", "") if anthropic_base else None

    return LLMConfig(provider=provider, api_key=api_key, base_url=base_url, model=model)
