package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestClassificationUsesExplicitKeywordsAndEvidenceWithoutProbability(t *testing.T) {
	labels := []d.Label{{ID: "finance", Name: "财务", Keywords: []string{"invoice", "发票"}}, {ID: "legal", Name: "法务", Keywords: []string{"合同"}}}
	a, p := classificationCandidate("😀收到发票 INVoiCE", labels)
	require.Equal(t, []string{"finance"}, a.Labels)
	require.Equal(t, "matched", p.DecisionStatus)
	require.Nil(t, p.Confidence)
	require.NotEmpty(t, p.EvidenceSpans)
	chars := []rune("😀收到发票 INVoiCE")
	for _, span := range p.EvidenceSpans {
		require.Equal(t, span.Match, string(chars[span.Start:span.End]))
		require.Equal(t, "finance", span.LabelID)
	}
	a, p = classificationCandidate("普通问候", labels)
	require.Empty(t, a.Labels)
	require.Equal(t, "uncertain", p.DecisionStatus)
	require.Nil(t, p.Confidence)
	a, p = classificationCandidate("Kelvin", []d.Label{{ID: "k", Name: "温度", Keywords: []string{"kelvin"}}})
	require.Equal(t, "matched", p.DecisionStatus)
	require.Equal(t, 0, p.EvidenceSpans[0].Start)
	require.Equal(t, 6, p.EvidenceSpans[0].End)
}
func TestEntityDictionaryUsesUnicodePositionsAndPreservesAmbiguity(t *testing.T) {
	labels := []d.Label{{ID: "org", Name: "机构", Terms: []string{"北鹭公司"}}, {ID: "person", Name: "人物", Terms: []string{"小李"}}}
	content := "😀小李在北鹭公司工作，小李喜欢这里。"
	a, p := entityCandidate(content, labels)
	require.Len(t, a.Entities, 3)
	require.Equal(t, "matched", p.DecisionStatus)
	require.Nil(t, p.Confidence)
	require.Equal(t, 1, a.Entities[0].Start)
	require.Equal(t, "小李", a.Entities[0].Text)
	require.NoError(t, validateAnnotation(d.AnnotationTask{Type: "named_entity", Labels: labels}, content, a))
	for _, e := range a.Entities {
		require.Equal(t, e.Text, string([]rune(content)[e.Start:e.End]))
	}
	labels = append(labels, d.Label{ID: "place", Name: "位置", Terms: []string{"北鹭"}})
	a, p = entityCandidate(content, labels)
	require.Len(t, a.Entities, 2, "overlapping dictionary meanings must not choose a label")
	require.Equal(t, "uncertain", p.DecisionStatus)
	a, p = entityCandidate("aaaa", []d.Label{{ID: "token", Name: "token", Terms: []string{"aaa"}}})
	require.Empty(t, a.Entities, "self-overlapping occurrences must not choose one span")
	require.Equal(t, "uncertain", p.DecisionStatus)
	require.Len(t, p.EvidenceSpans, 2)
	require.Equal(t, 1, p.EvidenceSpans[1].Start)
}
func TestQAMappingOnlyCopiesExplicitValidStringColumns(t *testing.T) {
	task := d.AnnotationTask{Name: "QA", Type: "qa", QAMapping: &d.QAMapping{QuestionColumn: "question", ReferenceAnswerColumn: "answer"}}
	input := l.Input{Kind: "table", Columns: []string{"question", "answer", "noise"}, Rows: [][]any{{"金额？", "00123", "must not infer"}}}
	require.NoError(t, validateAnnotationTask(task))
	sources, e := annotationSources(input, task)
	require.NoError(t, e)
	a, p, e := candidateFor(task, sources[0])
	require.NoError(t, e)
	require.Equal(t, "00123", a.QA.ReferenceAnswer)
	require.Equal(t, "source_mapping", p.Source)
	require.Nil(t, p.Confidence)
	require.NoError(t, validateAnnotation(task, sources[0].Content, a))
	input.Rows = [][]any{{"A\nreference_answer: B", "C"}, {"A", "B\nreference_answer: C"}}
	sources, e = annotationSources(input, task)
	require.NoError(t, e)
	require.NotEqual(t, sources[0].Content, sources[1].Content)
	for i, source := range sources {
		var pair d.QAPair
		require.NoError(t, json.Unmarshal([]byte(source.Content), &pair))
		require.Equal(t, input.Rows[i][0], pair.Question)
		require.Equal(t, input.Rows[i][1], pair.ReferenceAnswer)
	}
	for _, rows := range [][][]any{{{"question", "", nil}}, {{"question", json.Number("123"), nil}}, {{nil, "answer", nil}}} {
		input.Rows = rows
		_, e = annotationSources(input, task)
		require.ErrorIs(t, e, d.ErrInvalid)
	}
	task.QAMapping.ReferenceAnswerColumn = "missing"
	_, e = annotationSources(input, task)
	require.ErrorIs(t, e, d.ErrInvalid)
}
func TestLabelSchemeBoundsAndProfileSeparation(t *testing.T) {
	require.Error(t, ValidateLabelSchema([]d.Label{{ID: "x", Name: "one", Keywords: []string{""}}}))
	require.Error(t, ValidateLabelSchema([]d.Label{{ID: "x", Name: "one"}, {ID: "x", Name: "two"}}))
	_, e := labelSchema(d.TaggingConfig{EnableTags: true, TagSchema: []string{"one"}, LabelSchema: []d.Label{{ID: "x", Name: "one"}}})
	require.Error(t, e)
	_, _, e = candidateFor(d.AnnotationTask{Type: "relation"}, annotationSource{Content: "Alice works for Acme"})
	require.Error(t, e)
	task := d.AnnotationTask{Type: "classification", Labels: []d.Label{{ID: "x", Name: "x"}}}
	require.Error(t, validateAnnotation(task, "content", d.Annotation{QA: &d.QAPair{Question: "q", ReferenceAnswer: "a"}}))
	require.Error(t, validateAnnotation(d.AnnotationTask{Type: "qa"}, "content", d.Annotation{}))
}
func TestKnowledgeTagRulesLeaveMultipleCategoriesUncertain(t *testing.T) {
	out := d.TaggingResult{Content: "invoice 合同"}
	scheme := []d.Label{{ID: "finance", Name: "财务", Keywords: []string{"invoice"}}, {ID: "legal", Name: "法务", Keywords: []string{"合同"}}}
	applyTagRules(&out, scheme, d.TaggingConfig{EnableTags: true, EnableClassification: true})
	require.ElementsMatch(t, []string{"财务", "法务"}, out.Tags)
	require.Empty(t, out.Category)
	require.Equal(t, "uncertain", out.Provenance.DecisionStatus)
	require.Nil(t, out.TagConfidence)
	require.Nil(t, out.CategoryConfidence)
	out.Content = "sales New York"
	scheme = []d.Label{{ID: "sales", Name: "Sales", Keywords: []string{"sales"}, Terms: []string{"New York", "York"}}}
	applyTagRules(&out, scheme, d.TaggingConfig{EnableTags: true, EnableExtraction: true})
	require.Equal(t, "uncertain", out.Provenance.DecisionStatus)
	require.Equal(t, []string{"Sales"}, out.Tags)
	out.Content = strings.Repeat("sales a", 1000)
	scheme[0].Terms = []string{"a", "s", "l"}
	applyTagRules(&out, scheme, d.TaggingConfig{EnableTags: true, EnableExtraction: true})
	require.Equal(t, "uncertain", out.Provenance.DecisionStatus, "bounded candidate overflow remains uncertain")
	scheme[0].Terms = nil
	applyTagRules(&out, scheme, d.TaggingConfig{EnableTags: true, EnableExtraction: true})
	require.Equal(t, "matched", out.Provenance.DecisionStatus, "an absent entity scheme does not downgrade matched tags")
}
