package sandbox_test

import (
	"archive/tar"
	"bytes"
	svc "cognida/internal/service/sandbox"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCollectorInspectsActualTarBytes(t *testing.T) {
	v := validated(t)
	for _, tc := range []struct {
		name   string
		header tar.Header
		body   string
		ok     bool
	}{
		{"regular", tar.Header{Name: "result.json", Mode: 0644, Typeflag: tar.TypeReg}, `{"kind":"table","columns":["n"],"rows":[["9007199254740993.1"]]}`, true},
		{"traversal", tar.Header{Name: "../result.json", Typeflag: tar.TypeReg}, `{}`, false},
		{"symlink", tar.Header{Name: "result.json", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}, "", false},
		{"hardlink", tar.Header{Name: "result.json", Typeflag: tar.TypeLink, Linkname: "code"}, "", false},
		{"fifo", tar.Header{Name: "pipe.json", Typeflag: tar.TypeFifo}, "", false},
		{"archive-disguise", tar.Header{Name: "payload.json", Typeflag: tar.TypeReg}, "PK\x00binary", false},
		{"archive", tar.Header{Name: "payload.zip", Typeflag: tar.TypeReg}, "payload", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			w := tar.NewWriter(&b)
			tc.header.Size = int64(len(tc.body))
			require.NoError(t, w.WriteHeader(&tc.header))
			_, e := w.Write([]byte(tc.body))
			require.NoError(t, e)
			require.NoError(t, w.Close())
			m, files, e := svc.CollectArchive(v, b.Bytes())
			if !tc.ok {
				require.Error(t, e)
				return
			}
			require.NoError(t, e)
			require.Len(t, files, 1)
			require.Len(t, m.Artifacts, 1)
			require.Equal(t, []byte(tc.body), files[0].Body)
		})
	}
}

func TestCollectorAcceptsLegacyNULRegularFile(t *testing.T) {
	body := []byte(`{"kind":"table","columns":["n"],"rows":[[1]]}`)
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "result.json", Mode: 0644, Typeflag: tar.TypeReg, Size: int64(len(body))}))
	_, err := writer.Write(body)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	// Writer normalizes legacy NUL headers, so patch the actual wire header and checksum.
	raw := archive.Bytes()
	raw[156] = 0
	for i := 148; i < 156; i++ {
		raw[i] = ' '
	}
	checksum := 0
	for _, value := range raw[:512] {
		checksum += int(value)
	}
	copy(raw[148:156], fmt.Sprintf("%06o\x00 ", checksum))
	manifest, files, err := svc.CollectArchive(validated(t), raw)
	require.NoError(t, err)
	require.Len(t, manifest.Artifacts, 1)
	require.Len(t, files, 1)
	require.Equal(t, body, files[0].Body)
}
