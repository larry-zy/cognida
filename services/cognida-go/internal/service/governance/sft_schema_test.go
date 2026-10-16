package governance

import (
	"encoding/json"
	"strings"
	"testing"

	d "cognida/internal/model/governance"
	"github.com/stretchr/testify/require"
)

func TestSFTToolSchemaValidatesExactArguments(t *testing.T) {
	raw := `{"type":"object","properties":{"amount":{"type":"number","minimum":9007199254740993.123456789012345678,"maximum":9007199254740993.123456789012345679},"code":{"type":"string","minLength":2,"maxLength":5,"enum":["00123","中文"]},"n":{"type":"integer","minimum":1,"maximum":3},"ok":{"type":"boolean"},"empty":{"type":"null"},"items":{"type":"array","minItems":1,"maxItems":2,"items":{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}}},"required":["amount","code"],"additionalProperties":false}`
	schema, err := compileSFTSchema(json.RawMessage(raw))
	require.NoError(t, err)
	for _, tc := range []struct {
		name, args string
		valid      bool
	}{
		{"exact", `{"amount":9007199254740993.123456789012345678,"code":"00123","n":1.0,"ok":true,"empty":null,"items":[{"id":"x"}]}`, true},
		{"unicode_length", `{"amount":9007199254740993.123456789012345679,"code":"中文"}`, true},
		{"wrong_number", `{"amount":"9007199254740993.123456789012345678","code":"00123"}`, false},
		{"missing_required", `{"code":"00123"}`, false},
		{"null_required", `{"amount":null,"code":"00123"}`, false},
		{"below_precise_min", `{"amount":9007199254740993.123456789012345677,"code":"00123"}`, false},
		{"above_precise_max", `{"amount":9007199254740993.123456789012345680,"code":"00123"}`, false},
		{"wrong_enum", `{"amount":9007199254740993.123456789012345678,"code":"123"}`, false},
		{"extra_key", `{"amount":9007199254740993.123456789012345678,"code":"00123","extra":true}`, false},
		{"wrong_case", `{"amount":9007199254740993.123456789012345678,"Code":"00123"}`, false},
		{"fractional_integer", `{"amount":9007199254740993.123456789012345678,"code":"00123","n":1.1}`, false},
		{"bool_not_number", `{"amount":9007199254740993.123456789012345678,"code":"00123","n":true}`, false},
		{"number_not_bool", `{"amount":9007199254740993.123456789012345678,"code":"00123","ok":1}`, false},
		{"null_only", `{"amount":9007199254740993.123456789012345678,"code":"00123","empty":"null"}`, false},
		{"empty_array", `{"amount":9007199254740993.123456789012345678,"code":"00123","items":[]}`, false},
		{"nested_required", `{"amount":9007199254740993.123456789012345678,"code":"00123","items":[{}]}`, false},
		{"nested_extra", `{"amount":9007199254740993.123456789012345678,"code":"00123","items":[{"id":"x","extra":1}]}`, false},
		{"nested_type", `{"amount":9007199254740993.123456789012345678,"code":"00123","items":[1]}`, false},
		{"array_limit", `{"amount":9007199254740993.123456789012345678,"code":"00123","items":[{"id":"1"},{"id":"2"},{"id":"3"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var args any
			require.NoError(t, sftDecode(tc.args, &args))
			require.Equal(t, tc.valid, schema.validate(args))
		})
	}
}

func TestSFTToolSchemaRejectsUnsupportedAndMalformedDeclarations(t *testing.T) {
	for _, raw := range []string{
		`null`, `true`, `{"properties":{}}`, `{"type":["object","null"]}`, `{"type":"Object"}`,
		`{"type":"object","$ref":"https://private.invalid/schema"}`,
		`{"type":"object","oneOf":[{"type":"object"}]}`,
		`{"type":"object","Properties":{}}`, `{"type":"object","type":"object"}`,
		`{"type":"object","properties":null}`, `{"type":"object","required":null}`,
		`{"type":"object","required":["absent"]}`,
		`{"type":"object","properties":{"a":{"type":"string"}},"required":["a","a"]}`,
		`{"type":"object","additionalProperties":null}`,
		`{"type":"object","properties":{"a":{"type":"array"}}}`,
		`{"type":"object","properties":{"a":{"type":"array","items":[{"type":"string"}]}}}`,
		`{"type":"object","properties":{"a":{"type":"string","pattern":".*"}}}`,
		`{"type":"object","properties":{"a":{"type":"string","minLength":-1}}}`,
		`{"type":"object","properties":{"a":{"type":"string","minLength":5,"maxLength":2}}}`,
		`{"type":"object","properties":{"a":{"type":"number","minimum":2,"maximum":1}}}`,
		`{"type":"object","properties":{"a":{"type":"number","minimum":"1"}}}`,
		`{"type":"object","properties":{"a":{"type":"number","minimum":1e1000000}}}`,
		`{"type":"object","properties":{"a":{"type":"string","minimum":1}}}`,
		`{"type":"object","properties":{"a":{"type":"integer","enum":[1,1.0]}}}`,
		`{"type":"object","properties":{"a":{"type":"integer","enum":[1,true]}}}`,
		`{"type":"object","properties":{"a":{"type":"string","enum":[]}}}`,
		`{"type":"object","properties":{"a":{"type":"object","enum":[{}]}}}`,
	} {
		_, err := compileSFTSchema(json.RawMessage(raw))
		require.ErrorIs(t, err, d.ErrInvalid, raw)
		require.NotContains(t, err.Error(), "private.invalid")
	}
	deep := `{"type":"string"}`
	for range 14 {
		deep = `{"type":"object","properties":{"child":` + deep + `}}`
	}
	_, err := compileSFTSchema(json.RawMessage(deep))
	require.ErrorIs(t, err, d.ErrInvalid)
}

func TestSFTToolSchemaAdditionalPropertiesAndResourceBounds(t *testing.T) {
	for _, tc := range []struct {
		schema, args string
		valid        bool
	}{
		{`{"type":"object"}`, `{"a":[null,"text",true]}`, true},
		{`{"type":"object","additionalProperties":false}`, `{}`, true},
		{`{"type":"object","additionalProperties":false}`, `{"a":1}`, false},
		{`{"type":"object","additionalProperties":{"type":"integer","enum":[1,2]}}`, `{"a":1.0,"b":2}`, true},
		{`{"type":"object","additionalProperties":{"type":"integer","enum":[1,2]}}`, `{"a":"1"}`, false},
	} {
		schema, err := compileSFTSchema(json.RawMessage(tc.schema))
		require.NoError(t, err)
		var args any
		require.NoError(t, sftDecode(tc.args, &args))
		require.Equal(t, tc.valid, schema.validate(args))
	}
	for _, raw := range []string{`{"a":1e1000000}`, `{"a":[` + strings.Repeat(`0,`, 1000) + `0]}`, `{"a":` + strings.Repeat(`[`, 34) + `0` + strings.Repeat(`]`, 34) + `}`} {
		var args any
		require.NoError(t, sftDecode(raw, &args))
		nodes := 0
		require.False(t, boundedSFTArguments(args, 0, &nodes))
	}
}

func TestSFTMessagesEnforceDeclaredToolSchemaWithoutCoercion(t *testing.T) {
	q, answer := "amount?", "recorded response"
	tool := d.SFTTool{Type: "function", Function: d.SFTToolFunction{Name: "amount", Parameters: json.RawMessage(`{"type":"object","properties":{"amount":{"type":"number"}},"required":["amount"],"additionalProperties":false}`)}}
	// Keep the original argument lexeme; validation is not a conversion step.
	raw := `{"amount":9007199254740993.123456789012345678}`
	call := d.SFTToolCall{ID: "one", Type: "function", Function: d.SFTFunctionCall{Name: "amount", Arguments: raw}}
	messages := []d.SFTMessage{{Role: "user", Content: &q}, {Role: "assistant", ToolCalls: []d.SFTToolCall{call}}, {Role: "tool", ToolCallID: "one", Content: &answer}, {Role: "assistant", Content: &answer}}
	require.NoError(t, validateSFTMessages(messages, []d.SFTTool{tool}))
	require.Equal(t, raw, messages[1].ToolCalls[0].Function.Arguments)
	for _, bad := range []string{`{"amount":"secret-value"}`, `{}`, `{"amount":1,"extra":true}`} {
		messages[1].ToolCalls[0].Function.Arguments = bad
		err := validateSFTMessages(messages, []d.SFTTool{tool})
		require.ErrorIs(t, err, d.ErrInvalid)
		require.Contains(t, err.Error(), "violate declared parameters")
		require.NotContains(t, err.Error(), "secret-value")
	}
}
