// Package docreader provides gRPC client for Python document processing service
package docreader

import (
	"context"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcclient "cognida/internal/infrastructure/grpc"
	docreaderpb "cognida/api/proto/docreader"
	modeldocreader "cognida/internal/model/docreader"
)

// 编译期断言：*Client 满足领域端口 DocumentReader。
var _ modeldocreader.DocumentReader = (*Client)(nil)

// Client Python 文档处理服务客户端
type Client struct {
	conn   *grpc.ClientConn
	client docreaderpb.DocumentReaderServiceClient
}

// NewClient 创建文档处理客户端
func NewClient(target string) (*Client, error) {
	cfg := grpcclient.DefaultConfig()
	cfg.Target = target
	return NewClientConfig(cfg)
}

// NewClientConfig 使用自定义配置创建客户端
func NewClientConfig(cfg *grpcclient.Config) (*Client, error) {
	baseClient, err := grpcclient.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   baseClient.Conn(),
		client: docreaderpb.NewDocumentReaderServiceClient(baseClient.Conn()),
	}, nil
}

// Close 关闭客户端连接
func (c *Client) Close() error {
	return c.conn.Close()
}

// ParseDocument 解析文档
func (c *Client) ParseDocument(ctx context.Context, req *ParseDocumentRequest) (*ParseDocumentResponse, error) {
	grpcReq := &docreaderpb.ParseRequest{}

	// 设置数据源
	switch {
	case req.FilePath != "":
		grpcReq.Source = &docreaderpb.ParseRequest_FilePath{FilePath: req.FilePath}
	case req.URL != "":
		grpcReq.Source = &docreaderpb.ParseRequest_Url{Url: req.URL}
	case req.Content != nil:
		grpcReq.Source = &docreaderpb.ParseRequest_Content{Content: req.Content}
	default:
		return nil, fmt.Errorf("no data source specified")
	}

	grpcReq.Format = req.Format

	if req.Options != nil {
		grpcReq.Options = &docreaderpb.ParseOptions{
			IncludeMetadata: req.Options.IncludeMetadata,
			ExtractTables:   req.Options.ExtractTables,
			ExtractImages:   req.Options.ExtractImages,
			OcrEngine:       req.Options.OcrEngine,
			Language:        req.Options.Language,
		}
	}

	resp, err := c.client.ParseDocument(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("parse document failed: %w", err)
	}

	response := &ParseDocumentResponse{
		Success: resp.Success,
		Text:    resp.Text,
		Error:   resp.Error,
	}
	if resp.Metadata != nil {
		response.Metadata = &DocumentMetadata{
			Format:     resp.Metadata.Format,
			PageCount:  int(resp.Metadata.PageCount),
			CharCount:  int(resp.Metadata.CharCount),
			Properties: resp.Metadata.Properties,
		}
	}
	return response, nil
}

// ParseBatch 批量解析文档
func (c *Client) ParseBatch(ctx context.Context, requests []*ParseDocumentRequest) (*ParseBatchResponse, error) {
	grpcRequests := make([]*docreaderpb.ParseRequest, len(requests))
	for i, req := range requests {
		grpcReq := &docreaderpb.ParseRequest{}

		switch {
		case req.FilePath != "":
			grpcReq.Source = &docreaderpb.ParseRequest_FilePath{FilePath: req.FilePath}
		case req.URL != "":
			grpcReq.Source = &docreaderpb.ParseRequest_Url{Url: req.URL}
		case req.Content != nil:
			grpcReq.Source = &docreaderpb.ParseRequest_Content{Content: req.Content}
		default:
			return nil, fmt.Errorf("no data source specified for request %d", i)
		}

		grpcReq.Format = req.Format

		if req.Options != nil {
			grpcReq.Options = &docreaderpb.ParseOptions{
				IncludeMetadata: req.Options.IncludeMetadata,
				ExtractTables:   req.Options.ExtractTables,
				ExtractImages:   req.Options.ExtractImages,
			}
		}

		grpcRequests[i] = grpcReq
	}

	grpcReq := &docreaderpb.ParseBatchRequest{
		Requests: grpcRequests,
	}

	resp, err := c.client.ParseBatch(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("parse batch failed: %w", err)
	}

	responses := make([]*ParseDocumentResponse, len(resp.Responses))
	for i, r := range resp.Responses {
		responses[i] = &ParseDocumentResponse{
			Success: r.Success,
			Text:    r.Text,
			Error:   r.Error,
		}
	}

	return &ParseBatchResponse{
		Responses: responses,
	}, nil
}

