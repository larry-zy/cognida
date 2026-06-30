// Package knowledge provides document processing pipeline implementation
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/embedding"

	domain_knowledge "link/internal/model/knowledge"
	domain_llm "link/internal/model/llm"
	"link/internal/infrastructure/id"
	docreader "link/internal/model/docreader"
)

// ========================================
// Document Processing Service
// ========================================

// documentProcessorService 文档处理服务实现
type documentProcessorService struct {
	kbRepo       domain_knowledge.KnowledgeBaseRepository
	knowledgeRepo domain_knowledge.KnowledgeRepository
	chunkRepo    domain_knowledge.ChunkRepository
	graphRepo    domain_knowledge.GraphRepository
	vectorRepo   domain_knowledge.VectorRepository
	grpcClient   docreader.DocumentReader
	embedder     embedding.Embedder         // Embedding 生成器
	llmClient    domain_llm.LLMClient       // LLM 客户端（用于图谱提取）
	idGenerator  id.IDGenerator
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
		// TODO: 更新已有文档
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
		knowledge.EnableStatus = domain_knowledge.EnableStatusDisabled
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
			defer wg.Done()
			graphErr = s.extractGraph(ctx, tenantID, req.KnowledgeBaseID, knowledgeID, chunkEntities)
			if graphErr == nil {
				graphExtracted = true
			}
		}()
	}

	wg.Wait()

	// 9. 更新知识条目状态
	finalStatus := domain_knowledge.ParseStatusCompleted
	if vectorErr != nil {
		log.Printf("[DocumentProcessor] Vectorization warning: %v", vectorErr)
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
	if err := s.knowledgeRepo.UpdateParseStatus(ctx, knowledgeID, finalStatus, ""); err != nil {
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
func (s *documentProcessorService) detectDocumentType(metadata map[string]string) string {
	format := metadata["format"]
	switch format {
	case "pdf":
		return "pdf"
	case "docx", "doc":
		return "word"
	case "xlsx", "xls":
		return "excel"
	case "pptx", "ppt":
		return "ppt"
	case "md", "markdown":
		return "markdown"
	case "txt":
		return "text"
	case "html":
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
			ID:          id,
			TenantID:    tenantID,
		 KnowledgeBaseID:        kbID,
			KnowledgeID: knowledgeID,
			Content:     chunk.Text,
			ChunkIndex:  chunk.Index,
			IsEnabled:   true,
			ChunkType:   domain_knowledge.ChunkTypeText,
			StartAt:     chunk.StartPos,
			EndAt:       chunk.EndPos,
			CreatedAt:   now,
			UpdatedAt:   now,
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

	// 2. 批量生成 embedding 向量
	embeddings, err := s.embedder.EmbedStrings(ctx, texts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}
	if len(embeddings) != len(chunks) {
		return fmt.Errorf("embedding count mismatch: got %d, want %d", len(embeddings), len(chunks))
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
			json.Unmarshal([]byte(*chunk.Metadata), &metadata)
		}

		docs[i] = &domain_knowledge.VectorDocument{
			ID:              int64(i), // 临时 ID，由数据库生成
			DenseVector:     vector,
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
	// kbID 转换为 int64，如果为空则使用 0（统一 collection）
	var kbIDInt64 int64
	if kbID != "" {
		fmt.Sscanf(kbID, "%d", &kbIDInt64)
	}
	if err := s.vectorRepo.Insert(ctx, kbIDInt64, docs); err != nil {
		return fmt.Errorf("failed to insert vectors: %w", err)
	}

	log.Printf("[DocumentProcessor] Vectorization completed: %d chunks inserted", len(chunks))
	return nil
}

// extractGraph 提取图谱（保存到 Neo4j）
func (s *documentProcessorService) extractGraph(
	ctx context.Context,
	tenantID int64,
	kbID string,
	knowledgeID string,
	chunks []*domain_knowledge.Chunk,
) error {
	if s.graphRepo == nil {
		return fmt.Errorf("graph repository not available")
	}
	if s.llmClient == nil {
		return fmt.Errorf("llm client not available")
	}

	if len(chunks) == 0 {
		return nil
	}

	log.Printf("[DocumentProcessor] Starting graph extraction for %d chunks", len(chunks))

	// 构建命名空间
	namespace := domain_knowledge.NameSpace{
		TenantID:  fmt.Sprintf("%d", tenantID),
	 KnowledgeBaseID:      kbID,
		Knowledge: knowledgeID,
	}

	// 收集所有分块内容进行批量提取
	var allContent strings.Builder
	chunkIDMap := make(map[int]string) // 索引 -> chunkID
	for i, chunk := range chunks {
		chunkIDMap[i] = chunk.ID
		allContent.WriteString(fmt.Sprintf("[Chunk %d] %s\n", i+1, chunk.Content))
	}

	// 构建提取 prompt
	systemPrompt := `你是一个知识图谱构建专家。你的任务是从给定文本中提取实体和关系。

请以 JSON 格式返回结果，包含以下结构：
{
  "nodes": [
    {
      "name": "实体名称",
      "entity_type": "实体类型（如：人物、组织、概念、地点等）",
      "properties": {"key": "value"}  // 可选的额外属性
    }
  ],
  "relations": [
    {
      "source": "源实体名称",
      "target": "目标实体名称",
      "type": "关系类型（如：CONTAINS、RELATED_TO、CAUSES等）",
      "strength": 1.0,  // 关系强度 1-10
      "description": "关系描述"
    }
  ]
}

要求：
1. 只提取重要的实体，忽略无关紧要的词语
2. 实体类型应该是通用的分类（人物、组织、概念、事件、地点等）
3. 关系类型使用标准类型：CONTAINS、RELATED_TO、CAUSES、PART_OF、LOCATED_IN、BELONGS_TO 等
4. 确保源实体和目标实体在 nodes 列表中存在
5. 返回有效的 JSON 格式`

	userPrompt := fmt.Sprintf("请从以下文本中提取实体和关系构建知识图谱：\n\n%s", allContent.String())

	// 调用 LLM
	chatReq := &domain_llm.ChatRequest{
		Messages: []*domain_llm.Message{
			{Role: domain_llm.RoleSystem, Content: systemPrompt},
			{Role: domain_llm.RoleUser, Content: userPrompt},
		},
		Options: &domain_llm.ChatOptions{
			Temperature: 0.1, // 低温度以获得更稳定的结果
			MaxTokens:   4096,
		},
	}

	resp, err := s.llmClient.Chat(ctx, chatReq)
	if err != nil {
		return fmt.Errorf("llm chat failed: %w", err)
	}

	// 解析 LLM 响应
	graphData, err := s.parseGraphExtraction(resp.Content)
	if err != nil {
		log.Printf("[DocumentProcessor] Warning: failed to parse graph extraction: %v", err)
		// 不返回错误，允许文档处理继续
		return nil
	}

	if len(graphData.Node) == 0 && len(graphData.Relation) == 0 {
		log.Printf("[DocumentProcessor] No entities or relations extracted")
		return nil
	}

	// 关联 chunk ID 到节点和关系
	chunkIDs := make([]string, len(chunks))
	for i, chunk := range chunks {
		chunkIDs[i] = chunk.ID
	}
	for _, node := range graphData.Node {
		node.Chunks = chunkIDs
	}
	for _, relation := range graphData.Relation {
		relation.ChunkIDs = chunkIDs
	}

	// 保存到 Neo4j
	if err := s.graphRepo.AddGraph(ctx, namespace, []*domain_knowledge.GraphData{graphData}); err != nil {
		return fmt.Errorf("failed to save graph: %w", err)
	}

	log.Printf("[DocumentProcessor] Graph extraction completed: %d nodes, %d relations",
		len(graphData.Node), len(graphData.Relation))
	return nil
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
		Nodes     []struct {
			Name       string              `json:"name"`
			EntityType string              `json:"entity_type"`
			Properties map[string]string   `json:"properties"`
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
		Relation: make([]*domain_knowledge.GraphRelation, len(result.Relations)),
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

	// 构建节点名称到 ID 的映射
	nodeIDMap := make(map[string]string)
	for _, node := range graphData.Node {
		nodeIDMap[node.Name] = node.ID
	}

	for i, r := range result.Relations {
		// 验证源和目标节点存在
		sourceID, ok := nodeIDMap[r.Source]
		if !ok {
			log.Printf("[DocumentProcessor] Warning: source node '%s' not found", r.Source)
			continue
		}
		targetID, ok := nodeIDMap[r.Target]
		if !ok {
			log.Printf("[DocumentProcessor] Warning: target node '%s' not found", r.Target)
			continue
		}

		graphData.Relation[i] = &domain_knowledge.GraphRelation{
			ID:          s.idGenerator.Generate(),
			Source:      sourceID,
			Target:      targetID,
			Type:        r.Type,
			Strength:    r.Strength,
			ChunkIDs:    []string{}, // 将在调用方设置
			Description: r.Description,
		}
	}

	return graphData, nil
}
