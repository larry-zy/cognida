"""OCR 模块。"""

from .base import BaseOCR
from .paddle_ocr import PaddleOCREngine
from .vlm_ocr import VLMOCREngine
from ..formats import OCREngine

__all__ = ["BaseOCR", "PaddleOCREngine", "VLMOCREngine"]


def get_ocr_engine(engine: str = OCREngine.PADDLEOCR) -> BaseOCR:
    """获取 OCR 引擎。

    Args:
        engine: OCR 引擎名称 (paddleocr | vlm)

    Returns:
        OCR 引擎实例

    Raises:
        ValueError: 不支持的引擎
    """
    # key 用 OCREngine.value（wire 契约〔M13-P5〕），字面结果与原字符串一致。
    engines = {
        OCREngine.PADDLEOCR.value: PaddleOCREngine,
        OCREngine.VLM.value: VLMOCREngine,
    }

    engine_class = engines.get(engine.lower())
    if engine_class is None:
        raise ValueError(f"不支持的 OCR 引擎: {engine}")

    return engine_class()
