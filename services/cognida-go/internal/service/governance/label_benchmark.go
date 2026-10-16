package governance

// This benchmark deliberately calls the production literal-rule engine. Training
// and scoring are separate APIs: prediction callbacks never receive held-out gold.
import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	d "cognida/internal/model/governance"
)

const LabelBenchmarkRuleVersion = "train_literal_v1"

// Offsets use Unicode codepoints, start inclusive and end exclusive. The
// downloader must record any token-to-text reconstruction in its manifest.
type LabelBenchmarkSample struct {
	ID       string     `json:"id"`
	Split    string     `json:"split"`
	Text     string     `json:"text"`
	Label    string     `json:"label,omitempty"`
	Entities []d.Entity `json:"entities,omitempty"`
}
type LabelBenchmarkInput struct{ ID, Text, Kind string }
type LabelBenchmarkPrediction struct {
	Annotation     d.Annotation         `json:"annotation"`
	DecisionStatus string               `json:"decision_status"`
	Usage          *LabelBenchmarkUsage `json:"usage,omitempty"`
}
type LabelBenchmarkUsage struct {
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	TotalTokens      int      `json:"total_tokens"`
	CostUSD          *float64 `json:"cost_usd"`
}
type LabelBenchmarkPredictor func(context.Context, LabelBenchmarkInput) (LabelBenchmarkPrediction, error)
type LabelBenchmarkOutcome struct {
	ID           string                   `json:"id"`
	TextDigest   string                   `json:"text_sha256"`
	GoldLabel    string                   `json:"gold_label,omitempty"`
	GoldEntities []d.Entity               `json:"gold_entities,omitempty"`
	Prediction   LabelBenchmarkPrediction `json:"prediction"`
	ErrorCode    string                   `json:"error_code,omitempty"`
	Abstained    bool                     `json:"abstained"`
	Ambiguous    bool                     `json:"ambiguous"`
	LatencyMS    float64                  `json:"latency_ms"`
}
type LabelBenchmarkClassMetric struct {
	Label     string  `json:"label"`
	Support   int     `json:"support"`
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}
type LabelBenchmarkReport struct {
	Kind                  string                      `json:"kind"`
	Samples               int                         `json:"samples"`
	Covered               int                         `json:"covered"`
	Abstained             int                         `json:"abstained"`
	Ambiguous             int                         `json:"ambiguous"`
	ContractErrors        int                         `json:"contract_errors"`
	ExecutionErrors       int                         `json:"execution_errors"`
	Coverage              float64                     `json:"coverage"`
	AbstentionRate        float64                     `json:"abstention_rate"`
	ContractErrorRate     float64                     `json:"contract_error_rate"`
	Accuracy              *float64                    `json:"accuracy"`
	MicroPrecision        float64                     `json:"micro_precision"`
	MicroRecall           float64                     `json:"micro_recall"`
	MicroF1               float64                     `json:"micro_f1"`
	MacroPrecision        float64                     `json:"macro_precision"`
	MacroRecall           float64                     `json:"macro_recall"`
	MacroF1               float64                     `json:"macro_f1"`
	MacroF1SupportedOnly  float64                     `json:"macro_f1_supported_only"`
	LabelsWithGoldSupport int                         `json:"labels_with_gold_support"`
	LatencyMeanMS         float64                     `json:"latency_mean_ms"`
	LatencyP50MS          float64                     `json:"latency_p50_ms"`
	LatencyP95MS          float64                     `json:"latency_p95_ms"`
	UsageObservedSamples  int                         `json:"usage_observed_samples"`
	Usage                 LabelBenchmarkUsage         `json:"usage"`
	PerLabel              []LabelBenchmarkClassMetric `json:"per_label"`
	Outcomes              []LabelBenchmarkOutcome     `json:"outcomes"`
}