// OCRImage OCR 识别图片
func (c *Client) OCRImage(ctx context.Context, req *OCRImageRequest) (*OCRImageResponse, error) {
	grpcReq := &docreaderpb.OCRRequest{}

	// 设置数据源
	switch {
	case req.FilePath != "":
		grpcReq.Source = &docreaderpb.OCRRequest_FilePath{FilePath: req.FilePath}
	case req.ImageData != nil:
		grpcReq.Source = &docreaderpb.OCRRequest_ImageData{ImageData: req.ImageData}
	case req.URL != "":
		grpcReq.Source = &docreaderpb.OCRRequest_Url{Url: req.URL}
	default:
		return nil, fmt.Errorf("no data source specified")
	}

	grpcReq.Engine = req.Engine
	grpcReq.Language = req.Language

	if req.Options != nil {
		grpcReq.Options = &docreaderpb.OCROptions{
			Det:          req.Options.Det,
			Rec:          req.Options.Rec,
			UseCls:       req.Options.UseCls,
			ReturnDetails: req.Options.ReturnDetails,
		}
	}

	resp, err := c.client.OCRImage(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("ocr image failed: %w", err)
	}

	return &OCRImageResponse{
		Success:    resp.Success,
		Text:       resp.Text,
		Confidence: resp.Confidence,
		Error:      resp.Error,
		Blocks:     convertBlocks(resp.Blocks),
	}, nil
}

// convertBlocks 将 proto OCR blocks 转为领域类型。
// 关键：对 b.Box 做 nil 保护——proto 的 Box 为可选指针，上游未填充时 int(b.Box.X)
// 会空指针 panic；该转换在 OCRBatch 的 goroutine 内也会用到，一旦 panic 无 recover
// 将拖垮整个进程。缺失 Box 时保留其余字段、Box 置空。
func convertBlocks(pb []*docreaderpb.OCRBlock) []*OCRBlock {
	blocks := make([]*OCRBlock, len(pb))
	for i, b := range pb {
		ob := &OCRBlock{
			Text:       b.Text,
			Confidence: b.Confidence,
		}
		if b.Box != nil {
			ob.Box = &Rect{
				X:      int(b.Box.X),
				Y:      int(b.Box.Y),
				Width:  int(b.Box.Width),
				Height: int(b.Box.Height),
			}
		}
		blocks[i] = ob
	}
	return blocks
}

