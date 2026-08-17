// Package knowledge provides document processing pipeline implementation
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/embedding"

	"cognida/internal/infrastructure/id"
	docreader "cognida/internal/model/docreader"
	domain_knowledge "cognida/internal/model/knowledge"
	domain_llm "cognida/internal/model/llm"
	"cognida/internal/pkg/safego"
)

// ========================================
// Document Processing Service
// ========================================

// documentProcessorService 文档处理服务实现
type documentProcessorService struct {
	kbRepo        domain_knowledge.KnowledgeBaseRepository
	knowledgeRepo domain_knowledge.KnowledgeRepository
	chunkRepo     domain_knowledge.ChunkRepository
	graphRepo     domain_knowledge.GraphRepository
	vectorRepo    domain_knowledge.VectorRepository
	grpcClient    docreader.DocumentReader
	embedder      embedding.Embedder   // Embedding 生成器
	llmClient     domain_llm.LLMClient // LLM 客户端（用于图谱提取）
	idGenerator   id.IDGenerator
	vectorInitMu  sync.Mutex
}

// NewDocumentProcessorService 创建文档处理服务
func NewDocumentProcessorService(
	kbRepo domain_knowledge.KnowledgeBaseRepository,
	knowledgeRepo domain_knowledge.KnowledgeRepository,
	chunkRepo domain_knowledge.ChunkRepository,
	vectorRepo domain_knowledge.VectorRepository,
	graphRepo domain_knowledge.GraphRepository,
	grpcClient docreader.DocumentReader,
	embedder embedding.Embedder,
	llmClient domain_llm.LLMClient,
	idGenerator id.IDGenerator,
) DocumentProcessorService {
	return &documentProcessorService{
		kbRepo:        kbRepo,
		knowledgeRepo: knowledgeRepo,
		chunkRepo:     chunkRepo,
		vectorRepo:    vectorRepo,
		graphRepo:     graphRepo,
		grpcClient:    grpcClient,
		embedder:      embedder,
		llmClient:     llmClient,
		idGenerator:   idGenerator,
	}
}

