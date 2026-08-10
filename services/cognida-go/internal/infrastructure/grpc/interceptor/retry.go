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
	// 仅重试瞬时/资源类错误。Canceled 与 DeadlineExceeded 通常源自调用方 ctx 已取消/超时——
	// 此时重试既无意义又有害（调用方已放弃仍继续打上游），故不纳入重试集，交由 ctx 检查快速返回。
	case codes.Unavailable, codes.ResourceExhausted, codes.Aborted:
		return true
	default:
		return false
	}
}

// sleepCtx 在退避期间响应 ctx 取消：ctx 先结束则立即返回其错误，避免 time.Sleep 无视取消空等。
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
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
				if err := sleepCtx(ctx, backoff); err != nil {
					// 退避期间 ctx 取消：放弃重试，返回上次错误（若无则返回 ctx 错误）。
					if lastErr != nil {
						return lastErr
					}
					return status.FromContextError(err).Err()
				}
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
			// 上游可重试错误，但调用方 ctx 已终止：不再重试，直接返回。
			if ctx.Err() != nil {
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
				if err := sleepCtx(ctx, backoff); err != nil {
					if lastErr != nil {
						return nil, lastErr
					}
					return nil, status.FromContextError(err).Err()
				}
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
			if ctx.Err() != nil {
				return nil, lastErr
			}
		}
		return nil, lastErr
	}
}
