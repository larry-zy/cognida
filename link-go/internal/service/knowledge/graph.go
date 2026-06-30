package knowledge

import (
	"context"
	"fmt"
	"log"
	"sync"

	domain_knowledge "link/internal/model/knowledge"
)

// ========================================
// Graph Service - Application Orchestration Layer
// ========================================

// GraphService 图谱应用服务（纯编排层）
type GraphService struct {
	graphRepo      domain_knowledge.GraphRepository        // 图谱仓储
	graphQueryRepo domain_knowledge.GraphQueryRepository   // 图谱查询仓储
	chunkRepo      ChunkRepository                          // Chunk 仓储
	domainService  *domain_knowledge.GraphService           // 领域服务
	llmExtractor   domain_knowledge.GraphExtractor          // 图谱抽取器（经接口注入）
}

// ChunkRepository Chunk 仓储接口（用于图谱关联查询）
type ChunkRepository interface {
	FindEnabledChunks(ctx context.Context, kbID string, limit int) ([]Chunk, error)
}

// Chunk 简化的 Chunk 接口
type Chunk interface {
	GetID() string
	GetContent() string
}

// NewGraphService 创建图谱服务实例
func NewGraphService(graphRepo domain_knowledge.GraphRepository, extractor domain_knowledge.GraphExtractor) *GraphService {
	return &GraphService{
		graphRepo:     graphRepo,
		domainService: domain_knowledge.NewGraphService(),
		llmExtractor:  extractor,
	}
}

// NewGraphServiceWithQuery 创建图谱服务实例（包含查询仓储）
func NewGraphServiceWithQuery(
	graphRepo domain_knowledge.GraphRepository,
	graphQueryRepo domain_knowledge.GraphQueryRepository,
	extractor domain_knowledge.GraphExtractor,
) *GraphService {
	return &GraphService{
		graphRepo:      graphRepo,
		graphQueryRepo: graphQueryRepo,
		domainService:  domain_knowledge.NewGraphService(),
		llmExtractor:   extractor,
	}
}

// NewGraphServiceWithChunks 创建图谱服务实例（包含chunk仓储）
func NewGraphServiceWithChunks(
	graphRepo domain_knowledge.GraphRepository,
	graphQueryRepo domain_knowledge.GraphQueryRepository,
	chunkRepo ChunkRepository,
	extractor domain_knowledge.GraphExtractor,
) *GraphService {
	return &GraphService{
		graphRepo:      graphRepo,
		graphQueryRepo: graphQueryRepo,
		chunkRepo:      chunkRepo,
		domainService:  domain_knowledge.NewGraphService(),
		llmExtractor:   extractor,
	}
}

// GetGraphRepo 获取图谱仓储
func (s *GraphService) GetGraphRepo() domain_knowledge.GraphRepository {
	return s.graphRepo
}

// GetGraphQueryRepo 获取图谱查询仓储
func (s *GraphService) GetGraphQueryRepo() domain_knowledge.GraphQueryRepository {
	return s.graphQueryRepo
}

// ========================================
// Graph Extraction (Orchestration)
// ========================================

// ExtractGraphFromChunks 从文档块中提取图谱（并发处理）
// 编排层：协调 LLM 提取器和领域服务
func (s *GraphService) ExtractGraphFromChunks(
	ctx context.Context,
	inputs []*domain_knowledge.ChunkExtractionInput,
) (*domain_knowledge.GraphData, error) {
	return s.ExtractGraphFromChunksWithMode(ctx, inputs, domain_knowledge.ExtractionModeDocument)
}

// ExtractGraphFromChunksWithQuery 从文档块中提取图谱（查询模式）
func (s *GraphService) ExtractGraphFromChunksWithQuery(
	ctx context.Context,
	inputs []*domain_knowledge.ChunkExtractionInput,
) (*domain_knowledge.GraphData, error) {
	return s.ExtractGraphFromChunksWithMode(ctx, inputs, domain_knowledge.ExtractionModeQuery)
}

