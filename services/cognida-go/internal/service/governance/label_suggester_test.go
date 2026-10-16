package governance

import (
	d "cognida/internal/model/governance"
	"cognida/internal/model/llm"
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type labelModelStub struct {
	llm.LLMClient
	response *llm.ChatResponse
	err      error
	request  *llm.ChatRequest
}

func (s *labelModelStub) Chat(_ context.Context, r *llm.ChatRequest) (*llm.ChatResponse, error) {
	s.request = r
	return s.response, s.err
}
func labelCandidateFixture() (LabelSuggestionInput, LabelCandidate) {
	return LabelSuggestionInput{Kind: "classification", Content: "😀发票", Labels: []d.Label{{ID: "finance", Name: "财务"}}}, LabelCandidate{Annotation: &d.Annotation{Labels: []string{"finance"}}, DecisionStatus: "matched", Rationale: "原文明确提及发票", Evidence: []d.EvidenceSpan{{LabelID: "finance", Rule: "model_evidence", Match: "发票", Start: 1, End: 3}}}
}
func TestLabelModelStrictEvidenceAndNoTools(t *testing.T) {
	in, out := labelCandidateFixture()
	raw, _ := json.Marshal(out)
	client := &labelModelStub{response: &llm.ChatResponse{Content: string(raw), FinishReason: "stop"}}
	s := NewLLMLabelSuggester(client, "actual-model")
	got, e := s.Suggest(context.Background(), in)
	require.NoError(t, e)
	require.Equal(t, out, got)
	require.Empty(t, client.request.Options.Tools)
	require.Equal(t, 4096, client.request.Options.MaxTokens)
	for _, content := range []string{string(raw) + " {}", `{"decision_status":"uncertain","decision_status":"matched"}`, `{"decision_status":"matched","rationale":"ok","confidence":0.5}`, `{"annotation":{"labels":["finance"]},"decision_status":"matched","rationale":"ok","evidence_spans":[{"label_id":"finance","rule":"model_evidence","match":"发票","start":0,"end":2}]}`} {
		client.response.Content = content
		_, e = s.Suggest(context.Background(), in)
		require.ErrorIs(t, e, ErrLabelModelOutput)
	}
	client.err = errors.New("secret URL key")
	_, e = s.Suggest(context.Background(), in)
	require.ErrorIs(t, e, ErrLabelModelUnavailable)
	require.NotContains(t, e.Error(), "secret")
}
func TestLabelModelFourProfilesAndTagging(t *testing.T) {
	in, c := labelCandidateFixture()
	require.NoError(t, validateLabelCandidate(in, c))
	c.Evidence = nil
	require.Error(t, validateLabelCandidate(in, c))
	in.Kind = "named_entity"
	c.Annotation = &d.Annotation{Entities: []d.Entity{{ID: "e", Label: "finance", Start: 1, End: 3, Text: "发票"}}}
	require.NoError(t, validateLabelCandidate(in, c))
	c.Annotation.Entities[0].End = 2
	require.Error(t, validateLabelCandidate(in, c))
	in.Kind = "relation"
	in.Content = "小李任职蓝鹭"
	c.Annotation = &d.Annotation{Entities: []d.Entity{{ID: "a", Label: "person", Start: 0, End: 2, Text: "小李"}, {ID: "b", Label: "org", Start: 4, End: 6, Text: "蓝鹭"}}, Relations: []d.Relation{{From: "a", To: "b", Label: "finance"}}}
	c.Evidence = []d.EvidenceSpan{{LabelID: "finance", Rule: "model_evidence", Start: 0, End: 6, Match: in.Content}}
	require.NoError(t, validateLabelCandidate(in, c))
	c.Annotation.Relations[0].To = "invented"
	require.Error(t, validateLabelCandidate(in, c))
	in.Kind = "qa"
	in.QA = &d.QAPair{Question: "金额？", ReferenceAnswer: "00123"}
	c.Annotation = &d.Annotation{QA: &d.QAPair{Question: "金额？", ReferenceAnswer: "00123"}}
	c.Evidence = nil
	require.NoError(t, validateLabelCandidate(in, c))
	c.Annotation.QA.ReferenceAnswer = "123"
	require.Error(t, validateLabelCandidate(in, c))
	in, c = labelCandidateFixture()
	in.Kind = "tagging"
	c.Annotation = nil
	c.Tags = []string{"财务"}
	require.NoError(t, validateLabelCandidate(in, c))
	c.Tags = []string{"虚构"}
	require.Error(t, validateLabelCandidate(in, c))
}
