//go:build integration

package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cognida/internal/config"
	provider "cognida/internal/infrastructure/llm"
	d "cognida/internal/model/governance"
	"cognida/internal/model/llm"
	svc "cognida/internal/service/governance"
	"github.com/stretchr/testify/require"
)

type hfDiagnosticCapture struct {
	Messages  []*llm.Message `json:"messages"`
	Content   string         `json:"content"`
	Finish    string         `json:"finish_reason"`
	Usage     *llm.Usage     `json:"usage"`
	ToolCalls int            `json:"tool_call_count"`
}
type hfDiagnosticKey struct{}
type hfDiagnosticClient struct {
	llm.LLMClient
	calls atomic.Int64
}

func (c *hfDiagnosticClient) Chat(ctx context.Context, r *llm.ChatRequest) (*llm.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.calls.Add(1) > 12 {
		return nil, fmt.Errorf("diagnostic budget exhausted")
	}
	v := ctx.Value(hfDiagnosticKey{}).(*hfDiagnosticCapture)
	v.Messages = r.Messages
	out, err := c.LLMClient.Chat(ctx, r)
	if out != nil {
		v.Content = out.Content
		if v.Content == "" && out.Message != nil {
			v.Content = out.Message.Content
		}
		v.Finish = out.FinishReason
		v.Usage = out.Usage
		v.ToolCalls = len(out.ToolCalls)
	}
	return out, err
}

// Separately authorized engineering diagnostic: first six original train rows
// per classification corpus, never held-out reruns. Private raw evidence uses
// 0600 files; the public summary contains only fixed codes/counts/hashes.
func TestHFLabelContractDiagnostic(t *testing.T) {
	if os.Getenv("COGNIDA_LABEL_DIAGNOSTIC_LIVE") != "1" {
		t.Skip("separate 12-call training-only diagnostic authorization required")
	}
	root := os.Getenv("COGNIDA_LABEL_DIAGNOSTIC_ROOT")
	outDir := os.Getenv("COGNIDA_LABEL_DIAGNOSTIC_OUTPUT")
	require.NotEmpty(t, root)
	require.NotEmpty(t, outDir)
	require.NoError(t, os.MkdirAll(outDir, 0700))
	type sample struct {
		Dataset string
		Config  hfLabelConfig
		Row     svc.LabelBenchmarkSample
	}
	var selected []sample
	var registration []map[string]any
	for _, key := range []string{"banking77", "massive_zh"} {
		var cfg hfLabelConfig
		require.NoError(t, json.Unmarshal(hfRead(t, filepath.Join(root, key, "label-config.json")), &cfg))
		train, _ := hfReadData(t, cfg)
		require.GreaterOrEqual(t, len(train), 6)
		for _, row := range train[:6] {
			require.Equal(t, "train", row.Split)
			selected = append(selected, sample{key, cfg, row})
			registration = append(registration, map[string]any{"dataset": key, "revision": cfg.Revision, "id": row.ID, "text_sha256": svc.LabelBenchmarkTextDigest(row.Text)})
		}
	}
	chat := config.LoadChatConfig()
	require.NotEmpty(t, chat.APIKey)
	hfJSON(t, filepath.Join(outDir, "protocol.json"), map[string]any{"kind": "engineering_diagnostic_not_heldout_benchmark", "created_at": time.Now().UTC(), "selection": "first six original train rows per corpus, no outcome selection", "samples": registration, "max_calls": 12, "parallel": 2, "model": chat.ModelName, "provider": chat.Provider, "prompt_sha256": svc.LabelBenchmarkPromptSHA256("classification"), "code_sha256": hfCode(t)}, true)
	client, err := provider.NewChatRepository(&llm.ModelConfig{Provider: llm.Provider(chat.Provider), ModelName: chat.ModelName, BaseURL: chat.BaseURL, APIKey: chat.APIKey})
	require.NoError(t, err)
	observed := &hfDiagnosticClient{LLMClient: client}
	f := hfLabelDB(t)
	f.service.WithLabelSuggester(svc.NewLLMLabelSuggester(observed, chat.Provider+":"+chat.ModelName))
	type result struct {
		Index       int            `json:"index"`
		Dataset     string         `json:"dataset"`
		ID          string         `json:"id"`
		ErrorCode   string         `json:"error_code"`
		Diagnosis   map[string]any `json:"diagnosis"`
		ResponseSHA string         `json:"response_sha256"`
		Usage       *llm.Usage     `json:"usage"`
	}
	results := make([]result, len(selected))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < 2; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				s := selected[i]
				capture := &hfDiagnosticCapture{}
				ctx := context.WithValue(context.Background(), hfDiagnosticKey{}, capture)
				task, item, e := f.create(ctx, svc.LabelBenchmarkInput{ID: s.Dataset + ":" + s.Row.ID, Text: s.Row.Text, Kind: s.Config.Kind}, s.Config.Labels)
				if e == nil {
					_, e = f.service.SuggestLabel(ctx, f.p, "annotation", task.ID, item.ID, svc.LabelSuggestionRequest{ExpectedVersion: item.Version, Guidelines: svc.LabelBenchmarkGuidelines(s.Config.Kind)}, fmt.Sprintf("diagnostic:%d", i))
				}
				data, marshalErr := json.MarshalIndent(capture, "", "  ")
				if marshalErr != nil {
					t.Error("diagnostic encoding failed")
					continue
				}
				path := filepath.Join(outDir, fmt.Sprintf("private-response-%02d.json", i))
				file, writeErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if writeErr == nil {
					_, writeErr = file.Write(data)
					if writeErr == nil {
						writeErr = file.Sync()
					}
					_ = file.Close()
				}
				if writeErr != nil {
					t.Error("diagnostic persistence failed")
				}
				results[i] = result{Index: i, Dataset: s.Dataset, ID: s.Row.ID, ErrorCode: hfSafeError(e), Diagnosis: hfDiagnoseCandidate(s.Row.Text, s.Config.Labels, capture), ResponseSHA: svc.LabelBenchmarkTextDigest(capture.Content), Usage: capture.Usage}
			}
		}()
	}
	for i := range selected {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	require.Equal(t, int64(12), observed.calls.Load())
	hfJSON(t, filepath.Join(outDir, "summary.json"), map[string]any{"kind": "engineering_diagnostic_not_quality", "actual_calls": observed.calls.Load(), "results": results, "raw_private_files": "0600; never copy into public evidence"}, true)
}

