package huggingface

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func validRequest() Request {
	return Request{RepoID: "mteb/banking77", Revision: strings.Repeat("a", 40), Config: "default", Split: "test", Files: []string{"data/test.jsonl"}, FieldMapping: FieldMapping{TextField: "text"}, MaxRows: 100, MaxBytes: 1 << 20}
}
func TestRequestFixedCommitAndExplicitMapping(t *testing.T) {
	require.NoError(t, validRequest().Validate())
	cases := []func(*Request){func(r *Request) { r.Revision = "main" }, func(r *Request) { r.RepoID = "https://example.com/data" }, func(r *Request) { r.Files = []string{"../test.jsonl"} }, func(r *Request) { r.Files = []string{"test.py"} }, func(r *Request) { r.Files = []string{"a.jsonl?token=x"} }, func(r *Request) { r.Files = []string{"a.jsonl", "a.jsonl"} }, func(r *Request) { r.MaxRows = 1001 }, func(r *Request) { r.MaxBytes = MaxDownloadBytes + 1 }, func(r *Request) { r.FieldMapping.Columns = []ColumnMapping{{Source: "gold", Target: "answer"}} }, func(r *Request) {
		r.FieldMapping = FieldMapping{Columns: []ColumnMapping{{Source: "messages", Target: "messages", Encoding: "execute"}}}
	}}
	for i, change := range cases {
		r := validRequest()
		change(&r)
		require.ErrorIs(t, r.Validate(), ErrInvalid, "case %d", i)
	}
	r := validRequest()
	r.FieldMapping = FieldMapping{Columns: []ColumnMapping{{Source: "messages", Target: "messages", Encoding: "json"}, {Source: "label", Target: "supervised_answer"}}}
	require.NoError(t, r.Validate())
}