// ReadLabelBenchmarkJSONL rejects malformed input and duplicate IDs. File limits
// prevent an accidentally selected unrelated export becoming an unbounded run.
func ReadLabelBenchmarkJSONL(r io.Reader) ([]LabelBenchmarkSample, error) {
	limited := &io.LimitedReader{R: r, N: (256 << 20) + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	var out []LabelBenchmarkSample
	seen := map[string]bool{}
	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(strings.TrimSpace(string(raw))) == 0 {
			return nil, fmt.Errorf("empty benchmark row")
		}
		var v LabelBenchmarkSample
		if json.Unmarshal(raw, &v) != nil || v.ID == "" || seen[v.ID] || !utf8.ValidString(v.Text) || strings.TrimSpace(v.Text) == "" || (v.Split != "train" && v.Split != "validation" && v.Split != "test") {
			return nil, fmt.Errorf("invalid benchmark row %d", len(out)+1)
		}
		seen[v.ID] = true
		out = append(out, v)
		if len(out) > 250000 {
			return nil, fmt.Errorf("benchmark row limit exceeded")
		}
	}
	if limited.N <= 0 {
		return nil, fmt.Errorf("benchmark file limit exceeded")
	}
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty benchmark")
	}
	return out, nil
}

// FitLabelBenchmarkRules only accepts train rows. Classification selects at most
// five literal unigram/bigram rules per class, document frequency >=2, ranked by
// smoothed class-vs-rest log likelihood ratio. NER selects at most 50 exact
// surfaces per type by training frequency. Limits are fixed before test access.
// No label name, description, or held-out text is synthesized into a rule.
func FitLabelBenchmarkRules(kind string, schema []d.Label, train []LabelBenchmarkSample, language ...string) ([]d.Label, error) {
	if kind != "classification" && kind != "named_entity" {
		return nil, d.ErrInvalid
	}
	if ValidateLabelSchema(schema) != nil || len(train) == 0 {
		return nil, d.ErrInvalid
	}
	lang := "en"
	if len(language) > 1 {
		return nil, d.ErrInvalid
	}
	if len(language) == 1 {
		lang = language[0]
	}
	if lang != "en" && lang != "zh-CN" {
		return nil, d.ErrInvalid
	}
	labels := make([]d.Label, len(schema))
	byID := map[string]int{}
	counts := map[string]map[string]int{}
	totals := map[string]int{}
	global := map[string]int{}
	for i, v := range schema {
		labels[i] = d.Label{ID: v.ID, Name: v.Name}
		byID[v.ID] = i
		counts[v.ID] = map[string]int{}
	}
	for _, row := range train {
		if row.Split != "train" {
			return nil, fmt.Errorf("held-out data cannot fit rules")
		}
		if kind == "classification" {
			if _, ok := byID[row.Label]; !ok || len(row.Entities) != 0 {
				return nil, d.ErrInvalid
			}
			totals[row.Label]++
			for term := range benchmarkTerms(row.Text, lang) {
				counts[row.Label][term]++
				global[term]++
			}
		} else {
			if row.Label != "" {
				return nil, d.ErrInvalid
			}
			for _, entity := range row.Entities {
				if _, ok := byID[entity.Label]; !ok || !benchmarkSpanValid(row.Text, entity) {
					return nil, d.ErrInvalid
				}
				if utf8.RuneCountInString(entity.Text) <= 200 && strings.TrimSpace(entity.Text) == entity.Text {
					counts[entity.Label][entity.Text]++
				}
			}
		}
	}
	for id, countsForLabel := range counts {
		type scored struct {
			term  string
			count int
			score float64
		}
		candidates := []scored{}
		for term, count := range countsForLabel {
			score := float64(count)
			if kind == "classification" {
				if count < 2 {
					continue
				}
				other := global[term] - count
				score = math.Log(float64(count+1)/float64(totals[id]+2)) - math.Log(float64(other+1)/float64(len(train)-totals[id]+2))
				if score <= 0 {
					continue
				}
			}
			candidates = append(candidates, scored{term, count, score})
		}
		sort.Slice(candidates, func(i, j int) bool {
			a, b := candidates[i], candidates[j]
			if a.score != b.score {
				return a.score > b.score
			}
			if a.count != b.count {
				return a.count > b.count
			}
			return a.term < b.term
		})
		limit := 5
		if kind == "named_entity" {
			limit = 50
		}
		if len(candidates) > limit {
			candidates = candidates[:limit]
		}
		for _, v := range candidates {
			i := byID[id]
			if kind == "classification" {
				labels[i].Keywords = append(labels[i].Keywords, v.term)
			} else {
				labels[i].Terms = append(labels[i].Terms, v.term)
			}
		}
		// The production classification fallback is label.Name. A class with no
		// learned rules must explicitly abstain rather than acquire a name heuristic.
		if kind == "classification" && len(labels[byID[id]].Keywords) == 0 {
			return nil, fmt.Errorf("class %s has no eligible training rules", id)
		}
	}
	if ValidateLabelSchema(labels) != nil {
		return nil, d.ErrInvalid
	}
	return labels, nil
}
func benchmarkTerms(text, language string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	out := map[string]bool{}
	if language == "zh-CN" {
		for _, w := range words {
			runes := []rune(w)
			for _, n := range []int{2, 3} {
				for at := 0; at+n <= len(runes); at++ {
					out[string(runes[at:at+n])] = true
				}
			}
		}
		return out
	}
	for i, w := range words {
		if utf8.RuneCountInString(w) >= 2 && utf8.RuneCountInString(w) <= 200 {
			out[w] = true
		}
		if i+1 < len(words) {
			pair := w + " " + words[i+1]
			if utf8.RuneCountInString(pair) <= 200 && strings.Contains(strings.ToLower(text), pair) {
				out[pair] = true
			}
		}
	}
	return out
}
func benchmarkSpanValid(text string, e d.Entity) bool {
	r := []rune(text)
	return e.Start >= 0 && e.End > e.Start && e.End <= len(r) && e.Text == string(r[e.Start:e.End])
}
func NewLabelBenchmarkRulePredictor(kind string, labels []d.Label) (LabelBenchmarkPredictor, error) {
	if ValidateLabelSchema(labels) != nil || (kind != "classification" && kind != "named_entity") {
		return nil, d.ErrInvalid
	}
	// The JSON copy prevents later mutation of the trained schema during scoring.
	raw, _ := json.Marshal(labels)
	var frozen []d.Label
	_ = json.Unmarshal(raw, &frozen)
	return func(ctx context.Context, in LabelBenchmarkInput) (LabelBenchmarkPrediction, error) {
		if err := ctx.Err(); err != nil {
			return LabelBenchmarkPrediction{}, err
		}
		if in.Kind != kind {
			return LabelBenchmarkPrediction{}, d.ErrInvalid
		}
		a, p, err := candidateFor(d.AnnotationTask{Type: kind, Labels: frozen}, annotationSource{Content: in.Text})
		return LabelBenchmarkPrediction{Annotation: a, DecisionStatus: p.DecisionStatus}, err
	}, nil
}

