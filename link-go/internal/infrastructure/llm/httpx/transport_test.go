package httpx

import (
	"net/http"
	"testing"
	"time"
)

// TestNewTransportTuned 校验调优后的 transport 关键字段，尤其是 MaxIdleConnsPerHost
// 必须显著高于 Go 默认值(2)，否则并发 LLM 调用会持续重建连接。
func TestNewTransportTuned(t *testing.T) {
	tr := newTransport()

	if tr.MaxIdleConnsPerHost <= http.DefaultMaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost=%d, 必须大于默认值 %d",
			tr.MaxIdleConnsPerHost, http.DefaultMaxIdleConnsPerHost)
	}
	if tr.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns=%d, want 100", tr.MaxIdleConns)
	}
	if tr.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("TLSHandshakeTimeout=%v, want 10s", tr.TLSHandshakeTimeout)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout=%v, want 90s", tr.IdleConnTimeout)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 应为 true")
	}
	if tr.DialContext == nil {
		t.Error("DialContext 不应为 nil（需要拨号阶段超时）")
	}
	if tr.Proxy == nil {
		t.Error("Proxy 应为 ProxyFromEnvironment 而非 nil")
	}
}

// TestNewTransportNoOverallTimeout 回归保护：transport 不得引入会截断 SSE 流式响应
// 的整体超时。ResponseHeaderTimeout 若被设置，会在慢速生成时误杀长连接。
func TestNewTransportNoOverallTimeout(t *testing.T) {
	tr := newTransport()
	if tr.ResponseHeaderTimeout != 0 {
		t.Errorf("ResponseHeaderTimeout=%v, 必须为 0 以避免截断流式响应", tr.ResponseHeaderTimeout)
	}
}

// TestSharedTransportSingleton 校验进程级共享单例：多次调用返回同一指针，
// 从而所有客户端复用同一个连接池。
func TestSharedTransportSingleton(t *testing.T) {
	a := SharedTransport()
	b := SharedTransport()
	if a != b {
		t.Error("SharedTransport 应返回同一单例指针")
	}
	if a == nil {
		t.Fatal("SharedTransport 不应返回 nil")
	}
}
