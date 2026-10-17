package datasource

import (
	"bytes"
	m "cognida/internal/model/datasource"
)

func inputSource(in *SaveInput) *m.DataSource {
	return &m.DataSource{Type: m.Type(in.Type), Host: in.Host, Port: in.Port, DatabaseName: in.DatabaseName, Username: in.Username, Extra: in.Extra}
}

func sameCredentialTarget(a, b *m.DataSource) bool {
	if a.Type != b.Type || a.Host != b.Host || a.Port != b.Port || a.DatabaseName != b.DatabaseName || a.Username != b.Username {
		return false
	}
	x, e := m.CanonicalConnectionExtra(a.Extra)
	if e != nil {
		return false
	}
	y, e := m.CanonicalConnectionExtra(b.Extra)
	return e == nil && bytes.Equal(x, y)
}
