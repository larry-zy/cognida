package knowledge

import "testing"

// 锁定测试〔M13-P4〕：DocumentFormat 跨服务字符串契约。
//
// 跨语言锚点：本测试与 Python 侧
//   services/cognida-python/tests/test_document_contracts.py 的
//   test_document_format_* 互为对照，双侧断言同一 canonical 集合与别名归一。
// wire 值不变——集合或别名改动必须双侧同步。

func TestDocumentFormatCanonicalSet(t *testing.T) {
	want := map[DocumentFormat]struct{}{
		"pdf":  {},
		"docx": {},
		"xlsx": {},
		"pptx": {},
		"csv":  {},
		"md":   {},
		"txt":  {},
		"html": {},
	}

	if len(canonicalDocumentFormats) != len(want) {
		t.Fatalf("canonical 格式数量不符: got %d, want %d", len(canonicalDocumentFormats), len(want))
	}
	for f := range want {
		if _, ok := canonicalDocumentFormats[f]; !ok {
			t.Errorf("缺失 canonical 格式: %q", f)
		}
	}
	for f := range canonicalDocumentFormats {
		if _, ok := want[f]; !ok {
			t.Errorf("多出未预期 canonical 格式: %q", f)
		}
	}
}

func TestNormalizeFormatAliases(t *testing.T) {
	cases := []struct {
		in   string
		want DocumentFormat
	}{
		// 别名 → canonical
		{"doc", DocumentFormatDOCX},
		{"xls", DocumentFormatXLSX},
		{"ppt", DocumentFormatPPTX},
		{"markdown", DocumentFormatMD},
		// canonical 原样返回
		{"pdf", DocumentFormatPDF},
		{"docx", DocumentFormatDOCX},
		{"xlsx", DocumentFormatXLSX},
		{"pptx", DocumentFormatPPTX},
		{"csv", DocumentFormatCSV},
		{"md", DocumentFormatMD},
		{"txt", DocumentFormatTXT},
		{"html", DocumentFormatHTML},
		// 未知值原样返回（不兜底、不校验）
		{"unknown", DocumentFormat("unknown")},
		{"", DocumentFormat("")},
	}
	for _, c := range cases {
		if got := NormalizeFormat(c.in); got != c.want {
			t.Errorf("NormalizeFormat(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 别名映射真源与 canonical 集合一致性：每个别名的归一目标必须是 canonical。
func TestDocumentFormatAliasTargetsAreCanonical(t *testing.T) {
	wantAliases := map[DocumentFormat]DocumentFormat{
		"doc":      "docx",
		"xls":      "xlsx",
		"ppt":      "pptx",
		"markdown": "md",
	}
	if len(documentFormatAliases) != len(wantAliases) {
		t.Fatalf("别名数量不符: got %d, want %d", len(documentFormatAliases), len(wantAliases))
	}
	for alias, canonical := range wantAliases {
		got, ok := documentFormatAliases[alias]
		if !ok {
			t.Errorf("缺失别名: %q", alias)
			continue
		}
		if got != canonical {
			t.Errorf("别名 %q 归一目标 = %q, want %q", alias, got, canonical)
		}
		if _, isCanonical := canonicalDocumentFormats[got]; !isCanonical {
			t.Errorf("别名 %q 的归一目标 %q 不在 canonical 集合内", alias, got)
		}
	}
}

// TestDetectDocumentTypeEndToEnd 端到端锁定 detectDocumentType 的组合映射〔M13-P4〕。
//
// NormalizeFormat 的别名归一与 switch 的分派是两段独立代码，单独锁定二者
// 不能保证其「组合」结果稳定。此表驱动测试直接钉死 format→内部文档类型 的
// 端到端映射，覆盖全部 canonical + 4 别名 + csv + 未知/大小写/带点，
// 防止日后调整别名或某个 case 导致端到端映射静默回归。
func TestDetectDocumentTypeEndToEnd(t *testing.T) {
	svc := &documentProcessorService{}
	cases := map[string]string{
		// canonical
		"pdf":  "pdf",
		"docx": "word",
		"xlsx": "excel",
		"pptx": "ppt",
		"md":   "markdown",
		"txt":  "text",
		"html": "html",
		// 别名归一后落同一分支
		"doc":      "word",
		"xls":      "excel",
		"ppt":      "ppt",
		"markdown": "markdown",
		// canonical 但无内部类型 / 非 canonical → unknown（保留既有行为）
		"csv": "unknown",
		"":    "unknown",
		// 精确匹配：大小写/带点扩展名不归一 → unknown
		"PDF":  "unknown",
		".pdf": "unknown",
	}
	for format, want := range cases {
		got := svc.detectDocumentType(map[string]string{"format": format})
		if got != want {
			t.Errorf("detectDocumentType(format=%q) = %q, want %q", format, got, want)
		}
	}
}
