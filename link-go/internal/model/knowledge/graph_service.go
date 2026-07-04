// Package knowledge provides the knowledge management domain graph service
package knowledge

import (
	"fmt"
	"math"
	"slices"
)

// ========================================
// Graph Service - Domain Business Logic
// ========================================

// GraphService 图谱领域服务，处理图谱相关的业务逻辑
type GraphService struct{}

// NewGraphService 创建图谱领域服务
func NewGraphService() *GraphService {
	return &GraphService{}
}

// ========================================
// Graph Merging Logic
// ========================================

// MergeExtractedGraphs 合并多个提取的图谱数据
// 这是核心业务逻辑：合并节点、关系到，并计算 PMI 和 Weight
func (s *GraphService) MergeExtractedGraphs(dataList []*ExtractedGraphData) (*GraphData, error) {
	if len(dataList) == 0 {
		return &GraphData{}, nil
	}

	totalChunks := len(dataList)

	// 使用局部变量进行合并
	localNodes := make(map[string]*GraphNode, 64)
	localRelations := make(map[string]*GraphRelation, 128)

	// 第一阶段：合并节点
	for _, data := range dataList {
		for _, node := range data.Nodes {
			if existingNode, exists := localNodes[node.Name]; exists {
				// 合并 chunks（去重）
				for _, chunk := range node.Chunks {
					if !slices.Contains(existingNode.Chunks, chunk) {
						existingNode.Chunks = append(existingNode.Chunks, chunk)
					}
				}
				// 合并 attributes（去重）
				for _, attr := range node.Attributes {
					if !slices.Contains(existingNode.Attributes, attr) {
						existingNode.Attributes = append(existingNode.Attributes, attr)
					}
				}
			} else {
				localNodes[node.Name] = node
			}
		}
	}

	// 第二阶段：合并关系到局部变量
	for _, data := range dataList {
		for _, rel := range data.Relations {
			key := fmt.Sprintf("%s#%s", rel.Source, rel.Target)

			if existingRel, exists := localRelations[key]; exists {
				// 合并 chunk_ids（去重）
				for _, chunkID := range rel.ChunkIDs {
					if !slices.Contains(existingRel.ChunkIDs, chunkID) {
						existingRel.ChunkIDs = append(existingRel.ChunkIDs, chunkID)
					}
				}
				// 加权平均更新 strength
				if existingRel.Strength > 0 && rel.Strength > 0 {
					existingRel.Strength = (existingRel.Strength + rel.Strength) / 2
				} else if rel.Strength > 0 {
					existingRel.Strength = rel.Strength
				}
			} else {
				localRelations[key] = rel
			}
		}
	}

	// 第三阶段：计算 PMI、Weight 和 CombinedDegree
	s.calculateRelationMetrics(localNodes, localRelations, totalChunks)

	// 第四阶段：构建结果
	result := &GraphData{
		Node:     make([]*GraphNode, 0, len(localNodes)),
		Relation: make([]*GraphRelation, 0, len(localRelations)),
	}

	for _, node := range localNodes {
		result.Node = append(result.Node, node)
	}

	for _, rel := range localRelations {
		result.Relation = append(result.Relation, rel)
	}

	return result, nil
}