// ExtractGraphFromChunksWithMode 从文档块中提取图谱（指定模式）
func (s *GraphService) ExtractGraphFromChunksWithMode(
	ctx context.Context,
	inputs []*domain_knowledge.ChunkExtractionInput,
	mode domain_knowledge.ExtractionMode,
) (*domain_knowledge.GraphData, error) {
	if len(inputs) == 0 {
		return &domain_knowledge.GraphData{}, nil
	}

	log.Printf("[GraphService] Starting concurrent extraction for %d chunks, mode=%s", len(inputs), mode)

	// 并发提取每个文档块的图谱
	results := s.concurrentExtract(ctx, inputs, mode)

	// 过滤掉失败的结果
	validResults := make([]*domain_knowledge.ExtractedGraphData, 0, len(results))
	for _, result := range results {
		if result != nil && result.Error == nil {
			validResults = append(validResults, result)
		}
	}

	log.Printf("[GraphService] Extraction completed: %d/%d chunks succeeded", len(validResults), len(inputs))

	// 调用领域服务合并图谱（业务逻辑在领域层）
	mergedGraph, err := s.domainService.MergeExtractedGraphs(validResults)
	if err != nil {
		return nil, fmt.Errorf("failed to merge extracted graphs: %w", err)
	}

	log.Printf("[GraphService] Merged graph: %d nodes, %d relations",
		len(mergedGraph.Node), len(mergedGraph.Relation))

	return mergedGraph, nil
}

// ExtractGraphJoint 从文档块中联合提取图谱（单次 LLM 调用）
// 提高一致性，减少 LLM 调用次数
func (s *GraphService) ExtractGraphJoint(
	ctx context.Context,
	inputs []*domain_knowledge.ChunkExtractionInput,
) (*domain_knowledge.GraphData, error) {
	if len(inputs) == 0 {
		return &domain_knowledge.GraphData{}, nil
	}

	log.Printf("[GraphService] Starting joint extraction for %d chunks", len(inputs))

	// 使用第一个输入的命名空间获取已有上下文
	var existingContext *domain_knowledge.ExtractionContext
	if len(inputs) > 0 && inputs[0] != nil {
		// ChunkExtractionInput 只有 KnowledgeBaseID，没有 TenantID 和 KnowledgeID
		// 使用默认 TenantID，Knowledge 为空
		namespace := domain_knowledge.NameSpace{
			TenantID: "default", // 可以从其他地方获取
		 KnowledgeBaseID:     inputs[0].KnowledgeBaseID,
			Knowledge: "",
		}
		ctx, err := s.graphRepo.GetEntityContext(ctx, namespace)
		if err == nil && ctx != nil {
			existingContext = ctx
			log.Printf("[GraphService] Loaded existing context: %d entities, %d types",
				len(ctx.ExistingEntities), len(ctx.EntityTypes))
		}
	}

	// 并发提取
	results := s.concurrentExtractJoint(ctx, inputs, existingContext)

	// 过滤失败结果
	validResults := make([]*domain_knowledge.ExtractedGraphData, 0, len(results))
	for _, result := range results {
		if result != nil && result.Error == nil {
			validResults = append(validResults, result)
		}
	}

	log.Printf("[GraphService] Joint extraction completed: %d/%d chunks succeeded", len(validResults), len(inputs))

	// 合并图谱
	mergedGraph, err := s.domainService.MergeExtractedGraphs(validResults)
	if err != nil {
		return nil, fmt.Errorf("failed to merge extracted graphs: %w", err)
	}

	log.Printf("[GraphService] Joint extraction result: %d nodes, %d relations",
		len(mergedGraph.Node), len(mergedGraph.Relation))

	return mergedGraph, nil
}

