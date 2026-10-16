package collection

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestGovernedSnapshotValidationAndArtifactSummary(t *testing.T) {
	in := GovernedInvocation{OperationKey: strings.Repeat("a", 64), PayloadHash: strings.Repeat("b", 64), Profile: SnapshotProfile, URL: "https://example.com", MaxResponseBytes: 1024, Deadline: time.Now().Add(time.Minute), TimeoutSeconds: 60, CallsReserved: 3}
	require.NoError(t, in.Validate())
	other := in
	other.URL = "https://other.example"
	require.False(t, SameGoverned(in, other))
	in.URL = "http://example.com"
	require.Error(t, in.Validate())
	summary := SummarizeItems(1, []*Item{{Status: ItemStatusAccepted, AssetRefID: "actual#1"}})
	require.Equal(t, "success", summary.Outcome)
	require.Equal(t, 1, summary.Usable)
}
