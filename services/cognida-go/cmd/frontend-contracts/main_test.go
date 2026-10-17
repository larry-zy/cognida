package main

import (
	g "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOmitZeroMatchesFeatureValueJSON(t *testing.T) {
	output := fields(reflect.TypeOf(g.FeatureValue{}))
	if !strings.Contains(output, "available_at?: string;") || !strings.Contains(output, "timestamp: string;") {
		t.Fatalf("zero-omitted timestamp must be optional without changing required timestamp: %s", output)
	}
	for _, availableAt := range []time.Time{{}, time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)} {
		body, err := json.Marshal(g.FeatureValue{AvailableAt: availableAt})
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		_, present := fields["available_at"]
		if present == availableAt.IsZero() {
			t.Fatalf("available_at presence does not match omitzero: %s", body)
		}
		if _, present := fields["timestamp"]; !present {
			t.Fatalf("required timestamp was omitted: %s", body)
		}
	}
}

func TestJSONContractRespectsPrivateAndEmbeddedFields(t *testing.T) {
	output := fields(reflect.TypeOf(l.DataAssetRef{}))
	for _, expected := range []string{"asset_id: string;", "version: number;", "expires_at?: string | null;"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %s in %s", expected, output)
		}
	}
	if strings.Contains(output, "ObjectKey") || strings.Contains(output, "object_key") {
		t.Fatal("private storage locator leaked into frontend contract")
	}
	if ts(reflect.TypeOf(time.Time{})) != "string" {
		t.Fatal("timestamps must use JSON strings")
	}
}
func TestOutputDeterministic(t *testing.T) {
	first, second := generate(), generate()
	if string(first) != string(second) {
		t.Fatal("contract generation is nondeterministic")
	}
}

func TestNilJSONCollectionsAreNullable(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{{[]string(nil), "Array<string> | null"}, {map[string]float64(nil), "Record<string, number> | null"}, {[]byte(nil), "string | null"}, {[2]string{}, "Array<string>"}} {
		if got := ts(reflect.TypeOf(tc.value)); got != tc.want {
			t.Fatalf("got %s want %s", got, tc.want)
		}
	}
}
