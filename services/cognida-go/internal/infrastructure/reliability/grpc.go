package reliability

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// countableCode 判断某 gRPC 状态码是否应计入熔断失败——即代表「目标不健康」而非
// 「调用方/请求本身」的问题。Unavailable/ResourceExhausted/DeadlineExceeded 记；
// InvalidArgument/NotFound/Canceled 等业务或调用方原因不记。
func countableCode(code codes.Code) bool {
	switch code {
	case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}

// ServiceConfigJSON 生成 gRPC 默认服务配置（透明重试）。RetryableStatusCodes 仅取
// UNAVAILABLE/RESOURCE_EXHAUSTED——这两类可安全透明重试；DEADLINE_EXCEEDED 不列入
// （截止时间通常共享，重试无益且可能放大负载）。配合 grpc.WithDefaultServiceConfig 使用。
//
// gRPC 透明重试位于连接层、熔断拦截器之下：短暂抖动由重试吸收，持续故障由熔断拦截器
// 在多次失败回执后打开、快速失败——二者互补不冲突。
func ServiceConfigJSON(cfg Config) string {
	cfg = cfg.WithDefaults()
	return fmt.Sprintf(`{
  "methodConfig": [{
    "name": [{}],
    "retryPolicy": {
      "MaxAttempts": %d,
      "InitialBackoff": "%s",
      "MaxBackoff": "%s",
      "BackoffMultiplier": 2.0,
      "RetryableStatusCodes": ["UNAVAILABLE", "RESOURCE_EXHAUSTED"]
    }
  }]
}`, cfg.MaxAttempts, durSeconds(cfg.BaseBackoff), durSeconds(cfg.MaxBackoff))
}

// durSeconds 以 gRPC 服务配置要求的秒制 duration 字符串（如 "0.200s"）格式化。
func durSeconds(d time.Duration) string {
	return fmt.Sprintf("%.3fs", d.Seconds())
}

// UnaryClientInterceptor 返回一个按连接 target 熔断的一元客户端拦截器。open 时快速失败
// 返回 codes.Unavailable，不再打网络。放行的调用按状态码回执给熔断器。
func UnaryClientInterceptor(reg *Registry) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		b := reg.Get(cc.Target())
		if !b.Allow() {
			return status.Errorf(codes.Unavailable, "circuit breaker open for target %q (method %s)", cc.Target(), method)
		}
		err := invoker(ctx, method, req, reply, cc, opts...)
		recordResult(b, ctx, err)
		return err
	}
}

// StreamClientInterceptor 返回按连接 target 熔断的流式客户端拦截器。以「能否建立流」
// 作为健康信号：open 时快速失败；建流成功记成功，建流失败按状态码回执。
func StreamClientInterceptor(reg *Registry) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		b := reg.Get(cc.Target())
		if !b.Allow() {
			return nil, status.Errorf(codes.Unavailable, "circuit breaker open for target %q (stream %s)", cc.Target(), method)
		}
		cs, err := streamer(ctx, desc, cc, method, opts...)
		recordResult(b, ctx, err)
		return cs, err
	}
}

// recordResult 把一次调用结果回执给熔断器：区分成功、调用方取消（不计）、目标不健康（计）。
func recordResult(b *Breaker, ctx context.Context, err error) {
	if err == nil {
		b.OnResult(true, false)
		return
	}
	code := status.Code(err)
	// 调用方主动取消不能归因于目标，不计入熔断。
	if code == codes.Canceled || errors.Is(ctx.Err(), context.Canceled) {
		b.OnResult(false, false)
		return
	}
	b.OnResult(false, countableCode(code))
}
