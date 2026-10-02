// Package fetcher provides the collection bounded context with a safe, small
// HTTPS fetch primitive. It deliberately has no dependency on Agent tools.
package fetcher

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxBytes     int64 = 2 << 20 // 2 MiB
	defaultTimeout            = 20 * time.Second
	defaultMaxRedirects       = 5
	defaultUserAgent          = "CognidaCollector/1.0 (+https://github.com/larry-zy/cognida)"
)

var (
	ErrInvalidURL        = errors.New("invalid collection URL")
	ErrUnsafeDestination = errors.New("unsafe fetch destination")
	ErrUnsupportedType   = errors.New("unsupported response content type")
	ErrResponseTooLarge  = errors.New("response exceeds configured size limit")
	ErrUnexpectedStatus  = errors.New("unexpected HTTP response status")
	sharedAddressSpace   = netip.MustParsePrefix("100.64.0.0/10")
)

// Resolver is intentionally small so DNS policy can be tested without a real
// resolver. The production default is net.DefaultResolver.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// Options configures a Fetcher. Zero values choose conservative defaults.
// Transport is intended for controlled tests or specialised deployments; a nil
// transport installs the validating production transport.
type RequestGate interface {
	Acquire(context.Context, string) (func(), error)
}

type Options struct {
	Gate         RequestGate
	Resolver     Resolver
	Transport    http.RoundTripper
	TLSConfig    *tls.Config
	Timeout      time.Duration
	MaxBytes     int64
	MaxRedirects int
	UserAgent    string
}

// Result contains only the bounded, accepted textual response.
type Result struct {
	URL         string
	StatusCode  int
	ContentType string
	Header      http.Header
	Body        []byte
}

// Fetcher fetches only public HTTPS destinations. Its default transport
// resolves and validates the address again at dial time, preventing a DNS
// answer from changing between validation and connection.
type Fetcher struct {
	client    *http.Client
	resolver  Resolver
	maxBytes  int64
	userAgent string
}

// New creates an independently usable collection fetcher.
func New(opts Options) *Fetcher {
	resolver := opts.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxRedirects := opts.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = defaultMaxRedirects
	}
	userAgent := opts.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	f := &Fetcher{resolver: resolver, maxBytes: maxBytes, userAgent: userAgent}
	transport := opts.Transport
	if transport == nil {
		transport = f.safeTransport(opts.TLSConfig)
	}
	if opts.Gate != nil {
		transport = gateTransport{delegate: transport, gate: opts.Gate}
	}
	f.client = &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("%w: too many redirects", ErrInvalidURL)
			}
			_, err := f.validateURL(req.Context(), req.URL.String())
			return err
		},
	}
	return f
}

// NormalizeURL parses a user-supplied URL into its canonical fetch form. It
// accepts HTTPS only, strips fragments, lowercases the host, and adds a root
// path when absent. It does not perform network access.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Opaque != "" {
		return "", fmt.Errorf("%w: HTTPS absolute URL required", ErrInvalidURL)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("%w: hostname required", ErrInvalidURL)
	}
	port := u.Port()
	if port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("%w: invalid port", ErrInvalidURL)
		}
	}

	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return "", fmt.Errorf("%w: hostname required", ErrInvalidURL)
	}
	u.Scheme = "https"
	u.Host = host
	if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	}
	u.Fragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

// Fetch validates the initial destination, follows only validated redirects,
// and returns a bounded HTML or plain-text response.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (*Result, error) {
	normalized, err := f.validateURL(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalized, nil)
	if err != nil {
		return nil, fmt.Errorf("create fetch request: %w", err)
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html, text/plain;q=0.9")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", normalized, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: %s", ErrUnexpectedStatus, resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if mediaType != "text/html" && mediaType != "text/plain" {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, contentType)
	}
	if resp.ContentLength > f.maxBytes {
		return nil, fmt.Errorf("%w: declared %d bytes (limit %d)", ErrResponseTooLarge, resp.ContentLength, f.maxBytes)
	}
	body, err := readBounded(resp.Body, f.maxBytes)
	if err != nil {
		return nil, err
	}
	return &Result{
		URL:         resp.Request.URL.String(),
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		Header:      resp.Header.Clone(),
		Body:        body,
	}, nil
}

func (f *Fetcher) validateURL(ctx context.Context, raw string) (string, error) {
	normalized, err := NormalizeURL(raw)
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(normalized)
	if err := validateHost(ctx, f.resolver, u.Hostname()); err != nil {
		return "", err
	}
	return normalized, nil
}

func (f *Fetcher) safeTransport(tlsConfig *tls.Config) *http.Transport {
	dialer := &net.Dialer{}
	return &http.Transport{
		Proxy:                 nil, // environment proxies can bypass destination policy
		TLSClientConfig:       tlsConfig,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("split dial address: %w", err)
			}
			ips, err := approvedIPs(ctx, f.resolver, host)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, fmt.Errorf("dial approved destination: %w", lastErr)
		},
	}
}

func readBounded(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: limit %d", ErrResponseTooLarge, limit)
	}
	return body, nil
}

func validateHost(ctx context.Context, resolver Resolver, host string) error {
	_, err := approvedIPs(ctx, resolver, host)
	return err
}

func approvedIPs(ctx context.Context, resolver Resolver, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return nil, fmt.Errorf("%w: %s", ErrUnsafeDestination, ip)
		}
		return []net.IP{ip}, nil
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("%w: %s resolved without addresses", ErrUnsafeDestination, host)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if !isPublicIP(addr.IP) {
			return nil, fmt.Errorf("%w: %s resolves to %s", ErrUnsafeDestination, host, addr.IP)
		}
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

func isPublicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok || !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	// RFC 6598 shared address space is not publicly routable.
	return !sharedAddressSpace.Contains(addr)
}
