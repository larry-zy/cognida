package governance

import (
	"bytes"
	d "cognida/internal/model/governance"
	"cognida/internal/model/llm"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"
	"unicode/utf8"
)

var (
	ErrLabelModelUnavailable = errors.New("label suggestion model unavailable")
	ErrLabelModelOutput      = errors.New("label suggestion has invalid schema or evidence")
	ErrLabelModelUnknown     = errors.New("model call outcome requires reconciliation; automatic replay is disabled")
)

type LabelSuggestionInput struct {
	Kind       string    `json:"kind"`
	Content    string    `json:"content"`
	Labels     []d.Label `json:"label_schema"`
	Guidelines string    `json:"guidelines"`
	QA         *d.QAPair `json:"source_qa,omitempty"`
}
type LabelCandidate struct {
	Annotation     *d.Annotation    `json:"annotation,omitempty"`
	Tags           []string         `json:"tags,omitempty"`
	Category       string           `json:"category,omitempty"`
	DecisionStatus string           `json:"decision_status"`
	Rationale      string           `json:"rationale"`
	Evidence       []d.EvidenceSpan `json:"evidence_spans"`
}
type LabelSuggester interface {
	Suggest(context.Context, LabelSuggestionInput) (LabelCandidate, error)
	ModelID() string
}
type llmLabelSuggester struct {
	client llm.LLMClient
	model  string
}

func NewLLMLabelSuggester(client llm.LLMClient, modelID string) LabelSuggester {
	return &llmLabelSuggester{client, modelID}
}
func (s *llmLabelSuggester) ModelID() string { return s.model }

func (s *llmLabelSuggester) Suggest(parent context.Context, in LabelSuggestionInput) (LabelCandidate, error) {
	var out LabelCandidate
	if s == nil || s.client == nil || s.model == "" {
		return out, ErrLabelModelUnavailable
	}
	v := reflect.ValueOf(s.client)
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return out, ErrLabelModelUnavailable
	}
	prompt := labelSuggestionPromptForKind(in.Kind)
	if prompt == "" {
		return out, d.ErrInvalid
	}
	raw, e := json.Marshal(in)
	if e != nil || len(raw) > 80<<10 {
		return out, d.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	resp, e := s.client.Chat(ctx, &llm.ChatRequest{Messages: []*llm.Message{{Role: llm.RoleSystem, Content: prompt}, {Role: llm.RoleUser, Content: string(raw)}}, Options: &llm.ChatOptions{Temperature: llm.Float64Ptr(0), MaxTokens: 4096, BoundedJSONOutput: true}})
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if e != nil {
		return out, ErrLabelModelUnavailable
	}
	if resp == nil || len(resp.ToolCalls) > 0 || (resp.FinishReason != "" && resp.FinishReason != "stop" && resp.FinishReason != "end_turn") {
		return out, ErrLabelModelOutput
	}
	content := resp.Content
	if resp.Message != nil {
		if len(resp.Message.ToolCalls) > 0 || (resp.Message.Role != "" && resp.Message.Role != llm.RoleAssistant) {
			return out, ErrLabelModelOutput
		}
		if content == "" {
			content = resp.Message.Content
		} else if resp.Message.Content != "" && content != resp.Message.Content {
			return out, ErrLabelModelOutput
		}
	}
	raw = bytes.TrimSpace([]byte(content))
	if len(raw) == 0 || len(raw) > 64<<10 || !utf8.Valid(raw) || raw[0] != '{' || strictFeatureJSON(raw) != nil {
		return out, ErrLabelModelOutput
	}
	return decodeLabelLiteralCandidate(raw, in)
}
func validateLabelCandidate(in LabelSuggestionInput, c LabelCandidate) error {
	if c.DecisionStatus != "matched" && c.DecisionStatus != "uncertain" || len([]rune(c.Rationale)) < 1 || len([]rune(c.Rationale)) > 1000 || len(c.Evidence) > 60 {
		return ErrLabelModelOutput
	}
	known := map[string]string{}
	byName := map[string]string{}
	evidenced := map[string]bool{}
	for _, v := range in.Labels {
		known[v.ID] = v.Name
		byName[v.Name] = v.ID
	}
	text := []rune(in.Content)
	for _, e := range c.Evidence {
		if e.Start < 0 || e.End <= e.Start || e.End > len(text) || e.Match != string(text[e.Start:e.End]) || e.Rule != "model_evidence" {
			return ErrLabelModelOutput
		}
		if e.LabelID != "" && known[e.LabelID] == "" {
			return ErrLabelModelOutput
		}
		evidenced[e.LabelID] = true
	}
	requireLabel := func(id string) bool { return known[id] != "" && evidenced[id] }
	if in.Kind == "tagging" {
		if c.Annotation != nil || len(c.Tags) > 30 {
			return ErrLabelModelOutput
		}
		for _, name := range c.Tags {
			if !requireLabel(byName[name]) {
				return ErrLabelModelOutput
			}
		}
		if c.Category != "" && !requireLabel(byName[c.Category]) {
			return ErrLabelModelOutput
		}
		if len(c.Tags) == 0 && c.Category == "" && c.DecisionStatus != "uncertain" {
			return ErrLabelModelOutput
		}
		return nil
	}
	if c.Annotation == nil || len(c.Tags) > 0 || c.Category != "" {
		return ErrLabelModelOutput
	}
	a := *c.Annotation
	if a.EvaluationProfile != nil || len(a.Entities) > 30 || len(a.Relations) > 30 || len(a.Labels) > 30 || validateAnnotation(d.AnnotationTask{Type: in.Kind, Labels: in.Labels}, in.Content, a) != nil {
		return ErrLabelModelOutput
	}
	for _, label := range a.Labels {
		if !requireLabel(label) {
			return ErrLabelModelOutput
		}
	}
	for _, r := range a.Relations {
		if !requireLabel(r.Label) {
			return ErrLabelModelOutput
		}
	}
	if in.Kind == "qa" {
		if in.QA == nil || a.QA == nil || *in.QA != *a.QA {
			return ErrLabelModelOutput
		}
	} else if len(a.Labels)+len(a.Entities)+len(a.Relations) == 0 && c.DecisionStatus != "uncertain" {
		return ErrLabelModelOutput
	}
	return nil
}