// ExtractGraphIncremental 从文档块中增量提取图谱（基于已有上下文）
// 使用已有图谱知识，提高提取质量
func (s *GraphService) ExtractGraphIncremental(
	ctx context.Context,
	inputs []*domain_knowledge.ChunkExtractionInput,
) (*domain_knowledge.GraphData, error) {
	if len(inputs) == 0 {
		return &domain_knowledge.GraphData{}, nil
	}

	log.Printf("[GraphService] Starting incremental extraction for %d chunks", len(inputs))

	// 获取第一个输入的命名空间
	var namespace domain_knowledge.NameSpace
	if len(inputs) > 0 && inputs[0] != nil {
		namespace = domain_knowledge.NameSpace{
			TenantID: "default", // 可以从其他地方获取
		 KnowledgeBaseID:     inputs[0].KnowledgeBaseID,
			Knowledge: "",
		}
	}

	// 获取已有上下文
	existingContext, err := s.graphRepo.GetEntityContext(ctx, namespace)
	if err != nil {
		log.Printf("[GraphService] Warning: failed to get entity context, using joint extraction: %v", err)
		return s.ExtractGraphJoint(ctx, inputs)
	}

	if existingContext == nil || len(existingContext.ExistingEntities) == 0 {
		log.Printf("[GraphService] No existing context, using joint extraction")
		return s.ExtractGraphJoint(ctx, inputs)
	}

	log.Printf("[GraphService] Loaded context: %d entities, %d relations, %d entity types",
		len(existingContext.ExistingEntities),
		len(existingContext.RelationTypes),
		len(existingContext.EntityTypes))

	// 并发提取
	results := s.concurrentExtractIncremental(ctx, inputs, existingContext)

	// 过滤失败结果
	validResults := make([]*domain_knowledge.ExtractedGraphData, 0, len(results))
	for _, result := range results {
		if result != nil && result.Error == nil {
			validResults = append(validResults, result)
		}
	}

	log.Printf("[GraphService] Incremental extraction completed: %d/%d chunks succeeded", len(validResults), len(inputs))

	// 合并图谱
	mergedGraph, err := s.domainService.MergeExtractedGraphs(validResults)
	if err != nil {
		return nil, fmt.Errorf("failed to merge extracted graphs: %w", err)
	}

	log.Printf("[GraphService] Incremental extraction result: %d nodes, %d relations",
		len(mergedGraph.Node), len(mergedGraph.Relation))

	return mergedGraph, nil
}

// concurrentExtractJoint 并发联合提取
func (s *GraphService) concurrentExtractJoint(
	ctx context.Context,
	inputs []*domain_knowledge.ChunkExtractionInput,
	existingContext *domain_knowledge.ExtractionContext,
) []*domain_knowledge.ExtractedGraphData {
	semaphore := make(chan struct{}, 4)
	var wg sync.WaitGroup

	results := make([]*domain_knowledge.ExtractedGraphData, len(inputs))

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, inp *domain_knowledge.ChunkExtractionInput) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			log.Printf("[GraphService] Joint extracting chunk %s", inp.ChunkID)

			// 联合提取
			nodes, relations, err := s.llmExtractor.ExtractGraphJoint(ctx, inp.ChunkID, inp.Document, existingContext)
			if err != nil {
				log.Printf("[GraphService] Error in joint extraction for chunk %s: %v", inp.ChunkID, err)
				results[idx] = &domain_knowledge.ExtractedGraphData{
					ChunkID: inp.ChunkID,
					Error:   err,
				}
				return
			}

			results[idx] = &domain_knowledge.ExtractedGraphData{
				ChunkID:   inp.ChunkID,
				Nodes:     nodes,
				Relations: relations,
			}

			log.Printf("[GraphService] Completed joint extraction for chunk %s: %d nodes, %d relations",
				inp.ChunkID, len(nodes), len(relations))
		}(i, input)
	}

	wg.Wait()
	return results
}

