//go:build integration

package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"cognida/internal/model/llm"
	"github.com/stretchr/testify/require"
)

// Offline only: the originally invalid explicit assertions remain invalid.
// This is not a new model run and never rewrites the first experiment's scores.
func TestLabelLiteralOfflineDiagnosticReplay(t *testing.T) {
	dir := os.Getenv("COGNIDA_LABEL_DIAGNOSTIC_REPLAY")
	if dir == "" {
		t.Skip("local private training diagnostic required")
	}
	var summary struct {
		Results []struct {
			Index int    `json:"index"`
			Error string `json:"error_code"`
		} `json:"results"`
	}
	raw, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &summary))
	require.Len(t, summary.Results, 12)
	rejected := 0
	for _, row := range summary.Results {
		var capture struct {
			Messages []*llm.Message `json:"messages"`
			Content  string         `json:"content"`
			Finish   string         `json:"finish_reason"`
		}
		raw, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("private-response-%02d.json", row.Index)))
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &capture))
		require.Len(t, capture.Messages, 2)
		var input LabelSuggestionInput
		require.NoError(t, json.Unmarshal([]byte(capture.Messages[1].Content), &input))
		client := &labelModelStub{response: &llm.ChatResponse{Content: capture.Content, FinishReason: capture.Finish}}
		_, err = NewLLMLabelSuggester(client, "offline-fixture").Suggest(context.Background(), input)
		if row.Error != "" {
			require.ErrorIs(t, err, ErrLabelModelOutput)
			rejected++
		} else {
			require.NoError(t, err)
		}
	}
	require.Equal(t, 8, rejected)
}
