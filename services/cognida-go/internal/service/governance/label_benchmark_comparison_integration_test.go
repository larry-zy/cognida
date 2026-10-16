//go:build integration

package governance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	svc "cognida/internal/service/governance"
	"github.com/stretchr/testify/require"
)

// Read-only rescoring of already produced rule outputs on exactly the model
// selection. This cannot invoke a model, fit a rule or alter any prior result.
func TestHFLabelPairedRuleScores(t *testing.T) {
	root := os.Getenv("COGNIDA_LABEL_COMPARISON_ROOT")
	if root == "" {
		t.Skip("explicit evidence root required")
	}
	for _, key := range []string{"banking77", "massive_zh", "few_nerd"} {
		t.Run(key, func(t *testing.T) {
			var protocol hfLabelProtocol
			require.NoError(t, json.Unmarshal(hfRead(t, filepath.Join(root, key, "v3", "protocol.json")), &protocol))
			raw := hfRead(t, filepath.Join(root, key, "v2", "rules-report.json"))
			var baseline hfLabelResult
			require.NoError(t, json.Unmarshal(raw, &baseline))
			byID := map[string]svc.LabelBenchmarkOutcome{}
			for _, row := range baseline.Full.Outcomes {
				byID[row.ID] = row
			}
			var rows []svc.LabelBenchmarkSample
			selected := &svc.LabelBenchmarkReport{}
			for _, selection := range protocol.Selection {
				out, ok := byID[selection.ID]
				require.True(t, ok)
				require.Equal(t, selection.TextSHA256, out.TextDigest)
				// Text is supplied only for the unchanged annotation validator. It is
				// verified against the exact original file hash and selected digest.
				selected.Outcomes = append(selected.Outcomes, out)
			}
			_, test := hfReadData(t, protocol.Config)
			source := map[string]svc.LabelBenchmarkSample{}
			for _, row := range test {
				source[row.ID] = row
			}
			for _, selection := range protocol.Selection {
				row, ok := source[selection.ID]
				require.True(t, ok)
				require.Equal(t, selection.TextSHA256, svc.LabelBenchmarkTextDigest(row.Text))
				rows = append(rows, row)
			}
			report := hfSubset(t, protocol.Config, rows, selected, map[string]bool{}, false)
			hfJSON(t, filepath.Join(root, key, "v3", "paired-rules-report.json"), map[string]any{"kind": "offline_rescore_of_original_rule_outputs", "actual_new_model_calls": 0, "rule_report_sha256": svc.LabelBenchmarkTextDigest(string(raw)), "selected_ids_identical_to_model": true, "report": report}, true)
			t.Logf("%s paired rule samples=%d macro_f1=%.6f", key, report.Samples, report.MacroF1)
		})
	}
}