// EvaluateLabelBenchmark never discards failed rows. Single-label ambiguity or
// uncertain predictions abstain. NER scores all valid emitted spans exactly,
// including drafts marked uncertain, while coverage separately means matched.
// Macro includes the entire declared schema (zero support => zero F1); a second
// explicitly named metric reports supported classes only for small model samples.
func EvaluateLabelBenchmark(ctx context.Context, kind string, schema []d.Label, test []LabelBenchmarkSample, predict LabelBenchmarkPredictor, parallel ...int) (*LabelBenchmarkReport, error) {
	if predict == nil || ValidateLabelSchema(schema) != nil || len(test) == 0 || (kind != "classification" && kind != "named_entity") {
		return nil, d.ErrInvalid
	}
	r := &LabelBenchmarkReport{Kind: kind, Samples: len(test), PerLabel: []LabelBenchmarkClassMetric{}, Outcomes: []LabelBenchmarkOutcome{}}
	known := map[string]bool{}
	metrics := map[string]*LabelBenchmarkClassMetric{}
	for _, v := range schema {
		known[v.ID] = true
		metrics[v.ID] = &LabelBenchmarkClassMetric{Label: v.ID}
	}
	ids := map[string]bool{}
	for _, sample := range test {
		if sample.ID == "" || ids[sample.ID] || (sample.Split != "test" && sample.Split != "validation") || strings.TrimSpace(sample.Text) == "" || !utf8.ValidString(sample.Text) {
			return nil, d.ErrInvalid
		}
		ids[sample.ID] = true
		if kind == "classification" {
			if !known[sample.Label] || len(sample.Entities) > 0 {
				return nil, d.ErrInvalid
			}
		} else {
			if sample.Label != "" {
				return nil, d.ErrInvalid
			}
			seenSpans := map[string]bool{}
			for _, e := range sample.Entities {
				if !known[e.Label] || !benchmarkSpanValid(sample.Text, e) || seenSpans[benchmarkSpanKey(e)] {
					return nil, d.ErrInvalid
				}
				seenSpans[benchmarkSpanKey(e)] = true
			}
		}
	}
	workers := 1
	if len(parallel) > 1 {
		return nil, d.ErrInvalid
	}
	if len(parallel) == 1 {
		workers = parallel[0]
	}
	if workers < 1 || workers > 2 {
		return nil, d.ErrInvalid
	}
	type measured struct {
		prediction LabelBenchmarkPrediction
		err        error
		elapsed    float64
	}
	measuredResults := make([]measured, len(test))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				sample := test[index]
				started := time.Now()
				err := ctx.Err()
				var prediction LabelBenchmarkPrediction
				if err == nil {
					prediction, err = predict(ctx, LabelBenchmarkInput{ID: sample.ID, Text: sample.Text, Kind: kind})
				}
				measuredResults[index] = measured{prediction, err, float64(time.Since(started).Nanoseconds()) / 1e6}
			}
		}()
	}
	for i := range test {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	latencies := []float64{}
	correct := 0
	costKnown := true
	cost := 0.0
	for index, sample := range test {
		prediction, err, elapsed := measuredResults[index].prediction, measuredResults[index].err, measuredResults[index].elapsed
		latencies = append(latencies, elapsed)
		sum := sha256.Sum256([]byte(sample.Text))
		out := LabelBenchmarkOutcome{ID: sample.ID, TextDigest: hex.EncodeToString(sum[:]), GoldLabel: sample.Label, GoldEntities: sample.Entities, Prediction: prediction, LatencyMS: elapsed}
		if err == nil && (validateAnnotation(d.AnnotationTask{Type: kind, Labels: schema}, sample.Text, prediction.Annotation) != nil || (prediction.DecisionStatus != "matched" && prediction.DecisionStatus != "uncertain")) {
			err = ErrLabelModelOutput
		}
		if u := prediction.Usage; u != nil {
			r.UsageObservedSamples++
			r.Usage.PromptTokens += u.PromptTokens
			r.Usage.CompletionTokens += u.CompletionTokens
			r.Usage.TotalTokens += u.TotalTokens
			if u.CostUSD != nil {
				cost += *u.CostUSD
			} else {
				costKnown = false
			}
		} else {
			costKnown = false
		}
		if err != nil {
			out.ErrorCode = benchmarkErrorCode(err)
			if out.ErrorCode == "contract_error" {
				r.ContractErrors++
			} else {
				r.ExecutionErrors++
			}
		}
		out.Abstained = err != nil || prediction.DecisionStatus != "matched"
		if kind == "classification" {
			truth := metrics[sample.Label]
			truth.Support++
			out.Ambiguous = err == nil && len(prediction.Annotation.Labels) > 1
			if len(prediction.Annotation.Labels) != 1 {
				out.Abstained = true
			}
			if !out.Abstained {
				id := prediction.Annotation.Labels[0]
				if id == sample.Label {
					truth.TP++
					correct++
				} else {
					metrics[id].FP++
					truth.FN++
				}
			} else {
				truth.FN++
			}
		} else {
			gold := map[string]d.Entity{}
			for _, e := range sample.Entities {
				key := benchmarkSpanKey(e)
				if _, exists := gold[key]; exists {
					return nil, d.ErrInvalid
				}
				gold[key] = e
				metrics[e.Label].Support++
			}
			emitted := map[string]bool{}
			if err == nil {
				for _, e := range prediction.Annotation.Entities {
					key := benchmarkSpanKey(e)
					if emitted[key] {
						metrics[e.Label].FP++
						continue
					}
					emitted[key] = true
					if _, ok := gold[key]; ok {
						metrics[e.Label].TP++
					} else {
						metrics[e.Label].FP++
					}
				}
			}
			for key, e := range gold {
				if !emitted[key] {
					metrics[e.Label].FN++
				}
			}
		}
		if out.Abstained {
			r.Abstained++
		} else {
			r.Covered++
		}
		if out.Ambiguous {
			r.Ambiguous++
		}
		r.Outcomes = append(r.Outcomes, out)
	}
	if kind == "classification" {
		accuracy := float64(correct) / float64(len(test))
		r.Accuracy = &accuracy
	}
	r.Coverage = float64(r.Covered) / float64(r.Samples)
	r.AbstentionRate = float64(r.Abstained) / float64(r.Samples)
	r.ContractErrorRate = float64(r.ContractErrors) / float64(r.Samples)
	tp, fp, fn := 0, 0, 0
	for _, label := range schema {
		m := metrics[label.ID]
		m.Precision = benchmarkRatio(m.TP, m.TP+m.FP)
		m.Recall = benchmarkRatio(m.TP, m.TP+m.FN)
		m.F1 = benchmarkRatio(2*m.TP, 2*m.TP+m.FP+m.FN)
		r.MacroPrecision += m.Precision
		r.MacroRecall += m.Recall
		r.MacroF1 += m.F1
		if m.Support > 0 {
			r.LabelsWithGoldSupport++
			r.MacroF1SupportedOnly += m.F1
		}
		tp += m.TP
		fp += m.FP
		fn += m.FN
		r.PerLabel = append(r.PerLabel, *m)
	}
	r.MacroPrecision /= float64(len(schema))
	r.MacroRecall /= float64(len(schema))
	r.MacroF1 /= float64(len(schema))
	if r.LabelsWithGoldSupport > 0 {
		r.MacroF1SupportedOnly /= float64(r.LabelsWithGoldSupport)
	}
	r.MicroPrecision = benchmarkRatio(tp, tp+fp)
	r.MicroRecall = benchmarkRatio(tp, tp+fn)
	r.MicroF1 = benchmarkRatio(2*tp, 2*tp+fp+fn)
	sort.Float64s(latencies)
	for _, v := range latencies {
		r.LatencyMeanMS += v
	}
	r.LatencyMeanMS /= float64(len(latencies))
	r.LatencyP50MS = latencies[int(math.Ceil(.5*float64(len(latencies))))-1]
	r.LatencyP95MS = latencies[int(math.Ceil(.95*float64(len(latencies))))-1]
	if costKnown {
		r.Usage.CostUSD = &cost
	}
	return r, nil
}
func benchmarkRatio(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}
func benchmarkSpanKey(e d.Entity) string { return fmt.Sprintf("%s:%d:%d", e.Label, e.Start, e.End) }
func benchmarkErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrLabelModelOutput), errors.Is(err, d.ErrInvalid):
		return "contract_error"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrLabelModelUnavailable):
		return "model_unavailable"
	default:
		return "execution_error"
	}
}

// These task instructions are frozen independently of benchmark examples. Gold
// annotations and corpus-specific demonstrations are never part of this prompt.
func LabelBenchmarkGuidelines(kind string) string {
	if kind == "classification" {
		return "Classify the complete utterance into exactly one declared intent label when supported. If it is ambiguous or unsupported, return uncertain with no labels. Supply literal evidence from the utterance; do not infer approval or confidence."
	}
	if kind == "named_entity" {
		return "Extract all named entities whose types occur in the declared schema. Use exact original text. Omit offsets for unique literal anchors; repeated text requires correct Unicode codepoint offsets. If no supported entity is present, return uncertain with empty entities. Do not invent entities, change text, or infer approval or confidence."
	}
	return ""
}
func LabelBenchmarkPromptSHA256(kind string) string {
	sum := sha256.Sum256([]byte(labelSuggestionPromptForKind(kind) + "\n" + LabelBenchmarkGuidelines(kind)))
	return hex.EncodeToString(sum[:])
}
