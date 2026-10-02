package fetcher

import (
	"io"
	"net/http"
	"sync"
)

// Permits cover each actual HTTP hop, including redirects, until its body is
// closed. net/http closes redirect bodies before requesting the next hop.
type gateTransport struct {
	delegate http.RoundTripper
	gate     RequestGate
}

func (t gateTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	release, err := t.gate.Acquire(request.Context(), request.URL.String())
	if err != nil {
		return nil, err
	}
	response, err := t.delegate.RoundTrip(request)
	if err != nil {
		release()
		return nil, err
	}
	if response.Body == nil {
		release()
		return response, nil
	}
	response.Body = &gateBody{ReadCloser: response.Body, release: release}
	return response, nil
}

type gateBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *gateBody) Close() error { err := b.ReadCloser.Close(); b.once.Do(b.release); return err }