// ProcessDocument 处理文档（解析 + 分块 + 存储）
func (s *documentProcessorService) ProcessDocument(
	ctx context.Context,
	tenantID int64,
	userID int64,
	req *ProcessDocumentRequest,
) (*ProcessDocumentResponse, error) {
	startTime := time.Now()

	log.Printf("[DocumentProcessor] Starting document processing: KnowledgeBaseID=%s, Title=%s", req.KnowledgeBaseID, req.Title)

	// 1. 验证请求
	if err := s.validateRequest(req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// 2. 获取知识库信息
	kb, err := s.kbRepo.FindByID(ctx, req.KnowledgeBaseID)
	if err != nil {
		return nil, fmt.Errorf("knowledge base not found: %w", err)
	}

	// 3. 获取知识库设置（检查是否启用图谱）
	setting, err := s.getKnowledgeBaseSetting(ctx, req.KnowledgeBaseID)
	if err != nil {
		log.Printf("[DocumentProcessor] Warning: failed to get KB setting: %v", err)
	}
	graphEnabled := req.GraphEnabled || (setting != nil && setting.GraphEnabled)

	// 4. 解析文档（调用 Python gRPC）
	text, metadata, err := s.parseDocument(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to parse document: %w", err)
	}

	log.Printf("[DocumentProcessor] Document parsed: %d characters", len(text))

	// 5. 分块处理（调用 Python gRPC）
	chunks, err := s.chunkDocument(ctx, text, req)
	if err != nil {
		return nil, fmt.Errorf("failed to chunk document: %w", err)
	}

	log.Printf("[DocumentProcessor] Document chunked: %d chunks", len(chunks))

	// 6. 创建或更新知识条目
	var knowledgeID string
	if req.DocumentID != "" {
		knowledgeID = req.DocumentID
		// 重处理已有文档：先清除旧分块及其向量/图谱投影，否则后续 CreateBatch 直接追加
		// 新分块，旧数据残留 → 分块翻倍、检索重复命中（KB-8）。任一存储清理失败即中止，
		// 不产生"新旧混叠"。
		if err := s.purgeExistingDocument(ctx, tenantID, req.KnowledgeBaseID, knowledgeID); err != nil {
			return nil, fmt.Errorf("failed to purge existing document before reprocess: %w", err)
		}
	} else {
		// 创建新文档
		knowledgeID = s.idGenerator.Generate()
		documentType := s.detectDocumentType(metadata)

		// 使用工厂函数创建，确保时间戳正确设置
		knowledge, err := domain_knowledge.NewKnowledge(knowledgeID, tenantID, userID, req.KnowledgeBaseID, documentType, req.Title)
		if err != nil {
			return nil, fmt.Errorf("failed to create knowledge entity: %w", err)
		}

		// 设置额外属性
		knowledge.Source = s.getSource(req)
		knowledge.FilePath = req.FilePath
		knowledge.ParseStatus = domain_knowledge.ParseStatusProcessing
		// 新入库文档默认启用，可被检索命中；停用是显式管理动作（见 SetKnowledgeEnabled）。
		knowledge.EnableStatus = domain_knowledge.EnableStatusEnabled
		knowledge.StorageSize = int64(len(text))

		if err := s.knowledgeRepo.Create(ctx, knowledge); err != nil {
			return nil, fmt.Errorf("failed to create knowledge: %w", err)
		}
	}

	// 7. 批量创建分块
	chunkEntities, err := s.createChunkEntities(ctx, knowledgeID, tenantID, req.KnowledgeBaseID, chunks)
	if err != nil {
		return nil, fmt.Errorf("failed to create chunk entities: %w", err)
	}

	if err := s.chunkRepo.CreateBatch(ctx, chunkEntities); err != nil {
		return nil, fmt.Errorf("failed to save chunks: %w", err)
	}

	log.Printf("[DocumentProcessor] Chunks saved: %d chunks", len(chunkEntities))

	// 8. 并发执行：向量化 + 图谱提取
	var wg sync.WaitGroup
	var vectorErr, graphErr error
	var vectorized, graphExtracted bool

	chunkIDs := make([]string, len(chunkEntities))
	for i, c := range chunkEntities {
		chunkIDs[i] = c.ID
	}

	// 向量化（如果向量检索器可用）
	if s.vectorRepo != nil && s.embedder != nil {
		wg.Add(1)
		go func() {
			defer safego.Recover("doc-processor")
			defer wg.Done()
			log.Printf("[DocumentProcessor] Starting vectorization goroutine for %d chunks", len(chunkEntities))
			vectorErr = s.vectorizeChunks(ctx, tenantID, kb.ID, chunkEntities)
			if vectorErr == nil {
				vectorized = true
				log.Printf("[DocumentProcessor] Vectorization completed successfully for %d chunks", len(chunkEntities))
			} else {
				log.Printf("[DocumentProcessor] Vectorization failed: %v", vectorErr)
			}
		}()
	} else {
		log.Printf("[DocumentProcessor] Vectorization skipped: vectorRepo=%v, embedder=%v", s.vectorRepo != nil, s.embedder != nil)
	}

	// 图谱提取（如果启用且图谱仓储可用）
	if graphEnabled && s.graphRepo != nil {
		wg.Add(1)
		go func() {
			defer safego.Recover("doc-processor")
			defer wg.Done()
			_, _, graphErr = s.extractGraph(ctx, tenantID, req.KnowledgeBaseID, knowledgeID, chunkEntities)
			if graphErr == nil {
				graphExtracted = true
			}
		}()
	}

	wg.Wait()

	// 9. 更新知识条目状态
	// 向量化是语义检索的前置条件：失败则该文档无法被检索，标 completed 属谎报——
	// 置为 failed 并回传错误信息，前端可见失败并触发重试，避免"已完成却搜不到"的静默坏账。
	// 图谱抽取是增强能力、非检索必需，其失败仅记录、不改变解析状态（行为与既有一致）。
	finalStatus := domain_knowledge.ParseStatusCompleted
	var statusErrMsg string
	if vectorErr != nil {
		finalStatus = domain_knowledge.ParseStatusFailed
		statusErrMsg = fmt.Sprintf("vectorization failed: %v", vectorErr)
		log.Printf("[DocumentProcessor] Vectorization failed, marking document as failed: %v", vectorErr)
	}
	if graphErr != nil {
		log.Printf("[DocumentProcessor] Graph extraction warning: %v", graphErr)
	}

	// 更新分块数量
	chunkCount := len(chunkEntities)
	if err := s.knowledgeRepo.UpdateChunkCount(ctx, knowledgeID, chunkCount); err != nil {
		log.Printf("[DocumentProcessor] Warning: failed to update chunk count: %v", err)
	}

	// 更新解析状态
	if err := s.knowledgeRepo.UpdateParseStatus(ctx, knowledgeID, finalStatus, statusErrMsg); err != nil {
		log.Printf("[DocumentProcessor] Warning: failed to update parse status: %v", err)
	}

	// 更新知识库统计
	kb.IncrementDocumentCount()
	kb.AddChunks(chunkCount, int64(len(text)))
	if err := s.kbRepo.Update(ctx, kb); err != nil {
		log.Printf("[DocumentProcessor] Warning: failed to update KB stats: %v", err)
	}

	// 10. 启用分块（可选）
	// TODO: 根据配置决定是否立即启用

	processTime := time.Since(startTime).Milliseconds()

	log.Printf("[DocumentProcessor] Document processing completed: DocumentID=%s, Chunks=%d, Time=%dms",
		knowledgeID, chunkCount, processTime)

	return &ProcessDocumentResponse{
		DocumentID:     knowledgeID,
		ChunkCount:     chunkCount,
		ChunkIDs:       chunkIDs,
		ParseStatus:    finalStatus,
		ProcessTime:    processTime,
		StorageSize:    int64(len(text)),
		Vectorized:     vectorized,
		GraphExtracted: graphExtracted,
	}, nil
}

// validateRequest 验证请求
func (s *documentProcessorService) validateRequest(req *ProcessDocumentRequest) error {
	if req.KnowledgeBaseID == "" {
		return fmt.Errorf("KnowledgeBaseID is required")
	}
	if req.Title == "" {
		return fmt.Errorf("Title is required")
	}
	if req.FilePath == "" && req.URL == "" && len(req.Content) == 0 {
		return fmt.Errorf("one of FilePath, URL, or Content is required")
	}
	return nil
}

// getKnowledgeBaseSetting 获取知识库设置
func (s *documentProcessorService) getKnowledgeBaseSetting(ctx context.Context, kbID string) (*domain_knowledge.KnowledgeBaseSetting, error) {
	// TODO: 实现获取 KB 设置
	// 目前返回 nil，使用请求参数
	return nil, nil
}

// parseDocument 解析文档（调用 Python gRPC）
func (s *documentProcessorService) parseDocument(
	ctx context.Context,
	req *ProcessDocumentRequest,
) (string, map[string]string, error) {
	parseReq := &docreader.ParseDocumentRequest{
		Options: &docreader.ParseOptions{
			IncludeMetadata: true,
			ExtractTables:   false,
			ExtractImages:   false,
		},
	}

	// 设置数据源
	switch {
	case req.FilePath != "":
		parseReq.FilePath = req.FilePath
		log.Printf("[DocumentProcessor] Parsing file: %s", req.FilePath)
	case req.URL != "":
		parseReq.URL = req.URL
		log.Printf("[DocumentProcessor] Parsing URL: %s", req.URL)
	case len(req.Content) > 0:
		parseReq.Content = req.Content
		log.Printf("[DocumentProcessor] Parsing content, length: %d", len(req.Content))
	}

	// 调用 Python gRPC
	resp, err := s.grpcClient.ParseDocument(ctx, parseReq)
	if err != nil {
		log.Printf("[DocumentProcessor] ParseDocument gRPC error: %v", err)
		return "", nil, err
	}

	if !resp.Success {
		log.Printf("[DocumentProcessor] ParseDocument failed: %s", resp.Error)
		return "", nil, fmt.Errorf("parse failed: %s", resp.Error)
	}

	log.Printf("[DocumentProcessor] ParseDocument succeeded, text length: %d", len(resp.Text))

	// 构建元数据
	metadata := make(map[string]string)
	if resp.Metadata != nil {
		metadata["format"] = resp.Metadata.Format
		metadata["page_count"] = fmt.Sprintf("%d", resp.Metadata.PageCount)
		metadata["char_count"] = fmt.Sprintf("%d", resp.Metadata.CharCount)
		for k, v := range resp.Metadata.Properties {
			metadata[k] = v
		}
	}

	return resp.Text, metadata, nil
}

// chunkDocument 分块文档（调用 Python gRPC）
func (s *documentProcessorService) chunkDocument(
	ctx context.Context,
	text string,
	req *ProcessDocumentRequest,
) ([]*docreader.Chunk, error) {
	log.Printf("[DocumentProcessor] chunkDocument called with text length: %d", len(text))

	chunkSize := req.ChunkSize
	if chunkSize == 0 {
		chunkSize = 1000 // 默认值
	}

	chunkOverlap := req.ChunkOverlap
	if chunkOverlap == 0 {
		chunkOverlap = 200 // 默认值
	}

	chunkReq := &docreader.ChunkDocumentRequest{
		Text:     text,
		Strategy: s.mapChunkStrategy(req.ChunkStrategy),
		Options: &docreader.ChunkOptions{
			ChunkSize:    chunkSize,
			ChunkOverlap: chunkOverlap,
			Separator:    "\n\n",
			MinChunkSize: 100,
		},
	}

	resp, err := s.grpcClient.ChunkDocument(ctx, chunkReq)
	if err != nil {
		log.Printf("[DocumentProcessor] ChunkDocument gRPC error: %v", err)
		return nil, err
	}

	log.Printf("[DocumentProcessor] ChunkDocument succeeded, got %d chunks", len(resp.Chunks))
	if len(resp.Chunks) > 0 {
		log.Printf("[DocumentProcessor] First chunk text (first 200 chars): %s", truncateString(resp.Chunks[0].Text, 200))
	}

	return resp.Chunks, nil
}

// mapChunkStrategy 映射分块策略
func (s *documentProcessorService) mapChunkStrategy(strategy string) docreader.ChunkStrategy {
	switch strategy {
	case "sentence":
		return docreader.ChunkStrategySentence
	case "fixed_size":
		return docreader.ChunkStrategyFixedSize
	case "semantic":
		return docreader.ChunkStrategySemantic
	case "recursive":
		return docreader.ChunkStrategyRecursive
	default:
		return docreader.ChunkStrategyParagraph
	}
}

// detectDocumentType 检测文档类型
//
// 入参 metadata["format"] 是 docreader 返回的文档格式 wire 值〔M13-P4 契约〕。
// 先经 NormalizeFormat 把别名（doc/xls/ppt/markdown）归一为 canonical，再按类型化
// 常量分派，避免散落的字面量。返回值为内部知识文档类型（word/excel/... 非 wire 契约）。
func (s *documentProcessorService) detectDocumentType(metadata map[string]string) string {
	switch NormalizeFormat(metadata["format"]) {
	case DocumentFormatPDF:
		return "pdf"
	case DocumentFormatDOCX:
		return "word"
	case DocumentFormatXLSX:
		return "excel"
	case DocumentFormatPPTX:
		return "ppt"
	case DocumentFormatMD:
		return "markdown"
	case DocumentFormatTXT:
		return "text"
	case DocumentFormatHTML:
		return "html"
	default:
		return "unknown"
	}
}

// getSource 获取数据源描述
func (s *documentProcessorService) getSource(req *ProcessDocumentRequest) string {
	switch {
	case req.FilePath != "":
		return "file:" + req.FilePath
	case req.URL != "":
		return "url:" + req.URL
	case len(req.Content) > 0:
		return "upload"
	default:
		return "unknown"
	}
}

// purgeExistingDocument 重处理前清除文档的旧分块及其投影（向量/图谱），保证幂等重建（KB-8）。
// 顺序要点：先按旧 chunk_id 清图谱、再删向量，最后删 MySQL 分块——MySQL 分块删除后
// chunk_id 即不可得，故图谱清理必须在最前。任一步失败即返回错误，调用方中止本次重处理，
// 避免"清一半 + 追加新分块"造成的混叠。
func (s *documentProcessorService) purgeExistingDocument(ctx context.Context, tenantID int64, kbID, knowledgeID string) error {
	// 1. 取旧分块 id（含禁用），供图谱按 chunk_id 精确清理（节点存 chunk_id 而非 knowledge_id）
	if s.graphRepo != nil && s.chunkRepo != nil {
		oldChunks, err := s.chunkRepo.FindByKnowledgeID(ctx, knowledgeID, false)
		if err != nil {
			return fmt.Errorf("加载旧分块失败: %w", err)
		}
		if len(oldChunks) > 0 {
			chunkIDs := make([]string, 0, len(oldChunks))
			for _, c := range oldChunks {
				chunkIDs = append(chunkIDs, c.ID)
			}
			namespace := domain_knowledge.NameSpace{
				TenantID:        fmt.Sprintf("%d", tenantID),
				KnowledgeBaseID: kbID,
				Knowledge:       knowledgeID,
			}
			if err := s.graphRepo.DeleteByChunkIDs(ctx, namespace, chunkIDs); err != nil {
				return fmt.Errorf("清除旧图谱失败: %w", err)
			}
		}
	}

	// 2. 删旧向量（按 knowledge_id，向量行带该字段）
	if s.vectorRepo != nil {
		var kbIDInt int64
		if kbID != "" {
			_, _ = fmt.Sscanf(kbID, "%d", &kbIDInt) // 解析失败保持零值
		}
		if err := deleteVectorProjectionByKnowledgeID(ctx, s.vectorRepo, kbIDInt, knowledgeID); err != nil {
			return fmt.Errorf("清除旧向量失败: %w", err)
		}
	}

	// 3. 删旧 MySQL 分块
	if s.chunkRepo != nil {
		if err := s.chunkRepo.DeleteByKnowledgeID(ctx, knowledgeID); err != nil {
			return fmt.Errorf("清除旧分块失败: %w", err)
		}
	}

	return nil
}

// createChunkEntities 创建分块实体
func (s *documentProcessorService) createChunkEntities(
	ctx context.Context,
	knowledgeID string,
	tenantID int64,
	kbID string,
	chunks []*docreader.Chunk,
) ([]*domain_knowledge.Chunk, error) {
	entities := make([]*domain_knowledge.Chunk, len(chunks))
	now := time.Now()

	log.Printf("[DocumentProcessor] Creating %d chunk entities", len(chunks))

	for i, chunk := range chunks {
		id := s.idGenerator.Generate()
		metadataJSON, _ := json.Marshal(chunk.Metadata)

		// Log first chunk content for debugging
		if i == 0 {
			log.Printf("[DocumentProcessor] First chunk content (first 200 chars): %s", truncateString(chunk.Text, 200))
		}

		entities[i] = &domain_knowledge.Chunk{
			ID:              id,
			TenantID:        tenantID,
			KnowledgeBaseID: kbID,
			KnowledgeID:     knowledgeID,
			Content:         chunk.Text,
			ChunkIndex:      chunk.Index,
			IsEnabled:       true,
			ChunkType:       domain_knowledge.ChunkTypeText,
			StartAt:         chunk.StartPos,
			EndAt:           chunk.EndPos,
			CreatedAt:       now,
			UpdatedAt:       now,
		}

		if len(metadataJSON) > 0 {
			metadataStr := string(metadataJSON)
			entities[i].Metadata = &metadataStr
		}
	}

	return entities, nil
}

// truncateString 截断字符串用于日志输出
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// stableVectorID 由全局唯一的 chunk_id 派生稳定、唯一的 int64 主键（FNV-1a 64 位哈希取非负）。
// "link" collection 主键为非 AutoID，客户端必须提供唯一 id；用 chunk_id 派生既保证跨文档唯一，
// 又对同一 chunk 幂等（重复写入得到同一主键），避免旧的循环下标实现导致的跨文档主键冲突。
func stableVectorID(chunkID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(chunkID))
	return int64(h.Sum64() & 0x7FFFFFFFFFFFFFFF)
}

