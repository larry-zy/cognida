// Package httpx 为出站 LLM / Embedding 调用提供共享的、经过调优的 HTTP 传输层。
//
// 各 provider 客户端原先各自使用 &http.Client{}（即 http.DefaultTransport），
// 其 MaxIdleConnsPerHost 默认仅为 2。在「高并发打同一个 LLM 网关」的场景下，
// 超过 2 条的并发连接在请求结束后会被立即关闭而非放回连接池，导致持续的
// TCP/TLS 握手风暴，既增加延迟也加重对端压力。本包提供一个进程级共享的
// *http.Transport 来消除这一问题。
package httpx

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	// dialTimeout 限制建立 TCP 连接的耗时（拨号阶段），与整体/读取超时无关。
	dialTimeout = 10 * time.Second
	// tlsHandshakeTimeout 限制 TLS 握手耗时。
	tlsHandshakeTimeout = 10 * time.Second
	// idleConnTimeout 空闲连接在被关闭前的最大保活时长。
	idleConnTimeout = 90 * time.Second
	// keepAlive 保活探测间隔。
	keepAlive = 30 * time.Second
	// maxIdleConns 整个连接池允许的空闲连接总数。
	maxIdleConns = 100
	// maxIdleConnsPerHost 单个 host 允许的空闲连接数。默认值(2)对并发 LLM
	// 调用过小，提高后并发请求可复用 keep-alive 连接而非反复重建。
	maxIdleConnsPerHost = 32
)

var (
	sharedOnce      sync.Once
	sharedTransport *http.Transport
)

// SharedTransport 返回进程级共享的、经过调优的 *http.Transport。
//
// 所有出站 LLM / Embedding 客户端共用同一个连接池，从而最大化 keep-alive
// 连接复用。该 transport 只设置「连接阶段」超时（拨号 / TLS 握手），不设整体
// 超时——整体超时由调用方通过 http.Client.Timeout 或 context 控制，因此不会
// 截断长时间运行的流式（SSE）响应。
func SharedTransport() *http.Transport {
	sharedOnce.Do(func() {
		sharedTransport = newTransport()
	})
	return sharedTransport
}

// newTransport 构造一个调优后的 transport。抽出为纯函数以便单元测试无需依赖单例。
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: keepAlive,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          maxIdleConns,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		IdleConnTimeout:       idleConnTimeout,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
