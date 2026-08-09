"""OpenAI LLM 客户端实现。"""

import json
from typing import Any

import httpx
from .base import LLMClient
from .provider import resolve_llm_config


class OpenAIClient(LLMClient):
    """OpenAI LLM 客户端。"""

    name = "openai"

    def __init__(self) -> None:
        # 统一配置源（PY-3）：与 services.evaluation.llm.LLMClient 共享同一来源
        config = resolve_llm_config()
        self.api_key = config.api_key
        self.base_url = config.base_url or "https://api.openai.com/v1"
        self.model = config.model
        self._client = httpx.AsyncClient(
            base_url=self.base_url,
            headers={"Authorization": f"Bearer {self.api_key}"},
            timeout=60.0,
        )

    async def generate(
        self,
        prompt: str,
        temperature: float = 0.7,
        max_tokens: int = 2000,
        **kwargs: Any,
    ) -> str:
        """生成文本。"""
        response = await self._client.post(
            "/chat/completions",
            json={
                "model": self.model,
                "messages": [{"role": "user", "content": prompt}],
                "temperature": temperature,
                "max_tokens": max_tokens,
            },
        )
        response.raise_for_status()
        data = response.json()
        return data["choices"][0]["message"]["content"]

    async def generate_json(
        self,
        prompt: str,
        schema: dict[str, Any] | None = None,
        **kwargs: Any,
    ) -> dict[str, Any]:
        """生成 JSON 格式输出。"""
        if schema:
            prompt += f"\n\n请严格按照以下 JSON Schema 返回结果：\n{json.dumps(schema, ensure_ascii=False)}"

        response = await self._client.post(
            "/chat/completions",
            json={
                "model": self.model,
                "messages": [{"role": "user", "content": prompt}],
                "response_format": {"type": "json_object"},
                "temperature": kwargs.get("temperature", 0.3),
            },
        )
        response.raise_for_status()
        data = response.json()
        content = data["choices"][0]["message"]["content"]
        return json.loads(content)

    async def generate_with_image(
        self,
        prompt: str,
        image_base64: str,
        image_format: str = "png",
        temperature: float = 0.2,
        max_tokens: int = 4000,
        **kwargs: Any,
    ) -> str:
        """基于图片生成文本（GPT-4V / 兼容 OpenAI vision 协议）。"""
        data_uri = f"data:image/{image_format};base64,{image_base64}"
        model = kwargs.get("model") or self.model
        response = await self._client.post(
            "/chat/completions",
            json={
                "model": model,
                "messages": [
                    {
                        "role": "user",
                        "content": [
                            {"type": "text", "text": prompt},
                            {
                                "type": "image_url",
                                "image_url": {"url": data_uri},
                            },
                        ],
                    }
                ],
                "temperature": temperature,
                "max_tokens": max_tokens,
            },
        )
        response.raise_for_status()
        data = response.json()
        return data["choices"][0]["message"]["content"]

    async def close(self) -> None:
        """关闭客户端。"""
        await self._client.aclose()
