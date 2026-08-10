package knowledge

// ========================================
// 文档格式（DocumentFormat）跨服务字符串契约〔M13-P4〕
// ========================================
//
// DocumentFormat 是 docreader「文档格式 format」的 Go↔Python 跨服务 wire 契约。
// 契约仅收敛为「类型化 + 常量对照 + 锁定测试」，wire 字符串值一律保持不变（向后兼容），
// 且不升级 proto（不做 string→enum）。
//
// 跨语言锚点：本常量集与别名归一策略必须与 Python 侧
//   services/cognida-python/services/document/formats.py 的 DocumentFormat(StrEnum)
//   及 normalize_document_format() 一一对应。任一侧改动需同步另一侧并更新双侧锁定测试
//   （Go: document_format_test.go；Python: tests/test_document_contracts.py）。

// DocumentFormat 文档格式 wire 值（类型化字符串）。
type DocumentFormat string

// canonical 格式常量（wire 值不变）。
const (
	DocumentFormatPDF  DocumentFormat = "pdf"
	DocumentFormatDOCX DocumentFormat = "docx"
	DocumentFormatXLSX DocumentFormat = "xlsx"
	DocumentFormatPPTX DocumentFormat = "pptx"
	DocumentFormatCSV  DocumentFormat = "csv"
	DocumentFormatMD   DocumentFormat = "md"
	DocumentFormatTXT  DocumentFormat = "txt"
	DocumentFormatHTML DocumentFormat = "html"
)

// 别名格式常量（兼容历史 wire 值，归一到对应 canonical）。
const (
	DocumentFormatDOC      DocumentFormat = "doc"      // → docx
	DocumentFormatXLS      DocumentFormat = "xls"      // → xlsx
	DocumentFormatPPT      DocumentFormat = "ppt"      // → pptx
	DocumentFormatMarkdown DocumentFormat = "markdown" // → md
)

// canonicalDocumentFormats 全部 canonical 格式集合（锁定测试真源）。
var canonicalDocumentFormats = map[DocumentFormat]struct{}{
	DocumentFormatPDF:  {},
	DocumentFormatDOCX: {},
	DocumentFormatXLSX: {},
	DocumentFormatPPTX: {},
	DocumentFormatCSV:  {},
	DocumentFormatMD:   {},
	DocumentFormatTXT:  {},
	DocumentFormatHTML: {},
}

// documentFormatAliases 别名 → canonical 映射（锁定测试真源）。
var documentFormatAliases = map[DocumentFormat]DocumentFormat{
	DocumentFormatDOC:      DocumentFormatDOCX,
	DocumentFormatXLS:      DocumentFormatXLSX,
	DocumentFormatPPT:      DocumentFormatPPTX,
	DocumentFormatMarkdown: DocumentFormatMD,
}

// NormalizeFormat 把别名 wire 值归一为 canonical 格式。
// 已是 canonical 或未知值原样返回（保持既有行为，不做校验/兜底）。
func NormalizeFormat(format string) DocumentFormat {
	f := DocumentFormat(format)
	if canonical, ok := documentFormatAliases[f]; ok {
		return canonical
	}
	return f
}
