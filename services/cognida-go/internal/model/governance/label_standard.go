package governance

// Standards are authored governance records. Published copies are immutable;
// tasks bind the copy's ID and digest, never a mutable draft.
const (
	LabelStandardKind        = "label_standard"
	LabelStandardVersionKind = "label_standard_version"
)

type StandardExample struct {
	Text        string `json:"text"`
	Decision    string `json:"decision"`
	Explanation string `json:"explanation"`
}
type StandardLabel struct {
	Label
	Definition      string            `json:"definition"`
	IncludeCriteria []string          `json:"include_criteria"`
	ExcludeCriteria []string          `json:"exclude_criteria"`
	ExcludeKeywords []string          `json:"exclude_keywords"`
	Examples        []StandardExample `json:"examples"`
}
type StandardConstraints struct {
	MaxLabels    int  `json:"max_labels"`
	MaxEntities  int  `json:"max_entities"`
	MaxRelations int  `json:"max_relations"`
	AllowEmpty   bool `json:"allow_empty"`
}
type LabelStandardInput struct {
	Name        string              `json:"name"`
	TaskType    string              `json:"task_type"`
	Guidelines  string              `json:"guidelines"`
	Labels      []StandardLabel     `json:"labels"`
	Constraints StandardConstraints `json:"constraints"`
}
type LabelStandardDraft struct {
	LabelStandardInput
	LatestVersionID string `json:"latest_version_id"`
}
type LabelStandardVersion struct {
	Meta
	LabelStandardInput
	StandardID       string `json:"standard_id"`
	StandardRevision int    `json:"standard_revision"`
	ContentDigest    string `json:"content_digest"`
}

func (v LabelStandardInput) LabelSchema() []Label {
	labels := make([]Label, 0, len(v.Labels))
	for _, label := range v.Labels {
		labels = append(labels, label.Label)
	}
	return labels
}
