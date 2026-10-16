package governance

import "encoding/json"

const SFTDatasetKind = "sft_dataset"
const SFTDatasetVersionKind = "sft_dataset_version"

type SFTColumns struct {
	Question        string `json:"question,omitempty"`
	ReferenceAnswer string `json:"reference_answer,omitempty"`
	Messages        string `json:"messages,omitempty"`
	Tools           string `json:"tools,omitempty"`
	Split           string `json:"split,omitempty"`
	Group           string `json:"group,omitempty"`
}
type SFTInput struct {
	Name              string     `json:"name"`
	SourceRefID       string     `json:"source_ref_id"`
	StandardVersionID string     `json:"standard_version_id"`
	Format            string     `json:"format"`
	Columns           SFTColumns `json:"columns"`
}
type SFTFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type SFTToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function SFTFunctionCall `json:"function"`
}
type SFTMessage struct {
	Role       string        `json:"role"`
	Content    *string       `json:"content,omitempty"`
	ToolCalls  []SFTToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}
type SFTToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}
type SFTTool struct {
	Type     string          `json:"type"`
	Function SFTToolFunction `json:"function"`
}
type SFTRow struct {
	RowID          string       `json:"row_id"`
	SourceRowIndex int          `json:"source_row_index"`
	Messages       []SFTMessage `json:"messages"`
	Tools          []SFTTool    `json:"tools,omitempty"`
	Split          string       `json:"split"`
	Group          string       `json:"group,omitempty"`
}
type SFTDataset struct {
	SFTInput
	SourceDigest    string         `json:"source_digest"`
	StandardDigest  string         `json:"standard_digest"`
	ContentDigest   string         `json:"content_digest"`
	RowCount        int            `json:"row_count"`
	SplitCounts     map[string]int `json:"split_counts"`
	Rows            []SFTRow       `json:"rows,omitempty"`       // Legacy payloads only.
	RowsExact       string         `json:"rows_exact,omitempty"` // Internal authoritative JSON snapshot; not a public head field.
	ReviewComment   string         `json:"review_comment"`
	ReviewedBy      int64          `json:"reviewed_by"`
	LatestVersionID string         `json:"latest_version_id"`
}
type SFTDatasetVersion struct {
	SFTDataset
	DatasetID       string `json:"dataset_id"`
	DatasetRevision int    `json:"dataset_revision"`
}
