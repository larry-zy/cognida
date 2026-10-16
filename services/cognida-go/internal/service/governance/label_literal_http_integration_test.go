//go:build integration

package governance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"cognida/internal/handler"
	provider "cognida/internal/infrastructure/llm"
	ev "cognida/internal/model/evaluation"
	d "cognida/internal/model/governance"
	"cognida/internal/model/llm"
	svc "cognida/internal/service/governance"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLabelLiteralMySQLHTTPGroundingAndReplay(t *testing.T) {
	if os.Getenv("COGNIDA_DOMAIN_TEST_DSN") == "" {
		t.Skip("isolated database required")
	}
	for _, tc := range []struct {
		name, text, evidence string
		status               int
	}{
		{"unique omitted", "😀发票", `{"label_id":"finance","match":"发票"}`, 200},
		{"explicit wrong", "😀发票", `{"label_id":"finance","match":"发票","start":0,"end":2}`, 422},
		{"overlap ambiguous", "aaaa", `{"label_id":"finance","match":"aaa"}`, 422},
		{"explicit repeated", "发票和发票", `{"label_id":"finance","match":"发票","start":3,"end":5}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := hfLabelDB(t)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request map[string]json.RawMessage
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				require.JSONEq(t, `{"type":"disabled"}`, string(request["thinking"]))
				require.JSONEq(t, `4096`, string(request["max_tokens"]))
				content := `{"annotation":{"labels":["finance"]},"decision_status":"matched","rationale":"literal evidence","evidence_spans":[` + tc.evidence + `]}`
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}}})
			}))
			defer upstream.Close()
			client, err := provider.NewEinoLLMClient(&llm.ModelConfig{Provider: llm.ProviderDeepSeek, ModelName: "deepseek-v4-flash", BaseURL: upstream.URL, APIKey: "fixture-only"})
			require.NoError(t, err)
			f.service.WithLabelSuggester(svc.NewLLMLabelSuggester(client, "fixture:deepseek-v4-flash"))
			task, item, err := f.create(context.Background(), svc.LabelBenchmarkInput{ID: "fixed", Text: tc.text, Kind: "classification"}, []d.Label{{ID: "finance", Name: "finance"}})
			require.NoError(t, err)
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("tenant_id", f.p.TenantID); c.Set("user_id", f.p.UserID); c.Next() })
			handler.NewGovernanceHandler(f.service).RegisterLabelSuggestionRoutes(router.Group("/api"))
			server := httptest.NewServer(router)
			defer server.Close()
			call := func() (int, d.AnnotationItem) {
				raw, _ := json.Marshal(svc.LabelSuggestionRequest{ExpectedVersion: item.Version})
				request, err := http.NewRequest("POST", server.URL+"/api/annotation-tasks/"+task.ID+"/items/"+item.ID+"/suggest", bytes.NewReader(raw))
				require.NoError(t, err)
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Idempotency-Key", "once")
				response, err := server.Client().Do(request)
				require.NoError(t, err)
				defer response.Body.Close()
				var body struct {
					Data d.AnnotationItem `json:"data"`
				}
				require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
				return response.StatusCode, body.Data
			}
			code, result := call()
			require.Equal(t, tc.status, code)
			require.Equal(t, int32(1), calls.Load())
			if tc.status == 200 {
				require.Equal(t, "pending", result.ReviewStatus)
				_, publishErr := f.versions.Publish(context.Background(), f.p, ev.PublishDatasetVersion{DatasetID: "unreviewed", Name: "model output is not approved", Profile: "classification", Samples: []ev.SampleSelection{{TaskID: task.ID, ItemID: item.ID, ExpectedVersion: result.Version}}, Split: ev.DatasetSplit{Algorithm: "none"}}, "forbidden-auto-publish")
				require.ErrorIs(t, publishErr, ev.ErrVersionConflict)
				require.Equal(t, "model", result.Provenance.Source)
				require.NotEmpty(t, result.Provenance.ModelCallID)
				require.Len(t, result.Provenance.EvidenceSpans, 1)
				span := result.Provenance.EvidenceSpans[0]
				require.Equal(t, span.Match, string([]rune(tc.text)[span.Start:span.End]))
				code, replayed := call()
				require.Equal(t, 200, code)
				require.Equal(t, result.Version, replayed.Version)
			} else {
				code, _ = call()
				require.Equal(t, 502, code)
				unchanged, err := f.service.Get(context.Background(), f.p, "annotation_item", item.ID)
				require.NoError(t, err)
				require.Equal(t, item.Version, unchanged.Version)
			}
			require.Equal(t, int32(1), calls.Load(), "same durable key must not pay again")
		})
	}
}
