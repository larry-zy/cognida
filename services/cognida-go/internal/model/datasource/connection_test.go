package datasource

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestQueryConnectionVersionIgnoresDisplayButBindsCredentials(t *testing.T) {
	original := DataSource{ID: "source", TenantID: 7, Type: TypeHive, Host: "db", Port: 10000, DatabaseName: "default", Username: "readonly", PasswordEncrypted: "ciphertext", Extra: []byte(`{"auth":"NONE","tls":false}`), UpdatedAt: time.Now()}
	version := QueryConnectionVersion(&original)
	display := original
	display.Name, display.Description = "new name", "new description"
	display.UpdatedAt = original.UpdatedAt.Add(time.Hour)
	display.Extra = []byte(`{"tls":false,"auth":"NONE"}`)
	require.Equal(t, version, QueryConnectionVersion(&display))
	require.True(t, MatchesQueryConnectionVersion(&original, LegacyQueryConnectionVersion(&original)))
	require.False(t, MatchesQueryConnectionVersion(&display, LegacyQueryConnectionVersion(&original)), "legacy hash cannot prove an unknown prior connection")
	for _, change := range []func(*DataSource){
		func(v *DataSource) { v.ID = "other" }, func(v *DataSource) { v.TenantID++ }, func(v *DataSource) { v.Type = TypeMySQL },
		func(v *DataSource) { v.Host = "other" }, func(v *DataSource) { v.Port++ }, func(v *DataSource) { v.DatabaseName = "other" },
		func(v *DataSource) { v.Username = "other" }, func(v *DataSource) { v.PasswordEncrypted = "rotated" }, func(v *DataSource) { v.Extra = []byte(`{"tls":true}`) },
	} {
		next := original
		change(&next)
		require.NotEqual(t, version, QueryConnectionVersion(&next))
	}
}