// calculateRelationMetrics 计算关系指标（PMI、Weight、CombinedDegree）
// 这是领域业务逻辑的核心算法
func (s *GraphService) calculateRelationMetrics(
	nodes map[string]*GraphNode,
	relations map[string]*GraphRelation,
	totalChunks int,
) {
	// 统计实体出现的文档块数
	entityChunkCount := make(map[string]int, len(nodes))
	for _, node := range nodes {
		entityChunkCount[node.Name] = len(node.Chunks)
	}

	// 统计实体对共同出现的文档块数
	coOccurrenceCount := make(map[string]int, len(relations))
	for _, rel := range relations {
		sourceChunks := nodes[rel.Source].Chunks
		targetChunks := nodes[rel.Target].Chunks
		coOccurrenceCount[fmt.Sprintf("%s#%s", rel.Source, rel.Target)] = len(intersect(sourceChunks, targetChunks))
	}

	// 计算每个关系的 PMI、Weight 和 CombinedDegree
	for _, rel := range relations {
		key := fmt.Sprintf("%s#%s", rel.Source, rel.Target)

		// 计算概率
		p_x_y := float64(coOccurrenceCount[key]) / float64(totalChunks)
		p_x := float64(entityChunkCount[rel.Source]) / float64(totalChunks)
		p_y := float64(entityChunkCount[rel.Target]) / float64(totalChunks)

		// PMI = log2(P(x,y) / (P(x) * P(y)))
		var pmi float64
		if p_x > 0 && p_y > 0 && p_x_y > 0 {
			pmi = math.Log2(p_x_y / (p_x * p_y))
		}

		// 归一化 PMI 到 [0, 1]（假设 PMI 范围为 [-5, 10]）
		normalizedPMI := (pmi + 5) / 15
		normalizedPMI = math.Max(0, math.Min(1, normalizedPMI))

		// 归一化 Strength 到 [0, 1]（假设范围 [1, 10]）
		normalizedStrength := (rel.Strength - 1) / 9
		normalizedStrength = math.Max(0, math.Min(1, normalizedStrength))

		// Weight = 1.0 + 9.0 * (normalizedPMI * 0.6 + normalizedStrength * 0.4)
		rel.Weight = 1.0 + 9.0*(normalizedPMI*0.6+normalizedStrength*0.4)

		// 保存 PMI
		rel.PMI = pmi

		// 计算 CombinedDegree
		sourceDegree := 0
		targetDegree := 0
		for _, r := range relations {
			if r.Source == rel.Source || r.Target == rel.Source {
				sourceDegree++
			}
			if r.Source == rel.Target || r.Target == rel.Target {
				targetDegree++
			}
		}
		rel.CombinedDegree = sourceDegree + targetDegree
	}
}

// intersect 计算两个字符串数组的交集
func intersect(a, b []string) []string {
	set := make(map[string]bool)
	for _, item := range a {
		set[item] = true
	}

	result := make([]string, 0)
	for _, item := range b {
		if set[item] {
			result = append(result, item)
		}
	}
	return result
}

// ========================================
// Graph Validation
// ========================================

// ValidateNodes 验证节点列表
func (s *GraphService) ValidateNodes(nodes []*GraphNode) error {
	for _, node := range nodes {
		if node.Name == "" {
			return &GraphValidationError{Field: "node.name", Message: "节点名称不能为空"}
		}
		if node.ID == "" {
			return &GraphValidationError{Field: "node.id", Message: "节点ID不能为空"}
		}
	}
	return nil
}

// ValidateRelations 验证关系列表
func (s *GraphService) ValidateRelations(relations []*GraphRelation, nodeNames map[string]bool) error {
	for _, rel := range relations {
		if rel.Source == "" {
			return &GraphValidationError{Field: "relation.source", Message: "关系源节点不能为空"}
		}
		if rel.Target == "" {
			return &GraphValidationError{Field: "relation.target", Message: "关系目标节点不能为空"}
		}
		if rel.ID == "" {
			return &GraphValidationError{Field: "relation.id", Message: "关系ID不能为空"}
		}
		// 检查节点是否存在
		if nodeNames != nil {
			if !nodeNames[rel.Source] {
				return &GraphValidationError{Field: "relation.source", Message: "源节点不存在: " + rel.Source}
			}
			if !nodeNames[rel.Target] {
				return &GraphValidationError{Field: "relation.target", Message: "目标节点不存在: " + rel.Target}
			}
		}
	}
	return nil
}

// ========================================
// Graph Statistics
// ========================================

// CalculateRelationStatistics 计算关系统计信息
func (s *GraphService) CalculateRelationStatistics(relations []*GraphRelation) *RelationStatistics {
	if len(relations) == 0 {
		return &RelationStatistics{}
	}

	stats := &RelationStatistics{
		TotalRelations:       len(relations),
		StrengthDistribution: make(map[int]int),
		TypeDistribution:     make(map[string]int),
	}

	totalStrength := 0.0
	totalWeight := 0.0
	totalChunkCount := 0

	for _, rel := range relations {
		totalStrength += rel.Strength
		totalWeight += rel.Weight
		totalChunkCount += len(rel.ChunkIDs)

		strengthBucket := int(rel.Strength)
		stats.StrengthDistribution[strengthBucket]++
		stats.TypeDistribution[rel.Type]++
	}

	stats.AverageStrength = totalStrength / float64(stats.TotalRelations)
	stats.AverageWeight = totalWeight / float64(stats.TotalRelations)
	stats.AverageChunkCount = float64(totalChunkCount) / float64(stats.TotalRelations)

	return stats
}

