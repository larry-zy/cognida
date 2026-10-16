//go:build integration

package governance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cognida/internal/config"
	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	provider "cognida/internal/infrastructure/llm"
	store "cognida/internal/infrastructure/objectstore"
	ev "cognida/internal/model/evaluation"
	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"cognida/internal/model/llm"
	repo "cognida/internal/repository/mysql"
	evaluation "cognida/internal/service/evaluation"
	svc "cognida/internal/service/governance"
	lifecycle "cognida/internal/service/lifecycle"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type hfLabelConfig struct {
	Dataset   string    `json:"dataset"`
	Revision  string    `json:"revision"`
	Language  string    `json:"language"`
	Kind      string    `json:"kind"`
	TrainFile string    `json:"train_file"`
	TestFile  string    `json:"test_file"`
	TrainSHA  string    `json:"train_sha256"`
	TestSHA   string    `json:"test_sha256"`
	Labels    []d.Label `json:"labels"`
}
type hfLabelProtocol struct {
	Version     string                        `json:"version"`
	CreatedAt   time.Time                     `json:"created_at"`
	Config      hfLabelConfig                 `json:"config"`
	Rules       []d.Label                     `json:"rules"`
	RulesSHA    string                        `json:"rules_sha256"`
	PromptSHA   string                        `json:"prompt_sha256"`
	CodeSHA     map[string]string             `json:"code_sha256"`
	Selection   []svc.LabelBenchmarkSelection `json:"model_selection"`
	Seed        string                        `json:"seed"`
	Model       string                        `json:"model"`
	Provider    string                        `json:"provider"`
	ModelLimit  int                           `json:"model_call_limit"`
	Parallel    int                           `json:"parallel"`
	TrainRows   int                           `json:"train_rows"`
	TestRows    int                           `json:"official_test_rows"`
	OverlapRows int                           `json:"train_test_overlap_rows"`
	Notes       []string                      `json:"notes"`
}
type hfLabelResult struct {
	ProtocolSHA string                    `json:"protocol_sha256"`
	Mode        string                    `json:"mode"`
	ActualCalls int64                     `json:"actual_model_calls"`
	Full        *svc.LabelBenchmarkReport `json:"full"`
	Unseen      *svc.LabelBenchmarkReport `json:"unseen_text"`
	Overlap     *svc.LabelBenchmarkReport `json:"train_overlap"`
}

var hfLabelCodeFiles = []string{
	"label_benchmark.go", "label_benchmark_protocol.go", "label_benchmark_runner_integration_test.go", "label_suggester.go", "label_literal_contract.go", "label_suggestions.go", "label_rules.go", "annotation.go",
}

func hfRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}
func hfJSON(t *testing.T, path string, v any, exclusive bool) {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if exclusive {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	f, err := os.OpenFile(path, flags, 0600)
	require.NoError(t, err)
	_, err = f.Write(append(raw, '\n'))
	require.NoError(t, err)
	require.NoError(t, f.Sync())
	require.NoError(t, f.Close())
}
func hfReadData(t *testing.T, c hfLabelConfig) ([]svc.LabelBenchmarkSample, []svc.LabelBenchmarkSample) {
	t.Helper()
	raw := hfRead(t, c.TrainFile)
	require.Equal(t, c.TrainSHA, svc.LabelBenchmarkTextDigest(string(raw)))
	train, err := svc.ReadLabelBenchmarkJSONL(bytes.NewReader(raw))
	require.NoError(t, err)
	raw = hfRead(t, c.TestFile)
	require.Equal(t, c.TestSHA, svc.LabelBenchmarkTextDigest(string(raw)))
	test, err := svc.ReadLabelBenchmarkJSONL(bytes.NewReader(raw))
	require.NoError(t, err)
	return train, test
}
func hfCode(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range hfLabelCodeFiles {
		out[name] = svc.LabelBenchmarkTextDigest(string(hfRead(t, name)))
	}
	return out
}

// This opt-in runner never asserts a quality threshold. All outcomes, including
// provider/contract failures, are artifacts. A model.started O_EXCL marker makes
// accidental go-test reruns fail before any provider call; there is no resume.
func TestHFLabelBenchmark(t *testing.T) {
	path := os.Getenv("COGNIDA_LABEL_BENCHMARK_CONFIG")
	if path == "" {
		t.Skip("explicit fixed-revision benchmark config required")
	}
	mode := os.Getenv("COGNIDA_LABEL_BENCHMARK_MODE")
	require.Contains(t, []string{"prepare", "rules", "model", "oracle"}, mode)
	out := os.Getenv("COGNIDA_LABEL_BENCHMARK_OUTPUT")
	require.NotEmpty(t, out)
	require.NoError(t, os.MkdirAll(out, 0700))
	var cfg hfLabelConfig
	require.NoError(t, json.Unmarshal(hfRead(t, path), &cfg))
	require.Len(t, cfg.Revision, 40)
	train, test := hfReadData(t, cfg)
	seen := map[string]bool{}
	for _, row := range train {
		seen[svc.LabelBenchmarkTextDigest(row.Text)] = true
	}
	protocolPath := filepath.Join(out, "protocol.json")
	if mode == "prepare" {
		rules, err := svc.FitLabelBenchmarkRules(cfg.Kind, cfg.Labels, train, cfg.Language)
		require.NoError(t, err)
		selection, err := svc.SelectLabelBenchmarkModelSamples(cfg.Kind, cfg.Labels, train, test)
		require.NoError(t, err)
		limit := 64
		if cfg.Kind == "classification" {
			limit = len(selection)
		}
		require.LessOrEqual(t, limit, 154)
		require.Len(t, selection, limit)
		chat := config.LoadChatConfig()
		raw, _ := json.Marshal(rules)
		overlap := 0
		for _, row := range test {
			if seen[svc.LabelBenchmarkTextDigest(row.Text)] {
				overlap++
			}
		}
		p := hfLabelProtocol{Version: "hf_label_v3_literal_contract", CreatedAt: time.Now().UTC(), Config: cfg, Rules: rules, RulesSHA: svc.LabelBenchmarkTextDigest(string(raw)), PromptSHA: svc.LabelBenchmarkPromptSHA256(cfg.Kind), CodeSHA: hfCode(t), Selection: selection, Seed: svc.LabelBenchmarkSeed, Model: chat.ModelName, Provider: chat.Provider, ModelLimit: limit, Parallel: 2, TrainRows: len(train), TestRows: len(test), OverlapRows: overlap, Notes: []string{
			"Official full held-out rule denominator is unchanged; exact-text overlap and unseen-text metrics are separate.",
			"Classification model sample: min(2, official test class size) hash-ranked examples per declared class. NER: 64 hash-ranked unseen texts. No difficulty/length filtering. MASSIVE cooking_query has zero test rows, general_greet one: 117 selected, no train substitution or budget reallocation. This protocol correction preceded all model calls.",
			"One model call per selected sample, concurrency two, 20 second production deadline, 4096 tokens, no retry or repair.",
			"Gold labels never enter model requests; taxonomy names are public schema. Training-only literal rules do not enter model prompts.",
			"NER exact coarse spans use the downloader's space-joined token reconstruction. Uncertain valid spans are scored; coverage is reported separately.",
			"Manual gold review/publication is an oracle fixture, never automatic annotation quality. No billing estimates; missing cost remains null.",
		}}
		hfJSON(t, protocolPath, p, true)
		t.Logf("pre-registered %d model samples; %d official test rows, %d exact overlap", len(selection), len(test), overlap)
		return
	}
	raw := hfRead(t, protocolPath)
	var p hfLabelProtocol
	require.NoError(t, json.Unmarshal(raw, &p))
	require.Equal(t, cfg, p.Config)
	require.Equal(t, hfCode(t), p.CodeSHA, "code changed after registration")
	require.Equal(t, svc.LabelBenchmarkPromptSHA256(cfg.Kind), p.PromptSHA)
	rulesRaw, _ := json.Marshal(p.Rules)
	require.Equal(t, p.RulesSHA, svc.LabelBenchmarkTextDigest(string(rulesRaw)))
	selected, err := svc.SelectLabelBenchmarkModelSamples(cfg.Kind, cfg.Labels, train, test)
	require.NoError(t, err)
	require.Equal(t, selected, p.Selection)
	byID := map[string]svc.LabelBenchmarkSample{}
	for _, v := range test {
		byID[v.ID] = v
	}
	modelRows := []svc.LabelBenchmarkSample{}
	for _, v := range selected {
		row := byID[v.ID]
		require.Equal(t, v.TextSHA256, svc.LabelBenchmarkTextDigest(row.Text))
		modelRows = append(modelRows, row)
	}
	if mode == "oracle" {
		hfRunOracle(t, cfg, p.Rules, modelRows[:3], out)
		return
	}
	var predictor svc.LabelBenchmarkPredictor
	var calls *atomic.Int64
	rows := test
	if mode == "rules" {
		predictor, err = svc.NewLabelBenchmarkRulePredictor(cfg.Kind, p.Rules)
		require.NoError(t, err)
	} else {
		require.Equal(t, "1", os.Getenv("COGNIDA_LABEL_BENCHMARK_LIVE"), "live model requires explicit gate")
		chat := config.LoadChatConfig()
		require.Equal(t, p.Model, chat.ModelName)
		require.Equal(t, p.Provider, chat.Provider)
		require.NotEmpty(t, chat.APIKey)
		f := hfLabelDB(t)
		client, e := provider.NewChatRepository(&llm.ModelConfig{Provider: llm.Provider(chat.Provider), ModelName: chat.ModelName, BaseURL: chat.BaseURL, APIKey: chat.APIKey})
		require.NoError(t, e)
		observed := &hfObservedClient{LLMClient: client, limit: int64(p.ModelLimit)}
		calls = &observed.calls
		f.service.WithLabelSuggester(svc.NewLLMLabelSuggester(observed, chat.Provider+":"+chat.ModelName))
		journal, e := os.OpenFile(filepath.Join(out, "model-outcomes.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		require.NoError(t, e)
		defer journal.Close()
		hfJSON(t, filepath.Join(out, "model.started"), map[string]any{"protocol_sha256": svc.LabelBenchmarkTextDigest(string(raw)), "started_at": time.Now().UTC(), "tenant_id": f.p.TenantID}, true)
		var journalMu sync.Mutex
		predictor = func(ctx context.Context, in svc.LabelBenchmarkInput) (pred svc.LabelBenchmarkPrediction, err error) {
			started := time.Now()
			capture := &hfUsageCapture{}
			ctx = context.WithValue(ctx, hfUsageKey{}, capture)
			defer func() {
				pred.Usage = capture.usage
				entry := map[string]any{"id": in.ID, "text_sha256": svc.LabelBenchmarkTextDigest(in.Text), "prediction": pred, "error_code": hfSafeError(err), "latency_ms": float64(time.Since(started).Nanoseconds()) / 1e6}
				data, e := json.Marshal(entry)
				journalMu.Lock()
				defer journalMu.Unlock()
				if e == nil {
					_, e = journal.Write(append(data, '\n'))
				}
				if e == nil {
					e = journal.Sync()
				}
				if e != nil {
					err = errors.New("outcome journal write failed")
				}
			}()
			task, item, e := f.create(ctx, in, cfg.Labels)
			if e != nil {
				return pred, e
			}
			record, e := f.service.SuggestLabel(ctx, f.p, "annotation", task.ID, item.ID, svc.LabelSuggestionRequest{ExpectedVersion: item.Version, Guidelines: svc.LabelBenchmarkGuidelines(cfg.Kind)}, "model:"+svc.LabelBenchmarkTextDigest(in.ID)[:40])
			if e != nil {
				return pred, e
			}
			var result d.AnnotationItem
			if json.Unmarshal(record.Payload, &result) != nil {
				return pred, d.ErrInvalid
			}
			return svc.LabelBenchmarkPrediction{Annotation: result.Annotation, DecisionStatus: result.Provenance.DecisionStatus}, nil
		}
		rows = modelRows
	}
	report, err := svc.EvaluateLabelBenchmark(context.Background(), cfg.Kind, cfg.Labels, rows, predictor, 2)
	require.NoError(t, err)
	result := hfLabelResult{ProtocolSHA: svc.LabelBenchmarkTextDigest(string(raw)), Mode: mode, Full: report}
	if calls != nil {
		result.ActualCalls = calls.Load()
		require.LessOrEqual(t, result.ActualCalls, int64(p.ModelLimit))
	}
	result.Unseen = hfSubset(t, cfg, rows, report, seen, false)
	result.Overlap = hfSubset(t, cfg, rows, report, seen, true)
	hfJSON(t, filepath.Join(out, mode+"-report.json"), result, true)
	t.Logf("%s samples=%d macro_f1=%.6f micro_f1=%.6f coverage=%.6f contract_errors=%d execution_errors=%d model_calls=%d", mode, report.Samples, report.MacroF1, report.MicroF1, report.Coverage, report.ContractErrors, report.ExecutionErrors, result.ActualCalls)
}

func hfSubset(t *testing.T, cfg hfLabelConfig, rows []svc.LabelBenchmarkSample, report *svc.LabelBenchmarkReport, seen map[string]bool, overlap bool) *svc.LabelBenchmarkReport {
	t.Helper()
	subset := []svc.LabelBenchmarkSample{}
	original := map[string]svc.LabelBenchmarkOutcome{}
	for i, row := range rows {
		if seen[svc.LabelBenchmarkTextDigest(row.Text)] == overlap {
			subset = append(subset, row)
			original[row.ID] = report.Outcomes[i]
		}
	}
	if len(subset) == 0 {
		return nil
	}
	r, err := svc.EvaluateLabelBenchmark(context.Background(), cfg.Kind, cfg.Labels, subset, func(_ context.Context, in svc.LabelBenchmarkInput) (svc.LabelBenchmarkPrediction, error) {
		v := original[in.ID]
		return v.Prediction, hfError(v.ErrorCode)
	})
	require.NoError(t, err)
	latencies := []float64{}
	r.LatencyMeanMS = 0
	for i := range r.Outcomes {
		v := original[r.Outcomes[i].ID]
		r.Outcomes[i].LatencyMS = v.LatencyMS
		r.LatencyMeanMS += v.LatencyMS
		latencies = append(latencies, v.LatencyMS)
	}
	sort.Float64s(latencies)
	r.LatencyMeanMS /= float64(len(latencies))
	r.LatencyP50MS = latencies[(len(latencies)*50+99)/100-1]
	r.LatencyP95MS = latencies[(len(latencies)*95+99)/100-1]
	return r
}
func hfSafeError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, svc.ErrLabelModelOutput), errors.Is(err, d.ErrInvalid):
		return "contract_error"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, svc.ErrLabelModelUnavailable):
		return "model_unavailable"
	default:
		return "execution_error"
	}
}
func hfError(code string) error {
	switch code {
	case "":
		return nil
	case "contract_error":
		return svc.ErrLabelModelOutput
	case "timeout":
		return context.DeadlineExceeded
	case "cancelled":
		return context.Canceled
	case "model_unavailable":
		return svc.ErrLabelModelUnavailable
	default:
		return errors.New("execution error")
	}
}