const embeddingBatchSize = 10

const graphExtractionBatchSize = 4

const graphExtractionSystemPrompt = `你是一个知识图谱构建专家。你的任务是从给定文本中提取实体和关系。

请以 JSON 格式返回结果，包含 "nodes" 和 "relations"：
- nodes: name、entity_type，以及可选的 properties
- relations: source、target、type、strength 和 description

要求：
1. 只提取重要实体，使用人物、组织、概念、事件、地点等通用类型
2. 关系类型使用 CONTAINS、RELATED_TO、CAUSES、PART_OF、LOCATED_IN、BELONGS_TO 等标准类型
3. 确保 source 和 target 均在 nodes 中存在
4. 只返回有效 JSON，不要输出 Markdown 或解释文字`

func embedTextsInBatches(ctx context.Context, embedder embedding.Embedder, texts []string) ([][]float64, error) {
	embeddings := make([][]float64, 0, len(texts))
	for start := 0; start < len(texts); start += embeddingBatchSize {
		end := min(start+embeddingBatchSize, len(texts))
		batch, err := embedder.EmbedStrings(ctx, texts[start:end])
		if err != nil {
			return nil, fmt.Errorf("batch %d-%d: %w", start, end-1, err)
		}
		if len(batch) != end-start {
			return nil, fmt.Errorf("batch %d-%d embedding count mismatch: got %d, want %d", start, end-1, len(batch), end-start)
		}
		embeddings = append(embeddings, batch...)
	}
	return embeddings, nil
}

