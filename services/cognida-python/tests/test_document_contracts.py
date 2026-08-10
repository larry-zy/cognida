"""docreader 跨服务字符串契约锁定测试（format / OCR engine / language）。

契约点 M13-P4 / M13-P5。

跨语言锚点：本测试与 Go 侧互为对照，双侧断言同一 wire 取值集合与别名归一——
  * DocumentFormat / normalize_document_format
      ↔ services/cognida-go/internal/service/knowledge/document_format_test.go
  * OCREngine / OCRLanguage
      ↔ services/cognida-go/internal/model/docreader/ocr_test.go
wire 值不变——集合或别名改动必须双侧同步。
"""

from services.document.formats import (
    DocumentFormat,
    OCREngine,
    OCRLanguage,
    DOCUMENT_FORMAT_ALIASES,
    DEFAULT_OCR_ENGINE,
    DEFAULT_OCR_LANGUAGE,
    normalize_document_format,
)


# ---- P4: 文档格式 format ----

def test_document_format_canonical_set():
    """canonical 格式集合锁定（与 Go canonicalDocumentFormats 一致）。"""
    assert {f.value for f in DocumentFormat} == {
        "pdf",
        "docx",
        "xlsx",
        "pptx",
        "csv",
        "md",
        "txt",
        "html",
    }


def test_document_format_named_values():
    """具名成员值锁定（wire 值不变）。"""
    assert DocumentFormat.PDF == "pdf"
    assert DocumentFormat.DOCX == "docx"
    assert DocumentFormat.XLSX == "xlsx"
    assert DocumentFormat.PPTX == "pptx"
    assert DocumentFormat.CSV == "csv"
    assert DocumentFormat.MD == "md"
    assert DocumentFormat.TXT == "txt"
    assert DocumentFormat.HTML == "html"


def test_document_format_aliases_normalize():
    """别名归一（与 Go NormalizeFormat 一致）。"""
    # 别名 → canonical
    assert normalize_document_format("doc") == "docx"
    assert normalize_document_format("xls") == "xlsx"
    assert normalize_document_format("ppt") == "pptx"
    assert normalize_document_format("markdown") == "md"
    # canonical 原样返回
    for f in DocumentFormat:
        assert normalize_document_format(f.value) == f.value
    # 未知值原样返回（不兜底、不校验）
    assert normalize_document_format("unknown") == "unknown"
    assert normalize_document_format("") == ""


def test_document_format_alias_targets_are_canonical():
    """别名映射真源锁定：目标必为 canonical 成员。"""
    assert {k: v.value for k, v in DOCUMENT_FORMAT_ALIASES.items()} == {
        "doc": "docx",
        "xls": "xlsx",
        "ppt": "pptx",
        "markdown": "md",
    }
    canonical = set(DocumentFormat)
    for alias, target in DOCUMENT_FORMAT_ALIASES.items():
        assert target in canonical


# ---- P5: OCR engine / language ----

def test_ocr_engine_values():
    """OCR 引擎取值集合锁定（与 Go canonicalOCREngines 一致）。"""
    assert {e.value for e in OCREngine} == {"paddleocr", "vlm"}
    assert OCREngine.PADDLEOCR == "paddleocr"
    assert OCREngine.VLM == "vlm"


def test_ocr_language_values():
    """OCR 语言取值集合锁定（与 Go canonicalOCRLanguages 一致）。"""
    assert {l.value for l in OCRLanguage} == {"chi_sim", "eng"}
    assert OCRLanguage.CHI_SIM == "chi_sim"
    assert OCRLanguage.ENG == "eng"


def test_ocr_defaults():
    """契约默认值锁定（与 Go DefaultOCREngine / DefaultOCRLanguage 一致）。"""
    assert DEFAULT_OCR_ENGINE == "paddleocr"
    assert DEFAULT_OCR_LANGUAGE == "chi_sim"
