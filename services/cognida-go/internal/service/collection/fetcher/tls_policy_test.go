package fetcher

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeTransportRequiresTLS12WithoutMutatingCaller(t *testing.T) {
	f := &Fetcher{}
	require.Equal(t, uint16(tls.VersionTLS12), f.safeTransport(nil, nil).TLSClientConfig.MinVersion)
	for _, version := range []uint16{0, tls.VersionTLS10, tls.VersionTLS12, tls.VersionTLS13} {
		config := &tls.Config{MinVersion: version} // #nosec G402 -- Test deliberately supplies an obsolete version to verify it is raised.
		transport := f.safeTransport(config, nil)
		require.Equal(t, max(version, tls.VersionTLS12), transport.TLSClientConfig.MinVersion)
		require.Equal(t, version, config.MinVersion, "request setup must not mutate shared caller configuration")
	}
}