// concurrentExtractIncremental 并发增量提取
func (s *GraphService) concurrentExtractIncremental(
	ctx context.Context,
	inputs []*domain_knowledge.ChunkExtractionInput,
	existingContext *domain_knowledge.ExtractionContext,
) []*domain_knowledge.ExtractedGraphData {
	semaphore := make(chan struct{}, 4)
	var wg sync.WaitGroup

	results := make([]*domain_knowledge.ExtractedGraphData, len(inputs))

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, inp *domain_knowledge.ChunkExtractionInput) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			log.Printf("[GraphService] Incremental extracting chunk %s", inp.ChunkID)

			// 增量提取
			nodes, relations, err := s.llmExtractor.ExtractGraphIncremental(ctx, inp.ChunkID, inp.Document, existingContext)
			if err != nil {
				log.Printf("[GraphService] Error in incremental extraction for chunk %s: %v", inp.ChunkID, err)
				results[idx] = &domain_knowledge.ExtractedGraphData{
					ChunkID: inp.ChunkID,
					Error:   err,
				}
				return
			}

			results[idx] = &domain_knowledge.ExtractedGraphData{
				ChunkID:   inp.ChunkID,
				Nodes:     nodes,
				Relations: relations,
			}

			log.Printf("[GraphService] Completed incremental extraction for chunk %s: %d nodes, %d relations",
				inp.ChunkID, len(nodes), len(relations))
		}(i, input)
	}

	wg.Wait()
	return results
}

// concurrentExtract 并发提取图谱
func (s *GraphService) concurrentExtract(
	ctx context.Context,
	inputs []*domain_knowledge.ChunkExtractionInput,
	mode domain_knowledge.ExtractionMode,
) []*domain_knowledge.ExtractedGraphData {
	// 使用 semaphore 限制并发数为 4
	semaphore := make(chan struct{}, 4)
	var wg sync.WaitGroup

	results := make([]*domain_knowledge.ExtractedGraphData, len(inputs))

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, inp *domain_knowledge.ChunkExtractionInput) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 确定输入的模式
			extractionMode := inp.Mode
			if extractionMode == "" {
				extractionMode = mode
			}

			log.Printf("[GraphService] Processing chunk %s, mode=%s", inp.ChunkID, extractionMode)

			// 调用基础设施层提取实体
			nodes, err := s.llmExtractor.ExtractEntities(ctx, inp.ChunkID, inp.Document, inp.Query, extractionMode)
			if err != nil {
				log.Printf("[GraphService] Error extracting entities from chunk %s: %v", inp.ChunkID, err)
				results[idx] = &domain_knowledge.ExtractedGraphData{
					ChunkID: inp.ChunkID,
					Error:   err,
				}
				return
			}

			// 调用基础设施层提取关系
			relations, err := s.llmExtractor.ExtractRelations(ctx, inp.KnowledgeBaseID, inp.ChunkID, inp.Document, inp.Query, nodes, extractionMode)
			if err != nil {
				log.Printf("[GraphService] Error extracting relations from chunk %s: %v", inp.ChunkID, err)
				// 不中断流程，使用部分结果
			}

			results[idx] = &domain_knowledge.ExtractedGraphData{
				ChunkID:   inp.ChunkID,
				Nodes:     nodes,
				Relations: relations,
			}

			log.Printf("[GraphService] Completed chunk %s: %d nodes, %d relations",
				inp.ChunkID, len(nodes), len(relations))
		}(i, input)
	}

	wg.Wait()
	return results
}

// ========================================
// Graph Repository Operations (Delegation)
// ========================================

// AddGraph 添加图谱数据
func (s *GraphService) AddGraph(ctx context.Context, namespace domain_knowledge.NameSpace, graphs []*domain_knowledge.GraphData) error {
	log.Printf("[GraphService] AddGraph START: namespace.KnowledgeBaseID=%s, len(graphs)=%d", namespace.KnowledgeBaseID, len(graphs))

	// 验证图谱数据
	for _, graph := range graphs {
		if err := graph.Validate(); err != nil {
			return fmt.Errorf("graph validation failed: %w", err)
		}
	}

	err := s.graphRepo.AddGraph(ctx, namespace, graphs)

	if err != nil {
		log.Printf("[GraphService] AddGraph ERROR from Repo: %v", err)
		return err
	}

	log.Printf("[GraphService] AddGraph SUCCESS")
	return nil
}

// DeleteGraph 删除图谱数据
func (s *GraphService) DeleteGraph(ctx context.Context, namespaces []domain_knowledge.NameSpace) error {
	return s.graphRepo.DeleteGraph(ctx, namespaces)
}

// GetGraph 获取完整图谱数据
func (s *GraphService) GetGraph(ctx context.Context, namespace domain_knowledge.NameSpace) (*domain_knowledge.GraphData, error) {
	return s.graphRepo.GetGraph(ctx, namespace)
}