// vectorizeChunks 向量化分块（保存到向量存储）
func (s *documentProcessorService) vectorizeChunks(
	ctx context.Context,
	tenantID int64,
	kbID string,
	chunks []*domain_knowledge.Chunk,
) error {
	if s.vectorRepo == nil {
		return fmt.Errorf("vector repository not available")
	}

	if len(chunks) == 0 {
		return nil
	}

	log.Printf("[DocumentProcessor] Starting vectorization for %d chunks", len(chunks))

	// 1. 提取分块内容用于 embedding
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Content
	}

	// 2. 分批生成 embedding 向量。DashScope 单请求最多接收 10 条文本；保序合并各批结果。
	embeddings, err := embedTextsInBatches(ctx, s.embedder, texts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}
	if len(embeddings) != len(chunks) {
		return fmt.Errorf("embedding count mismatch: got %d, want %d", len(embeddings), len(chunks))
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return fmt.Errorf("embedding service returned an empty vector")
	}

	// kbID 转换为 int64，如果为空则使用 0（统一 collection）。首次写入时根据真实
	// embedding 维度初始化 collection；锁内检查保证并发上传不会重复建表/建索引。
	var kbIDInt64 int64
	if kbID != "" {
		_, _ = fmt.Sscanf(kbID, "%d", &kbIDInt64) // 解析失败保持零值
	}
	s.vectorInitMu.Lock()
	err = ensureVectorCollection(ctx, s.vectorRepo, kbIDInt64, len(embeddings[0]))
	s.vectorInitMu.Unlock()
	if err != nil {
		return fmt.Errorf("failed to initialize vector collection: %w", err)
	}

	// 3. 转换为 VectorDocument 格式
	docs := make([]*domain_knowledge.VectorDocument, len(chunks))
	for i, chunk := range chunks {
		// 转换向量维度为 float32
		vector := make([]float32, len(embeddings[i]))
		for j, v := range embeddings[i] {
			vector[j] = float32(v)
		}

		// 处理 TokenCount (可能为 nil)
		var tokenCount int64
		if chunk.TokenCount != nil {
			tokenCount = int64(*chunk.TokenCount)
		}

		// 解析 Metadata
		var metadata map[string]interface{}
		if chunk.Metadata != nil {
			_ = json.Unmarshal([]byte(*chunk.Metadata), &metadata) // 解析失败保持零值
		}

		docs[i] = &domain_knowledge.VectorDocument{
			// 由全局唯一的 chunk_id 派生稳定且唯一的主键，替换旧的循环下标 int64(i)——
			// 后者会让不同文档的第 i 个分块拿到相同主键，在非 AutoID 的 "link" collection
			// 里造成跨文档主键冲突/覆盖（KB-2）。
			ID:          stableVectorID(chunk.ID),
			DenseVector: vector,
			// Text 为 BM25 全文检索的分词源，写入分块内容；旧代码从不设置该字段，
			// 导致 collection 的 text 列恒空、BM25 永远召回为空（KB-3）。
			Text:            chunk.Content,
			ChunkID:         chunk.ID,
			KnowledgeID:     chunk.KnowledgeID,
			KnowledgeBaseID: chunk.KnowledgeBaseID,
			TenantID:        chunk.TenantID,
			ChunkIndex:      chunk.ChunkIndex,
			Content:         chunk.Content,
			IsEnabled:       chunk.IsEnabled,
			StartAt:         int64(chunk.StartAt),
			EndAt:           int64(chunk.EndAt),
			TokenCount:      tokenCount,
			Metadata:        metadata,
		}
	}

	// 4. 批量插入到向量存储
	if err := s.vectorRepo.Insert(ctx, kbIDInt64, docs); err != nil {
		return fmt.Errorf("failed to insert vectors: %w", err)
	}

	log.Printf("[DocumentProcessor] Vectorization completed: %d chunks inserted", len(chunks))
	return nil
}

