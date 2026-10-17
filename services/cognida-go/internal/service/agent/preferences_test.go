package agent

import (
	d "cognida/internal/model/agent"
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type preferenceFixture struct {
	value   d.Preferences
	err     error
	updates int
	scope   d.ConfigScope
}

func (f *preferenceFixture) Read(_ context.Context, p d.ConfigScope) (d.Preferences, error) {
	f.scope = p
	raw, _ := json.Marshal(f.value)
	var copy d.Preferences
	_ = json.Unmarshal(raw, &copy)
	return copy, f.err
}
func (f *preferenceFixture) Update(_ context.Context, p d.ConfigScope, version int64, key, hash string, next d.Preferences) (d.Preferences, error) {
	f.updates++
	f.scope = p
	next.Version = version + 1
	f.value = next
	return next, f.err
}
func preferenceUpdate(t *testing.T, body string) ConfigUpdate {
	t.Helper()
	var r ConfigUpdate
	require.NoError(t, json.Unmarshal([]byte(body), &r))
	return r
}
func TestAgentPreferencesValidationAndPatch(t *testing.T) {
	f := &preferenceFixture{}
	s := NewConfigService(f)
	p := d.ConfigScope{TenantID: 7, UserID: 9}
	ctx := context.Background()
	got, err := s.Update(ctx, p, preferenceUpdate(t, `{"expected_version":0,"agentic_rag":{"default_top_k":12,"min_score":0,"rerank_enabled":false},"deep_research":{"max_sources":9}}`), "set")
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Version)
	require.Equal(t, 12, *got.AgenticRAG.DefaultTopK)
	require.False(t, *got.AgenticRAG.RerankEnabled)
	require.Zero(t, *got.AgenticRAG.MinScore)
	got, err = s.Update(ctx, p, preferenceUpdate(t, `{"expected_version":1,"agentic_rag":{"default_top_k":null}}`), "reset")
	require.NoError(t, err)
	require.Nil(t, got.AgenticRAG.DefaultTopK)
	require.Equal(t, 9, *got.DeepResearch.MaxSources)
	require.Equal(t, 2, f.updates)
	for _, body := range []string{
		`{}`, `{"expected_version":0}`, `{"expected_version":-1,"agentic_rag":{"default_top_k":3}}`,
		`{"expected_version":0,"agentic_rag":{"default_top_k":101}}`, `{"expected_version":0,"agentic_rag":{"default_top_k":1.5}}`,
		`{"expected_version":0,"agentic_rag":{"max_iterations":0}}`, `{"expected_version":0,"agentic_rag":{"alpha":1.1}}`,
		`{"expected_version":0,"agentic_rag":{"alpha":0}}`,
		`{"expected_version":0,"agentic_rag":{"search_config":{"api_key":"secret"}}}`, `{"expected_version":0,"agentic_rag":{"enable_result_eval":true}}`,
		`{"expected_version":0,"deep_research":{"max_sub_questions":6}}`, `{"expected_version":0,"deep_research":{"max_concurrency":4}}`,
		`{"expected_version":0,"deep_research":{"timeout_seconds":301}}`, `{"expected_version":0,"deep_research":{"enable_fact_check":false}}`,
	} {
		_, err = s.Update(ctx, p, preferenceUpdate(t, body), "invalid")
		require.ErrorIs(t, err, d.ErrConfigInvalid, body)
	}
	require.Equal(t, 2, f.updates)
	_, err = s.Read(ctx, d.ConfigScope{})
	require.ErrorIs(t, err, d.ErrConfigInvalid)
	_, err = NewConfigService().Read(ctx, p)
	require.ErrorIs(t, err, d.ErrConfigUnavailable)
}
func TestAgentPreferencesRAGSnapshotAndSessionPriority(t *testing.T) {
	f := &preferenceFixture{}
	s := NewConfigService(f)
	_, err := s.Update(context.Background(), d.ConfigScope{TenantID: 7, UserID: 9}, preferenceUpdate(t, `{"expected_version":0,"agentic_rag":{"max_iterations":2,"default_top_k":12,"min_score":0.4,"rerank_enabled":false,"alpha":0.8}}`), "set")
	require.NoError(t, err)
	top := 3
	ctx := d.WithRetrievalParams(researchTestContext(), &d.RetrievalParams{TopK: &top})
	snapshot, err := s.ApplyRAG(ctx)
	require.NoError(t, err)
	params, ok := d.GetRetrievalParams(snapshot)
	require.True(t, ok)
	require.Equal(t, 3, *params.TopK)
	require.Equal(t, 0.4, *params.MinScore)
	require.False(t, *params.RerankEnabled)
	require.Equal(t, 2, d.EffectiveIterationLimit(snapshot, 8))
	require.Equal(t, 1, d.EffectiveIterationLimit(snapshot, 1))
	*f.value.AgenticRAG.MinScore = 0.9
	*f.value.AgenticRAG.MaxIterations = 5
	require.Equal(t, 0.4, *params.MinScore)
	require.Equal(t, 2, d.EffectiveIterationLimit(snapshot, 8))
	require.Equal(t, d.ConfigScope{TenantID: 7, UserID: 9}, f.scope)
	f.err = errors.New("database offline")
	_, err = s.ApplyRAG(ctx)
	require.Error(t, err)
}
func TestAgentPreferencesResearchDefaultsAndGovernedIsolation(t *testing.T) {
	f := &preferenceFixture{}
	prefs := NewConfigService(f)
	_, err := prefs.Update(context.Background(), d.ConfigScope{TenantID: 7, UserID: 9}, preferenceUpdate(t, `{"expected_version":0,"deep_research":{"max_sub_questions":1,"max_concurrency":1,"max_sources":8,"timeout_seconds":30}}`), "set")
	require.NoError(t, err)
	s := newResearchTestService(t, false, researchGoodDraft()).WithConfig(prefs)
	req := &DeepResearchRequest{Query: "question"}
	limits, err := s.validate(researchTestContext(), req)
	require.NoError(t, err)
	require.Equal(t, researchLimits{questions: 1, concurrency: 1, sources: 8, timeout: 30 * time.Second}, limits)
	req.Options = &DeepResearchOptions{MaxSources: 11}
	limits, err = s.validate(researchTestContext(), req)
	require.NoError(t, err)
	require.Equal(t, 11, limits.sources)
	require.Equal(t, 1, limits.questions)
	// Durable plans retain their explicit parameters even if private preferences
	// become unavailable or change after the plan was authorized.
	f.err = errors.New("offline")
	_, err = s.validate(researchTestContext(), req)
	require.ErrorIs(t, err, ErrResearchUnavailable)
	req.Options = &DeepResearchOptions{MaxSubQuestions: 3, MaxConcurrency: 2, MaxSources: 20, TimeoutSeconds: 120}
	limits, err = s.validatePreferences(researchTestContext(), req, false)
	require.NoError(t, err)
	require.Equal(t, 3, limits.questions)
	require.Equal(t, 120*time.Second, limits.timeout)
}