// OCRBatch 批量 OCR 识别（流式）
func (c *Client) OCRBatch(ctx context.Context, requests []*OCRImageRequest) (<-chan *OCRBatchResponse, error) {
	grpcRequests := make([]*docreaderpb.OCRRequest, len(requests))
	for i, req := range requests {
		grpcReq := &docreaderpb.OCRRequest{}

		switch {
		case req.FilePath != "":
			grpcReq.Source = &docreaderpb.OCRRequest_FilePath{FilePath: req.FilePath}
		case req.ImageData != nil:
			grpcReq.Source = &docreaderpb.OCRRequest_ImageData{ImageData: req.ImageData}
		case req.URL != "":
			grpcReq.Source = &docreaderpb.OCRRequest_Url{Url: req.URL}
		default:
			return nil, fmt.Errorf("no data source specified for request %d", i)
		}

		grpcReq.Engine = req.Engine
		grpcReq.Language = req.Language

		if req.Options != nil {
			grpcReq.Options = &docreaderpb.OCROptions{
				Det:          req.Options.Det,
				Rec:          req.Options.Rec,
				UseCls:       req.Options.UseCls,
				ReturnDetails: req.Options.ReturnDetails,
			}
		}

		grpcRequests[i] = grpcReq
	}

	stream, err := c.client.OCRBatch(ctx, &docreaderpb.OCRBatchRequest{
		Requests: grpcRequests,
	})
	if err != nil {
		return nil, fmt.Errorf("ocr batch failed: %w", err)
	}

	respChan := make(chan *OCRBatchResponse)

	go func() {
		defer close(respChan)
		// 兜底 recover：流处理在独立 goroutine 内，任何未预期 panic 若不拦截会直接
		// 终止整个进程；这里转成一条错误响应，隔离故障。
		defer func() {
			if r := recover(); r != nil {
				respChan <- &OCRBatchResponse{Error: fmt.Sprintf("ocr batch stream panic: %v", r)}
			}
		}()
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				st, _ := status.FromError(err)
				if st.Code() == codes.Canceled {
					return
				}
				respChan <- &OCRBatchResponse{
					Error: err.Error(),
				}
				return
			}

			// 上游可能回一条不含 Response 的分片：跳过而非解引用 nil。
			if resp.Response == nil {
				continue
			}

			respChan <- &OCRBatchResponse{
				Index: int(resp.Index),
				Response: &OCRImageResponse{
					Success:    resp.Response.Success,
					Text:       resp.Response.Text,
					Confidence: resp.Response.Confidence,
					Error:      resp.Response.Error,
					Blocks:     convertBlocks(resp.Response.Blocks),
				},
			}
		}
	}()

	return respChan, nil
}

// ChunkDocument 文本分块
func (c *Client) ChunkDocument(ctx context.Context, req *ChunkDocumentRequest) (*ChunkDocumentResponse, error) {
	strategyMap := map[ChunkStrategy]docreaderpb.ChunkStrategy{
		ChunkStrategyParagraph:  docreaderpb.ChunkStrategy_PARAGRAPH,
		ChunkStrategySentence:   docreaderpb.ChunkStrategy_SENTENCE,
		ChunkStrategyFixedSize:  docreaderpb.ChunkStrategy_FIXED_SIZE,
		ChunkStrategySemantic:   docreaderpb.ChunkStrategy_SEMANTIC,
		ChunkStrategyRecursive:  docreaderpb.ChunkStrategy_RECURSIVE,
	}

	grpcStrategy := strategyMap[req.Strategy]

	grpcReq := &docreaderpb.ChunkRequest{
		Text:     req.Text,
		Strategy: grpcStrategy,
	}

	if req.Options != nil {
		grpcReq.Options = &docreaderpb.ChunkOptions{
			ChunkSize:    int32(req.Options.ChunkSize),
			ChunkOverlap: int32(req.Options.ChunkOverlap),
			Separator:    req.Options.Separator,
			MinChunkSize: int32(req.Options.MinChunkSize),
		}
	}

	resp, err := c.client.ChunkDocument(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("chunk document failed: %w", err)
	}

	chunks := make([]*Chunk, len(resp.Chunks))
	for i, ch := range resp.Chunks {
		chunks[i] = &Chunk{
			Text:     ch.Text,
			Index:    int(ch.Index),
			StartPos: int(ch.StartPos),
			EndPos:   int(ch.EndPos),
			Metadata: ch.Metadata,
		}
	}

	return &ChunkDocumentResponse{
		Chunks:     chunks,
		TotalCount: int(resp.TotalCount),
	}, nil
}