// extractGraph 提取图谱（保存到 Neo4j），返回提取的节点数与关系数
func (s *documentProcessorService) extractGraph(
	ctx context.Context,
	tenantID int64,
	kbID string,
	knowledgeID string,
	chunks []*domain_knowledge.Chunk,
) (int, int, error) {
	if s.graphRepo == nil {
		return 0, 0, fmt.Errorf("graph repository not available")
	}
	if s.llmClient == nil {
		return 0, 0, fmt.Errorf("llm client not available")
	}

	if len(chunks) == 0 {
		return 0, 0, nil
	}

	log.Printf("[DocumentProcessor] Starting graph extraction for %d chunks", len(chunks))
	namespace := domain_knowledge.NameSpace{
		TenantID:        fmt.Sprintf("%d", tenantID),
		KnowledgeBaseID: kbID,
		Knowledge:       knowledgeID,
	}

	graphData, err := s.extractGraphData(ctx, chunks)
	if err != nil {
		return 0, 0, err
	}

	if len(graphData.Node) == 0 && len(graphData.Relation) == 0 {
		log.Printf("[DocumentProcessor] No entities or relations extracted")
		return 0, 0, nil
	}

	if err := s.graphRepo.AddGraph(ctx, namespace, []*domain_knowledge.GraphData{graphData}); err != nil {
		return 0, 0, fmt.Errorf("failed to save graph: %w", err)
	}

	log.Printf("[DocumentProcessor] Graph extraction completed: %d nodes, %d relations",
		len(graphData.Node), len(graphData.Relation))
	return len(graphData.Node), len(graphData.Relation), nil
}

