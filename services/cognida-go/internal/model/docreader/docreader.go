// Package docreader 定义文档读取（解析/分块）领域端口与其 DTO。
//
// 该端口供 service 层（如文档处理流水线）依赖，具体实现位于
// infrastructure/grpc/docreader（gRPC 客户端），从而消除 service→infra 的直接耦合。
// 仅纳入领域用例真正需要的解析与分块契约；OCR、批量、抓取等能力仍由 infra 客户端自有类型承载。
package docreader

import "context"

// ========================================
// 解析（Parse）DTO
// ========================================

// ParseDocumentRequest 文档解析请求
type ParseDocumentRequest struct {
	FilePath string
	URL      string
	Content  []byte
	Format   string
	Options  *ParseOptions
}

// ParseOptions 解析选项
type ParseOptions struct {
	IncludeMetadata bool
	ExtractTables   bool
	ExtractImages   bool
	// OcrEngine / Language 为 OCR wire 契约字符串〔M13-P5〕，保持 string 以直通 proto。
	// 取值请引用类型化常量：OcrEngine ∈ {OCREnginePaddleOCR, OCREngineVLM}，
	// Language ∈ {OCRLanguageChiSim, OCRLanguageEng}；空值时由 Python 服务端按
	// DefaultOCREngine / DefaultOCRLanguage 兜底（见 ocr.go）。
	OcrEngine string
	Language  string
}

// ParseDocumentResponse 文档解析响应
type ParseDocumentResponse struct {
	Success  bool
	Text     string
	Error    string
	Metadata *DocumentMetadata
}

// DocumentMetadata 文档元数据
type DocumentMetadata struct {
	Format     string
	PageCount  int
	CharCount  int
	Properties map[string]string
}

// ========================================
// 分块（Chunk）DTO
// ========================================

// ChunkDocumentRequest 文本分块请求
type ChunkDocumentRequest struct {
	Text     string
	Strategy ChunkStrategy
	Options  *ChunkOptions
}

// ChunkStrategy 分块策略
type ChunkStrategy int

const (
	ChunkStrategyParagraph ChunkStrategy = iota
	ChunkStrategySentence
	ChunkStrategyFixedSize
	ChunkStrategySemantic
	ChunkStrategyRecursive
)

// ChunkOptions 分块选项
type ChunkOptions struct {
	ChunkSize    int
	ChunkOverlap int
	Separator    string
	MinChunkSize int
}

// ChunkDocumentResponse 文本分块响应
type ChunkDocumentResponse struct {
	Chunks     []*Chunk
	TotalCount int
}

// Chunk 文本块
type Chunk struct {
	Text     string
	Index    int
	StartPos int
	EndPos   int
	Metadata map[string]string
}

// ========================================
// 文档读取端口（消费端接口）
// ========================================

// DocumentReader 文档读取端口：解析与分块。
// service 层经此接口依赖文档处理能力，避免直接耦合 gRPC 实现。
type DocumentReader interface {
	ParseDocument(ctx context.Context, req *ParseDocumentRequest) (*ParseDocumentResponse, error)
	ChunkDocument(ctx context.Context, req *ChunkDocumentRequest) (*ChunkDocumentResponse, error)
}