func hfDiagnoseCandidate(text string, labels []d.Label, capture *hfDiagnosticCapture) map[string]any {
	finish := "other"
	switch capture.Finish {
	case "", "stop", "end_turn", "length", "tool_calls", "content_filter":
		finish = capture.Finish
	}
	diag := map[string]any{"finish_reason": finish, "tool_calls": capture.ToolCalls, "content_bytes": len(capture.Content)}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(capture.Content), &fields) != nil || fields == nil {
		diag["code"] = "invalid_json_object"
		return diag
	}
	allowed := map[string]bool{"annotation": true, "tags": true, "category": true, "decision_status": true, "rationale": true, "evidence_spans": true}
	unknown := 0
	for k := range fields {
		if !allowed[k] {
			unknown++
		}
	}
	diag["unknown_top_level_fields"] = unknown
	if unknown > 0 {
		diag["code"] = "unknown_top_level_fields"
		return diag
	}
	var c svc.LabelCandidate
	dec := json.NewDecoder(strings.NewReader(capture.Content))
	dec.DisallowUnknownFields()
	if dec.Decode(&c) != nil {
		diag["code"] = "nested_schema_or_type"
		return diag
	}
	if c.Annotation == nil {
		diag["code"] = "annotation_missing"
		return diag
	}
	known := map[string]bool{}
	for _, label := range labels {
		known[label.ID] = true
	}
	diag["labels_count"] = len(c.Annotation.Labels)
	diag["evidence_count"] = len(c.Evidence)
	diag["rationale_runes"] = len([]rune(c.Rationale))
	if c.DecisionStatus != "matched" && c.DecisionStatus != "uncertain" {
		diag["code"] = "decision_status"
		return diag
	}
	if len([]rune(c.Rationale)) < 1 || len([]rune(c.Rationale)) > 1000 {
		diag["code"] = "rationale_length"
		return diag
	}
	chars := []rune(text)
	evidenced := map[string]bool{}
	for i, e := range c.Evidence {
		if e.Start < 0 || e.End <= e.Start || e.End > len(chars) {
			diag["code"] = "evidence_offset_bounds"
			diag["evidence_index"] = i
			return diag
		}
		if e.Match != string(chars[e.Start:e.End]) {
			diag["code"] = "evidence_literal_offset_mismatch"
			diag["evidence_index"] = i
			diag["literal_occurrences"] = strings.Count(text, e.Match)
			return diag
		}
		if e.Rule != "model_evidence" {
			diag["code"] = "evidence_rule"
			return diag
		}
		if e.LabelID != "" && !known[e.LabelID] {
			diag["code"] = "evidence_unknown_label"
			return diag
		}
		evidenced[e.LabelID] = true
	}
	for _, id := range c.Annotation.Labels {
		if !known[id] {
			diag["code"] = "annotation_unknown_label"
			return diag
		}
		if !evidenced[id] {
			diag["code"] = "annotation_without_evidence"
			return diag
		}
	}
	diag["code"] = "no_classification_violation_identified"
	return diag
}
