package main

import (
	l "cognida/internal/model/lifecycle"
	"reflect"
	"strings"
	"testing"
	"time"
)

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
