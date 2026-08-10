package docreader

import "testing"

// 锁定测试〔M13-P5〕：OCR 引擎 / 语言跨服务字符串契约。
//
// 跨语言锚点：本测试与 Python 侧
//   services/cognida-python/tests/test_document_contracts.py 的
//   test_ocr_engine_* / test_ocr_language_* 互为对照，双侧断言同一 wire 取值集合。
// wire 值不变——集合改动必须双侧同步。

func TestOCREngineValues(t *testing.T) {
	want := map[OCREngine]struct{}{
		"paddleocr": {},
		"vlm":       {},
	}
	if len(canonicalOCREngines) != len(want) {
		t.Fatalf("OCR 引擎数量不符: got %d, want %d", len(canonicalOCREngines), len(want))
	}
	for e := range want {
		if _, ok := canonicalOCREngines[e]; !ok {
			t.Errorf("缺失 OCR 引擎: %q", e)
		}
	}
	for e := range canonicalOCREngines {
		if _, ok := want[e]; !ok {
			t.Errorf("多出未预期 OCR 引擎: %q", e)
		}
	}
	// 具名常量值锁定
	if OCREnginePaddleOCR != "paddleocr" {
		t.Errorf("OCREnginePaddleOCR = %q, want paddleocr", OCREnginePaddleOCR)
	}
	if OCREngineVLM != "vlm" {
		t.Errorf("OCREngineVLM = %q, want vlm", OCREngineVLM)
	}
}

func TestOCRLanguageValues(t *testing.T) {
	want := map[OCRLanguage]struct{}{
		"chi_sim": {},
		"eng":     {},
	}
	if len(canonicalOCRLanguages) != len(want) {
		t.Fatalf("OCR 语言数量不符: got %d, want %d", len(canonicalOCRLanguages), len(want))
	}
	for l := range want {
		if _, ok := canonicalOCRLanguages[l]; !ok {
			t.Errorf("缺失 OCR 语言: %q", l)
		}
	}
	for l := range canonicalOCRLanguages {
		if _, ok := want[l]; !ok {
			t.Errorf("多出未预期 OCR 语言: %q", l)
		}
	}
	if OCRLanguageChiSim != "chi_sim" {
		t.Errorf("OCRLanguageChiSim = %q, want chi_sim", OCRLanguageChiSim)
	}
	if OCRLanguageEng != "eng" {
		t.Errorf("OCRLanguageEng = %q, want eng", OCRLanguageEng)
	}
}

func TestOCRDefaults(t *testing.T) {
	if DefaultOCREngine != OCREnginePaddleOCR {
		t.Errorf("DefaultOCREngine = %q, want %q", DefaultOCREngine, OCREnginePaddleOCR)
	}
	if DefaultOCRLanguage != OCRLanguageChiSim {
		t.Errorf("DefaultOCRLanguage = %q, want %q", DefaultOCRLanguage, OCRLanguageChiSim)
	}
}
