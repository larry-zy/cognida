package datasource

import (
	m "cognida/internal/model/datasource"
	"cognida/internal/service/datasource/hivewire"
	h "github.com/beltran/gohive/v2/hiveserver"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAsyncHandlesSurviveDisplayChangesButNotConnectionChanges(t *testing.T) {
	original := m.DataSource{ID: "source", TenantID: 7, Type: m.TypeHive, Host: "db", Port: 10000, DatabaseName: "default", Username: "user", PasswordEncrypted: "cipher", UpdatedAt: time.Now().UTC()}
	version := m.QueryConnectionVersion(&original)
	cipher, err := NewCipher("synthetic-handle-only")
	require.NoError(t, err)
	a := &hiveAsyncAdapter{sqlQueryAdapter: &sqlQueryAdapter{source: &original}, cipher: cipher}
	envelope := &hiveEnvelope{Tenant: 7, Source: "source", Version: version, MaxRows: 10, MaxBytes: 2048, Handles: &hivewire.Handles{Session: &h.TSessionHandle{}}}
	id, err := a.seal(envelope)
	require.NoError(t, err)
	changed := original
	changed.Name = "renamed"
	changed.UpdatedAt = original.UpdatedAt.Add(time.Minute)
	a.source = &changed
	_, err = a.open(id)
	require.NoError(t, err)
	changed.Host = "different"
	_, err = a.open(id)
	require.Error(t, err)
	// Old sealed sessions are accepted only while their old binding is provable.
	a.source = &original
	envelope.Version = original.UpdatedAt.UTC().Format(time.RFC3339Nano)
	legacy, err := a.seal(envelope)
	require.NoError(t, err)
	_, err = a.open(legacy)
	require.NoError(t, err)
	a.source = &changed
	_, err = a.open(legacy)
	require.Error(t, err)

	original.Type = m.TypeBigQuery
	changed = original
	changed.Name = "renamed"
	changed.UpdatedAt = original.UpdatedAt.Add(time.Minute)
	b := &bigQueryAdapter{source: &original}
	require.NoError(t, b.BindQuerySourceVersion(m.QueryConnectionVersion(&original)))
	prefix := b.prefix
	b.source = &changed
	require.NoError(t, b.BindQuerySourceVersion(m.QueryConnectionVersion(&original)))
	require.Equal(t, prefix, b.prefix)
	changed.PasswordEncrypted = "rotated"
	require.Error(t, b.BindQuerySourceVersion(m.QueryConnectionVersion(&original)))
	b.source = &original
	require.NoError(t, b.BindQuerySourceVersion(m.LegacyQueryConnectionVersion(&original)))
	require.Equal(t, bigQueryPrefix(&original, true), b.prefix, "old unknown submissions must be looked up in their original namespace")
}
