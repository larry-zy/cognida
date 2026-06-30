"""PaddleOCR 引擎。"""

from typing import Any

from core import get_logger
from .base import BaseOCR, OCRResult


class PaddleOCREngine(BaseOCR):
    """PaddleOCR 引擎。"""

    def __init__(self) -> None:
        self._logger = get_logger(__name__)
        self._engine = None

    @property
    def name(self) -> str:
        return "paddleocr"

    def _get_engine(self):
        """延迟加载 PaddleOCR。"""
        if self._engine is None:
            try:
                from paddleocr import PaddleOCR

                self._engine = PaddleOCR(
                    use_angle_cls=True,
                    lang="ch",
                    show_log=False,
                )
                self._logger.info("PaddleOCR 引擎初始化成功")
            except ImportError:
                self._logger.error("PaddleOCR 未安装，请运行: pip install paddleocr")
                raise
        return self._engine

    async def recognize(
        self,
        source: str | bytes,
        language: str = "chi_sim",
        det: bool = True,
        rec: bool = True,
        use_cls: bool = True,
        return_details: bool = False,
        **kwargs: Any,
    ) -> OCRResult:
        """识别图片中的文字。

        Args:
            source: 图片路径或字节内容
            language: 语言
            det: 是否检测
            rec: 是否识别
            use_cls: 是否使用方向分类
            return_details: 是否返回详细信息
            **kwargs: 其他选项

        Returns:
            OCR 识别结果
        """
        try:
            engine = self._get_engine()

            # 处理输入
            if isinstance(source, bytes):
                import tempfile

                with tempfile.NamedTemporaryFile(delete=False, suffix=".png") as f:
                    f.write(source)
                    temp_path = f.name
                source = temp_path

            # 执行识别
            result = engine.ocr(source, cls=use_cls)

            if not result or not result[0]:
                return OCRResult(
                    success=True,
                    text="",
                    blocks=[],
                    confidence=0.0,
                )

            # 处理结果
            text_parts = []
            blocks = []
            total_confidence = 0.0
            count = 0

            for line in result[0]:
                box = line[0]
                text_info = line[1]
                text = text_info[0]
                confidence = text_info[1]

                text_parts.append(text)
                total_confidence += confidence
                count += 1

                if return_details:
                    # 计算边界框
                    x_coords = [p[0] for p in box]
                    y_coords = [p[1] for p in box]
                    blocks.append(
                        {
                            "box": {
                                "x": int(min(x_coords)),
                                "y": int(min(y_coords)),
                                "width": int(max(x_coords) - min(x_coords)),
                                "height": int(max(y_coords) - min(y_coords)),
                            },
                            "text": text,
                            "confidence": confidence,
                        }
                    )

            avg_confidence = total_confidence / count if count > 0 else 0.0

            return OCRResult(
                success=True,
                text="\n".join(text_parts),
                blocks=blocks if return_details else None,
                confidence=avg_confidence,
            )

        except Exception as e:
            self._logger.error("OCR 识别失败", error=str(e))
            return OCRResult(
                success=False,
                text="",
                error=str(e),
            )
