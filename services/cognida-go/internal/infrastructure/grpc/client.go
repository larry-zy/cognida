// Package grpc 提供统一的 gRPC 客户端管理
package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"cognida/internal/infrastructure/grpc/interceptor"
	"cognida/internal/infrastructure/reliability"
)

// Config gRPC 客户端配置
type Config struct {
	Target         string           // 目标地址
	Block          bool             // 是否阻塞等待连接
	Timeout        time.Duration    // 连接超时
	TLS            *TLSConfig       // TLS 配置
	Keepalive      *KeepaliveConfig // 保活配置
	MaxRecvMsgSize int              // 最大接收消息大小
	MaxSendMsgSize int              // 最大发送消息大小
	// Reliability 统一可靠性策略（透明重试 + per-target 熔断）。nil 表示采用默认
	// 开启配置（见 reliability.DefaultConfig）；如需关闭可传 &reliability.Config{Enabled:false}。
	// 所有走本基础客户端的通路（docreader、quality 等）由此获得一致的重试/熔断（〔X-4〕）。
	Reliability *reliability.Config
}

// TLSConfig TLS 配置
type TLSConfig struct {
	Enabled            bool
	ServerName         string
	InsecureSkipVerify bool
}

// KeepaliveConfig 保活配置
type KeepaliveConfig struct {
	Time                time.Duration
	Timeout             time.Duration
	PermitWithoutStream bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Timeout:        30 * time.Second,
		Block:          false,
		MaxRecvMsgSize: 100 * 1024 * 1024,
		MaxSendMsgSize: 100 * 1024 * 1024,
		TLS:            &TLSConfig{Enabled: false},
		Keepalive: &KeepaliveConfig{
			Time:                30 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		},
	}
}

// Client gRPC 客户端管理器
type Client struct {
	cfg    *Config
	conn   *grpc.ClientConn
	mu     sync.RWMutex
	closed bool
}

// NewClient 创建 gRPC 客户端
func NewClient(cfg *Config, opts ...ClientOption) (*Client, error) {
	return newClient(cfg, opts)
}

// NewClientWithInterceptors 创建带拦截器的客户端
func NewClientWithInterceptors(
	cfg *Config,
	unaryInterceptors []grpc.UnaryClientInterceptor,
	streamInterceptors []grpc.StreamClientInterceptor,
) (*Client, error) {
	var opts []ClientOption
	if len(unaryInterceptors) > 0 {
		opts = append(opts, WithUnaryInterceptors(unaryInterceptors...))
	}
	if len(streamInterceptors) > 0 {
		opts = append(opts, WithStreamInterceptors(streamInterceptors...))
	}
	return newClient(cfg, opts)
}

// ClientOption 客户端选项
type ClientOption struct {
	unaryInterceptors  []grpc.UnaryClientInterceptor
	streamInterceptors []grpc.StreamClientInterceptor
	dialOptions        []grpc.DialOption
}

func WithUnaryInterceptors(interceptors ...grpc.UnaryClientInterceptor) ClientOption {
	return ClientOption{unaryInterceptors: interceptors}
}

func WithStreamInterceptors(interceptors ...grpc.StreamClientInterceptor) ClientOption {
	return ClientOption{streamInterceptors: interceptors}
}

func WithDialOptions(opts ...grpc.DialOption) ClientOption {
	return ClientOption{dialOptions: opts}
}

