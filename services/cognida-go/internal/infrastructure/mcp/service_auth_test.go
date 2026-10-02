package mcp

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestServiceAuthenticationHeaders(t *testing.T) {
	transport := &headerRecordingClient{response: []byte(`{"jsonrpc":"2.0","id":"1","result":{"tools":[]}}`)}
	client, err := NewMCPClient(&Config{Endpoint: "http://internal", AuthToken: "service-secret", HTTPClient: transport})
	require.NoError(t, err)
	_, err = client.postMCP("http://internal/mcp", nil, "request-1")
	require.NoError(t, err)
	require.Equal(t, "Bearer service-secret", transport.lastHeaders["Authorization"])
	require.Equal(t, "request-1", transport.lastHeaders[HTTPRequestIDHeader])

	client.client.httpClient = &plainClient{}
	_, err = client.postMCP("http://internal/mcp", nil, "")
	require.Error(t, err, "configured authentication must never silently disappear")
}