type hfUsageKey struct{}
type hfUsageCapture struct{ usage *svc.LabelBenchmarkUsage }
type hfObservedClient struct {
	llm.LLMClient
	calls atomic.Int64
	limit int64
}

func (c *hfObservedClient) Chat(ctx context.Context, r *llm.ChatRequest) (*llm.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.calls.Add(1) > c.limit {
		return nil, errors.New("model budget exhausted")
	}
	out, err := c.LLMClient.Chat(ctx, r)
	if capture, ok := ctx.Value(hfUsageKey{}).(*hfUsageCapture); ok && out != nil && out.Usage != nil {
		u := out.Usage
		capture.usage = &svc.LabelBenchmarkUsage{PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens, TotalTokens: u.TotalTokens}
	}
	return out, err
}

// Synchronous asset import/annotation never uses a queue. Any unexpected
// asynchronous call is rejected instead of pretending to execute a worker.
type hfNoQueue struct{}

func (hfNoQueue) Enqueue(context.Context, job.Job) error {
	return errors.New("unexpected async operation")
}
func (hfNoQueue) Claim(context.Context, time.Duration) (*job.Job, error) {
	return nil, errors.New("unused queue")
}
func (hfNoQueue) Ack(context.Context, job.Job) error { return errors.New("unused queue") }
func (hfNoQueue) Retry(context.Context, job.Job, time.Duration) error {
	return errors.New("unused queue")
}
func (hfNoQueue) PromoteDue(context.Context, int) (int, error) { return 0, errors.New("unused queue") }
func (hfNoQueue) Recover(context.Context) (int, error)         { return 0, errors.New("unused queue") }

