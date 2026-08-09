"""评测 LLM 客户端代理免疫回归测试。

背景：开发机常设置 Clash 等本地代理（all_proxy=socks5://127.0.0.1:7897）。httpx
默认 trust_env=True 会在构造阶段就为 socks5 代理创建传输，一旦缺少 socksio 依赖便
抛 ImportError——历史上这让 llm_judge / llm_safety 在 compute-metrics 的
try/except 中静默失败，评分恒为 null，表现为“LLM 裁判显示和选择都用不了”。

修复：LLMClient 用 trust_env=False 的 httpx 客户端直连，免疫宿主机代理环境。
本测试锁死该行为，防止回归。
"""

import os
from unittest.mock import patch

from services.evaluation.llm.client import LLMClient, _direct_httpx_clients


def test_direct_httpx_clients_ignore_env_proxy():
    """_direct_httpx_clients 返回的同步/异步客户端均忽略环境代理。"""
    sync_client, async_client = _direct_httpx_clients()
    try:
        assert sync_client.trust_env is False
        assert async_client.trust_env is False
    finally:
        sync_client.close()


def test_llmclient_construct_survives_socks_proxy_env():
    """即便设置了 socks5 代理且未装 socksio，构造 DeepSeek 客户端也不应抛异常。"""
    proxy_env = {
        "all_proxy": "socks5://127.0.0.1:7897",
        "http_proxy": "http://127.0.0.1:7897",
        "https_proxy": "http://127.0.0.1:7897",
    }
    with patch.dict(os.environ, proxy_env, clear=False):
        # 历史上这一行会因 socksio 缺失抛 ImportError
        client = LLMClient(
            provider="deepseek",
            model="deepseek-chat",
            api_key="sk-test",
            base_url="https://api.deepseek.com",
        )
        assert client._llm is not None


def test_openai_provider_honors_custom_base_url():
    """provider=openai 时必须透传 base_url，否则第三方 OpenAI 兼容网关请求会打到官方
    api.openai.com、401/连不通导致裁判恒 null（Rank 9）。"""
    client = LLMClient(
        provider="openai",
        model="gpt-4",
        api_key="sk-test",
        base_url="https://gw.example.com/v1",
    )
    assert client._llm.openai_api_base == "https://gw.example.com/v1"
