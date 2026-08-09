"""OCR 模块。"""

from .base import BaseOCR
from .paddle_ocr import PaddleOCREngine
from .vlm_ocr import VLMOCREngine

__all__ = ["BaseOCR", "PaddleOCREngine", "VLMOCREngine"]


def get_ocr_engine(engine: str = "paddleocr") -> BaseOCR:
    """获取 OCR 引擎。

    Args:
        engine: OCR 引擎名称 (paddleocr | vlm)

    Returns:
        OCR 引擎实例

    Raises:
        ValueError: 不支持的引擎
    """
    engines = {
        "paddleocr": PaddleOCREngine,
        "vlm": VLMOCREngine,
    }

    engine_class = engines.get(engine.lower())
    if engine_class is None:
        raise ValueError(f"不支持的 OCR 引擎: {engine}")

    return engine_class()
