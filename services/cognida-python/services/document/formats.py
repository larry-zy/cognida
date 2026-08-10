"""docreader 跨服务字符串契约（format / OCR engine / language）。

契约点 M13-P4（文档格式 format）与 M13-P5（OCR 引擎 engine / 语言 language）。

这些取值是 Go↔Python 之间的 wire 契约：契约仅收敛为「类型化枚举 + 常量对照 +
锁定测试」，wire 字符串值一律**保持不变**（向后兼容），且不升级 proto。

跨语言锚点：本模块与 Go 侧一一对应——
  * DocumentFormat / normalize_document_format
      ↔ services/cognida-go/internal/service/knowledge/document_format.go
  * OCREngine / OCRLanguage
      ↔ services/cognida-go/internal/model/docreader/ocr.go
任一侧改动需同步另一侧并更新双侧锁定测试
（Python: tests/test_document_contracts.py；Go: document_format_test.go / ocr_test.go）。
"""

from enum import StrEnum


class DocumentFormat(StrEnum):
    """文档格式 canonical wire 值〔M13-P4〕。

    成员既是枚举又是 ``str``（StrEnum），故可直接作 dict key / 比较，
    字面结果与原字符串一致。别名（doc/xls/ppt/markdown）不作枚举成员，
    统一经 :func:`normalize_document_format` 归一到 canonical。
    """

    PDF = "pdf"
    DOCX = "docx"
    XLSX = "xlsx"
    PPTX = "pptx"
    CSV = "csv"
    MD = "md"
    TXT = "txt"
    HTML = "html"


# 别名（兼容历史 wire 值）→ canonical。跨语言锚点：Go documentFormatAliases。
DOCUMENT_FORMAT_ALIASES: dict[str, DocumentFormat] = {
    "doc": DocumentFormat.DOCX,
    "xls": DocumentFormat.XLSX,
    "ppt": DocumentFormat.PPTX,
    "markdown": DocumentFormat.MD,
}


def normalize_document_format(value: str) -> str:
    """把别名 wire 值归一为 canonical 格式字符串。

    已是 canonical 或未知值原样返回（不兜底、不校验），与 Go NormalizeFormat 行为一致。
    返回 ``str``（StrEnum 成员本身即 str），便于直接作 dict key / 比较。
    """
    return str(DOCUMENT_FORMAT_ALIASES.get(value, value))


class OCREngine(StrEnum):
    """OCR 引擎 wire 值〔M13-P5〕。跨语言锚点：Go OCREngine。"""

    PADDLEOCR = "paddleocr"
    VLM = "vlm"


class OCRLanguage(StrEnum):
    """OCR 语言 wire 值〔M13-P5〕。跨语言锚点：Go OCRLanguage。"""

    CHI_SIM = "chi_sim"
    ENG = "eng"


# 契约默认值（与 Go DefaultOCREngine / DefaultOCRLanguage 一致，服务端兜底用）。
DEFAULT_OCR_ENGINE = OCREngine.PADDLEOCR
DEFAULT_OCR_LANGUAGE = OCRLanguage.CHI_SIM