func (s *documentProcessorService) extractGraphData(
	ctx context.Context,
	chunks []*domain_knowledge.Chunk,
) (*domain_knowledge.GraphData, error) {
	graphs := make([]*domain_knowledge.GraphData, 0, (len(chunks)+graphExtractionBatchSize-1)/graphExtractionBatchSize)
	for start := 0; start < len(chunks); start += graphExtractionBatchSize {
		end := min(start+graphExtractionBatchSize, len(chunks))
		graph, err := s.extractGraphBatchWithRetry(ctx, chunks[start:end], start, end)
		if err != nil {
			return nil, err
		}
		graphs = append(graphs, graph)
	}
	return mergeGraphExtractionResults(graphs), nil
}

func (s *documentProcessorService) extractGraphBatchWithRetry(
	ctx context.Context,
	chunks []*domain_knowledge.Chunk,
	start, end int,
) (*domain_knowledge.GraphData, error) {
	graph, retryable, err := s.extractGraphBatch(ctx, chunks, start, end)
	if err == nil {
		return graph, nil
	}
	if !retryable || len(chunks) == 1 {
		return nil, fmt.Errorf("graph extraction batch %d-%d failed: %w", start, end-1, err)
	}

	middle := len(chunks) / 2
	log.Printf("[DocumentProcessor] Retrying graph extraction with smaller batches: range=%d-%d chunks=%d reason=%v", start, end-1, len(chunks), err)
	left, err := s.extractGraphBatchWithRetry(ctx, chunks[:middle], start, start+middle)
	if err != nil {
		return nil, err
	}
	right, err := s.extractGraphBatchWithRetry(ctx, chunks[middle:], start+middle, end)
	if err != nil {
		return nil, err
	}
	return mergeGraphExtractionResults([]*domain_knowledge.GraphData{left, right}), nil
}

