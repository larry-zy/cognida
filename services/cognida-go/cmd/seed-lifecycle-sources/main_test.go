package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFixtureSeedFailsClosedBeforeOpeningResources(t *testing.T) {
	for _, tc := range []struct {
		name, dsn, isolated, api, redis, message string
	}{
		{"ordinary database", "fixture@tcp(127.0.0.1:3306)/cognida", "true", "", "", "isolated cognida_review_"},
		{"missing explicit isolation", "fixture@tcp(127.0.0.1:3306)/cognida_review_guard", "", "", "", "ISOLATED=true"},
		{"remote API", "fixture@tcp(127.0.0.1:3306)/cognida_review_guard", "true", "https://production.example", "", "GO_SERVICE_URL must use loopback"},
		{"remote Redis", "fixture@tcp(127.0.0.1:3306)/cognida_review_guard", "true", "http://127.0.0.1:18588", "production.example:6379", "REDIS_ADDR must use loopback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MYSQL_DSN", tc.dsn)
			t.Setenv("ISOLATED", tc.isolated)
			t.Setenv("GO_SERVICE_URL", tc.api)
			t.Setenv("REDIS_ADDR", tc.redis)
			for _, cleanup := range []bool{false, true} {
				require.ErrorContains(t, run(t.TempDir(), false, cleanup), tc.message)
			}
		})
	}
}
