package docreader

// ========================================
// OCR 引擎 / 语言跨服务字符串契约〔M13-P5〕
// ========================================
//
// OCREngine / OCRLanguage 是 docreader「OCR 引擎 engine / 语言 language」的
// Go↔Python 跨服务 wire 契约。契约仅收敛为「类型化 + 常量对照 + 锁定测试」，
// wire 字符串值一律保持不变（向后兼容），且不升级 proto。
//
// 跨语言锚点：本常量集必须与 Python 侧
//   services/cognida-python/services/document/formats.py 的
//   OCREngine(StrEnum) / OCRLanguage(StrEnum) 一一对应。任一侧改动需同步另一侧并
//   更新双侧锁定测试（Go: ocr_test.go；Python: tests/test_document_contracts.py）。
//
// 运行时默认值由 Python 服务端兜底（servicer.py: request.engine or "paddleocr"、
// request.language or "chi_sim"）；Go 侧以 Default* 常量声明同一契约默认，避免字面量散落。

// OCREngine OCR 引擎 wire 值（类型化字符串）。
type OCREngine string

const (
	OCREnginePaddleOCR OCREngine = "paddleocr"
	OCREngineVLM       OCREngine = "vlm"
)

// OCRLanguage OCR 语言 wire 值（类型化字符串）。
type OCRLanguage string

const (
	OCRLanguageChiSim OCRLanguage = "chi_sim"
	OCRLanguageEng    OCRLanguage = "eng"
)

// 契约默认值（与 Python 服务端兜底保持一致）。
const (
	DefaultOCREngine   = OCREnginePaddleOCR
	DefaultOCRLanguage = OCRLanguageChiSim
)

// canonicalOCREngines / canonicalOCRLanguages 为锁定测试真源。
var (
	canonicalOCREngines = map[OCREngine]struct{}{
		OCREnginePaddleOCR: {},
		OCREngineVLM:       {},
	}
	canonicalOCRLanguages = map[OCRLanguage]struct{}{
		OCRLanguageChiSim: {},
		OCRLanguageEng:    {},
	}
)
