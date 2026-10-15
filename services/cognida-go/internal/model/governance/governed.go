package governance

import (
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const GovernedKind = "governed_execution"
const TaggingProfile = "governance-tagging-rules-v1"
const ClassificationProfile = "governance-classification-rules-v1"
const EntityProfile = "governance-entity-dictionary-v1"
const QAProfile = "governance-qa-columns-v1"
const FeatureProfile = "governance-feature-engineering-v1"

type GovernedInvocation struct {
	OperationKey   string     `json:"operation_key"`
	PayloadHash    string     `json:"payload_hash"`
	Profile        string     `json:"profile"`
	InputRef       l.RefID    `json:"input_ref"`
	Labels         []string   `json:"labels,omitempty"`
	LabelSchema    []Label    `json:"label_schema,omitempty"`
	QAMapping      *QAMapping `json:"qa_mapping,omitempty"`
	FeatureID      string     `json:"feature_id,omitempty"`
	FeatureVersion int        `json:"feature_version,omitempty"`
	Deadline       time.Time  `json:"deadline"`
	TimeoutSeconds int        `json:"timeout_seconds"`
	CallsReserved  int        `json:"calls_reserved"`
}

func (in GovernedInvocation) Validate() error {
	if len(in.OperationKey) != 64 || len(in.PayloadHash) != 64 || strings.Trim(in.OperationKey+in.PayloadHash, "0123456789abcdef") != "" || in.InputRef.Validate() != nil || in.Deadline.IsZero() || in.TimeoutSeconds < 1 {
		return ErrInvalid
	}
	if in.Profile == FeatureProfile {
		if in.FeatureID == "" || len(in.FeatureID) > 64 || in.FeatureVersion < 1 || in.TimeoutSeconds > 300 || in.CallsReserved != 3 || len(in.Labels)+len(in.LabelSchema) > 0 || in.QAMapping != nil {
			return ErrInvalid
		}
		return nil
	}
	if in.FeatureID != "" || in.FeatureVersion != 0 || in.TimeoutSeconds > 120 || in.CallsReserved != 34 {
		return ErrInvalid
	}
	if in.Profile == QAProfile {
		if len(in.Labels)+len(in.LabelSchema) != 0 || in.QAMapping == nil || in.QAMapping.QuestionColumn == "" || in.QAMapping.ReferenceAnswerColumn == "" || in.QAMapping.QuestionColumn == in.QAMapping.ReferenceAnswerColumn {
			return ErrInvalid
		}
		return nil
	}
	if in.Profile != TaggingProfile && in.Profile != ClassificationProfile && in.Profile != EntityProfile {
		return ErrInvalid
	}
	if in.QAMapping != nil || len(in.Labels) > 0 && len(in.LabelSchema) > 0 {
		return ErrInvalid
	}
	labels := in.Schema()
	if len(labels) < 1 || len(labels) > 30 {
		return ErrInvalid
	}
	ids, names := map[string]bool{}, map[string]bool{}
	terms, total := 0, 0
	for _, v := range labels {
		if v.ID == "" || len(v.ID) > 128 || strings.TrimSpace(v.Name) != v.Name || v.Name == "" || utf8.RuneCountInString(v.Name) > 80 || ids[v.ID] || names[v.Name] {
			return ErrInvalid
		}
		ids[v.ID], names[v.Name] = true, true
		for _, rules := range [][]string{v.Keywords, v.Terms} {
			if len(rules) > 50 {
				return ErrInvalid
			}
			seen := map[string]bool{}
			for _, rule := range rules {
				if rule == "" || strings.TrimSpace(rule) != rule || utf8.RuneCountInString(rule) > 200 || seen[rule] {
					return ErrInvalid
				}
				seen[rule] = true
				total++
			}
		}
		terms += len(v.Terms)
	}
	if total > 500 || in.Profile == EntityProfile && terms == 0 {
		return ErrInvalid
	}
	return nil
}
func (in GovernedInvocation) Schema() []Label {
	if len(in.LabelSchema) > 0 {
		return in.LabelSchema
	}
	labels := []Label{}
	for i, name := range in.Labels {
		labels = append(labels, Label{ID: fmt.Sprintf("label-%d", i+1), Name: name, Keywords: []string{name}})
	}
	return labels
}
func (in GovernedInvocation) AnnotationType() string {
	switch in.Profile {
	case ClassificationProfile:
		return "classification"
	case EntityProfile:
		return "named_entity"
	case QAProfile:
		return "qa"
	}
	return ""
}

type GovernedExecution struct {
	Invocation GovernedInvocation `json:"invocation"`
	DomainID   string             `json:"domain_id"`
	DomainKind string             `json:"domain_kind"`
	OutputRefs []l.RefID          `json:"output_refs"`
	Error      string             `json:"error,omitempty"`
}

func SameGoverned(a, b GovernedInvocation) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

type GovernedRepository interface {
	SubmitGoverned(context.Context, l.Principal, GovernedInvocation, bool) (*Record, error)
	FindGoverned(context.Context, l.Principal, string) (*Record, error)
	UpdateGoverned(context.Context, l.Principal, string, func(*Record, *GovernedExecution, Repository) error) (*Record, error)
}
