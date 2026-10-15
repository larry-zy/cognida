package taskcontrol

import g "cognida/internal/model/governance"

// NodeParameters is a closed union persisted in the plan and dispatch digest.
type NodeParameters struct {
	Governance *GovernanceParameters `json:"governance,omitempty"`
	Feature    *FeatureParameters    `json:"feature,omitempty"`
}
type GovernanceParameters struct {
	Labels      []string     `json:"labels,omitempty"`
	LabelSchema []g.Label    `json:"label_schema,omitempty"`
	QAMapping   *g.QAMapping `json:"qa_mapping,omitempty"`
}
type FeatureParameters struct {
	FeatureID string `json:"feature_id"`
	Version   int    `json:"version"`
}

func parameterKind(p *NodeParameters) (string, bool) {
	if p == nil {
		return "", true
	}
	if p.Governance != nil && p.Feature == nil {
		return "governance", true
	}
	if p.Feature != nil && p.Governance == nil {
		return "feature", true
	}
	return "", false
}
