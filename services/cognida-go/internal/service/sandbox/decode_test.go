package sandbox_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	d "cognida/internal/model/sandbox"
	svc "cognida/internal/service/sandbox"
)

func TestDecodeSubmitRequestStrictBoundary(t *testing.T) {
	r := request()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.DecodeSubmitRequest(raw)
	if err != nil || !reflect.DeepEqual(got, r) {
		t.Fatalf("valid contract roundtrip: %v", err)
	}
	tests := map[string]string{
		"empty": "", "null": "null", "array": "[]", "malformed": "{", "no schema": "{}",
		"trailing request":  string(raw) + ` {"schema_version":"1"}`,
		"trailing junk":     string(raw) + ` secret`,
		"duplicate schema":  `{"schema_version":"1","schema_version":"2"}`,
		"case alias":        `{"schema_version":"1","SCHEMA_VERSION":"2"}`,
		"noncanonical key":  `{"SCHEMA_VERSION":"1"}`,
		"escaped duplicate": `{"schema_version":"1","schema\u005fversion":"2"}`,
		"nested duplicate":  `{"schema_version":"1","resources":{"cpu_millis":1,"cpu_millis":2}}`,
		"nested alias":      `{"schema_version":"1","resources":{"cpu_millis":1,"CPU_MILLIS":2}}`,
		"deep nesting":      `{"schema_version":"1","inputs":` + strings.Repeat("[", 10) + `0` + strings.Repeat("]", 10) + `}`,
		"resource float":    `{"schema_version":"1","resources":{"cpu_millis":1.1}}`,
		"resource overflow": `{"schema_version":"1","resources":{"cpu_millis":9223372036854775808}}`,
		"oversized":         strings.Repeat(" ", svc.MaxSubmissionBytes+1),
	}
	for _, key := range []string{"host_path", "mounts", "docker_socket", "environment", "credentials", "image", "capabilities", "profile"} {
		tests[key] = `{"schema_version":"1","` + key + `":"secret-token"}`
	}
	tests["nested host path"] = `{"schema_version":"1","code":{"host_path":"/etc/passwd"}}`
	tests["nested credential"] = `{"schema_version":"1","inputs":[{"ref":{"asset_id":"x","credentials":"secret-token"}}]}`
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := svc.DecodeSubmitRequest([]byte(payload))
			if !errors.Is(err, d.ErrInvalid) {
				t.Fatalf("accepted ambiguous/unknown contract: %v", err)
			}
			if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "/etc/passwd") {
				t.Fatal("error exposed caller values")
			}
		})
	}
}
