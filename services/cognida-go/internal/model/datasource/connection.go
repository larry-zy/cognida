package datasource

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"
)

func connectionDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// CanonicalConnectionExtra retains numeric precision and normalizes SQL JSON
// key order. It accepts only one configuration object, never trailing data.
func CanonicalConnectionExtra(raw []byte) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return []byte("{}"), nil
	}
	var object map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&object); err != nil {
		return nil, err
	}
	var tail any
	if err := dec.Decode(&tail); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("connection configuration contains trailing JSON")
	}
	return json.Marshal(object)
}

// QueryConnectionVersion binds the executable connection, not presentation
// metadata or health-check timestamps. Credential changes always change it.
func QueryConnectionVersion(s *DataSource) string {
	extra, err := CanonicalConnectionExtra(s.Extra)
	if err != nil {
		extra = s.Extra
	}
	return "connection-v2:" + connectionDigest(struct {
		ID                           string
		Tenant                       int64
		Type                         Type
		Host                         string
		Port                         int
		Database, Username, Password string
		Extra                        []byte
	}{s.ID, s.TenantID, s.Type, s.Host, s.Port, s.DatabaseName, s.Username, s.PasswordEncrypted, extra})
}

// LegacyQueryConnectionVersion is only for reading pre-v2 receipts. A changed
// legacy configuration cannot be reconstructed from a hash and stays denied.
func LegacyQueryConnectionVersion(s *DataSource) string {
	return connectionDigest(struct {
		ID                           string
		Tenant                       int64
		Type                         Type
		Host                         string
		Port                         int
		Database, Username, Password string
		Extra                        []byte
		Updated                      time.Time
	}{s.ID, s.TenantID, s.Type, s.Host, s.Port, s.DatabaseName, s.Username, s.PasswordEncrypted, s.Extra, s.UpdatedAt})
}

func MatchesQueryConnectionVersion(s *DataSource, version string) bool {
	return s != nil && (version == QueryConnectionVersion(s) || version == LegacyQueryConnectionVersion(s))
}

// QuerySourceVersionBinder preserves the remote job namespace of older durable
// receipts. It is called with a trusted persisted version before remote I/O.
type QuerySourceVersionBinder interface{ BindQuerySourceVersion(string) error }
