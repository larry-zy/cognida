package taskcontrol

import g "cognida/internal/model/governance"
import hf "cognida/internal/model/huggingface"
import sb "cognida/internal/model/sandbox"

// NodeParameters is a closed union persisted in the plan and dispatch digest.
type NodeParameters struct {
	Sandbox     *SandboxParameters    `json:"sandbox,omitempty"`
	Governance  *GovernanceParameters `json:"governance,omitempty"`
	Feature     *FeatureParameters    `json:"feature,omitempty"`
	Collection  *CollectionParameters `json:"collection,omitempty"`
	Evaluation  *EvaluationParameters `json:"evaluation,omitempty"`
	Research    *ResearchParameters   `json:"research,omitempty"`
	Warehouse   *WarehouseParameters  `json:"warehouse,omitempty"`
	HuggingFace *HFImportParameters   `json:"huggingface,omitempty"`
	SFT         *SFTParameters        `json:"sft,omitempty"`
}
type HFImportParameters struct {
	Request hf.Request `json:"request"`
}
type SFTParameters struct {
	Name              string       `json:"name"`
	StandardVersionID string       `json:"standard_version_id"`
	Format            string       `json:"format"`
	Columns           g.SFTColumns `json:"columns"`
}
type SandboxParameters struct {
	ProfileID      string            `json:"profile_id"`
	ProfileVersion int               `json:"profile_version"`
	ImageSHA256    string            `json:"image_sha256"`
	Code           sb.InputRef       `json:"code"`
	Inputs         []sb.InputRef     `json:"inputs"`
	Resources      sb.ResourceLimits `json:"resources"`
}
type WarehouseParameters struct {
	ModelID       string `json:"model_id"`
	Version       int    `json:"version"`
	Mode          string `json:"mode"`
	FactSourceRef string `json:"fact_source_ref,omitempty"`
	FactFromNode  string `json:"fact_from_node,omitempty"`
	EventFrom     string `json:"event_from,omitempty"`
	EventTo       string `json:"event_to,omitempty"`
}
type CollectionParameters struct {
	URL              string `json:"url"`
	MaxResponseBytes int64  `json:"max_response_bytes"`
}
type EvaluationParameters struct {
	QuestionColumn        string `json:"question_column"`
	ReferenceAnswerColumn string `json:"reference_answer_column"`
	GeneratedAnswerColumn string `json:"generated_answer_column"`
}
type ResearchParameters struct {
	Query           string   `json:"query"`
	KBIDs           []string `json:"kb_ids,omitempty"`
	GraphEnabled    bool     `json:"graph_enabled,omitempty"`
	EnableWebSearch bool     `json:"enable_web_search,omitempty"`
}
type GovernanceParameters struct {
	StandardVersionID string       `json:"standard_version_id,omitempty"`
	Labels            []string     `json:"labels,omitempty"`
	LabelSchema       []g.Label    `json:"label_schema,omitempty"`
	QAMapping         *g.QAMapping `json:"qa_mapping,omitempty"`
}
type FeatureParameters struct {
	FeatureID string `json:"feature_id"`
	Version   int    `json:"version"`
}

func parameterKind(p *NodeParameters) (string, bool) {
	if p == nil {
		return "", true
	}
	kind, count := "", 0
	for _, v := range []struct {
		kind    string
		present bool
	}{
		{"governance", p.Governance != nil}, {"feature", p.Feature != nil},
		{"collection", p.Collection != nil}, {"evaluation", p.Evaluation != nil}, {"research", p.Research != nil},
		{"warehouse", p.Warehouse != nil}, {"sandbox", p.Sandbox != nil},
		{"huggingface", p.HuggingFace != nil}, {"sft", p.SFT != nil},
	} {
		if v.present {
			kind = v.kind
			count++
		}
	}
	return kind, count == 1
}

// HasOnlyParameter validates the entire closed union, including newly added domains.
func HasOnlyParameter(p *NodeParameters, expected string) bool {
	kind, valid := parameterKind(p)
	return valid && kind == expected
}
