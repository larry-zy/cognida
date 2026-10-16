package datasource

import (
	"encoding/json"
	"testing"

	m "cognida/internal/model/datasource"
	"github.com/stretchr/testify/require"
)

func TestHiveIdentityAndSanitizedOptions(t *testing.T) {
	in := SaveInput{Name: "Hive", Type: "hive", Host: "localhost", Port: 10000, DatabaseName: "samples", Username: "analyst", Extra: json.RawMessage(`{"auth":"NONE","tls":false}`)}
	require.NoError(t, in.validate(true), "NONE uses an explicit identity without a dummy password")
	in.Extra = json.RawMessage(`{"auth":"LDAP","tls":false}`)
	in.Password = "private"
	require.Error(t, in.validate(true))
	in.Extra = json.RawMessage(`{"auth":"LDAP","tls":true}`)
	require.NoError(t, in.validate(true))
	in.Password = ""
	require.Error(t, in.validate(true))
	in.Extra = json.RawMessage(`{"auth":"KERBEROS","tls":true}`)
	require.Error(t, in.validate(true))
	out := sanitize(&m.DataSource{Type: m.TypeHive, Extra: json.RawMessage(`{"auth":"LDAP","tls":true,"password":"private","token":"secret"}`), PasswordEncrypted: "ciphertext"})
	body, err := json.Marshal(out)
	require.NoError(t, err)
	require.JSONEq(t, `{"auth":"LDAP","tls":true}`, string(out.Extra))
	require.NotContains(t, string(body), "private")
	require.NotContains(t, string(body), "secret")
	require.NotContains(t, string(body), "ciphertext")
}