func (s *documentProcessorService) extractGraphBatch(
	ctx context.Context,
	chunks []*domain_knowledge.Chunk,
	start, end int,
) (*domain_knowledge.GraphData, bool, error) {
	userPrompt, inputChars := graphExtractionPrompt(chunks, start)
	resp, err := s.llmClient.Chat(ctx, &domain_llm.ChatRequest{
		Messages: []*domain_llm.Message{
			{Role: domain_llm.RoleSystem, Content: graphExtractionSystemPrompt},
			{Role: domain_llm.RoleUser, Content: userPrompt},
		},
		Options: &domain_llm.ChatOptions{
			Temperature: domain_llm.Float64Ptr(0.1),
			MaxTokens:   4096,
		},
	})
	if err != nil {
		return nil, false, fmt.Errorf("llm chat failed: %w", err)
	}
	if resp == nil {
		return nil, false, fmt.Errorf("llm returned an empty response")
	}

	usage := "unavailable"
	if resp.Usage != nil {
		usage = fmt.Sprintf("prompt=%d completion=%d total=%d", resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}
	if resp.FinishReason == "length" {
		log.Printf("[DocumentProcessor] Graph extraction response truncated: range=%d-%d chunks=%d input_chars=%d output_chars=%d finish_reason=%q usage=%s", start, end-1, len(chunks), inputChars, len(resp.Content), resp.FinishReason, usage)
		return nil, true, fmt.Errorf("response truncated with finish_reason=%q", resp.FinishReason)
	}

	graph, err := s.parseGraphExtraction(resp.Content)
	if err != nil {
		log.Printf("[DocumentProcessor] Graph extraction response parse failed: range=%d-%d chunks=%d input_chars=%d output_chars=%d finish_reason=%q usage=%s error=%v", start, end-1, len(chunks), inputChars, len(resp.Content), resp.FinishReason, usage, err)
		return nil, true, fmt.Errorf("parse graph extraction failed: %w", err)
	}
	attachGraphChunkIDs(graph, chunks)
	return graph, false, nil
}

func graphExtractionPrompt(chunks []*domain_knowledge.Chunk, start int) (string, int) {
	var content strings.Builder
	for i, chunk := range chunks {
		content.WriteString(fmt.Sprintf("[Chunk %d] %s\n", start+i+1, chunk.Content))
	}
	return fmt.Sprintf("请从以下文本中提取实体和关系构建知识图谱：\n\n%s", content.String()), content.Len()
}

func attachGraphChunkIDs(graph *domain_knowledge.GraphData, chunks []*domain_knowledge.Chunk) {
	chunkIDs := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunkIDs = append(chunkIDs, chunk.ID)
	}
	for _, node := range graph.Node {
		node.Chunks = chunkIDs
	}
	for _, relation := range graph.Relation {
		relation.ChunkIDs = chunkIDs
	}
}

func mergeGraphExtractionResults(graphs []*domain_knowledge.GraphData) *domain_knowledge.GraphData {
	result := &domain_knowledge.GraphData{}
	nodesByName := make(map[string]*domain_knowledge.GraphNode)
	for _, graph := range graphs {
		for _, node := range graph.Node {
			if node == nil {
				continue
			}
			key := normalizeGraphName(node.Name)
			if key == "" {
				continue
			}
			existing, found := nodesByName[key]
			if !found {
				nodesByName[key] = node
				result.Node = append(result.Node, node)
				continue
			}
			existing.Chunks = mergeGraphChunkIDs(existing.Chunks, node.Chunks)
		}
	}

	relationsByKey := make(map[string]*domain_knowledge.GraphRelation)
	for _, graph := range graphs {
		for _, relation := range graph.Relation {
			if relation == nil {
				continue
			}
			source, sourceFound := nodesByName[normalizeGraphName(relation.Source)]
			target, targetFound := nodesByName[normalizeGraphName(relation.Target)]
			if !sourceFound || !targetFound {
				continue
			}
			relation.Source = source.Name
			relation.Target = target.Name
			key := normalizeGraphName(relation.Source) + "\x00" + normalizeGraphName(relation.Target) + "\x00" + normalizeGraphName(relation.Type)
			existing, found := relationsByKey[key]
			if !found {
				relationsByKey[key] = relation
				result.Relation = append(result.Relation, relation)
				continue
			}
			existing.ChunkIDs = mergeGraphChunkIDs(existing.ChunkIDs, relation.ChunkIDs)
			if relation.Strength > existing.Strength {
				existing.Strength = relation.Strength
			}
			if existing.Description == "" {
				existing.Description = relation.Description
			}
		}
	}
	return result
}

func normalizeGraphName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func mergeGraphChunkIDs(first, second []string) []string {
	seen := make(map[string]struct{}, len(first)+len(second))
	merged := make([]string, 0, len(first)+len(second))
	for _, chunkIDs := range [][]string{first, second} {
		for _, chunkID := range chunkIDs {
			if _, found := seen[chunkID]; found {
				continue
			}
			seen[chunkID] = struct{}{}
			merged = append(merged, chunkID)
		}
	}
	return merged
}

