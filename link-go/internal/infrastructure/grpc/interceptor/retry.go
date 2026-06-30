package interceptor

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryConfig 重试配置
type RetryConfig struct {
	Max        int
	Backoff    time.Duration
	Multiplier float64
	MaxBackoff time.Duration
	Retryable  func(error) bool
}

func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		Max:        3,
		Backoff:    100 * time.Millisecond,
		Multiplier: 2.0,
		MaxBackoff: 3 * time.Second,
		Retryable:  IsRetryable,
	}
}

func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.DeadlineExceeded, codes.Canceled, codes.Unavailable, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}

func RetryUnary(cfg *RetryConfig) grpc.UnaryClientInterceptor {
	if cfg == nil {
		cfg = DefaultRetryConfig()
	}
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lastErr error
		backoff := cfg.Backoff
		for attempt := 0; attempt <= cfg.Max; attempt++ {
			if attempt > 0 {
				time.Sleep(backoff)
				backoff = time.Duration(float64(backoff) * cfg.Multiplier)
				if backoff > cfg.MaxBackoff {
					backoff = cfg.MaxBackoff
				}
			}
			lastErr = invoker(ctx, method, req, reply, cc, opts...)
			if lastErr == nil {
				return nil
			}
			if !cfg.Retryable(lastErr) {
				return lastErr
			}
		}
		return lastErr
	}
}

func RetryStream(cfg *RetryConfig) grpc.StreamClientInterceptor {
	if cfg == nil {
		cfg = DefaultRetryConfig()
	}
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		var lastErr error
		backoff := cfg.Backoff
		for attempt := 0; attempt <= cfg.Max; attempt++ {
			if attempt > 0 {
				time.Sleep(backoff)
				backoff = time.Duration(float64(backoff) * cfg.Multiplier)
				if backoff > cfg.MaxBackoff {
					backoff = cfg.MaxBackoff
				}
			}
			stream, err := streamer(ctx, desc, cc, method, opts...)
			if err == nil {
				return stream, nil
			}
			lastErr = err
			if !cfg.Retryable(err) {
				return nil, lastErr
			}
		}
		return nil, lastErr
	}
}