// SearchNode 搜索节点（按节点名称列表）
// 使用 SearchNodes 方法进行全文搜索
func (s *GraphService) SearchNode(
	ctx context.Context,
	namespace domain_knowledge.NameSpace,
	nodes []string,
) (*domain_knowledge.GraphData, error) {
	// 使用新的 SearchNodes API
	// 对于节点名称列表，我们获取所有匹配的节点
	var allNodes []*domain_knowledge.GraphNode
	for _, nodeName := range nodes {
		foundNodes, err := s.graphRepo.SearchNodes(ctx, namespace, nodeName, &domain_knowledge.NodeQueryOptions{
			Limit: 100, // 足够大的限制
		})
		if err != nil {
			continue // 忽略错误，继续查找其他节点
		}
		allNodes = append(allNodes, foundNodes...)
	}

	// 获取这些节点之间的关系
	if len(allNodes) == 0 {
		return &domain_knowledge.GraphData{}, nil
	}

	// 获取完整的图谱数据以包含关系
	graph, err := s.graphRepo.GetGraph(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// 过滤出匹配的节点和它们的关系
	nodeMap := make(map[string]*domain_knowledge.GraphNode)
	for _, node := range allNodes {
		nodeMap[node.Name] = node
	}

	var filteredRelations []*domain_knowledge.GraphRelation
	for _, rel := range graph.Relation {
		if _, ok := nodeMap[rel.Source]; ok {
			filteredRelations = append(filteredRelations, rel)
		}
	}

	// 转换 map 到切片
	var resultNodes []*domain_knowledge.GraphNode
	for _, node := range nodeMap {
		resultNodes = append(resultNodes, node)
	}

	return &domain_knowledge.GraphData{
		Node:     resultNodes,
		Relation: filteredRelations,
	}, nil
}

// SearchPath 搜索路径（使用新的 FindShortestPath API）
func (s *GraphService) SearchPath(
	ctx context.Context,
	namespace domain_knowledge.NameSpace,
	startNode, endNode string,
	maxDepth int,
) ([]*domain_knowledge.GraphData, error) {
	// 使用新的 FindShortestPath API
	result, err := s.graphRepo.FindShortestPath(ctx, namespace, startNode, endNode, &domain_knowledge.PathQueryOptions{
		MaxDepth: maxDepth,
	})
	if err != nil {
		return nil, err
	}

	if result == nil || len(result.Nodes) == 0 {
		return []*domain_knowledge.GraphData{}, nil
	}

	// PathQueryResult 直接包含 Nodes 和 Relations
	graphData := &domain_knowledge.GraphData{
		Node:     result.Nodes,
		Relation: result.Relations,
	}

	return []*domain_knowledge.GraphData{graphData}, nil
}

// CheckHealth 检查图谱服务健康状态
func (s *GraphService) CheckHealth(ctx context.Context) error {
	return s.graphRepo.CheckHealth(ctx)
}

// UpdateNode 更新节点属性
func (s *GraphService) UpdateNode(ctx context.Context, namespace domain_knowledge.NameSpace, node *domain_knowledge.GraphNode) error {
	return s.graphRepo.UpdateNode(ctx, namespace, node)
}

// AddRelation 添加单个关系
func (s *GraphService) AddRelation(ctx context.Context, namespace domain_knowledge.NameSpace, relation *domain_knowledge.GraphRelation) (*domain_knowledge.GraphRelation, error) {
	log.Printf("[GraphService] AddRelation START: namespace.KnowledgeBaseID=%s, ID=%s, Source=%q, Target=%q, Type=%q, Strength=%f",
		namespace.KnowledgeBaseID, relation.ID, relation.Source, relation.Target, relation.Type, relation.Strength)

	result, err := s.graphRepo.AddRelation(ctx, namespace, relation)

	if err != nil {
		log.Printf("[GraphService] AddRelation ERROR from Repo: %v", err)
		return nil, err
	}

	if result == nil {
		log.Printf("[GraphService] AddRelation WARNING: Repo returned nil")
	} else {
		log.Printf("[GraphService] AddRelation SUCCESS: returning ID=%s, Source=%q, Target=%q, Type=%q, Strength=%f, Weight=%f",
			result.ID, result.Source, result.Target, result.Type, result.Strength, result.Weight)
	}

	return result, nil
}

// AddNode 添加单个节点
func (s *GraphService) AddNode(ctx context.Context, namespace domain_knowledge.NameSpace, node *domain_knowledge.GraphNode) error {
	log.Printf("[GraphService] AddNode START: namespace.KnowledgeBaseID=%s, ID=%s, Name=%q, EntityType=%q",
		namespace.KnowledgeBaseID, node.ID, node.Name, node.EntityType)

	err := s.graphRepo.AddNode(ctx, namespace, node)

	if err != nil {
		log.Printf("[GraphService] AddNode ERROR from Repo: %v", err)
		return err
	}

	log.Printf("[GraphService] AddNode SUCCESS: ID=%s, Name=%q", node.ID, node.Name)
	return nil
}

// UpdateRelation 更新关系属性
func (s *GraphService) UpdateRelation(ctx context.Context, namespace domain_knowledge.NameSpace, relation *domain_knowledge.GraphRelation) (*domain_knowledge.GraphRelation, error) {
	log.Printf("[GraphService] UpdateRelation START: namespace.KnowledgeBaseID=%s, relation.ID=%s, relation.Type=%q",
		namespace.KnowledgeBaseID, relation.ID, relation.Type)

	result, err := s.graphRepo.UpdateRelation(ctx, namespace, relation)

	if err != nil {
		log.Printf("[GraphService] UpdateRelation ERROR from Repo: %v", err)
		return nil, err
	}

	if result == nil {
		log.Printf("[GraphService] UpdateRelation WARNING: Repo returned nil")
	} else {
		log.Printf("[GraphService] UpdateRelation SUCCESS: returning ID=%s, Source=%q, Target=%q, Type=%q, Strength=%f, Weight=%f",
			result.ID, result.Source, result.Target, result.Type, result.Strength, result.Weight)
	}

	return result, nil
}

// DeleteNode 删除单个节点
func (s *GraphService) DeleteNode(ctx context.Context, namespace domain_knowledge.NameSpace, nodeID string) error {
	log.Printf("[GraphService] DeleteNode START: namespace.KnowledgeBaseID=%s, node_id=%s", namespace.KnowledgeBaseID, nodeID)

	err := s.graphRepo.DeleteNode(ctx, namespace, nodeID)

	if err != nil {
		log.Printf("[GraphService] DeleteNode ERROR: %v", err)
		return err
	}

	log.Printf("[GraphService] DeleteNode SUCCESS: node_id=%s", nodeID)

	return nil
}

// DeleteRelation 删除单个关系
func (s *GraphService) DeleteRelation(ctx context.Context, namespace domain_knowledge.NameSpace, relationID string) error {
	log.Printf("[GraphService] DeleteRelation START: namespace.KnowledgeBaseID=%s, relation_id=%s", namespace.KnowledgeBaseID, relationID)

	err := s.graphRepo.DeleteRelation(ctx, namespace, relationID)

	if err != nil {
		log.Printf("[GraphService] DeleteRelation ERROR: %v", err)
		return err
	}

	log.Printf("[GraphService] DeleteRelation SUCCESS: relation_id=%s", relationID)

	return nil
}

// ========================================
// Delete by Chunk/Knowledge ID
// ========================================

// DeleteByChunkID 删除与指定 chunk_id 相关的所有图谱数据
func (s *GraphService) DeleteByChunkID(ctx context.Context, namespace domain_knowledge.NameSpace, chunkID string) error {
	log.Printf("[GraphService] DeleteByChunkID START: namespace.KnowledgeBaseID=%s, chunk_id=%s", namespace.KnowledgeBaseID, chunkID)

	err := s.graphRepo.DeleteByChunkID(ctx, namespace, chunkID)

	if err != nil {
		log.Printf("[GraphService] DeleteByChunkID ERROR: %v", err)
		return err
	}

	log.Printf("[GraphService] DeleteByChunkID SUCCESS: chunk_id=%s", chunkID)

	return nil
}

// DeleteByKnowledgeID 删除与指定 knowledge_id 相关的所有图谱数据
func (s *GraphService) DeleteByKnowledgeID(ctx context.Context, namespace domain_knowledge.NameSpace, knowledgeID string) error {
	log.Printf("[GraphService] DeleteByKnowledgeID START: namespace.KnowledgeBaseID=%s, knowledge_id=%s", namespace.KnowledgeBaseID, knowledgeID)

	err := s.graphRepo.DeleteByKnowledgeID(ctx, namespace, knowledgeID)

	if err != nil {
		log.Printf("[GraphService] DeleteByKnowledgeID ERROR: %v", err)
		return err
	}

	log.Printf("[GraphService] DeleteByKnowledgeID SUCCESS: knowledge_id=%s", knowledgeID)

	return nil
}

// ========================================
// Graph Query Operations (Delegation)
// ========================================

// GetChunksByGraphNodes 根据图谱节点名称获取关联的分片
func (s *GraphService) GetChunksByGraphNodes(ctx context.Context, kbID string, nodeNames []string) ([]*domain_knowledge.Chunk, error) {
	if s.graphQueryRepo == nil {
		return nil, fmt.Errorf("GraphQueryRepository 未初始化")
	}
	return s.graphQueryRepo.GetChunksByGraphNodes(ctx, kbID, nodeNames)
}

// GetKnowledgeByGraphNodes 根据图谱节点名称获取关联的知识条目
func (s *GraphService) GetKnowledgeByGraphNodes(ctx context.Context, kbID string, nodeNames []string) (*domain_knowledge.Knowledge, error) {
	if s.graphQueryRepo == nil {
		return nil, fmt.Errorf("GraphQueryRepository 未初始化")
	}
	return s.graphQueryRepo.GetKnowledgeByGraphNodes(ctx, kbID, nodeNames)
}

// GetGraphStats 获取图谱统计信息
func (s *GraphService) GetGraphStats(ctx context.Context, kbID string) (*domain_knowledge.DetailedGraphStats, error) {
	if s.graphQueryRepo == nil {
		return nil, fmt.Errorf("GraphQueryRepository 未初始化")
	}
	return s.graphQueryRepo.GetGraphStats(ctx, kbID)
}

// CalculateRelationStatistics 计算关系统计信息（使用领域服务）
func (s *GraphService) CalculateRelationStatistics(relations []*domain_knowledge.GraphRelation) *RelationStatistics {
	stats := s.domainService.CalculateRelationStatistics(relations)
	return &RelationStatistics{
		TotalRelations:       stats.TotalRelations,
		AverageStrength:      stats.AverageStrength,
		AverageWeight:        stats.AverageWeight,
		AverageChunkCount:    stats.AverageChunkCount,
		StrengthDistribution: stats.StrengthDistribution,
	}
}

// RelationStatistics 关系统计信息
type RelationStatistics struct {
	TotalRelations       int             // 总关系数
	AverageStrength      float64         // 平均强度
	AverageWeight        float64         // 平均权重
	AverageChunkCount    float64         // 平均每关系关联的文档块数
	StrengthDistribution map[int]int     // 强度分布 (1-10 各有多少关系)
}

// FindCommonChunks 查找两个实体共同出现的文档块
func (s *GraphService) FindCommonChunks(
	ctx context.Context,
	namespace domain_knowledge.NameSpace,
	entity1, entity2 string,
) ([]string, error) {
	graph, err := s.graphRepo.GetGraph(ctx, namespace)
	if err != nil {
		return nil, err
	}

	var node1, node2 *domain_knowledge.GraphNode
	for _, node := range graph.Node {
		if node.Name == entity1 {
			node1 = node
		}
		if node.Name == entity2 {
			node2 = node
		}
	}

	if node1 == nil || node2 == nil {
		return []string{}, nil
	}

	return s.domainService.FindCommonChunks(node1, node2), nil
}
