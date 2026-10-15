package tools

import (
	"context"
	"fmt"
	"strings"

	agentctx "cognida/internal/model/agent"
	domainrag "cognida/internal/model/rag"
	ragpipeline "cognida/internal/service/knowledge/pipeline"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type RAGOptimizationRequest struct {
	Query   string `json:"query" jsonschema:"required,description=要在当前授权知识库范围内研究的问题"`
	TopK    int    `json:"top_k,omitempty" jsonschema:"description=最终证据数量，1至20，默认5"`
	MaxHops int    `json:"max_hops,omitempty" jsonschema:"description=多跳次数，1至3，默认2"`
}

type RAGOptimizationResult struct {
	Query          string          `json:"query"`
	ProcessedQuery string          `json:"processed_query,omitempty"`
	Technique      string          `json:"technique"`
	Chunks         []DocumentChunk `json:"chunks"`
	Count          int             `json:"count"`
	Hops           int             `json:"hops,omitempty"`
	HasAnswer      bool            `json:"has_answer"`
	Warnings       []string        `json:"warnings,omitempty"`
	Note           string          `json:"note,omitempty"`
}

// Optimization tools never accept a tenant or KB argument. Each retrieval calls
// the same scoped adapter as rag_query, including every hop and generated query.
func NewRAGOptimizationTools(svc RAGQueryService, llm model.BaseChatModel) []tool.BaseTool {
	result := make([]tool.BaseTool, 0, 3)
	params, err := utils.GoStruct2ParamsOneOf[RAGOptimizationRequest]()
	if err != nil {
		panic(fmt.Sprintf("invalid static retrieval optimization schema: %v", err))
	}
	for _, technique := range []string{"rewrite", "hyde", "multi_hop"} {
		kind := technique
		optimized := NewTypedBaseTool("rag_"+kind,
			"在用户当前授权知识库范围内执行"+kind+"检索并返回真实证据。改写或假设文本不是证据；不会扩展知识库/租户权限。多跳最多3次。",
			func(ctx context.Context, req *RAGOptimizationRequest) (*RAGOptimizationResult, error) {
				tenant, tenantOK := agentctx.GetTenantID(ctx)
				user, userOK := agentctx.GetUserID(ctx)
				if !tenantOK || !userOK || tenant <= 0 || user <= 0 {
					return nil, fmt.Errorf("authenticated research context is required")
				}
				if llm == nil || svc == nil {
					return nil, fmt.Errorf("retrieval optimization is not configured")
				}
				query := strings.TrimSpace(req.Query)
				if query == "" || len([]rune(query)) > 4000 {
					return nil, fmt.Errorf("query must contain 1 to 4000 characters")
				}
				topK := req.TopK
				if topK == 0 {
					topK = 5
				}
				if topK < 1 || topK > 20 {
					return nil, fmt.Errorf("top_k must be between 1 and 20")
				}
				response := &RAGOptimizationResult{Query: query, Technique: kind, Chunks: []DocumentChunk{}}
				processed := query
				switch kind {
				case "rewrite":
					rewritten, err := ragpipeline.NewQueryRewriter(llm).RewriteQuery(ctx, query, domainrag.DefaultRewriteOptions())
					if err != nil {
						return nil, err
					}
					processed = rewritten.Rewritten
				case "hyde":
					hypothetical, err := ragpipeline.NewHyDEGenerator(llm).GenerateHypotheticalDoc(ctx, query, &domainrag.HyDEOptions{MaxTokens: 600, Temperature: 0.2})
					if err != nil {
						return nil, err
					}
					processed = hypothetical
					response.Note = "假设文档仅用于定位真实文档，不属于引用证据。"
				case "multi_hop":
					hops := req.MaxHops
					if hops == 0 {
						hops = 2
					}
					if hops < 1 || hops > 3 {
						return nil, fmt.Errorf("max_hops must be between 1 and 3")
					}
					// This adapter ignores model-supplied scope arguments and reuses ctx.
					bounded := &scopedRAGRetriever{service: svc, topK: topK}
					multi := ragpipeline.NewMultiHopRetriever(bounded, ragpipeline.EinoLLMAdapter{Model: llm})
					found, err := multi.RetrieveMultiHop(ctx, query, &domainrag.MultiHopOptions{MaxHops: hops, HopStrategy: "sequential"})
					if err != nil {
						return nil, err
					}
					response.Hops = len(found.Hops)
					response.Warnings = bounded.warnings
					for _, doc := range found.AllDocuments {
						response.Chunks = append(response.Chunks, documentToChunk(doc))
					}
					if len(response.Chunks) > topK {
						response.Chunks = response.Chunks[:topK]
					}
					response.Count, response.HasAnswer = len(response.Chunks), len(response.Chunks) > 0
					return response, nil
				}
				if strings.TrimSpace(processed) == "" || len([]rune(processed)) > 8000 {
					return nil, fmt.Errorf("model produced an invalid retrieval query")
				}
				response.ProcessedQuery = processed
				found, err := ragQuery(ctx, &RAGQueryRequest{Query: processed, TopK: topK, RetrievalMode: "hybrid"}, svc)
				if err != nil {
					return nil, err
				}
				response.Chunks, response.Count, response.HasAnswer = found.Chunks, found.Count, found.HasAnswer
				response.Warnings = found.Warnings
				return response, nil
			})
		optimized.ParamsOneOf = params
		result = append(result, optimized)
	}
	return result
}

type scopedRAGRetriever struct {
	domainrag.Retriever
	service  RAGQueryService
	topK     int
	warnings []string
}

func (s *scopedRAGRetriever) ScopeBound() bool { return true }

func (s *scopedRAGRetriever) Retrieve(ctx context.Context, _, _, query string, _ *domainrag.RetrieveOptions) (*domainrag.RetrieveResponse, error) {
	res, err := ragQuery(ctx, &RAGQueryRequest{Query: query, TopK: s.topK, RetrievalMode: "hybrid"}, s.service)
	if err != nil {
		return nil, err
	}
	docs := make([]*domainrag.Document, 0, len(res.Chunks))
	s.warnings = append(s.warnings, res.Warnings...)
	for _, c := range res.Chunks {
		docs = append(docs, &domainrag.Document{Content: c.Content, Score: float32(c.Score), ChunkID: c.ChunkID, KnowledgeID: c.KnowledgeID, KnowledgeBaseID: c.KnowledgeBaseID, ChunkIndex: c.ChunkIndex, Metadata: c.Metadata})
	}
	return &domainrag.RetrieveResponse{Results: docs, Query: query, TotalCount: len(docs)}, nil
}

func documentToChunk(d *domainrag.Document) DocumentChunk {
	return DocumentChunk{Content: d.Content, Score: float64(d.Score), Source: d.KnowledgeID, ChunkID: d.ChunkID, KnowledgeID: d.KnowledgeID, KnowledgeBaseID: d.KnowledgeBaseID, ChunkIndex: d.ChunkIndex, Metadata: d.Metadata}
}
