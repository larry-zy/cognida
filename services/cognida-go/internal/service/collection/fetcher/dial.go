package fetcher

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

const (
	connectTimeout = 10 * time.Second
	addressTimeout = 3 * time.Second
)

// dialApproved tries only addresses from one fully validated DNS answer. TLS
// belongs to connection establishment: a TCP endpoint that stalls its TLS
// handshake must not prevent another approved address from connecting. No HTTP
// request bytes are sent here, and no HTTP response failure is retried.
func dialApproved(parent context.Context, resolver Resolver, dial func(context.Context, string, string) (net.Conn, error), network, address string, config *tls.Config) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(parent, connectTimeout)
	defer cancel()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split dial address: %w", err)
	}
	ips, err := approvedIPs(ctx, resolver, host)
	if err != nil {
		return nil, err
	}
	// Deduplicate only after validating every answer, including mixed public /
	// private answers. Never resolve the hostname again through the dialer.
	unique := make([]net.IP, 0, len(ips))
	seen := make(map[string]bool, len(ips))
	for _, ip := range ips {
		if !seen[ip.String()] {
			seen[ip.String()] = true
			unique = append(unique, ip)
		}
	}
	deadline, _ := ctx.Deadline()
	var lastErr error
	for i, ip := range unique {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Share the remaining connection budget with every remaining address;
		// neither a blackholed TCP endpoint nor TLS can consume the entire budget.
		budget := time.Until(deadline) / time.Duration(len(unique)-i)
		if budget > addressTimeout {
			budget = addressTimeout
		}
		attempt, stop := context.WithTimeout(ctx, budget)
		conn, err := dial(attempt, network, net.JoinHostPort(ip.String(), port))
		if err == nil && config != nil {
			cfg := config.Clone()
			if cfg.ServerName == "" {
				cfg.ServerName = host
			}
			secure := tls.Client(conn, cfg)
			err = secure.HandshakeContext(attempt)
			conn = secure
		}
		if err == nil {
			err = attempt.Err()
		}
		stop()
		if err == nil {
			return conn, nil
		}
		if conn != nil {
			_ = conn.Close()
		}
		lastErr = err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("connect approved destination: %w", lastErr)
}
