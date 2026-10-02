package chat

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"github.com/stretchr/testify/require"
)

func TestBuildRequestPreservesToolParameterSchema(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params *schema.ParamsOneOf
	}{
		{"parameter map", schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"analysis_type": {Type: schema.String, Required: true, Enum: []string{"describe", "trend"}},
			"data":          {Type: schema.Object},
		})},
		{"declared dialect", schema.NewParamsOneOfByJSONSchema(&jsonschema.Schema{
			Version: "https://json-schema.org/draft/2020-12/schema", Type: "object",
		})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &openaiClient{model: "test", toolInfos: []*schema.ToolInfo{{Name: "data_analysis", ParamsOneOf: tc.params}}}
			original, err := tc.params.ToJSONSchema()
			require.NoError(t, err)
			want, err := json.Marshal(original)
			require.NoError(t, err)
			request := client.buildRequest([]*schema.Message{schema.UserMessage("analyze")}, nil, false)
			require.Len(t, request.Tools, 1)
			actual, err := json.Marshal(request.Tools[0].Function.Parameters)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(actual), "provider requests must preserve the generated schema and its dialect")
			require.NotContains(t, string(actual), "http://json.org/")
		})
	}
}
