//go:build integration

package governance

import (
	"cognida/internal/config"
	provider "cognida/internal/infrastructure/llm"
	d "cognida/internal/model/governance"
	"cognida/internal/model/llm"
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

type observedLabelClient struct {
	llm.LLMClient
	response *llm.ChatResponse
}

func (c *observedLabelClient) Chat(ctx context.Context, r *llm.ChatRequest) (*llm.ChatResponse, error) {
	v, e := c.LLMClient.Chat(ctx, r)
	c.response = v
	return v, e
}
func TestLiveLabelSuggestionSyntheticQA(t *testing.T) {
	if os.Getenv("COGNIDA_TEST_LIVE_LABELS") != "1" {
		t.Skip("requires explicitly configured live model; synthetic input only")
	}
	cfg := config.LoadChatConfig()
	repo, e := provider.NewChatRepository(&llm.ModelConfig{Provider: llm.Provider(cfg.Provider), BaseURL: cfg.BaseURL, ModelName: cfg.ModelName, APIKey: cfg.APIKey})
	require.NoError(t, e)
	client := &observedLabelClient{LLMClient: repo}
	s := NewLLMLabelSuggester(client, cfg.Provider+":"+cfg.ModelName)
	in := LabelSuggestionInput{Kind: "qa", Content: `{"question":"What is the capital of France?","reference_answer":"Paris"}`, QA: &d.QAPair{Question: "What is the capital of France?", ReferenceAnswer: "Paris"}, Guidelines: "Preserve the exact question and reference answer."}
	candidate, e := s.Suggest(context.Background(), in)
	if e != nil && client.response != nil {
		t.Logf("synthetic model response: finish=%q content=%q message=%+v", client.response.FinishReason, client.response.Content, client.response.Message)
	}
	require.NoError(t, e)
	require.Equal(t, in.QA, candidate.Annotation.QA)
}
