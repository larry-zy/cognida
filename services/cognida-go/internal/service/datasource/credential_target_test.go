package datasource

import (
	m "cognida/internal/model/datasource"
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// This driver captures configuration before any socket could be opened.
type targetCaptureDriver struct {
	mysqlDriver
	calls    int
	target   *m.DataSource
	password string
}

func (d *targetCaptureDriver) BuildDSN(source *m.DataSource, password string) (string, error) {
	d.calls++
	copy := *source
	d.target, d.password = &copy, password
	return "synthetic-only", nil
}
func (*targetCaptureDriver) DriverName() string                  { return "cm_fake" }
func (*targetCaptureDriver) Ping(context.Context, *sql.DB) error { return nil }

func TestStoredCredentialCannotMoveToCallerTarget(t *testing.T) {
	s, repo := newTestService(t)
	ctx := context.Background()
	in := validInput("protected")
	in.Extra = json.RawMessage(`{"charset":"utf8mb4","tls":true}`)
	created, err := s.Create(ctx, 1, 10, in)
	require.NoError(t, err)
	original, err := repo.Get(ctx, created.ID, 1)
	require.NoError(t, err)
	capture := &targetCaptureDriver{}
	prior := drivers[m.TypeMySQL]
	drivers[m.TypeMySQL] = capture
	t.Cleanup(func() { drivers[m.TypeMySQL] = prior })

	for _, mutate := range []struct {
		name   string
		change func(*SaveInput)
	}{
		{"host", func(v *SaveInput) { v.Host = "attacker.invalid" }},
		{"port", func(v *SaveInput) { v.Port++ }},
		{"type", func(v *SaveInput) { v.Type = "postgres" }},
		{"database", func(v *SaveInput) { v.DatabaseName = "other" }},
		{"username", func(v *SaveInput) { v.Username = "other" }},
		{"tls", func(v *SaveInput) { v.Extra = json.RawMessage(`{"charset":"utf8mb4","tls":false}`) }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			changed := *in
			changed.Password = ""
			mutate.change(&changed)
			require.ErrorIs(t, s.TestConnection(ctx, 1, &TestInput{DatasourceID: created.ID, SaveInput: changed}), ErrCredentialTargetChanged)
			_, err := s.Update(ctx, 1, created.ID, &changed)
			require.ErrorIs(t, err, ErrCredentialTargetChanged)
			after, err := repo.Get(ctx, created.ID, 1)
			require.NoError(t, err)
			require.Equal(t, original, after)
		})
	}
	require.Zero(t, capture.calls, "rejected target changes must not reach a driver")
	require.ErrorIs(t, s.TestConnection(ctx, 1, &TestInput{DatasourceID: created.ID, SaveInput: SaveInput{Type: "postgres"}}), ErrCredentialTargetChanged)
	require.ErrorIs(t, s.TestConnection(ctx, 2, &TestInput{DatasourceID: created.ID}), ErrNotFound)

	allowed := *in
	allowed.Password = ""
	allowed.Name, allowed.Description = "renamed", "new display description"
	allowed.Extra = json.RawMessage(`{ "tls": true, "charset": "utf8mb4" }`)
	require.NoError(t, s.TestConnection(ctx, 1, &TestInput{DatasourceID: created.ID, SaveInput: allowed}))
	require.Equal(t, original.Host, capture.target.Host)
	require.Equal(t, in.Password, capture.password)
	require.NoError(t, s.TestConnection(ctx, 1, &TestInput{DatasourceID: created.ID}))
	require.Equal(t, 2, capture.calls)

	// An explicit new credential may target a new endpoint, never the stored one.
	allowed.Host, allowed.Password = "new.invalid", "synthetic-new-password"
	require.NoError(t, s.TestConnection(ctx, 1, &TestInput{DatasourceID: created.ID, SaveInput: allowed}))
	require.Equal(t, allowed.Password, capture.password)
	_, err = s.Update(ctx, 1, created.ID, &allowed)
	require.NoError(t, err)
}
