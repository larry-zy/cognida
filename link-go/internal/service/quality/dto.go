// Package quality 数据质量应用服务：封装 Python 质量 gRPC 服务，
// 对上层暴露干净的 DTO，并把质检结果落库以支持历史回看。
package quality

// Issue 质量问题（结构化评估）
type Issue struct {
	Severity    string   `json:"severity"`
	Dimension   string   `json:"dimension"`
	Field       string   `json:"field"`
	Description string   `json:"description"`
	Count       int32    `json:"count"`
	Sample      []string `json:"sample,omitempty"`
}

// DimensionScore 单维度得分
type DimensionScore struct {
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
	Passed bool    `json:"passed"`
	Issues []Issue `json:"issues,omitempty"`
}

// StructuredReport 结构化数据质量报告
type StructuredReport struct {
	OverallScore float64           `json:"overall_score"`
	RecordCount  int32             `json:"record_count"`
	Dimensions   []DimensionScore  `json:"dimensions"`
	Issues       []Issue           `json:"issues"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// TextIssue 文本质量问题
type TextIssue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Snippet     string `json:"snippet,omitempty"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// UnstructuredDimensionScore 非结构化文本单维度得分
type UnstructuredDimensionScore struct {
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
	Passed bool    `json:"passed"`
}

// UnstructuredReport 非结构化文本质量报告
type UnstructuredReport struct {
	OverallScore float64                      `json:"overall_score"`
	Dimensions   []UnstructuredDimensionScore `json:"dimensions"`
	TextStats    map[string]string            `json:"text_stats,omitempty"`
	Issues       []TextIssue                  `json:"issues"`
}

// CleaningOperation 单次清洗操作
type CleaningOperation struct {
	Type        string `json:"type"`
	Field       string `json:"field"`
	Count       int32  `json:"count"`
	Description string `json:"description"`
}

// CleaningReport 数据清洗结果
type CleaningReport struct {
	OriginalCount int32               `json:"original_count"`
	CleanedCount  int32               `json:"cleaned_count"`
	RemovedCount  int32               `json:"removed_count"`
	Operations    []CleaningOperation `json:"operations"`
	// CleanedCSV 清洗后的 CSV 文本，供前端预览/下载。
	CleanedCSV string `json:"cleaned_csv,omitempty"`
}

// DimensionInfo 支持的质量维度说明
type DimensionInfo struct {
	Name                 string `json:"name"`
	Description          string `json:"description"`
	SupportsStructured   bool   `json:"supports_structured"`
	SupportsUnstructured bool   `json:"supports_unstructured"`
}