type hfLabelFixture struct {
	db       *gorm.DB
	p        l.Principal
	assets   *lifecycle.Service
	service  *svc.Service
	versions *evaluation.VersionService
	denied   atomic.Bool
}

func hfLabelDB(t *testing.T) *hfLabelFixture {
	t.Helper()
	dsn := os.Getenv("COGNIDA_DOMAIN_TEST_DSN")
	parsed, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(parsed.DBName, "cognida_review_"), "isolated review DB required")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	f := &hfLabelFixture{db: db, p: l.Principal{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 7411}}
	t.Cleanup(func() {
		for _, table := range []string{"evaluation_dataset_versions", "evaluation_dataset_heads", "governance_records", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", f.p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	objects, err := store.NewLocal(t.TempDir())
	require.NoError(t, err)
	auth := func(_ context.Context, p l.Principal, _ string) error {
		if f.denied.Load() || p != f.p {
			return l.ErrForbidden
		}
		return nil
	}
	f.assets, err = lifecycle.NewService(repo.NewLifecycleRepository(db), objects, hfNoQueue{}, lifecycle.Config{Authorize: auth})
	require.NoError(t, err)
	f.service = svc.NewService(repo.NewGovernanceRepository(db), f.assets, nil, hfNoQueue{}, auth)
	f.versions = evaluation.NewVersionService(repo.NewDatasetVersionRepository(db), f.service, f.assets, func(ctx context.Context, p l.Principal, _ d.EvaluationProfile) error { return auth(ctx, p, "read") })
	return f
}
func (f *hfLabelFixture) create(ctx context.Context, in svc.LabelBenchmarkInput, labels []d.Label) (*d.Record, *d.Record, error) {
	ref, err := f.assets.Import(ctx, f.p, l.Input{Kind: "text", Text: in.Text}, "hf benchmark "+in.ID)
	if err != nil {
		return nil, nil, err
	}
	task, err := f.service.CreateAnnotation(ctx, f.p, d.AnnotationTask{Name: "HF held-out draft", Type: in.Kind, Labels: labels, SourceRefID: ref.AssetRefID}, "sample:"+svc.LabelBenchmarkTextDigest(in.ID)[:40])
	if err != nil {
		return nil, nil, err
	}
	page, err := f.service.List(ctx, f.p, d.Filter{Kind: "annotation_item", ParentID: task.ID, Limit: 2})
	if err != nil {
		return nil, nil, err
	}
	if len(page.Items) != 1 {
		return nil, nil, errors.New("expected one original-text annotation")
	}
	return task, &page.Items[0], nil
}

func hfRunOracle(t *testing.T, cfg hfLabelConfig, rules []d.Label, rows []svc.LabelBenchmarkSample, out string) {
	t.Helper()
	f := hfLabelDB(t)
	ctx := context.Background()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("tenant_id", f.p.TenantID)
		user := f.p.UserID
		if c.GetHeader("X-Other-User") == "1" {
			user++
		}
		c.Set("user_id", user)
		c.Next()
	})
	group := router.Group("/api")
	handler.NewGovernanceHandler(f.service).Register(group)
	handler.NewDatasetVersionHandler(f.versions).Register(group)
	server := httptest.NewServer(router)
	defer server.Close()
	request := func(method, path, key string, body any, other bool) (int, json.RawMessage) {
		raw, _ := json.Marshal(body)
		req, err := http.NewRequest(method, server.URL+"/api"+path, bytes.NewReader(raw))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		if other {
			req.Header.Set("X-Other-User", "1")
		}
		response, err := server.Client().Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		payload, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(payload, &envelope)
		return response.StatusCode, envelope.Data
	}
	publication := ev.PublishDatasetVersion{DatasetID: "hf-oracle-" + cfg.Language, Name: "HF oracle_fixture (not automatic quality)", Profile: cfg.Kind, Split: ev.DatasetSplit{Algorithm: "none"}}
	for _, row := range rows {
		task, item, err := f.create(ctx, svc.LabelBenchmarkInput{ID: row.ID, Text: row.Text, Kind: cfg.Kind}, rules)
		require.NoError(t, err)
		require.NoError(t, f.service.Preannotate(ctx, f.p, task.ID))
		item, err = f.service.Get(ctx, f.p, "annotation_item", item.ID)
		require.NoError(t, err)
		pred, err := svc.NewLabelBenchmarkRulePredictor(cfg.Kind, rules)
		require.NoError(t, err)
		expected, err := pred(ctx, svc.LabelBenchmarkInput{ID: row.ID, Text: row.Text, Kind: cfg.Kind})
		require.NoError(t, err)
		var actual d.AnnotationItem
		require.NoError(t, json.Unmarshal(item.Payload, &actual))
		require.Equal(t, expected.Annotation, actual.Annotation)
		require.Equal(t, "pending", actual.ReviewStatus)
		publication.Samples = append(publication.Samples, ev.SampleSelection{TaskID: task.ID, ItemID: item.ID, ExpectedVersion: item.Version})
		code, _ := request("POST", "/evaluation/dataset-versions", "unreviewed:"+row.ID, publication, false)
		require.Equal(t, 409, code)
		gold := d.Annotation{Labels: []string{}, Entities: []d.Entity{}, Relations: []d.Relation{}}
		if cfg.Kind == "classification" {
			gold.Labels = []string{row.Label}
		} else {
			gold.Entities = append(gold.Entities, row.Entities...)
			for i := range gold.Entities {
				gold.Entities[i].ID = fmt.Sprintf("e%d", i)
			}
		}
		path := "/annotation-tasks/" + task.ID + "/items/" + item.ID
		code, body := request("PUT", path, "", map[string]any{"annotation": gold, "expected_version": item.Version}, false)
		require.Equal(t, 200, code)
		var updated d.AnnotationItem
		require.NoError(t, json.Unmarshal(body, &updated))
		code, body = request("POST", path+"/review", "", map[string]any{"decision": "approved", "comment": "oracle_fixture: fixed HF gold; excluded from automated quality", "expected_version": updated.Version}, false)
		require.Equal(t, 200, code)
		var reviewed d.AnnotationItem
		require.NoError(t, json.Unmarshal(body, &reviewed))
		publication.Samples[len(publication.Samples)-1].ExpectedVersion = reviewed.Version
	}
	code, raw := request("POST", "/evaluation/dataset-versions", "oracle-publish", publication, false)
	require.Equal(t, 200, code)
	var version ev.DatasetVersion
	require.NoError(t, json.Unmarshal(raw, &version))
	require.Len(t, version.Samples, len(rows))
	require.Empty(t, version.QAPairs)
	code, replay := request("POST", "/evaluation/dataset-versions", "oracle-publish", publication, false)
	require.Equal(t, 200, code)
	require.JSONEq(t, string(raw), string(replay))
	code, read := request("GET", "/evaluation/dataset-versions/"+version.ID, "", nil, false)
	require.Equal(t, 200, code)
	require.JSONEq(t, string(raw), string(read))

	// A later annotation edit cannot rewrite a published immutable snapshot.
	selected := publication.Samples[0]
	code, _ = request("PUT", "/annotation-tasks/"+selected.TaskID+"/items/"+selected.ItemID, "", map[string]any{"annotation": d.Annotation{}, "expected_version": selected.ExpectedVersion}, false)
	require.Equal(t, 200, code)
	code, read = request("GET", "/evaluation/dataset-versions/"+version.ID, "", nil, false)
	require.Equal(t, 200, code)
	require.JSONEq(t, string(raw), string(read))
	code, _ = request("GET", "/evaluation/dataset-versions/"+version.ID, "", nil, true)
	require.Equal(t, 404, code)
	f.denied.Store(true)
	code, _ = request("GET", "/evaluation/dataset-versions/"+version.ID, "", nil, false)
	require.Equal(t, 403, code)
	hfJSON(t, filepath.Join(out, "oracle-publication.json"), map[string]any{"kind": "oracle_fixture_not_automatic_quality", "dataset": cfg.Dataset, "revision": cfg.Revision, "profile": version.Profile, "samples": len(version.Samples), "manifest_sha256": version.ManifestDigest, "unreviewed_rejected": true, "reviewed_published": true, "idempotent_replay": true, "other_owner_rejected": true, "revoked_source_rejected": true, "service_rule_parity": true, "immutable_after_annotation_edit": true, "transport": "real TCP HTTP, real MySQL and local object store; fixture identity, unused rejecting queue"}, true)
}

func TestHFLabelBenchmarkOracleFixture(t *testing.T) {
	if os.Getenv("COGNIDA_DOMAIN_TEST_DSN") == "" {
		t.Skip("isolated MySQL required")
	}
	for _, kind := range []string{"classification", "named_entity"} {
		t.Run(kind, func(t *testing.T) {
			labels := []d.Label{{ID: "a", Name: "alpha", Keywords: []string{"alpha"}, Terms: []string{"alpha"}}, {ID: "b", Name: "beta", Keywords: []string{"beta"}, Terms: []string{"beta"}}}
			rows := []svc.LabelBenchmarkSample{}
			for i := 0; i < 3; i++ {
				row := svc.LabelBenchmarkSample{ID: fmt.Sprint(i), Split: "test", Text: "alpha"}
				if kind == "classification" {
					row.Label = "a"
				} else {
					row.Entities = []d.Entity{{Label: "a", Start: 0, End: 5, Text: "alpha"}}
				}
				rows = append(rows, row)
			}
			hfRunOracle(t, hfLabelConfig{Dataset: "fixture/synthetic", Revision: strings.Repeat("a", 40), Language: "en", Kind: kind, Labels: labels}, labels, rows, t.TempDir())
		})
	}
}
