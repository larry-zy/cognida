//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cognida/internal/handler"
	"cognida/internal/infrastructure/job"
	"cognida/internal/infrastructure/llm/chat"
	objects "cognida/internal/infrastructure/objectstore"
	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	r "cognida/internal/model/researchrun"
	d "cognida/internal/model/taskcontrol"
	repo "cognida/internal/repository/mysql"
	"cognida/internal/service/agent"
	agenttools "cognida/internal/service/agent/tools"
	"cognida/internal/service/lifecycle"
	"cognida/internal/service/researchrun"
	"github.com/cloudwego/eino/components/tool"
	"github.com/gin-gonic/gin"
	sqlmysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The evidence engine and KB authorizer are deterministic test fixtures. MySQL,
// HTTP, exact durable receipts and lifecycle objects are real. Semantic model
// quality and live retrieval are deliberately verified separately by E2E.
type researchHTTPQueue struct{ job.Queue }
type researchHTTPRegistry map[string]tool.BaseTool

func (r researchHTTPRegistry) Get(name string) (tool.BaseTool, bool) { v, ok := r[name]; return v, ok }

type researchHTTPEngine struct {
	calls     atomic.Int32
	requestID atomic.Value
	fail      atomic.Bool
}

func (e *researchHTTPEngine) Available() bool { return true }
func (e *researchHTTPEngine) ExecuteGoverned(ctx context.Context, _ *agent.DeepResearchRequest) (*agent.DeepResearchResponse, error) {
	e.calls.Add(1)
	e.requestID.Store(agentctx.MustGetRequestID(ctx))
	if e.fail.Load() {
		return &agent.DeepResearchResponse{Success: false, Sources: []agent.ReportSourceDTO{{ID: "S1", Excerpt: "retained source"}}, Metadata: agent.ReportMetadataDTO{ModelCallsUsed: 2, Partial: true, Failure: &agent.ResearchFailureDTO{Stage: "synthesizing", Code: "http_error", HTTPStatus: 503}}}, errors.New("private provider body must never be persisted")
	}
	// Exercise the actual persisted heartbeat while a paid provider would be in flight.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(1500 * time.Millisecond):
	}
	return &agent.DeepResearchResponse{Success: true, Query: "fixture question", ExecutiveSummary: "A qualified conclusion [S1]", Sources: []agent.ReportSourceDTO{{ID: "S1", Type: "rag", Title: "Fixture evidence", Excerpt: "The source explicitly limits the claim to 2024.", KnowledgeBaseID: "fixture-kb", KnowledgeID: "fixture-doc", ChunkID: "fixture-chunk"}}, Metadata: agent.ReportMetadataDTO{CitationChecked: true, Partial: true, SemanticReview: true}, Unverified: []string{"Later years remain unverified"}}, nil
}
func TestResearchDurableHTTPRealReceiptAndPartialArtifact(t *testing.T) {
	dsn := os.Getenv("COGNIDA_TASKCONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated DSN required")
	}
	cfg, err := sqlmysql.ParseDSN(dsn)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(cfg.DBName, "cognida_review_"))
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	p := d.Scope{TenantID: time.Now().UnixNano() & 0x1fffffffffffff, UserID: 819}
	ctx := context.Background()
	t.Cleanup(func() {
		for _, table := range []string{"research_runs", "lifecycle_assets"} {
			require.NoError(t, db.Table(table).Where("tenant_id=?", p.TenantID).Delete(map[string]any{}).Error)
		}
		require.NoError(t, sql.Close())
	})
	store, err := objects.NewLocal(t.TempDir())
	require.NoError(t, err)
	var service *researchrun.Service
	var revoked atomic.Bool
	assets, err := lifecycle.NewService(repo.NewLifecycleRepository(db), store, researchHTTPQueue{}, lifecycle.Config{Authorize: func(_ context.Context, who l.Principal, _ string) error {
		if who.TenantID != p.TenantID || who.UserID != p.UserID || revoked.Load() {
			return l.ErrForbidden
		}
		return nil
	}, ValidateGovernedOrigin: func(ctx context.Context, who l.Principal, ref l.DataAssetRef) error {
		return service.ValidateGovernedOrigin(ctx, who, ref)
	}})
	require.NoError(t, err)
	service, err = researchrun.New(repo.NewResearchRunRepository(db), func(_ context.Context, who d.Scope, ids []string, _ *agent.DeepResearchResponse) error {
		if who != p || revoked.Load() || len(ids) != 1 || ids[0] != "fixture-kb" {
			return d.ErrForbidden
		}
		return nil
	}, func(ctx context.Context, who d.Scope, id string, report *agent.DeepResearchResponse) (l.RefID, error) {
		body, _ := json.Marshal(report)
		ref, err := assets.ImportDerived(ctx, l.Principal{TenantID: who.TenantID, UserID: who.UserID}, l.Insight, "research-governed:"+id, nil, body, l.PolicySnapshot{})
		return ref.RefID, err
	})
	require.NoError(t, err)
	engine := &researchHTTPEngine{}
	service.SetEngine(engine)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		user := p.UserID
		if c.GetHeader("X-Test-Other") == "1" {
			user++
		}
		if c.GetHeader("X-Test-NoIdentity") != "1" {
			c.Set("tenant_id", p.TenantID)
			c.Set("user_id", user)
		}
		c.Next()
	})
	handler.NewResearchRunHandler(service).Register(router.Group("/api/v1"))
	call := func(method, path, body, key, identity string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", key)
		if identity != "" {
			request.Header.Set(identity, "1")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	body := `{"query":"fixture question","kb_ids":["fixture-kb"],"graph_enabled":false,"enable_web_search":false}`
	response := call("POST", "/api/v1/research/runs", body, "receipt-key", "")
	require.Equal(t, 201, response.Code, response.Body.String())
	var envelope struct {
		Data r.Run `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	id := envelope.Data.ID
	require.NotEmpty(t, id)
	require.Zero(t, engine.calls.Load())
	replay := call("POST", "/api/v1/research/runs", body, "receipt-key", "")
	require.Equal(t, 201, replay.Code)
	var same struct {
		Data r.Run `json:"data"`
	}
	require.NoError(t, json.Unmarshal(replay.Body.Bytes(), &same))
	require.Equal(t, id, same.Data.ID)
	require.Equal(t, 409, call("POST", "/api/v1/research/runs", strings.ReplaceAll(body, "fixture question", "changed question"), "receipt-key", "").Code)
	require.Equal(t, 401, call("GET", "/api/v1/research/runs/"+id, "", "", "X-Test-NoIdentity").Code)
	require.Equal(t, 404, call("GET", "/api/v1/research/runs/"+id, "", "", "X-Test-Other").Code)
	require.NoError(t, service.Process(ctx, p, id))
	require.Equal(t, "research:"+id, engine.requestID.Load(), "durable HTTP runs must provide a stable retrieval strategy pin identity")
	response = call("GET", "/api/v1/research/runs/"+id, "", "", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, "partial", envelope.Data.Status)
	require.Contains(t, string(envelope.Data.Report), "Later years remain unverified")
	var refs []l.RefID
	require.NoError(t, json.Unmarshal(envelope.Data.OutputRefs, &refs))
	require.Len(t, refs, 1)
	_, actual, err := assets.Resolve(ctx, p.TenantID, p.UserID, refs[0])
	require.NoError(t, err)
	require.JSONEq(t, string(envelope.Data.Report), string(actual))
	response = call("GET", "/api/v1/research/runs/"+id+"/events", "", "", "")
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), "event:terminal")
	require.Contains(t, response.Body.String(), "Later years remain unverified")
	require.EqualValues(t, 1, engine.calls.Load())
	// Failure diagnostics survive real persistence and HTTP/SSE reads without
	// exposing the provider error, publishing output or replaying paid work.
	engine.fail.Store(true)
	failedResponse := call("POST", "/api/v1/research/runs", body, "diagnostic-key", "")
	require.Equal(t, 201, failedResponse.Code)
	var failed struct {
		Data r.Run `json:"data"`
	}
	require.NoError(t, json.Unmarshal(failedResponse.Body.Bytes(), &failed))
	failedID := failed.Data.ID
	require.NoError(t, service.Process(ctx, p, failedID))
	require.ErrorIs(t, service.Process(ctx, p, failedID), d.ErrConflict)
	require.EqualValues(t, 2, engine.calls.Load(), "failed terminal runs must not regenerate")
	failedResponse = call("GET", "/api/v1/research/runs/"+failedID, "", "", "")
	require.Equal(t, 200, failedResponse.Code)
	require.NoError(t, json.Unmarshal(failedResponse.Body.Bytes(), &failed))
	require.Equal(t, "failed", failed.Data.Status)
	var failedReport agent.DeepResearchResponse
	require.NoError(t, json.Unmarshal(failed.Data.Report, &failedReport))
	require.Equal(t, &agent.ResearchFailureDTO{Stage: "synthesizing", Code: "http_error", HTTPStatus: 503}, failedReport.Metadata.Failure)
	require.Len(t, failedReport.Sources, 1)
	require.False(t, failedReport.Success)
	require.NotContains(t, failedResponse.Body.String(), "private provider body")
	require.NoError(t, json.Unmarshal(failed.Data.OutputRefs, &refs))
	require.Empty(t, refs)
	failedEvents := call("GET", "/api/v1/research/runs/"+failedID+"/events", "", "", "")
	require.Equal(t, 200, failedEvents.Code)
	require.Contains(t, failedEvents.Body.String(), `"code":"http_error"`)
	require.NotContains(t, failedEvents.Body.String(), "private provider body")
	// Exercise the real research engine and provider adapter against an HTTP
	// fixture, with real durable state. A valid JSON prefix marked length must
	// remain failed; an explicitly new run can complete all three model stages.
	var providerCalls atomic.Int32
	var truncateOutput atomic.Bool
	var supplementOutput atomic.Bool
	var reviewCalls, retrievalCalls atomic.Int32
	var finishAction, activeRun atomic.Value
	var draftMode, repairMode atomic.Value
	finishAction.Store("")
	activeRun.Store("")
	draftMode.Store("")
	repairMode.Store("")
	truncateOutput.Store(true)
	requests := make(chan map[string]json.RawMessage, 8)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var input map[string]json.RawMessage
		if e := json.NewDecoder(request.Body).Decode(&input); e != nil {
			t.Error(e)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- input
		providerCalls.Add(1)
		var messages []struct {
			Content string `json:"content"`
		}
		if e := json.Unmarshal(input["messages"], &messages); e != nil || len(messages) != 2 {
			t.Error("invalid structured request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		content, finish, reasoning := "", "stop", ""
		switch {
		case strings.Contains(messages[0].Content, "研究规划器"):
			content = `{"questions":[{"question":"specific evidence question","strategy":"hybrid"}]}`
		case strings.Contains(messages[0].Content, "报告撰写者"):
			content = `{"summary":"Summary [S1]","analysis":"Analysis [S1]","findings":[{"statement":"The claim is limited to 2024.","citations":[{"source_id":"S1","quote":"The source explicitly limits the claim to 2024."}]}]}`
			mode := draftMode.Load().(string)
			if strings.Contains(messages[0].Content, "一次独立的结构修复") && repairMode.Load().(string) != "" {
				mode = ""
				// The real MySQL checkpoint must precede the external repair request.
				var persisted r.Run
				require.NoError(t, db.Where("tenant_id=? AND user_id=? AND id=?", p.TenantID, p.UserID, activeRun.Load().(string)).Take(&persisted).Error)
				require.Equal(t, "repairing", persisted.Stage)
				var checkpoint agent.DeepResearchResponse
				require.NoError(t, json.Unmarshal(persisted.Report, &checkpoint))
				require.Equal(t, 2, checkpoint.Metadata.ModelCallsUsed)
				require.Equal(t, 2, checkpoint.Metadata.ModelCallsReserved)
				require.Equal(t, "reserved", checkpoint.Metadata.DraftRepair.Status)
				require.Zero(t, checkpoint.Metadata.DraftRepair.Attempts)
				wantTrigger := "draft_required_fields"
				switch draftMode.Load().(string) {
				case "summary_citations", "analysis_citations":
					wantTrigger = "draft_citations_missing"
				case "finding_citations":
					wantTrigger = "draft_finding_citations_missing"
				}
				require.Equal(t, wantTrigger, checkpoint.Metadata.DraftRepair.Trigger.Code)
				require.Empty(t, checkpoint.ExecutiveSummary)
				require.Len(t, checkpoint.Sources, 1)
				if repairMode.Load().(string) == "cancel" {
					_, e := service.Cancel(context.Background(), p, activeRun.Load().(string))
					require.NoError(t, e)
				}
			}
			if mode != "" {
				var fields map[string]any
				_ = json.Unmarshal([]byte(content), &fields)
				one := fields["findings"].([]any)[0]
				switch mode {
				case "missing":
					delete(fields, "findings")
				case "empty":
					fields["findings"] = []any{}
				case "excess":
					fields["findings"] = []any{one, one, one, one, one, one, one, one, one}
				case "summary_citations":
					fields["summary"] = "Summary without a source marker"
				case "analysis_citations":
					fields["analysis"] = "Analysis without a source marker"
				case "finding_citations":
					delete(one.(map[string]any), "citations")
				case "summary_unknown", "summary_quote", "finding_unknown", "finding_quote":
					// Missing evidence at the first validation site must not hide
					// invalid, already-present evidence in a later finding.
					second := map[string]any{"statement": "Another claim", "citations": []any{map[string]any{"source_id": "S1", "quote": "The source explicitly limits the claim to 2024."}}}
					fields["findings"] = []any{one, second}
					if strings.HasPrefix(mode, "summary_") {
						fields["summary"] = "Summary without a source marker"
					} else {
						delete(one.(map[string]any), "citations")
					}
					citation := second["citations"].([]any)[0].(map[string]any)
					if strings.HasSuffix(mode, "unknown") {
						citation["source_id"] = "S999"
					} else {
						citation["quote"] = "private invented quotation must never be retained"
					}
				}
				encoded, _ := json.Marshal(fields)
				content = string(encoded)
			}
			if truncateOutput.Load() {
				finish, reasoning = "length", "private reasoning must never be persisted"
			}
		case strings.Contains(messages[0].Content, "证据审查员"):
			content = `{"claims":[{"id":"C1","status":"supported","reason":"The evidence states the same time limit.","evidence":[{"source_id":"S1","quote":"The source explicitly limits the claim to 2024.","relation":"supports"}]}]}`
			switch repairMode.Load().(string) {
			case "insufficient":
				content = `{"claims":[{"id":"C1","status":"insufficient","reason":"The remaining period is not established.","evidence":[{"source_id":"S1","quote":"The source explicitly limits the claim to 2024.","relation":"context"}]}]}`
			case "review_invalid":
				content = `{"claims":[]}`
			case "revoke":
				revoked.Store(true)
			}
			if supplementOutput.Load() {
				if reviewCalls.Add(1) == 1 {
					content = `{"claims":[{"id":"C1","status":"insufficient","reason":"Need to check later evidence before applying the time limit.","evidence":[{"source_id":"S1","quote":"The source explicitly limits the claim to 2024.","relation":"context"}]}]}`
				} else {
					content = `{"claims":[{"id":"C1","status":"supported","reason":"Unacceptable invented quotation.","evidence":[{"source_id":"S2","quote":"private invented quotation must not be retained","relation":"supports"}]}]}`
					switch finishAction.Load().(string) {
					case "cancel":
						if _, e := service.Cancel(context.Background(), p, activeRun.Load().(string)); e != nil {
							t.Error(e)
						}
					case "revoke":
						revoked.Store(true)
					}
				}
			}
		default:
			t.Error("unexpected research model stage")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content, "reasoning_content": reasoning}, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 1200, "completion_tokens": 8192, "total_tokens": 9392}})
	}))
	defer provider.Close()
	model, err := chat.NewToolCallingChatModel(ctx, &chat.ChatConfig{BaseURL: provider.URL, APIKey: "fixture-only", Provider: "deepseek", ModelName: "deepseek-v4-flash"})
	require.NoError(t, err)
	rag := agenttools.NewTypedBaseTool("rag_query", "authorized fixture retrieval", func(c context.Context, _ *agenttools.RAGQueryRequest) (*agenttools.RAGQueryResult, error) {
		require.Equal(t, p.TenantID, agentctx.MustGetTenantID(c))
		require.Equal(t, []string{"fixture-kb"}, agentctx.MustGetAllowedKBIDs(c))
		if supplementOutput.Load() && retrievalCalls.Add(1) > 1 {
			return &agenttools.RAGQueryResult{Chunks: []agenttools.DocumentChunk{{Content: "The later record states a different limit in 2025.", ChunkID: "new-chunk", KnowledgeID: "new-doc", KnowledgeBaseID: "fixture-kb", Source: "New counter-evidence"}}}, nil
		}
		return &agenttools.RAGQueryResult{Chunks: []agenttools.DocumentChunk{{Content: "The source explicitly limits the claim to 2024.", ChunkID: "fixture-chunk", KnowledgeID: "fixture-doc", KnowledgeBaseID: "fixture-kb", Source: "Fixture evidence"}}}, nil
	})
	service.SetEngine(agent.NewResearchService(nil, nil).WithDependencies(model, researchHTTPRegistry{"rag_query": rag}))
	for _, truncated := range []bool{true, false} {
		truncateOutput.Store(truncated)
		key := "bounded-success"
		wantStatus, wantCalls := "succeeded", 3
		if truncated {
			key, wantStatus, wantCalls = "bounded-truncated", "failed", 2
		}
		response = call("POST", "/api/v1/research/runs", body, key, "")
		require.Equal(t, 201, response.Code)
		var bounded struct {
			Data r.Run `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &bounded))
		before := providerCalls.Load()
		require.NoError(t, service.Process(ctx, p, bounded.Data.ID))
		require.ErrorIs(t, service.Process(ctx, p, bounded.Data.ID), d.ErrConflict)
		require.EqualValues(t, wantCalls, providerCalls.Load()-before)
		response = call("GET", "/api/v1/research/runs/"+bounded.Data.ID, "", "", "")
		require.Equal(t, 200, response.Code)
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &bounded))
		require.Equal(t, wantStatus, bounded.Data.Status)
		var report agent.DeepResearchResponse
		require.NoError(t, json.Unmarshal(bounded.Data.Report, &report))
		require.Len(t, report.Sources, 1)
		require.Equal(t, wantCalls, report.Metadata.ModelCallsUsed)
		var outputRefs []l.RefID
		require.NoError(t, json.Unmarshal(bounded.Data.OutputRefs, &outputRefs))
		if truncated {
			require.Empty(t, outputRefs)
			require.Equal(t, "truncated_output", report.Metadata.Failure.Code)
			require.Equal(t, "synthesizing", report.Metadata.Failure.Stage)
			require.Equal(t, 8192, report.Metadata.Failure.Output.MaxOutputTokens)
			require.Equal(t, 8192, report.Metadata.Failure.Output.Usage.CompletionTokens)
			require.Positive(t, report.Metadata.Failure.Output.ReasoningCharacters)
			require.False(t, report.Metadata.CitationChecked)
			require.Contains(t, report.Metadata.VerificationLimit, "尚未完成引用")
		} else {
			require.Len(t, outputRefs, 1)
			require.True(t, report.Metadata.CitationChecked)
			require.True(t, report.Metadata.SemanticReview)
			require.False(t, report.Metadata.FactChecked)
			require.True(t, report.Claims[0].Evidence[0].QuoteMatched)
			_, data, e := assets.Resolve(ctx, p.TenantID, p.UserID, outputRefs[0])
			require.NoError(t, e)
			require.JSONEq(t, string(bounded.Data.Report), string(data))
		}
		for index := 0; index < wantCalls; index++ {
			request := <-requests
			wantTokens := "8192"
			if index == 0 {
				wantTokens = "2048"
			}
			require.JSONEq(t, wantTokens, string(request["max_tokens"]))
			require.JSONEq(t, `{"type":"disabled"}`, string(request["thinking"]))
			require.JSONEq(t, `{"type":"json_object"}`, string(request["response_format"]))
		}
		events := call("GET", "/api/v1/research/runs/"+bounded.Data.ID+"/events", "", "", "")
		require.Equal(t, 200, events.Code)
		require.Contains(t, events.Body.String(), "event:terminal")
		require.NotContains(t, events.Body.String(), "private reasoning")
		require.NotContains(t, response.Body.String(), "private reasoning")
	}
	// An optional review failure can publish only an explicitly partial report
	// with a valid completed snapshot. Revocation and cancellation still win.
	supplementOutput.Store(true)
	for _, action := range []string{"", "revoke", "cancel"} {
		finishAction.Store(action)
		reviewCalls.Store(0)
		retrievalCalls.Store(0)
		response = call("POST", "/api/v1/research/runs", body, "partial-"+action, "")
		require.Equal(t, 201, response.Code)
		var result struct {
			Data r.Run `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		activeRun.Store(result.Data.ID)
		before := providerCalls.Load()
		require.NoError(t, service.Process(ctx, p, result.Data.ID))
		require.ErrorIs(t, service.Process(ctx, p, result.Data.ID), d.ErrConflict)
		require.EqualValues(t, 4, providerCalls.Load()-before)
		for index := 0; index < 4; index++ {
			<-requests
		}
		if action == "revoke" {
			require.Equal(t, 403, call("GET", "/api/v1/research/runs/"+result.Data.ID, "", "", "").Code)
			revoked.Store(false)
		}
		response = call("GET", "/api/v1/research/runs/"+result.Data.ID, "", "", "")
		require.Equal(t, 200, response.Code)
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		var outputRefs []l.RefID
		require.NoError(t, json.Unmarshal(result.Data.OutputRefs, &outputRefs))
		if action != "" {
			status := "failed"
			if action == "cancel" {
				status = "cancelled"
			}
			require.Equal(t, status, result.Data.Status)
			require.Empty(t, outputRefs)
			continue
		}
		require.Equal(t, "partial", result.Data.Status)
		require.Len(t, outputRefs, 1)
		var partial agent.DeepResearchResponse
		require.NoError(t, json.Unmarshal(result.Data.Report, &partial))
		require.False(t, partial.Success)
		require.True(t, partial.Metadata.Partial)
		require.True(t, partial.Metadata.SupplementalReviewIncomplete)
		require.Equal(t, "review_quote_mismatch", partial.Metadata.Failure.Code)
		require.Equal(t, "supplement_reviewing", partial.Metadata.Failure.Stage)
		require.Equal(t, []string{"S1"}, partial.Metadata.ReviewedSourceIDs)
		require.Equal(t, []string{"S2"}, partial.Metadata.UnreviewedSourceIDs)
		require.Len(t, partial.ReviewHistory, 1)
		require.Len(t, partial.Sources, 2)
		require.Equal(t, 1, partial.Metadata.ReviewRounds)
		require.NotEmpty(t, partial.Unverified)
		require.NotContains(t, string(result.Data.Report), "private invented quotation")
		_, artifact, e := assets.Resolve(ctx, p.TenantID, p.UserID, outputRefs[0])
		require.NoError(t, e)
		require.JSONEq(t, string(result.Data.Report), string(artifact))
		events := call("GET", "/api/v1/research/runs/"+result.Data.ID+"/events", "", "", "")
		require.Equal(t, 200, events.Code)
		require.Contains(t, events.Body.String(), `"status":"partial"`)
		require.Contains(t, events.Body.String(), `"review_quote_mismatch"`)
		require.NotContains(t, events.Body.String(), "private invented quotation")
	}
	// Missing fields, a zero-length array and nine findings must remain distinct
	// diagnostic failures after at most one repair through persistence and SSE,
	// never truncated into a superficially valid report or sent to semantic review.
	supplementOutput.Store(false)
	for _, mode := range []string{"missing", "empty", "excess", "summary_citations", "analysis_citations", "finding_citations"} {
		draftMode.Store(mode)
		response = call("POST", "/api/v1/research/runs", body, "draft-contract-"+mode, "")
		require.Equal(t, 201, response.Code)
		var result struct {
			Data r.Run `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		before := providerCalls.Load()
		require.NoError(t, service.Process(ctx, p, result.Data.ID))
		require.ErrorIs(t, service.Process(ctx, p, result.Data.ID), d.ErrConflict)
		require.EqualValues(t, 3, providerCalls.Load()-before)
		<-requests
		<-requests
		<-requests
		response = call("GET", "/api/v1/research/runs/"+result.Data.ID, "", "", "")
		require.Equal(t, 200, response.Code)
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		require.Equal(t, "failed", result.Data.Status)
		var report agent.DeepResearchResponse
		require.NoError(t, json.Unmarshal(result.Data.Report, &report))
		wantCount := 0
		wantCode := "draft_finding_count"
		if mode == "excess" {
			wantCount = 9
		}
		if mode == "missing" {
			wantCode = "draft_required_fields"
		}
		if mode == "summary_citations" || mode == "analysis_citations" {
			wantCode, wantCount = "draft_citations_missing", 1
		}
		if mode == "finding_citations" {
			wantCode, wantCount = "draft_finding_citations_missing", 1
		}
		require.Equal(t, wantCode, report.Metadata.Failure.Code)
		require.Equal(t, "repair_validating", report.Metadata.Failure.Stage)
		require.Equal(t, 1, report.Metadata.DraftRepair.Attempts)
		require.Equal(t, "failed", report.Metadata.DraftRepair.Status)
		require.Equal(t, wantCount, report.Metadata.Failure.Structure.FindingsCount)
		require.Equal(t, mode != "missing", report.Metadata.Failure.Structure.FindingsPresent)
		require.False(t, report.Success)
		require.False(t, report.Metadata.CitationChecked)
		require.False(t, report.Metadata.SemanticReview)
		var outputRefs []l.RefID
		require.NoError(t, json.Unmarshal(result.Data.OutputRefs, &outputRefs))
		require.Empty(t, outputRefs)
		events := call("GET", "/api/v1/research/runs/"+result.Data.ID+"/events", "", "", "")
		require.Equal(t, 200, events.Code)
		require.Contains(t, events.Body.String(), `"findings_count":`)
		require.Contains(t, events.Body.String(), `"findings_present":`)
	}
	// One explicit structure repair consumes the fourth-call allowance needed
	// for review; it never replaces independent review or creates a fifth call.
	for _, omission := range []string{"missing", "summary_citations", "analysis_citations", "finding_citations"} {
		draftMode.Store(omission)
		for _, mode := range []string{"success", "insufficient", "review_invalid", "cancel", "revoke"} {
			repairMode.Store(mode)
			response = call("POST", "/api/v1/research/runs", body, "repair-"+omission+"-"+mode, "")
			require.Equal(t, 201, response.Code)
			var result struct {
				Data r.Run `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			activeRun.Store(result.Data.ID)
			before := providerCalls.Load()
			require.NoError(t, service.Process(ctx, p, result.Data.ID))
			require.ErrorIs(t, service.Process(ctx, p, result.Data.ID), d.ErrConflict)
			wantCalls := 4
			if mode == "cancel" {
				wantCalls = 3
			}
			require.EqualValues(t, wantCalls, providerCalls.Load()-before)
			for i := 0; i < wantCalls; i++ {
				<-requests
			}
			if mode == "revoke" {
				require.Equal(t, 403, call("GET", "/api/v1/research/runs/"+result.Data.ID, "", "", "").Code)
				revoked.Store(false)
			}
			response = call("GET", "/api/v1/research/runs/"+result.Data.ID, "", "", "")
			require.Equal(t, 200, response.Code)
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			var report agent.DeepResearchResponse
			require.NoError(t, json.Unmarshal(result.Data.Report, &report))
			var refs []l.RefID
			require.NoError(t, json.Unmarshal(result.Data.OutputRefs, &refs))
			wantStatus := "succeeded"
			switch mode {
			case "insufficient":
				wantStatus = "partial"
			case "review_invalid", "revoke":
				wantStatus = "failed"
			case "cancel":
				wantStatus = "cancelled"
			}
			require.Equal(t, wantStatus, result.Data.Status)
			if mode == "success" || mode == "insufficient" {
				require.Len(t, refs, 1)
				require.True(t, report.Metadata.CitationChecked)
				require.True(t, report.Metadata.SemanticReview)
				require.Equal(t, 1, report.Metadata.ReviewRounds)
				require.Equal(t, 4, report.Metadata.ModelCallsUsed)
				require.Zero(t, report.Metadata.ModelCallsReserved)
				require.Equal(t, 1, report.Metadata.DraftRepair.Attempts)
				require.Equal(t, "completed", report.Metadata.DraftRepair.Status)
				_, artifact, e := assets.Resolve(ctx, p.TenantID, p.UserID, refs[0])
				require.NoError(t, e)
				require.JSONEq(t, string(result.Data.Report), string(artifact))
				if mode == "insufficient" {
					require.Equal(t, "model_call_budget_exhausted", report.Metadata.SupplementalSkippedReason)
					require.Len(t, report.Sources, 1)
					require.NotEmpty(t, report.Unverified)
				}
			} else {
				require.Empty(t, refs)
			}
			if mode == "review_invalid" {
				require.False(t, report.Success)
				require.False(t, report.Metadata.SemanticReview)
				require.Equal(t, "reviewing", report.Metadata.Failure.Stage)
			}
			events := call("GET", "/api/v1/research/runs/"+result.Data.ID+"/events", "", "", "")
			require.Equal(t, 200, events.Code)
			require.Contains(t, events.Body.String(), `"draft_repair"`)
			wantTrigger := "draft_required_fields"
			if omission == "summary_citations" || omission == "analysis_citations" {
				wantTrigger = "draft_citations_missing"
			}
			if omission == "finding_citations" {
				wantTrigger = "draft_finding_citations_missing"
			}
			require.Contains(t, events.Body.String(), `"`+wantTrigger+`"`)
		}
	}
	// Compound omissions and invalid existing evidence are rejected before any
	// repair call, even when normal validation reports the omission first.
	repairMode.Store("")
	for _, mode := range []string{"summary_unknown", "summary_quote", "finding_unknown", "finding_quote"} {
		draftMode.Store(mode)
		response = call("POST", "/api/v1/research/runs", body, "compound-"+mode, "")
		require.Equal(t, 201, response.Code)
		var result struct {
			Data r.Run `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		before := providerCalls.Load()
		require.NoError(t, service.Process(ctx, p, result.Data.ID))
		require.ErrorIs(t, service.Process(ctx, p, result.Data.ID), d.ErrConflict)
		require.EqualValues(t, 2, providerCalls.Load()-before)
		<-requests
		<-requests
		response = call("GET", "/api/v1/research/runs/"+result.Data.ID, "", "", "")
		require.Equal(t, 200, response.Code)
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		require.Equal(t, "failed", result.Data.Status)
		var report agent.DeepResearchResponse
		require.NoError(t, json.Unmarshal(result.Data.Report, &report))
		require.Equal(t, 2, report.Metadata.ModelCallsUsed)
		require.Nil(t, report.Metadata.DraftRepair)
		require.False(t, report.Success)
		require.False(t, report.Metadata.CitationChecked)
		require.False(t, report.Metadata.SemanticReview)
		require.Empty(t, report.ReviewHistory)
		var refs []l.RefID
		require.NoError(t, json.Unmarshal(result.Data.OutputRefs, &refs))
		require.Empty(t, refs)
		require.NotContains(t, response.Body.String(), "private invented quotation")
		events := call("GET", "/api/v1/research/runs/"+result.Data.ID+"/events", "", "", "")
		require.Equal(t, 200, events.Code)
		require.Contains(t, events.Body.String(), `"status":"failed"`)
		require.NotContains(t, events.Body.String(), "private invented quotation")
	}
	// Cancel after completion preserves the actual partial fact; subsequent reads
	// expose the same durable output. Revocation prevents source text disclosure.
	require.Equal(t, 200, call("POST", "/api/v1/research/runs/"+id+"/cancel", "{}", "", "").Code)
	saved, err := service.Read(ctx, p, id)
	require.NoError(t, err)
	require.Equal(t, "partial", saved.Status)
	revoked.Store(true)
	response = call("GET", "/api/v1/research/runs/"+id, "", "", "")
	require.Equal(t, 403, response.Code)
	require.NotContains(t, response.Body.String(), "source explicitly")
	require.Equal(t, 200, call("POST", "/api/v1/research/runs/"+id+"/cancel", "{}", "", "").Code)
}
