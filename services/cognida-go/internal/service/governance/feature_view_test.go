package governance

import (
	d "cognida/internal/model/governance"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFeatureViewRestoresExactDefinitionNumbers(t *testing.T) {
	exact := `{"kind":"expression","expression":{"op":"literal","value":9007199254740993.123456789012345678}}`
	payload, err := json.Marshal(map[string]any{"definition_exact": exact, "definition": map[string]any{"kind": "expression", "expression": map[string]any{"op": "literal", "value": float64(9007199254740992)}}})
	require.NoError(t, err)
	view, err := View(&d.Record{Kind: "feature", Payload: payload})
	require.NoError(t, err)
	definition, err := json.Marshal(view["definition"])
	require.NoError(t, err)
	require.Contains(t, string(definition), "9007199254740993.123456789012345678")
	require.NotContains(t, view, "definition_exact")
}
