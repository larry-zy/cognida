package reliability

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// ServiceConfigJSON 生成 gRPC 默认服务配置（透明重试）。按幂等性分层设置可重试码集
// （〔M10〕，杜绝非幂等方法被盲目重试致重复副作用）：
//   - 默认（blanket，匹配所有方法）：仅 UNAVAILABLE。该码表示请求在连接层未送达/服务
//     未就绪，对任何方法（含非幂等写）重试都安全。
//   - cfg.RetryableMethods 显式声明的幂等方法：追加 RESOURCE_EXHAUSTED（该码可能在服务端
//     已开始处理、产生副作用后返回，故只对确认幂等的方法开放）。
//
// 两类均不含 DEADLINE_EXCEEDED（截止时间通常共享，重试无益且放大负载）。gRPC 透明重试
// 位于连接层、熔断拦截器之下：短暂抖动由重试吸收，持续故障由熔断拦截器打开后快速失败。
func ServiceConfigJSON(cfg Config) string {
	cfg = cfg.WithDefaults()

	// blanket：所有方法，仅 UNAVAILABLE（对任何方法安全）。
	methodConfigs := []string{retryMethodConfig(`[{}]`, cfg, `"UNAVAILABLE"`)}

	// 幂等白名单：追加 RESOURCE_EXHAUSTED。gRPC 服务配置「最长匹配优先」——精确 name
	// 覆盖 blanket，故白名单方法命中此条、其余仍走 blanket。
	if names := methodNames(cfg.RetryableMethods); names != "" {
		methodConfigs = append(methodConfigs,
			retryMethodConfig(names, cfg, `"UNAVAILABLE", "RESOURCE_EXHAUSTED"`))
	}

	return fmt.Sprintf("{\n  \"methodConfig\": [%s]\n}", strings.Join(methodConfigs, ", "))
}

// retryMethodConfig 组装单条 methodConfig 的 JSON（name 选择器 + retryPolicy）。
func retryMethodConfig(names string, cfg Config, retryCodes string) string {
	return fmt.Sprintf(`{
    "name": %s,
    "retryPolicy": {
      "MaxAttempts": %d,
      "InitialBackoff": "%s",
      "MaxBackoff": "%s",
      "BackoffMultiplier": 2.0,
      "RetryableStatusCodes": [%s]
    }
  }`, names, cfg.MaxAttempts, durSeconds(cfg.BaseBackoff), durSeconds(cfg.MaxBackoff), retryCodes)
}

// methodNames 把 "pkg.Service/Method" / "pkg.Service" 列表转成 gRPC 服务配置的 name
// 选择器 JSON 数组；空列表返回空串（调用方据此跳过白名单 methodConfig）。
func methodNames(methods []string) string {
	entries := make([]string, 0, len(methods))
	for _, m := range methods {
		m = strings.TrimPrefix(strings.TrimSpace(m), "/")
		if m == "" {
			continue
		}
		if svc, method, ok := strings.Cut(m, "/"); ok {
			entries = append(entries, fmt.Sprintf(`{"service": %q, "method": %q}`, svc, method))
		} else {
			entries = append(entries, fmt.Sprintf(`{"service": %q}`, m))
		}
	}
	if len(entries) == 0 {
		return ""
	}
	return "[" + strings.Join(entries, ", ") + "]"
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
