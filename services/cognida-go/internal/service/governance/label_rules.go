package governance

import (
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// ValidateLabelSchema bounds deterministic literal rules. No regex, code or
// model-created label silently enters the user's explicit scheme.
func ValidateLabelSchema(labels []d.Label) error {
	if len(labels) == 0 || len(labels) > 100 {
		return d.ErrInvalid
	}
	ids, names := map[string]bool{}, map[string]bool{}
	total := 0
	for _, label := range labels {
		if label.ID == "" || len(label.ID) > 128 || strings.TrimSpace(label.Name) != label.Name || label.Name == "" || utf8.RuneCountInString(label.Name) > 128 || ids[label.ID] || names[label.Name] {
			return d.ErrInvalid
		}
		ids[label.ID], names[label.Name] = true, true
		for _, rules := range [][]string{label.Keywords, label.Terms} {
			if len(rules) > 50 {
				return d.ErrInvalid
			}
			seen := map[string]bool{}
			for _, term := range rules {
				if term == "" || strings.TrimSpace(term) != term || utf8.RuneCountInString(term) > 200 || seen[term] {
					return d.ErrInvalid
				}
				seen[term] = true
				total++
			}
		}
	}
	if total > 500 {
		return d.ErrInvalid
	}
	return nil
}
func validateAnnotationTask(in d.AnnotationTask) error {
	if in.Name == "" || len(in.Name) > 128 {
		return d.ErrInvalid
	}
	switch in.Type {
	case "classification", "named_entity", "relation":
		if in.QAMapping != nil {
			return d.ErrInvalid
		}
		return ValidateLabelSchema(in.Labels)
	case "qa":
		if len(in.Labels) != 0 || in.QAMapping == nil || in.QAMapping.QuestionColumn == "" || in.QAMapping.ReferenceAnswerColumn == "" || in.QAMapping.QuestionColumn == in.QAMapping.ReferenceAnswerColumn {
			return d.ErrInvalid
		}
		return nil
	default:
		return d.ErrInvalid
	}
}
func labelSchema(config d.TaggingConfig) ([]d.Label, error) {
	if len(config.TagSchema) > 0 && len(config.LabelSchema) > 0 {
		return nil, d.ErrInvalid
	}
	labels := config.LabelSchema
	if len(labels) == 0 {
		for i, name := range config.TagSchema {
			labels = append(labels, d.Label{ID: fmt.Sprintf("label-%d", i+1), Name: name, Keywords: []string{name}})
		}
	}
	if len(labels) > 30 {
		return nil, d.ErrInvalid
	}
	if len(labels) > 0 {
		if e := ValidateLabelSchema(labels); e != nil {
			return nil, e
		}
	}
	if (config.EnableTags || config.EnableClassification) && len(labels) == 0 {
		return nil, d.ErrInvalid
	}
	return labels, nil
}

type annotationSource struct {
	Content string
	QA      *d.QAPair
}

func annotationSources(input l.Input, task d.AnnotationTask) ([]annotationSource, error) {
	if task.Type != "qa" {
		contents, e := texts(input)
		if e != nil {
			return nil, e
		}
		out := make([]annotationSource, 0, len(contents))
		for _, content := range contents {
			out = append(out, annotationSource{Content: content})
		}
		return out, nil
	}
	if input.Kind != "table" || task.QAMapping == nil || len(input.Rows) == 0 || len(input.Rows) > 1000 {
		return nil, d.ErrInvalid
	}
	q, a := -1, -1
	for i, col := range input.Columns {
		if col == task.QAMapping.QuestionColumn {
			q = i
		}
		if col == task.QAMapping.ReferenceAnswerColumn {
			a = i
		}
	}
	if q < 0 || a < 0 || q == a {
		return nil, d.ErrInvalid
	}
	out := make([]annotationSource, 0, len(input.Rows))
	for _, row := range input.Rows {
		if q >= len(row) || a >= len(row) {
			return nil, d.ErrInvalid
		}
		question, ok := row[q].(string)
		if !ok {
			return nil, d.ErrInvalid
		}
		answer, ok := row[a].(string)
		if !ok || strings.TrimSpace(question) == "" || strings.TrimSpace(answer) == "" || utf8.RuneCountInString(question) > 20000 || utf8.RuneCountInString(answer) > 100000 {
			return nil, d.ErrInvalid
		}
		pair := &d.QAPair{Question: question, ReferenceAnswer: answer}
		// JSON encodes field boundaries unambiguously even for multiline source
		// cells. Content is also the immutable lookup key used by preannotation.
		content, _ := json.Marshal(pair)
		out = append(out, annotationSource{Content: string(content), QA: pair})
	}
	return out, nil
}
func literalEvidence(content, term, labelID, rule string, fold bool) []d.EvidenceSpan {
	if term == "" {
		return nil
	}
	original := []rune(content)
	haystack, needle := content, term
	if fold {
		haystack, needle = strings.ToLower(content), strings.ToLower(term)
	}
	offsets := make(map[int]int, len(haystack))
	pos := 0
	for offset := range haystack {
		offsets[offset] = pos
		pos++
	}
	offsets[len(haystack)] = pos
	out := []d.EvidenceSpan{}
	for base := 0; base <= len(haystack) && len(out) < 1000; {
		at := strings.Index(haystack[base:], needle)
		if at < 0 {
			break
		}
		start := base + at
		end := start + len(needle)
		rs, ok1 := offsets[start]
		re, ok2 := offsets[end]
		if ok1 && ok2 && re <= len(original) {
			out = append(out, d.EvidenceSpan{LabelID: labelID, Rule: rule, Match: string(original[rs:re]), Start: rs, End: re})
		}
		// Advance one rune from the match start so self-overlapping dictionary
		// occurrences are retained and can be classified as ambiguous.
		_, width := utf8.DecodeRuneInString(haystack[start:])
		base = start + width
	}
	return out
}
func classificationCandidate(content string, labels []d.Label) (d.Annotation, d.Provenance) {
	out := d.Annotation{Labels: []string{}, Entities: []d.Entity{}, Relations: []d.Relation{}}
	p := d.Provenance{Source: "rules", Method: "literal_keywords", DecisionStatus: "uncertain", Evidence: []string{}, EvidenceSpans: []d.EvidenceSpan{}}
	for _, label := range labels {
		terms := label.Keywords
		if len(terms) == 0 {
			terms = []string{label.Name}
		}
		matched := false
		for _, term := range terms {
			spans := literalEvidence(content, term, label.ID, "keyword", true)
			if len(spans) > 0 {
				matched = true
				p.Evidence = append(p.Evidence, term)
				p.EvidenceSpans = append(p.EvidenceSpans, spans...)
			}
			if len(p.EvidenceSpans) > 1000 {
				p.EvidenceSpans = p.EvidenceSpans[:1000]
			}
		}
		if matched {
			out.Labels = append(out.Labels, label.ID)
		}
	}
	if len(out.Labels) > 0 {
		p.DecisionStatus = "matched"
	}
	return out, p
}
func entityCandidate(content string, labels []d.Label) (d.Annotation, d.Provenance) {
	out := d.Annotation{Labels: []string{}, Entities: []d.Entity{}, Relations: []d.Relation{}}
	p := d.Provenance{Source: "rules", Method: "entity_dictionary", DecisionStatus: "uncertain", Evidence: []string{}, EvidenceSpans: []d.EvidenceSpan{}}
	candidates := []d.EvidenceSpan{}
	for _, label := range labels {
		for _, term := range label.Terms {
			candidates = append(candidates, literalEvidence(content, term, label.ID, "entity_dictionary", false)...)
			if len(candidates) > 2000 {
				return out, p
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		if a.End != b.End {
			return a.End > b.End
		}
		return a.LabelID < b.LabelID
	})
	ambiguous := false
	for i := 0; i < len(candidates); {
		end := candidates[i].End
		j := i + 1
		for j < len(candidates) && candidates[j].Start < end {
			if candidates[j].End > end {
				end = candidates[j].End
			}
			j++
		}
		// Overlapping dictionary meanings need review; no arbitrary label wins.
		if j-i > 1 {
			ambiguous = true
			p.EvidenceSpans = append(p.EvidenceSpans, candidates[i:j]...)
		} else {
			span := candidates[i]
			out.Entities = append(out.Entities, d.Entity{ID: fmt.Sprintf("entity-%d", len(out.Entities)+1), Label: span.LabelID, Start: span.Start, End: span.End, Text: span.Match})
			p.EvidenceSpans = append(p.EvidenceSpans, span)
			p.Evidence = append(p.Evidence, span.Match)
		}
		i = j
	}
	if len(out.Entities) > 0 && !ambiguous {
		p.DecisionStatus = "matched"
	}
	return out, p
}
func candidateFor(task d.AnnotationTask, source annotationSource) (d.Annotation, d.Provenance, error) {
	switch task.Type {
	case "classification":
		a, p := classificationCandidate(source.Content, task.Labels)
		return a, p, nil
	case "named_entity":
		a, p := entityCandidate(source.Content, task.Labels)
		return a, p, nil
	case "qa":
		if source.QA == nil {
			return d.Annotation{}, d.Provenance{}, d.ErrInvalid
		}
		return d.Annotation{Labels: []string{}, Entities: []d.Entity{}, Relations: []d.Relation{}, QA: source.QA}, d.Provenance{Source: "source_mapping", Method: "explicit_qa_columns", DecisionStatus: "matched", Evidence: []string{task.QAMapping.QuestionColumn, task.QAMapping.ReferenceAnswerColumn}}, nil
	default:
		return d.Annotation{}, d.Provenance{}, fmt.Errorf("%w: relation preannotation requires explicit relation rules; manual annotation only", d.ErrInvalid)
	}
}
func applyTagRules(out *d.TaggingResult, labels []d.Label, config d.TaggingConfig) {
	a, p := classificationCandidate(out.Content, labels)
	names := map[string]string{}
	for _, label := range labels {
		names[label.ID] = label.Name
	}
	out.Tags = []string{}
	if config.EnableTags {
		for _, id := range a.Labels {
			out.Tags = append(out.Tags, names[id])
		}
	}
	out.Category = ""
	if config.EnableClassification {
		if len(a.Labels) == 1 {
			out.Category = names[a.Labels[0]]
		} else {
			p.DecisionStatus = "uncertain"
		}
	}
	if config.EnableExtraction {
		entities, ep := entityCandidate(out.Content, labels)
		out.Entities, _ = json.Marshal(entities.Entities)
		p.EvidenceSpans = append(p.EvidenceSpans, ep.EvidenceSpans...)
		for _, label := range labels {
			if len(label.Terms) > 0 && ep.DecisionStatus == "uncertain" {
				p.DecisionStatus = "uncertain"
				break
			}
		}
	}
	out.Provenance = p
	out.TagConfidence = nil
	out.CategoryConfidence = nil
	out.QualityScore = nil
}
