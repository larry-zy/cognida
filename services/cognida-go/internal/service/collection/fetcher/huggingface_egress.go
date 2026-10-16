package fetcher

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// HuggingFaceEgressConfig is operator configuration, never an HTTP/tool input.
// Empty preserves direct networking. The only alternative is an explicitly
// trusted local SOCKS5 service; it never receives an unresolved target hostname.
type HuggingFaceEgressConfig struct {
	SOCKS5Address string
}

var (
	ErrHuggingFaceEgress = errors.New("invalid Hugging Face operator egress configuration")
	errHFResolution      = errors.New("Hugging Face trusted DNS resolution failed")
)

const hfDoHEndpoint = "https://cloudflare-dns.com/dns-query"
const hfDNSBudget = 8 * time.Second

// NewHuggingFaceOperatorFetcher changes only the dedicated HF client. Standard
// proxy environment variables, arbitrary DoH URLs and caller-supplied TLS roots
// are deliberately not configuration options.
func NewHuggingFaceOperatorFetcher(cfg HuggingFaceEgressConfig) (*Fetcher, error) {
	if cfg.SOCKS5Address == "" {
		return New(Options{}), nil
	}
	dial, err := newHFNumericSOCKSDial(cfg.SOCKS5Address)
	if err != nil {
		return nil, err
	}
	return newHFOperatorFetcher(dial, nil), nil
}

// The TLS config seam is package-private for real local TLS tests. Production
// always uses platform certificate verification and hostname verification.
func newHFOperatorFetcher(dial func(context.Context, string, string) (net.Conn, error), cfg *tls.Config) *Fetcher {
	resolver := newHFDoHResolver(dial, cfg)
	return New(Options{Resolver: resolver, DialContext: dial, TLSConfig: cfg})
}

func newHFNumericSOCKSDial(address string) (func(context.Context, string, string) (net.Conn, error), error) {
	host, port, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	p, portErr := strconv.Atoi(port)
	if err != nil || ip == nil || !ip.IsLoopback() || portErr != nil || p < 1 || p > 65535 || strconv.Itoa(p) != port {
		return nil, ErrHuggingFaceEgress
	}
	d, err := proxy.SOCKS5("tcp", net.JoinHostPort(ip.String(), port), nil, &net.Dialer{Timeout: connectTimeout})
	if err != nil {
		return nil, ErrHuggingFaceEgress
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		return nil, ErrHuggingFaceEgress
	}
	return func(ctx context.Context, network, target string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(target)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !isPublicIP(ip) || port != "443" || (network != "tcp" && network != "tcp4" && network != "tcp6") {
			return nil, ErrUnsafeDestination
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		conn, err := cd.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errors.New("Hugging Face trusted egress connection failed")
		}
		return conn, nil
	}, nil
}

type hfDoHResolver struct{ client *http.Client }

type hfBootstrapResolver struct{}

func (hfBootstrapResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if host != "cloudflare-dns.com" {
		return nil, errHFResolution
	}
	// Operator-pinned addresses of the fixed DoH service, not HF/CDN addresses.
	return []net.IPAddr{{IP: net.ParseIP("1.1.1.1")}, {IP: net.ParseIP("1.0.0.1")}}, nil
}

func newHFDoHResolver(dial func(context.Context, string, string) (net.Conn, error), cfg *tls.Config) *hfDoHResolver {
	f := &Fetcher{resolver: hfBootstrapResolver{}}
	return &hfDoHResolver{client: &http.Client{
		Timeout:       hfDNSBudget,
		Transport:     f.safeTransport(cfg, dial),
		CheckRedirect: func(*http.Request, []*http.Request) error { return errHFResolution },
	}}
}

func hfAllowedHost(host string) bool {
	switch host {
	case "huggingface.co", "cdn-lfs.huggingface.co", "cdn-lfs-us-1.hf.co", "cdn-lfs-eu-1.hf.co", "cas-bridge.xethub.hf.co", "us.aws.cdn.hf.co":
		return true
	default:
		return false
	}
}

// LookupIPAddr completes both address families under one budget. Any incomplete
// or malformed answer fails closed; no TTL cache, stale result or local DNS
// fallback is used. Every address is checked before returning any candidate.
func (r *hfDoHResolver) LookupIPAddr(parent context.Context, host string) ([]net.IPAddr, error) {
	if !hfAllowedHost(host) {
		return nil, ErrUnsafeDestination
	}
	ctx, cancel := context.WithTimeout(parent, hfDNSBudget)
	defer cancel()
	var all []net.IPAddr
	for _, kind := range []int{1, 28} {
		ips, err := r.lookup(ctx, host, kind)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		all = append(all, ips...)
	}
	if len(all) == 0 || len(all) > 64 {
		return nil, errHFResolution
	}
	return all, nil
}

func (r *hfDoHResolver) lookup(ctx context.Context, host string, kind int) ([]net.IPAddr, error) {
	u := hfDoHEndpoint + "?" + url.Values{"name": {host}, "type": {strconv.Itoa(kind)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errHFResolution
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, errHFResolution
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > 64<<10 {
		return nil, errHFResolution
	}
	body, err := readBounded(resp.Body, 64<<10)
	if err != nil {
		return nil, errHFResolution
	}
	return decodeHFDoHAnswer(body, host, kind)
}

func decodeHFDoHAnswer(body []byte, host string, kind int) ([]net.IPAddr, error) {
	var answer struct {
		Status   *int
		TC       *bool
		Question []struct {
			Name string `json:"name"`
			Type int    `json:"type"`
		}
		Answer []struct {
			Name string `json:"name"`
			Type int    `json:"type"`
			Data string `json:"data"`
		}
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if dec.Decode(&answer) != nil || dec.Decode(new(any)) != io.EOF || answer.Status == nil || *answer.Status != 0 || answer.TC == nil || *answer.TC || len(answer.Question) != 1 || !strings.EqualFold(strings.TrimSuffix(answer.Question[0].Name, "."), host) || answer.Question[0].Type != kind || len(answer.Answer) > 64 {
		return nil, errHFResolution
	}
	var out []net.IPAddr
	for _, item := range answer.Answer {
		if item.Type == 5 { // CNAME is informational; never delegated to SOCKS DNS.
			if item.Name == "" || item.Data == "" || len(item.Data) > 254 {
				return nil, errHFResolution
			}
			continue
		}
		ip := net.ParseIP(item.Data)
		if item.Type != kind || ip == nil || (kind == 1) != (ip.To4() != nil) {
			return nil, errHFResolution
		}
		if !isPublicIP(ip) {
			return nil, ErrUnsafeDestination
		}
		out = append(out, net.IPAddr{IP: ip})
	}
	return out, nil
}
