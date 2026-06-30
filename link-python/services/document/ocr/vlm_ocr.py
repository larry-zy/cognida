"""VLM (视觉大模型) OCR 引擎。"""

from typing import Any

from core import get_logger
from .base import BaseOCR, OCRResult


class VLMOCREngine(BaseOCR):
    """VLM OCR 引擎。

    使用视觉大模型（如 GPT-4V、Claude 3.5 Sonnet 等）进行 OCR。
    """

    def __init__(self) -> None:
        self._logger = get_logger(__name__)
        self._llm_client = None

    def _get_client(self):
        """获取 LLM 客户端。"""
        if self._llm_client is None:
            from services.llm import get_llm_client

            self._llm_client = get_llm_client()
        return self._llm_client

    @property
    def name(self) -> str:
        return "vlm"

    async def recognize(
        self,
        source: str | bytes,
        language: str = "chi_sim",
        det: bool = True,
        rec: bool = True,
        use_cls: bool = False,
        return_details: bool = False,
        **kwargs: Any,
    ) -> OCRResult:
        """使用视觉大模型识别图片中的文字。

        Args:
            source: 图片路径或字节内容
            language: 语言
            det: 是否检测（不适用）
            rec: 是否识别（不适用）
            use_cls: 是否使用方向分类（不适用）
            return_details: 是否返回详细信息
            **kwargs: 其他选项

        Returns:
            OCR 识别结果
        """
        try:
            import base64
            import os

            # 将图片转换为 base64，并推断图片格式
            if isinstance(source, str):
                with open(source, "rb") as f:
                    raw = f.read()
                image_format = (os.path.splitext(source)[1].lstrip(".") or "png").lower()
            else:
                raw = source
                image_format = self._detect_image_format(raw)

            image_data = base64.b64encode(raw).decode("utf-8")
            # OpenAI 兼容协议使用 jpeg 而非 jpg
            if image_format == "jpg":
                image_format = "jpeg"

            client = self._get_client()

            # 构建提示词
            lang_hint = "中文" if language.startswith("chi") else "英文"
            prompt = f"""请识别这张图片中的所有{lang_hint}文字。

要求：
1. 保持原始的文字排列和格式
2. 只输出识别到的文字，不要添加任何解释
3. 如果有表格，用 Markdown 格式输出
4. 如果无法识别文字，返回空字符串

请开始识别："""

            # 调用支持视觉输入的 LLM 客户端
            try:
                text = await client.generate_with_image(
                    prompt=prompt,
                    image_base64=image_data,
                    image_format=image_format,
                    temperature=kwargs.get("temperature", 0.2),
                    max_tokens=kwargs.get("max_tokens", 4000),
                )
            except NotImplementedError as e:
                self._logger.warning("当前 LLM 客户端不支持图片输入", error=str(e))
                return OCRResult(
                    success=False,
                    text="",
                    error=str(e),
                )

            text = (text or "").strip()
            return OCRResult(
                success=True,
                text=text,
                confidence=1.0 if text else 0.0,
            )

        except Exception as e:
            self._logger.error("VLM OCR 识别失败", error=str(e))
            return OCRResult(
                success=False,
                text="",
                error=str(e),
            )

    @staticmethod
    def _detect_image_format(data: bytes) -> str:
        """根据魔数推断图片格式，默认 png。"""
        if data[:3] == b"\xff\xd8\xff":
            return "jpeg"
        if data[:8] == b"\x89PNG\r\n\x1a\n":
            return "png"
        if data[:6] in (b"GIF87a", b"GIF89a"):
            return "gif"
        if data[:4] == b"RIFF" and data[8:12] == b"WEBP":
            return "webp"
        return "png"
