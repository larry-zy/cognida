package apisource

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	d "cognida/internal/model/apisource"
	"cognida/internal/service/datasource"
)

func pageFixture(t *testing.T) (*Service, *d.Run) {
	t.Helper()
	cipher, e := datasource.NewCipher("synthetic-unit-key")
	if e != nil {
		t.Fatal(e)
	}
	return &Service{cipher: cipher}, &d.Run{Config: d.Config{ItemsPointer: "/items", RecordKey: "id", WatermarkField: "updated", DeletedField: "deleted", Pagination: d.Pagination{Mode: "cursor", NextPointer: "/next", SnapshotPointer: "/snapshot", SnapshotParameter: "snapshot"}, Limits: d.Limits{MaxRecords: 100}}, Keys: map[string]string{}, Records: []json.RawMessage{}}
}
func TestPageSnapshotDedupExactAndWatermark(t *testing.T) {
	s, run := pageFixture(t)
	raw := []byte(`{"items":[{"id":"00123","amount":9007199254740993.123456789012345678,"updated":"2026-10-16T00:00:00Z","deleted":false}],"next":"opaque-one","snapshot":"snapshot-one"}`)
	cp, e := s.parsePage(run, raw)
	if e != nil {
		t.Fatal(e)
	}
	if cp.Complete || cp.SnapshotDigest == "" || cp.CursorCiphertext == "opaque-one" || cp.WatermarkAfter != "2026-10-16T00:00:00Z" {
		t.Fatal("checkpoint lost source contract")
	}
	run.Records = cp.Records
	run.Keys = cp.Keys
	run.SnapshotDigest = cp.SnapshotDigest
	run.SnapshotCiphertext = cp.SnapshotCiphertext
	run.CursorCiphertext = cp.CursorCiphertext
	run.Pages = []d.PageReceipt{cp.Page}
	cp, e = s.parsePage(run, []byte(strings.Replace(string(raw), `"next":"opaque-one"`, `"next":null`, 1)))
	if e != nil || !cp.Complete || len(cp.Records) != 1 || cp.DuplicateCount != 1 {
		t.Fatalf("dedup: %v %#v", e, cp)
	}
	input, e := table(cp.Records)
	if e != nil {
		t.Fatal(e)
	}
	if input.ColumnTypes["amount"] != "NUMERIC" || input.ColumnTypes["id"] != "VARCHAR" {
		t.Fatal("number provenance lost")
	}
	values := map[string]any{}
	for i, column := range input.Columns {
		values[column] = input.Rows[0][i]
	}
	if values["amount"] != "9007199254740993.123456789012345678" || values["id"] != "00123" {
		t.Fatal("exact strings were changed")
	}
}
func TestPageRejectsSnapshotKeyAndShapeConflicts(t *testing.T) {
	for _, test := range []struct {
		name, raw string
		want      error
	}{{"snapshot", `{"items":[],"next":null,"snapshot":"changed"}`, errSnapshot}, {"trailing", `{"items":[]} garbage`, errRecord}, {"ambiguous pagination", `{"items":[],"next":true,"snapshot":"fixed"}`, errRecord}, {"missing key", `{"items":[{"updated":"2026-10-16T00:00:00Z"}],"next":null,"snapshot":"fixed"}`, errRecord}, {"bad watermark", `{"items":[{"id":"one","updated":"tomorrow"}],"next":null,"snapshot":"fixed"}`, errRecord}} {
		t.Run(test.name, func(t *testing.T) {
			s, run := pageFixture(t)
			run.SnapshotDigest = hash([]byte("fixed"))
			_, e := s.parsePage(run, []byte(test.raw))
			if !errors.Is(e, test.want) {
				t.Fatalf("error %v", e)
			}
		})
	}
	s, run := pageFixture(t)
	cp, e := s.parsePage(run, []byte(`{"items":[{"id":"one","updated":"2026-10-16T00:00:00Z"}],"next":null,"snapshot":"fixed"}`))
	if e != nil {
		t.Fatal(e)
	}
	run.Records = cp.Records
	run.Keys = cp.Keys
	_, e = s.parsePage(run, []byte(`{"items":[{"id":"one","updated":"2026-10-17T00:00:00Z"}],"next":null,"snapshot":"fixed"}`))
	if !errors.Is(e, errRecord) {
		t.Fatal("changed stable record accepted")
	}
}
func TestCredentialReflectionEscapedJSON(t *testing.T) {
	if !containsSecret([]byte(`{"value":"synthetic-\u0074oken-value"}`), "synthetic-token-value") {
		t.Fatal("escaped reflected credential accepted")
	}
	if containsSecret([]byte(`{"value":"ordinary"}`), "synthetic-token-value") {
		t.Fatal("false reflection")
	}
}
func TestMixedNumericAndStringColumnStaysText(t *testing.T) {
	input, e := table([]json.RawMessage{[]byte(`{"id":1,"value":10}`), []byte(`{"id":2,"value":"00123"}`), []byte(`{"id":3,"value":20}`)})
	if e != nil || input.ColumnTypes["value"] != "VARCHAR" {
		t.Fatalf("mixed values mislabeled %v %+v", e, input)
	}
}