func newClient(cfg *Config, opts []ClientOption) (*Client, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.Target == "" {
		return nil, fmt.Errorf("grpc target is required")
	}

	dialOpts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(cfg.MaxRecvMsgSize),
			grpc.MaxCallSendMsgSize(cfg.MaxSendMsgSize),
		),
	}

	// TLS
	if cfg.TLS != nil && cfg.TLS.Enabled {
		tlsCfg := &tls.Config{
			// #nosec G402 -- 仅在配置显式开启的开发/内网场景跳过校验，生产默认关闭
			InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
			ServerName:         cfg.TLS.ServerName,
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Keepalive
	if cfg.Keepalive != nil {
		dialOpts = append(dialOpts, grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                cfg.Keepalive.Time,
			Timeout:             cfg.Keepalive.Timeout,
			PermitWithoutStream: cfg.Keepalive.PermitWithoutStream,
		}))
	}

	if cfg.Block {
		dialOpts = append(dialOpts, grpc.WithBlock())
	}

	dialOpts = append(dialOpts, grpc.WithConnectParams(grpc.ConnectParams{
		Backoff: backoff.Config{
			BaseDelay:  100 * time.Millisecond,
			Multiplier: 1.6,
			MaxDelay:   3 * time.Second,
		},
		MinConnectTimeout: 5 * time.Second,
	}))

	// 统一可靠性：透明重试（服务配置）+ per-target 熔断（拦截器）。默认开启，覆盖所有
	// 走本基础客户端的通路（docreader、quality），无需各通路自行实现（〔X-4〕）。
	// 熔断拦截器置于最外层，open 时先于 request_id 透传与实际调用快速失败。
	rcfg := reliability.DefaultConfig()
	if cfg.Reliability != nil {
		rcfg = cfg.Reliability.WithDefaults()
	}
	if rcfg.Enabled {
		reg := reliability.NewRegistry(rcfg)
		dialOpts = append(dialOpts,
			grpc.WithDefaultServiceConfig(reliability.ServiceConfigJSON(rcfg)),
			grpc.WithChainUnaryInterceptor(reliability.UnaryClientInterceptor(reg)),
			grpc.WithChainStreamInterceptor(reliability.StreamClientInterceptor(reg)),
		)
	}

	// 默认拦截器：所有 gRPC 客户端自动透传 request_id 到下游（Python）服务，
	// 实现跨进程链路追踪。多次 WithChain* 会按顺序合并，故置于调用方自定义拦截器之前。
	dialOpts = append(dialOpts,
		grpc.WithChainUnaryInterceptor(interceptor.RequestIDUnary()),
		grpc.WithChainStreamInterceptor(interceptor.RequestIDStream()),
	)

	// 添加拦截器
	for _, opt := range opts {
		if len(opt.unaryInterceptors) > 0 {
			dialOpts = append(dialOpts, grpc.WithChainUnaryInterceptor(opt.unaryInterceptors...))
		}
		if len(opt.streamInterceptors) > 0 {
			dialOpts = append(dialOpts, grpc.WithChainStreamInterceptor(opt.streamInterceptors...))
		}
		dialOpts = append(dialOpts, opt.dialOptions...)
	}

	ctx := context.Background()
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	conn, err := grpc.DialContext(ctx, cfg.Target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", cfg.Target, err)
	}

	return &Client{cfg: cfg, conn: conn, closed: false}, nil
}

func (c *Client) Conn() *grpc.ClientConn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

func (c *Client) Target() string {
	return c.cfg.Target
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

func (c *Client) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// ClientPool 客户端连接池
type ClientPool struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

func NewClientPool() *ClientPool {
	return &ClientPool{clients: make(map[string]*Client)}
}

func (p *ClientPool) GetOrCreate(target string, cfg *Config) (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if client, ok := p.clients[target]; ok && !client.IsClosed() {
		return client, nil
	}

	if cfg == nil {
		cfg = DefaultConfig()
	}
	cfg.Target = target

	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}

	p.clients[target] = client
	return client, nil
}

func (p *ClientPool) Get(target string) (*Client, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	client, ok := p.clients[target]
	if !ok || client.IsClosed() {
		return nil, false
	}
	return client, true
}

func (p *ClientPool) Remove(target string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	client, ok := p.clients[target]
	if !ok {
		return nil
	}

	delete(p.clients, target)
	return client.Close()
}

func (p *ClientPool) CloseAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error
	for target, client := range p.clients {
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(p.clients, target)
	}
	return firstErr
}

func WithUnaryInterceptor(interceptor grpc.UnaryClientInterceptor) ClientOption {
	return ClientOption{unaryInterceptors: []grpc.UnaryClientInterceptor{interceptor}}
}

func WithStreamInterceptor(interceptor grpc.StreamClientInterceptor) ClientOption {
	return ClientOption{streamInterceptors: []grpc.StreamClientInterceptor{interceptor}}
}
