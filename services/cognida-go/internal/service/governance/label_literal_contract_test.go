package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	d "cognida/internal/model/governance"
	"cognida/internal/model/llm"
	"github.com/stretchr/testify/require"
)

func TestLabelLiteralEvidenceUniqueUnicodeAndExplicitPositions(t *testing.T) {
	in, _ := labelCandidateFixture()
	base := `{"annotation":{"labels":["finance"]},"decision_status":"matched","rationale":"literal source","evidence_spans":[%s]}`
	for _, tc := range []struct {
		name, evidence, text string
		start, end           int
		valid                bool
	}{
		{"unique unicode", `{"label_id":"finance","match":"发票"}`, "😀发票", 1, 3, true},
		{"explicit legacy", `{"label_id":"finance","match":"发票","rule":"model_evidence","start":1,"end":3}`, "😀发票", 1, 3, true},
		{"explicit zero", `{"label_id":"finance","match":"发票","start":0,"end":2}`, "发票", 0, 2, true},
		{"wrong explicit", `{"label_id":"finance","match":"发票","start":0,"end":2}`, "😀发票", 0, 0, false},
		{"repeated ambiguous", `{"label_id":"finance","match":"发票"}`, "发票和发票", 0, 0, false},
		{"repeated explicit", `{"label_id":"finance","match":"发票","start":3,"end":5}`, "发票和发票", 3, 5, true},
		{"overlapping ambiguous", `{"label_id":"finance","match":"aaa"}`, "aaaa", 0, 0, false},
		{"empty", `{"label_id":"finance","match":""}`, "abc", 0, 0, false},
		{"fabricated", `{"label_id":"finance","match":"refund"}`, "abc", 0, 0, false},
		{"partial start", `{"label_id":"finance","match":"发票","start":1}`, "😀发票", 0, 0, false},
		{"partial end", `{"label_id":"finance","match":"发票","end":3}`, "😀发票", 0, 0, false},
		{"null offset", `{"label_id":"finance","match":"发票","start":null,"end":null}`, "😀发票", 0, 0, false},
		{"float offset", `{"label_id":"finance","match":"发票","start":1.0,"end":3}`, "😀发票", 0, 0, false},
		{"null rule", `{"label_id":"finance","match":"发票","rule":null}`, "😀发票", 0, 0, false},
		{"empty rule", `{"label_id":"finance","match":"发票","rule":""}`, "😀发票", 0, 0, false},
		{"wrong rule", `{"label_id":"finance","match":"发票","rule":"manual"}`, "😀发票", 0, 0, false},
		{"case key alias", `{"label_id":"finance","match":"发票","Start":1,"end":3}`, "😀发票", 0, 0, false},
		{"unicode alias", `{"label_id":"finance","match":"发票","ſtart":1,"end":3}`, "😀发票", 0, 0, false},
		{"duplicate offset", `{"label_id":"finance","match":"发票","start":1,"start":0,"end":3}`, "😀发票", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in.Content = tc.text
			got, err := decodeLabelLiteralCandidate([]byte(fmt.Sprintf(base, tc.evidence)), in)
			if !tc.valid {
				require.ErrorIs(t, err, ErrLabelModelOutput)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.start, got.Evidence[0].Start)
			require.Equal(t, tc.end, got.Evidence[0].End)
			require.Equal(t, "model_evidence", got.Evidence[0].Rule)
		})
	}
}
func TestLabelLiteralEntitiesRelationsAndForbiddenBranches(t *testing.T) {
	in := LabelSuggestionInput{Kind: "named_entity", Content: "😀李在京，李离开", Labels: []d.Label{{ID: "person", Name: "person"}, {ID: "location", Name: "location"}, {ID: "works", Name: "works"}}}
	base := `{"annotation":%s,"decision_status":"matched","rationale":"literal","evidence_spans":[]}`
	for _, tc := range []struct {
		annotation string
		valid      bool
	}{
		{`{"entities":[{"id":"e","label":"location","text":"京"}]}`, true},
		{`{"entities":[{"id":"e","label":"person","text":"李"}]}`, false},
		{`{"entities":[{"id":"e","label":"person","text":"李","start":1,"end":2}]}`, true},
		{`{"entities":[{"id":"e","label":"person","text":"李","start":0,"end":1}]}`, false},
		{`{"entities":[{"id":"e","label":"location","text":"京"},{"id":"e","label":"location","text":"京"}]}`, false},
		{`{"entities":[{"id":"e","label":"unknown","text":"京"}]}`, false},
		{`{"entities":[{"id":"e","label":"location","text":"京","start":null,"end":null}]}`, false},
		{`{"entities":[{"id":"e","label":"location","text":"京"}],"labels":["person"]}`, false},
		{`{"entities":[{"id":"e","label":"location","text":"京"}],"evaluation_profile":{}}`, false},
	} {
		got, err := decodeLabelLiteralCandidate([]byte(fmt.Sprintf(base, tc.annotation)), in)
		if tc.valid {
			require.NoError(t, err)
			require.Len(t, got.Annotation.Entities, 1)
		} else {
			require.Error(t, err)
		}
	}
	in.Kind = "relation"
	raw := `{"annotation":{"entities":[{"id":"p","label":"person","text":"李","start":1,"end":2},{"id":"l","label":"location","text":"京"}],"relations":[{"from":"p","to":"l","label":"works"}]},"decision_status":"matched","rationale":"literal","evidence_spans":[{"label_id":"works","match":"李在京"}]}`
	_, err := decodeLabelLiteralCandidate([]byte(raw), in)
	require.NoError(t, err)
	_, err = decodeLabelLiteralCandidate([]byte(strings.Replace(raw, `"to":"l"`, `"to":"missing"`, 1)), in)
	require.Error(t, err)
	in, _ = labelCandidateFixture()
	valid := `{"annotation":{"labels":["finance"],"entities":null,"relations":[]},"decision_status":"matched","rationale":"literal","evidence_spans":[{"label_id":"finance","match":"发票"}]}`
	_, err = decodeLabelLiteralCandidate([]byte(valid), in)
	require.NoError(t, err)
	for _, extra := range []string{`,"tags":["财务"]`, `,"category":"财务"`, `,"confidence":0.99`, `,"review_status":"approved"`, `,"Annotation":{}`} {
		_, err = decodeLabelLiteralCandidate([]byte(valid[:len(valid)-1]+extra+"}"), in)
		require.Error(t, err)
	}
}
func TestLabelLiteralSuggesterUsesKindContractAndUnchangedFinalValidator(t *testing.T) {
	in, _ := labelCandidateFixture()
	client := &labelModelStub{response: &llm.ChatResponse{Content: `{"annotation":{"labels":["finance"]},"decision_status":"matched","rationale":"literal","evidence_spans":[{"label_id":"finance","match":"发票"}]}`, FinishReason: "stop"}}
	got, err := NewLLMLabelSuggester(client, "fixture").Suggest(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, 1, got.Evidence[0].Start)
	require.NoError(t, validateLabelCandidate(in, got))
	require.Contains(t, client.request.Messages[0].Content, "This request is classification")
	require.NotContains(t, client.request.Messages[0].Content, "This request is tagging")
	in.Kind = "unknown"
	client.request = nil
	_, err = NewLLMLabelSuggester(client, "fixture").Suggest(context.Background(), in)
	require.ErrorIs(t, err, d.ErrInvalid)
	require.Nil(t, client.request)
	// QA and tagging remain real supported wire contracts, not discarded branches.
	qa := LabelSuggestionInput{Kind: "qa", Content: "question", QA: &d.QAPair{Question: "q", ReferenceAnswer: "a"}}
	for _, kind := range []string{"qa", "tagging"} {
		var candidate LabelCandidate
		input := qa
		if kind == "qa" {
			candidate = LabelCandidate{Annotation: &d.Annotation{QA: qa.QA}, DecisionStatus: "matched", Rationale: "exact"}
		} else {
			input, _ = labelCandidateFixture()
			input.Kind = "tagging"
			candidate = LabelCandidate{Tags: []string{"财务"}, DecisionStatus: "matched", Rationale: "literal", Evidence: []d.EvidenceSpan{{LabelID: "finance", Rule: "model_evidence", Match: "发票", Start: 1, End: 3}}}
		}
		raw, _ := json.Marshal(candidate)
		_, err := decodeLabelLiteralCandidate(raw, input)
		require.NoError(t, err)
	}
}
