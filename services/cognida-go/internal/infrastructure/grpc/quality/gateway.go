// Package quality 提供数据质量 gRPC 网关：把对 Python 质量服务的远程调用
// 封装为 service 层定义的 Gateway 端口实现（依赖倒置，供 wire 注入）。
package quality

import (
	"context"
	"fmt"
	"sync"
	"time"

	qualitypb "cognida/api/proto/quality"
	infragrpc "cognida/internal/infrastructure/grpc"
)

// Gateway 基于 gRPC 的数据质量网关。懒连接：Python 服务未启动时不阻断应用启动，
// 首次调用时才拨号，实现 service.quality.Gateway 端口。
type Gateway struct {
	target  string
	timeout time.Duration

	mu     sync.Mutex
	client *infragrpc.Client
}

// NewGateway 创建数据质量 gRPC 网关
func NewGateway(target string, timeout time.Duration) *Gateway {
	if target == "" {
		target = "localhost:50051"
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Gateway{target: target, timeout: timeout}
}

// stub 懒建立连接并返回 gRPC 客户端。
func (g *Gateway) stub() (qualitypb.QualityServiceClient, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.client == nil {
		cfg := infragrpc.DefaultConfig()
		cfg.Target = g.target
		cfg.Timeout = g.timeout
		client, err := infragrpc.NewClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("连接数据质量服务失败(%s): %w", g.target, err)
		}
		g.client = client
	}
	return qualitypb.NewQualityServiceClient(g.client.Conn()), nil
}

// Close 关闭底层 gRPC 连接
func (g *Gateway) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.client != nil {
		err := g.client.Close()
		g.client = nil
		return err
	}
	return nil
}

// EvaluateQuality 结构化数据质量评估
func (g *Gateway) EvaluateQuality(ctx context.Context, req *qualitypb.EvaluateQualityRequest) (*qualitypb.EvaluateQualityResponse, error) {
	stub, err := g.stub()
	if err != nil {
		return nil, err
	}
	return stub.EvaluateQuality(ctx, req)
}

// EvaluateUnstructuredQuality 非结构化文本质量评估
func (g *Gateway) EvaluateUnstructuredQuality(ctx context.Context, req *qualitypb.EvaluateUnstructuredQualityRequest) (*qualitypb.EvaluateUnstructuredQualityResponse, error) {
	stub, err := g.stub()
	if err != nil {
		return nil, err
	}
	return stub.EvaluateUnstructuredQuality(ctx, req)
}

// CleanData 数据清洗
func (g *Gateway) CleanData(ctx context.Context, req *qualitypb.CleanDataRequest) (*qualitypb.CleanDataResponse, error) {
	stub, err := g.stub()
	if err != nil {
		return nil, err
	}
	return stub.CleanData(ctx, req)
}

// ListDimensions 支持的质量维度
func (g *Gateway) ListDimensions(ctx context.Context, req *qualitypb.ListDimensionsRequest) (*qualitypb.ListDimensionsResponse, error) {
	stub, err := g.stub()
	if err != nil {
		return nil, err
	}
	return stub.ListDimensions(ctx, req)
}