// FetchURL 获取 URL 内容
func (c *Client) FetchURL(ctx context.Context, req *FetchURLRequest) (*FetchURLResponse, error) {
	grpcReq := &docreaderpb.FetchURLRequest{
		Url: req.URL,
	}

	if req.Options != nil {
		grpcReq.Options = &docreaderpb.FetchOptions{
			Timeout:           int32(req.Options.Timeout),
			WaitForJavascript: req.Options.WaitForJavaScript,
			WaitTime:          int32(req.Options.WaitTime),
			Screenshot:        req.Options.Screenshot,
		}
	}

	resp, err := c.client.FetchURL(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("fetch url failed: %w", err)
	}

	return &FetchURLResponse{
		Success:   resp.Success,
		Text:      resp.Text,
		Html:      resp.Html,
		Title:     resp.Title,
		Links:     resp.Links,
		Screenshot: resp.Screenshot,
		Error:     resp.Error,
	}, nil
}

// Request types

// 解析/分块相关 DTO 已下沉至 model/docreader（领域端口）。此处保留别名，
// 使 infrastructure 内部实现与外部依赖均指向同一领域类型。
type (
	// ParseDocumentRequest 文档解析请求
	ParseDocumentRequest = modeldocreader.ParseDocumentRequest
	// ParseOptions 解析选项
	ParseOptions = modeldocreader.ParseOptions
	// ParseDocumentResponse 文档解析响应
	ParseDocumentResponse = modeldocreader.ParseDocumentResponse
	// DocumentMetadata 文档元数据
	DocumentMetadata = modeldocreader.DocumentMetadata
)

// ParseBatchResponse 批量解析响应
type ParseBatchResponse struct {
	Responses []*ParseDocumentResponse
}

// OCRImageRequest OCR 请求
type OCRImageRequest struct {
	FilePath  string
	ImageData []byte
	URL       string
	Engine    string
	Language  string
	Options   *OCROptions
}

// OCROptions OCR 选项
type OCROptions struct {
	Det          bool
	Rec          bool
	UseCls       bool
	ReturnDetails bool
}

// OCRImageResponse OCR 响应
type OCRImageResponse struct {
	Success    bool
	Text       string
	Confidence float64
	Error      string
	Blocks     []*OCRBlock
}

// OCRBlock OCR 文本块
type OCRBlock struct {
	Text       string
	Confidence float64
	Box        *Rect
}

// Rect 矩形区域
type Rect struct {
	X, Y, Width, Height int
}

// OCRBatchResponse 批量 OCR 响应
type OCRBatchResponse struct {
	Index   int
	Response *OCRImageResponse
	Error   string
}

// 分块相关 DTO 已下沉至 model/docreader，此处保留别名与常量再导出。
type (
	// ChunkDocumentRequest 文本分块请求
	ChunkDocumentRequest = modeldocreader.ChunkDocumentRequest
	// ChunkStrategy 分块策略
	ChunkStrategy = modeldocreader.ChunkStrategy
	// ChunkOptions 分块选项
	ChunkOptions = modeldocreader.ChunkOptions
	// ChunkDocumentResponse 文本分块响应
	ChunkDocumentResponse = modeldocreader.ChunkDocumentResponse
	// Chunk 文本块
	Chunk = modeldocreader.Chunk
)

const (
	ChunkStrategyParagraph = modeldocreader.ChunkStrategyParagraph
	ChunkStrategySentence  = modeldocreader.ChunkStrategySentence
	ChunkStrategyFixedSize = modeldocreader.ChunkStrategyFixedSize
	ChunkStrategySemantic  = modeldocreader.ChunkStrategySemantic
	ChunkStrategyRecursive = modeldocreader.ChunkStrategyRecursive
)

// FetchURLRequest URL 获取请求
type FetchURLRequest struct {
	URL     string
	Options *FetchOptions
}

// FetchOptions URL 获取选项
type FetchOptions struct {
	Timeout           time.Duration
	WaitForJavaScript bool
	WaitTime          time.Duration
	Screenshot        bool
}

// FetchURLResponse URL 获取响应
type FetchURLResponse struct {
	Success   bool
	Text      string
	Html      string
	Title     string
	Links     []string
	Screenshot []byte
	Error     string
}
