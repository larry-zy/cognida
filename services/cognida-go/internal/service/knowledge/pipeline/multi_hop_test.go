package rag

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestMultiHopParsesStructuredReasoning(t *testing.T) {
	m := &MultiHopRetrieverImpl{}
	for _, input := range []string{`{"intermediate_answer":"partial","should_continue":true,"next_query":"second question"}`, "{\n  \"intermediate_answer\": \"partial\",\n  \"should_continue\": true,\n  \"next_query\": \"second question\"\n}", "```json\n{\"intermediate_answer\":\"partial\",\"should_continue\":true,\"next_query\":\"second question\"}\n```"} {
		q, answer, next, err := m.parseReasoningResponse(input, 1, 3)
		require.NoError(t, err)
		require.Equal(t, "second question", q)
		require.Equal(t, "partial", answer)
		require.True(t, next)
		_, _, next, err = m.parseReasoningResponse(input, 3, 3)
		require.NoError(t, err)
		require.False(t, next)
	}
	_, _, next, err := m.parseReasoningResponse(`{"intermediate_answer":"enough","should_continue":false}`, 1, 3)
	require.NoError(t, err)
	require.False(t, next)
	for _, input := range []string{"", "plain text", `{}`, `{"intermediate_answer":"x","should_continue":"true"}`, `{"intermediate_answer":"x","should_continue":true,"next_query":""}`, `{"intermediate_answer":"","should_continue":false}`, `{"intermediate_answer":"x","should_continue":true,"next_query":"` + strings.Repeat("x", 4001) + `"}`} {
		_, _, _, err = m.parseReasoningResponse(input, 1, 3)
		require.Error(t, err)
	}
}
