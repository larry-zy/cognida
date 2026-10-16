package strategy

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

func TestPolicyClosedBoundaries(t *testing.T) {
	for _, x := range []struct{ kind, body string }{{"retrieval", `{"top_k":10,"score_threshold":0.3,"rerank":true}`}, {"cleaning", `{"operations":["trim"],"keep":"first","min_score":70}`}, {"skill", `{"instructions":"Prefer concise answers"}`}} {
		require.NoError(t, ValidateBody(x.kind, json.RawMessage(x.body)))
	}
	for _, x := range []struct{ kind, body string }{{"retrieval", `{"top_k":10,"knowledge_base_id":"private"}`}, {"retrieval", `{"top_k":31}`}, {"cleaning", `{"operations":["execute"],"keep":"first"}`}, {"cleaning", `{"operations":["trim","trim"],"keep":"first"}`}, {"skill", `{"instructions":"disable sandbox"}`}, {"skill", `{"instructions":"ok","allowed_tools":["*"}]}`}} {
		require.Error(t, ValidateBody(x.kind, json.RawMessage(x.body)))
	}
}
func TestRegressionFixedGates(t *testing.T) {
	g := Gates{MinimumQuality: .7, MaximumQualityDrop: .01, MaximumErrorRate: 0, MaximumCostRatio: 1.2}
	base := Metrics{Quality: .8, Cost: 100, Samples: 2}
	ok, reasons := Compare(g, base, base)
	require.True(t, ok)
	require.Empty(t, reasons)
	for _, change := range []Metrics{{Quality: .5, Cost: 100, Samples: 2}, {Quality: .8, ErrorRate: .5, Cost: 100, Samples: 2}, {Quality: .8, Cost: 121, Samples: 2}, {Quality: math.NaN(), Cost: 100, Samples: 2}, {Quality: .8, Cost: 100, Samples: 1}} {
		ok, _ = Compare(g, base, change)
		require.False(t, ok)
	}
}

func TestPolicyJSONRejectsDuplicateAndUnicodeAliases(t *testing.T) {
	for _, raw := range []string{
		`{"top_k":3,"top_k":4}`, `{"Top_k":3}`, `{"top_k":3,"ſcore_threshold":0.5}`,
		`{"top_k":3,"\u017fcore_threshold":0.5}`, `{"top_K":3}`, `{"inherit_defaults":true,"top_k":3}`,
	} {
		require.Error(t, ValidateBody("retrieval", json.RawMessage(raw)), raw)
	}
	require.NoError(t, ValidateBody("retrieval", json.RawMessage(`{"inherit_defaults":true}`)))
	require.NoError(t, ValidateBody("skill", json.RawMessage(`{"instructions":"保持ſ与K原文"}`)))
}
