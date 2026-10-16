package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func standardFixture(kind string) d.LabelStandardInput {
	in := d.LabelStandardInput{Name: "explicit scheme", TaskType: kind, Guidelines: "Use only this source. Ambiguity requires review.", Labels: []d.StandardLabel{{Label: d.Label{ID: "sales", Name: "Sales", Keywords: []string{"sales"}, Terms: []string{"Sales"}}, Definition: "sales topic", ExcludeKeywords: []string{"not sales"}}}, Constraints: d.StandardConstraints{MaxLabels: 1}}
	switch kind {
	case "qa":
		in.Labels = nil
		in.Constraints = d.StandardConstraints{}
	case "named_entity":
		in.Constraints = d.StandardConstraints{MaxEntities: 10}
	case "relation":
		in.Constraints = d.StandardConstraints{MaxEntities: 10, MaxRelations: 10}
	}
	return in
}
func TestLabelStandardCardinalityExclusionAndUncertainty(t *testing.T) {
	in := standardFixture("classification")
	require.NoError(t, ValidateLabelStandard(in))
	in.Labels = append(in.Labels, d.StandardLabel{Label: d.Label{ID: "legal", Name: "Legal", Keywords: []string{"legal"}}, Definition: "law topic"})
	task := d.AnnotationTask{Type: "classification", Labels: in.LabelSchema(), StandardSnapshot: &in}
	require.Error(t, validateAnnotation(task, "sales", d.Annotation{Labels: []string{"sales", "sales"}}))
	require.Error(t, validateAnnotation(task, "sales legal", d.Annotation{Labels: []string{"sales", "legal"}}))
	require.Error(t, validateAnnotation(task, "not sales", d.Annotation{Labels: []string{"sales"}}))
	require.NoError(t, validateAnnotation(task, "sales", d.Annotation{Labels: []string{"sales"}}))
	for _, text := range []string{"sales legal", "not sales"} {
		a, p, e := candidateFor(task, annotationSource{Content: text})
		require.NoError(t, e)
		require.Empty(t, a.Labels)
		require.Equal(t, "uncertain", p.DecisionStatus)
		require.NotEmpty(t, p.EvidenceSpans)
		require.Error(t, validateStandardAnnotation(&in, text, a, true))
	}
	in.Constraints.AllowEmpty = true
	require.NoError(t, validateStandardAnnotation(&in, "none", d.Annotation{}, true))
	candidate := LabelCandidate{Annotation: &d.Annotation{Labels: []string{"sales", "legal"}}, DecisionStatus: "matched", Rationale: "two topics", Evidence: []d.EvidenceSpan{{LabelID: "sales", Rule: "model_evidence", Match: "sales", Start: 0, End: 5}, {LabelID: "legal", Rule: "model_evidence", Match: "legal", Start: 6, End: 11}}}
	require.ErrorIs(t, validateLabelCandidate(LabelSuggestionInput{Kind: "classification", Content: "sales legal", Labels: in.LabelSchema(), Standard: &in}, candidate), ErrLabelModelOutput)
}
func TestLabelStandardValidationKeepsHundredClassContract(t *testing.T) {
	in := standardFixture("classification")
	in.Labels = nil
	for i := 0; i < 100; i++ {
		in.Labels = append(in.Labels, d.StandardLabel{Label: d.Label{ID: fmt.Sprint(i), Name: fmt.Sprintf("label %d", i)}, Definition: "definition"})
	}
	require.NoError(t, ValidateLabelStandard(in))
	in.TaskType = "tagging"
	require.Error(t, ValidateLabelStandard(in))
	in.TaskType = "classification"
	in.Labels[1].ID = in.Labels[0].ID
	require.Error(t, ValidateLabelStandard(in))
	require.NoError(t, ValidateLabelStandard(standardFixture("qa")))
}
func TestLabelStandardBindRequiresPreparedAndPublishedOwnerVersion(t *testing.T) {
	p := l.Principal{TenantID: 1, UserID: 2}
	standard := standardFixture("classification")
	version := d.LabelStandardVersion{LabelStandardInput: standard, ContentDigest: d.LabelContentDigest(string(mustLabelJSON(standard)))}
	repo := &unitRepo{records: map[string]*d.Record{"version": {Meta: d.Meta{ID: "version", TenantID: 1, UserID: 2}, Kind: d.LabelStandardVersionKind, Status: "published", Payload: mustLabelJSON(version)}}}
	id := l.RefID{AssetID: uuid.NewString(), Version: 1}
	parent := l.RefID{AssetID: uuid.NewString(), Version: 1}
	digest := strings.Repeat("a", 64)
	ref := l.DataAssetRef{RefID: id, TenantID: 1, Kind: l.CleanedAsset, Parents: []l.RefID{parent}, Provenance: l.Provenance{Digest: digest}}
	assets := &unitAssets{refs: map[string]l.DataAssetRef{id.String(): ref}, input: l.Input{Kind: "text", Text: "sales"}}
	s := NewService(repo, assets, nil, nil, func(context.Context, l.Principal, string) error { return nil })
	input := d.AnnotationTask{Type: "classification", SourceRefID: id.String(), StandardVersionID: "version", StandardSnapshot: &d.LabelStandardInput{Name: "forged"}}
	_, e := s.BindAnnotationStandard(context.Background(), p, input)
	require.ErrorIs(t, e, d.ErrInvalid)
	ref.Provenance.Preparation = &l.Preparation{Version: 1, SourceRefID: parent.String(), SourceDigest: digest, OutputDigest: digest, SchemaDigest: digest, Rows: 1}
	assets.refs[id.String()] = ref
	bound, e := s.BindAnnotationStandard(context.Background(), p, input)
	require.NoError(t, e)
	require.Equal(t, standard.Name, bound.StandardSnapshot.Name)
	require.Equal(t, version.ContentDigest, bound.StandardDigest)
	input.Labels = []d.Label{{ID: "bad", Name: "Bad"}}
	_, e = s.BindAnnotationStandard(context.Background(), p, input)
	require.ErrorIs(t, e, d.ErrInvalid)
	p.UserID++
	_, e = s.GetLabelStandardVersion(context.Background(), p, "version")
	require.ErrorIs(t, e, d.ErrNotFound)
	repo.records["version"].UserID = p.UserID
	repo.records["version"].Status = "draft"
	_, e = s.GetLabelStandardVersion(context.Background(), p, "version")
	require.ErrorIs(t, e, d.ErrInvalid)
}
func TestSFTExactSourceMappingAndSplitIsolation(t *testing.T) {
	in := d.SFTInput{Name: "QA", Format: "qa", Columns: d.SFTColumns{Question: "q", ReferenceAnswer: "a", Split: "split", Group: "group"}}
	input := l.Input{Kind: "table", Columns: []string{"q", "a", "split", "group"}, Rows: [][]any{{"😀 amount?", "9007199254740993.123456789012345678", "train", "a"}, {"code?", "00123", "test", "b"}}}
	rows, counts, e := buildSFTRows(in, input)
	require.NoError(t, e)
	require.Equal(t, input.Rows[0][1], *rows[0].Messages[1].Content)
	require.Equal(t, 1, counts["test"])
	input.Rows[1][0] = "😀 amount?"
	_, _, e = buildSFTRows(in, input)
	require.ErrorContains(t, e, "duplicate input crosses")
	input.Rows[1][0] = "code?"
	input.Rows[1][3] = "a"
	_, _, e = buildSFTRows(in, input)
	require.ErrorContains(t, e, "group crosses")
	input.Rows[1][3] = "b"
	input.Rows[1][1] = json.Number("123")
	_, _, e = buildSFTRows(in, input)
	require.Error(t, e)
}
func TestSFTToolCallingStrictConversationContract(t *testing.T) {
	str := func(v string) *string { return &v }
	tools := []d.SFTTool{{Type: "function", Function: d.SFTToolFunction{Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`)}}}
	messages := []d.SFTMessage{{Role: "user", Content: str("lookup 00123")}, {Role: "assistant", ToolCalls: []d.SFTToolCall{{ID: "call-1", Type: "function", Function: d.SFTFunctionCall{Name: "lookup", Arguments: `{"id":"00123"}`}}}}, {Role: "tool", ToolCallID: "call-1", Content: str("source result")}, {Role: "assistant", Content: str("source result")}}
	require.NoError(t, validateSFTMessages(messages, tools))
	require.Error(t, validateSFTMessages(messages, nil))
	require.Error(t, validateSFTMessages(messages[:3], tools))
	messages[2].ToolCallID = "other"
	require.Error(t, validateSFTMessages(messages, tools))
	messages[2].ToolCallID = "call-1"
	messages[1].ToolCalls[0].Function.Arguments = `{"id":"a","id":"b"}`
	require.Error(t, validateSFTMessages(messages, tools))
	var decoded []d.SFTMessage
	require.Error(t, sftDecode(`[{"role":"user","content":"x","unexpected":true}]`, &decoded))
	require.Error(t, sftDecode(`null`, &decoded))
}
