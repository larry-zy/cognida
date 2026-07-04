package quality

import (
	"context"

	qualitypb "link/api/proto/quality"
)

// Gateway 数据质量传输端口：抽象对 Python 质量服务的远程调用。
// 具体的 gRPC 拨号/连接实现位于 infrastructure 层，经 wire 注入，
// 从而保持 service 层不直接依赖 infrastructure（依赖倒置）。
type Gateway interface {
	EvaluateQuality(ctx context.Context, req *qualitypb.EvaluateQualityRequest) (*qualitypb.EvaluateQualityResponse, error)
	EvaluateUnstructuredQuality(ctx context.Context, req *qualitypb.EvaluateUnstructuredQualityRequest) (*qualitypb.EvaluateUnstructuredQualityResponse, error)
	CleanData(ctx context.Context, req *qualitypb.CleanDataRequest) (*qualitypb.CleanDataResponse, error)
	ListDimensions(ctx context.Context, req *qualitypb.ListDimensionsRequest) (*qualitypb.ListDimensionsResponse, error)
}
