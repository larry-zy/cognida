package governance

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestJSONEqual(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		equal bool
	}{{`{"x":[1,2],"y":3}`, `{ "y":3.0,"x":[1.0,2e0]}`, true}, {`9007199254740993`, `9007199254740992`, false}, {`"1"`, `1`, false}, {`null`, `null`, true}, {`1e999999999`, `10e999999998`, true}, {`-0.00`, `0`, true}, {`1 2`, `1`, false}} {
		require.Equal(t, tc.equal, JSONEqual(json.RawMessage(tc.a), json.RawMessage(tc.b)), tc.a)
	}
}