// RebuildKnowledgeBaseGraph 为知识库内所有已解析完成的历史文档补建/重建知识图谱。
// 复用已存储的分块（不重新解析/分块），只在全部抽取成功后原子替换旧图谱。
func (s *documentProcessorService) RebuildKnowledgeBaseGraph(
	ctx context.Context,
	tenantID int64,
	kbID string,
) (*RebuildGraphResponse, error) {
	if s.graphRepo == nil {
		return nil, fmt.Errorf("graph repository not available")
	}
	if s.llmClient == nil {
		return nil, fmt.Errorf("llm client not available")
	}

	resp := &RebuildGraphResponse{}
	graphs := make([]*domain_knowledge.GraphData, 0)

	// 分页遍历知识库下所有已完成的文档
	const pageSize = 100
	for page := 1; ; page++ {
		docs, total, err := s.knowledgeRepo.FindByKnowledgeBaseID(ctx, kbID, &domain_knowledge.KnowledgeListQuery{
			KnowledgeBaseID: kbID,
			ParseStatus:     domain_knowledge.ParseStatusCompleted,
			Page:            page,
			PageSize:        pageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("list knowledge failed: %w", err)
		}
		if len(docs) == 0 {
			break
		}
		resp.TotalDocuments = int(total)

		for _, doc := range docs {
			// 读取该文档已启用的分块，复用而非重新解析
			chunks, cerr := s.chunkRepo.FindByKnowledgeID(ctx, doc.ID, true)
			if cerr != nil {
				log.Printf("[DocumentProcessor] Rebuild: load chunks failed for doc %s: %v", doc.ID, cerr)
				resp.FailedDocuments++
				continue
			}
			if len(chunks) == 0 {
				resp.SkippedDocuments++
				continue
			}

			graph, gerr := s.extractGraphData(ctx, chunks)
			if gerr != nil {
				log.Printf("[DocumentProcessor] Rebuild: extract graph failed for doc %s: %v", doc.ID, gerr)
				resp.FailedDocuments++
				continue
			}
			graphs = append(graphs, graph)
			resp.ProcessedDocuments++
			resp.TotalNodes += len(graph.Node)
			resp.TotalRelations += len(graph.Relation)
		}

		if page*pageSize >= int(total) {
			break
		}
	}
	namespace := domain_knowledge.NameSpace{
		TenantID:        fmt.Sprintf("%d", tenantID),
		KnowledgeBaseID: kbID,
	}
	if err := s.replaceRebuiltGraph(ctx, namespace, graphs, resp.FailedDocuments); err != nil {
		return nil, fmt.Errorf("replace graph failed: %w", err)
	}

	log.Printf("[DocumentProcessor] Rebuild graph completed for KB %s: total=%d processed=%d skipped=%d failed=%d nodes=%d relations=%d",
		kbID, resp.TotalDocuments, resp.ProcessedDocuments, resp.SkippedDocuments, resp.FailedDocuments, resp.TotalNodes, resp.TotalRelations)
	return resp, nil
}

func (s *documentProcessorService) replaceRebuiltGraph(
	ctx context.Context,
	namespace domain_knowledge.NameSpace,
	graphs []*domain_knowledge.GraphData,
	failedDocuments int,
) error {
	if failedDocuments > 0 {
		log.Printf("[DocumentProcessor] Rebuild graph skipped replacement for KB %s after %d document failures", namespace.KnowledgeBaseID, failedDocuments)
		return nil
	}
	return s.graphRepo.ReplaceGraph(ctx, namespace, []*domain_knowledge.GraphData{mergeGraphExtractionResults(graphs)})
}

// parseGraphExtraction 解析 LLM 返回的图谱提取结果
func (s *documentProcessorService) parseGraphExtraction(content string) (*domain_knowledge.GraphData, error) {
	// 尝试提取 JSON（LLM 可能返回带额外文本的响应）
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, fmt.Errorf("no valid JSON found in response")
	}

	jsonStr := content[jsonStart : jsonEnd+1]

	// 解析 JSON
	var result struct {
		Nodes []struct {
			Name       string            `json:"name"`
			EntityType string            `json:"entity_type"`
			Properties map[string]string `json:"properties"`
		} `json:"nodes"`
		Relations []struct {
			Source      string  `json:"source"`
			Target      string  `json:"target"`
			Type        string  `json:"type"`
			Strength    float64 `json:"strength"`
			Description string  `json:"description"`
		} `json:"relations"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// 转换为 GraphData
	graphData := &domain_knowledge.GraphData{
		Node:     make([]*domain_knowledge.GraphNode, len(result.Nodes)),
		Relation: make([]*domain_knowledge.GraphRelation, 0, len(result.Relations)),
	}

	for i, n := range result.Nodes {
		graphData.Node[i] = &domain_knowledge.GraphNode{
			ID:         s.idGenerator.Generate(),
			Name:       n.Name,
			EntityType: n.EntityType,
			Properties: n.Properties,
			Chunks:     []string{}, // 将在调用方设置
		}
	}

	// 构建节点名称集合用于校验（关系按节点名称存储/匹配，见 Neo4jRepository.AddGraph）
	nodeNames := make(map[string]struct{}, len(graphData.Node))
	for _, node := range graphData.Node {
		nodeNames[node.Name] = struct{}{}
	}

	for _, r := range result.Relations {
		// 验证源和目标节点存在
		if _, ok := nodeNames[r.Source]; !ok {
			log.Printf("[DocumentProcessor] Warning: source node '%s' not found", r.Source)
			continue
		}
		if _, ok := nodeNames[r.Target]; !ok {
			log.Printf("[DocumentProcessor] Warning: target node '%s' not found", r.Target)
			continue
		}

		// Source/Target 存节点名称：AddGraph 用 MATCH (Entity {name: $source}) 匹配，
		// 存 ID 会导致匹配不到、关系被静默丢弃（计数虚高但落库为 0）。
		graphData.Relation = append(graphData.Relation, &domain_knowledge.GraphRelation{
			ID:          s.idGenerator.Generate(),
			Source:      r.Source,
			Target:      r.Target,
			Type:        r.Type,
			Strength:    r.Strength,
			ChunkIDs:    []string{}, // 将在调用方设置
			Description: r.Description,
		})
	}

	return graphData, nil
}
