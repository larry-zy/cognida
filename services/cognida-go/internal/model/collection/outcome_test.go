package collection

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSummarizeItemsDistinguishesUsableDuplicatesAndIncompleteResults(t *testing.T) {
	for _, tc := range []struct {
		name     string
		expected int
		items    []*Item
		want     OutcomeSummary
	}{
		{"no items yet", 2, nil, OutcomeSummary{Outcome: "running", Total: 2, Pending: 2}},
		{"no sources", 0, nil, OutcomeSummary{Outcome: "failed"}},
		{"unchanged", 1, []*Item{{Status: ItemStatusSkipped, KnowledgeID: "existing"}}, OutcomeSummary{Outcome: "success", Total: 1, Skipped: 1, Usable: 1}},
		{"missing accepted receipt", 1, []*Item{{Status: ItemStatusAccepted}}, OutcomeSummary{Outcome: "failed", Total: 1, Failed: 1}},
		{"missing duplicate receipt", 1, []*Item{{Status: ItemStatusSkipped}}, OutcomeSummary{Outcome: "failed", Total: 1, Failed: 1}},
		{"partial", 3, []*Item{{Status: ItemStatusAccepted, KnowledgeID: "new"}, {Status: ItemStatusSkipped, KnowledgeID: "old"}, {Status: ItemStatusRejected}}, OutcomeSummary{Outcome: "partial", Total: 3, Accepted: 1, Skipped: 1, Rejected: 1, Usable: 2}},
		{"in progress", 2, []*Item{{Status: ItemStatusFailed}}, OutcomeSummary{Outcome: "running", Total: 2, Failed: 1, Pending: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.want, SummarizeItems(tc.expected, tc.items)) })
	}
}