// CalculateDetailedStats 计算详细图谱统计信息
func (s *GraphService) CalculateDetailedStats(graph *GraphData) *DetailedGraphStats {
	if graph == nil {
		return &DetailedGraphStats{}
	}

	stats := &DetailedGraphStats{
		NodeCount:     len(graph.Node),
		RelationCount: len(graph.Relation),
	}

	if stats.RelationCount > 0 {
		totalStrength := 0.0
		totalWeight := 0.0
		degreeMap := make(map[string]int)

		for _, rel := range graph.Relation {
			totalStrength += rel.Strength
			totalWeight += rel.Weight
			degreeMap[rel.Source]++
			degreeMap[rel.Target]++
		}

		stats.AvgStrength = totalStrength / float64(stats.RelationCount)
		stats.AvgWeight = totalWeight / float64(stats.RelationCount)

		// 计算平均度
		totalDegree := 0
		maxDegree := 0
		for _, degree := range degreeMap {
			totalDegree += degree
			if degree > maxDegree {
				maxDegree = degree
			}
		}
		stats.MaxDegree = maxDegree
		if len(degreeMap) > 0 {
			stats.AvgDegree = float64(totalDegree) / float64(len(degreeMap))
		}
	}

	// 计算孤立节点
	isolatedCount := 0
	nodeInRelation := make(map[string]bool)
	for _, rel := range graph.Relation {
		nodeInRelation[rel.Source] = true
		nodeInRelation[rel.Target] = true
	}
	for _, node := range graph.Node {
		if !nodeInRelation[node.Name] {
			isolatedCount++
		}
	}
	stats.IsolatedNodes = isolatedCount

	// 计算连通分量数（并查集，节点以 Name 为键，孤立节点各自成一个分量）
	stats.ComponentCount = countConnectedComponents(graph)

	return stats
}

// countConnectedComponents 用并查集统计图中连通分量数量。
// 关系端点以节点 Name 关联；仅统计出现在 Node 列表中的节点所在分量，
// 孤立节点（无任何关系）各自计为一个独立分量。
func countConnectedComponents(graph *GraphData) int {
	if len(graph.Node) == 0 {
		return 0
	}

	parent := make(map[string]string, len(graph.Node))
	var find func(string) string
	find = func(x string) string {
		p, ok := parent[x]
		if !ok {
			parent[x] = x
			return x
		}
		if p != x {
			parent[x] = find(p)
		}
		return parent[x]
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	// 先让每个节点自成一集
	for _, node := range graph.Node {
		find(node.Name)
	}
	// 按关系合并（端点名可能不在 Node 列表中，一并纳入以正确反映连通性）
	for _, rel := range graph.Relation {
		union(rel.Source, rel.Target)
	}

	// 仅统计真实节点所属的根，避免把游离端点名算成额外分量
	roots := make(map[string]struct{}, len(graph.Node))
	for _, node := range graph.Node {
		roots[find(node.Name)] = struct{}{}
	}
	return len(roots)
}

// ========================================
// Entity-Chunk Association
// ========================================

// FindCommonChunks 查找两个实体共同出现的文档块
func (s *GraphService) FindCommonChunks(node1, node2 *GraphNode) []string {
	if node1 == nil || node2 == nil {
		return []string{}
	}
	return intersect(node1.Chunks, node2.Chunks)
}

// GetRelatedEntities 获取与指定实体相关的所有实体
func (s *GraphService) GetRelatedEntities(entityName string, relations []*GraphRelation) []string {
	related := make([]string, 0)
	seen := make(map[string]bool)

	for _, rel := range relations {
		if rel.Source == entityName {
			if !seen[rel.Target] {
				related = append(related, rel.Target)
				seen[rel.Target] = true
			}
		}
		if rel.Target == entityName {
			if !seen[rel.Source] {
				related = append(related, rel.Source)
				seen[rel.Source] = true
			}
		}
	}

	return related
}
