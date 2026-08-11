package neo4j

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"cognida/internal/model/knowledge"
)

// GetGraphStats get basic graph statistics
func (r *Neo4jRepository) GetGraphStats(ctx context.Context, namespace knowledge.NameSpace) (*knowledge.GraphStats, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		WITH count(n) as nodeCount
		OPTIONAL MATCH ()-[r:RELATION {tenant_id: $tenantId, kb_id: $kbId}]-()
		RETURN nodeCount, count(DISTINCT r) as relationCount
	`

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询图谱统计失败: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		nodeCount := getFloat64Value(record, "nodeCount")
		relationCount := getFloat64Value(record, "relationCount")
		return &knowledge.GraphStats{
			NodeCount:     int64(nodeCount),
			RelationCount: int64(relationCount),
			ChunkCount:    0, // Not calculated in this simplified query
		}, nil
	}

	return &knowledge.GraphStats{}, nil
}

// GetDegreeStats get degree statistics
func (r *Neo4jRepository) GetDegreeStats(ctx context.Context, namespace knowledge.NameSpace) (*knowledge.DegreeStats, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		OPTIONAL MATCH (n)-[r:RELATION]-()
		WITH n, count(r) as degree
		WITH collect(degree) as degrees, avg(degree) as avgDegree, max(degree) as maxDegree, min(degree) as minDegree
		RETURN avgDegree, maxDegree, minDegree,
		       size([d IN degrees WHERE d = 0]) as isolatedNodes,
		       size([d IN degrees WHERE d > 2 * avgDegree]) as highDegreeNodes
	`

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询度统计失败: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &knowledge.DegreeStats{
			AverageDegree:   getFloat64Value(record, "avgDegree"),
			MaxDegree:       int(getFloat64Value(record, "maxDegree")),
			MinDegree:       int(getFloat64Value(record, "minDegree")),
			IsolatedNodes:   int(getFloat64Value(record, "isolatedNodes")),
			HighDegreeNodes: int(getFloat64Value(record, "highDegreeNodes")),
		}, nil
	}

	return &knowledge.DegreeStats{}, nil
}

// GetTypeDistribution get type distribution statistics
func (r *Neo4jRepository) GetTypeDistribution(ctx context.Context, namespace knowledge.NameSpace) (*knowledge.TypeDistribution, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	// Get entity type distribution
	entityCypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		RETURN n.entity_type as type, count(n) as count
		ORDER BY count DESC
	`

	entityResult, err := session.Run(ctx, entityCypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询实体类型分布失败: %w", err)
	}

	entityDist := make(map[string]int)
	topEntities := make([]knowledge.TypeCount, 0)
	for entityResult.Next(ctx) {
		record := entityResult.Record()
		entityType := getStringValue(record, "type")
		count := int(getFloat64Value(record, "count"))
		entityDist[entityType] = count
		topEntities = append(topEntities, knowledge.TypeCount{Type: entityType, Count: count})
	}

	// Get relation type distribution
	relCypher := `
		MATCH (s:Entity {tenant_id: $tenantId, kb_id: $kbId})-[r:RELATION]->(t:Entity {tenant_id: $tenantId, kb_id: $kbId})
		RETURN r.type as type, count(r) as count
		ORDER BY count DESC
	`

	relResult, err := session.Run(ctx, relCypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询关系类型分布失败: %w", err)
	}

	relDist := make(map[string]int)
	topRelations := make([]knowledge.TypeCount, 0)
	for relResult.Next(ctx) {
		record := relResult.Record()
		relType := getStringValue(record, "type")
		count := int(getFloat64Value(record, "count"))
		relDist[relType] = count
		topRelations = append(topRelations, knowledge.TypeCount{Type: relType, Count: count})
	}

	return &knowledge.TypeDistribution{
		EntityTypeDistribution:   entityDist,
		RelationTypeDistribution: relDist,
		TopEntityTypes:           topEntities,
		TopRelationTypes:         topRelations,
	}, nil
}

// GetDensityMetrics get density metrics
func (r *Neo4jRepository) GetDensityMetrics(ctx context.Context, namespace knowledge.NameSpace) (*knowledge.DensityMetrics, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		OPTIONAL MATCH (n)-[r:RELATION]-()
		WITH n, count(r) as degree
		WITH count(n) as nodeCount, sum(degree) as totalDegree
		WITH nodeCount, totalDegree, (totalDegree / 2.0) as edgeCount
		RETURN nodeCount, edgeCount,
		       CASE WHEN nodeCount > 1 THEN (2.0 * edgeCount) / (nodeCount * (nodeCount - 1)) ELSE 0 END as density
	`

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询密度指标失败: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		nodeCount := int(getFloat64Value(record, "nodeCount"))

		return &knowledge.DensityMetrics{
			Density:               getFloat64Value(record, "density"),
			ComponentCount:        1,         // Simplified - would require more complex query
			LargestComponentSize:  nodeCount, // Simplified
			ClusteringCoefficient: 0.0,       // Would require APOC or complex query
		}, nil
	}

	return &knowledge.DensityMetrics{}, nil
}

// GetCentralitySummaries get centrality summaries (pagerank, betweenness)
func (r *Neo4jRepository) GetCentralitySummaries(ctx context.Context, namespace knowledge.NameSpace, centralityType string) (*knowledge.CentralitySummary, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	var propName string
	switch centralityType {
	case "pagerank":
		propName = "pagerank"
	case "betweenness":
		propName = "betweenness"
	default:
		return nil, fmt.Errorf("不支持的中心性类型: %s", centralityType)
	}

	cypher := fmt.Sprintf(`
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		WHERE n.%s IS NOT NULL
		WITH collect({id: n.id, name: n.name, score: n.%s}) as nodes,
		     avg(n.%s) as avgScore, max(n.%s) as maxScore, min(n.%s) as minScore
		UNWIND nodes as node
		WITH avgScore, maxScore, minScore, node
		ORDER BY node.score DESC
		LIMIT 10
		RETURN avgScore, maxScore, minScore,
		       node.id as nodeId, node.name as nodeName, node.score as score
	`, propName, propName, propName, propName, propName)

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询中心性统计失败: %w", err)
	}

	topNodes := make([]knowledge.CentralityNode, 0)
	var min, max, avg float64

	for result.Next(ctx) {
		record := result.Record()
		nodeID := getStringValue(record, "nodeId")
		nodeName := getStringValue(record, "nodeName")
		score := getFloat64Value(record, "score")

		if len(topNodes) == 0 {
			min = getFloat64Value(record, "minScore")
			max = getFloat64Value(record, "maxScore")
			avg = getFloat64Value(record, "avgScore")
		}

		topNodes = append(topNodes, knowledge.CentralityNode{
			NodeID:   nodeID,
			NodeName: nodeName,
			Score:    score,
			Rank:     len(topNodes) + 1,
		})
	}

	return &knowledge.CentralitySummary{
		Min:      min,
		Max:      max,
		Average:  avg,
		TopNodes: topNodes,
	}, nil
}

// GetCommunityStats get community detection statistics
func (r *Neo4jRepository) GetCommunityStats(ctx context.Context, namespace knowledge.NameSpace) (*knowledge.CommunityStats, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (c:Community {kb_id: $kbId})
		RETURN count(c) as communityCount, avg(c.modularity) as modularity,
		       avg(c.size) as avgSize, max(c.size) as maxSize
	`

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"kbId": namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询社区统计失败: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &knowledge.CommunityStats{
			CommunityCount:       int(getFloat64Value(record, "communityCount")),
			Modularity:           getFloat64Value(record, "modularity"),
			AverageCommunitySize: getFloat64Value(record, "avgSize"),
			LargestCommunitySize: int(getFloat64Value(record, "maxSize")),
		}, nil
	}

	return &knowledge.CommunityStats{}, nil
}

// GetStatsByEntityType get statistics filtered by entity type
func (r *Neo4jRepository) GetStatsByEntityType(ctx context.Context, namespace knowledge.NameSpace, entityType string) (*knowledge.GraphStats, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId, entity_type: $entityType})
		OPTIONAL MATCH (n)-[r:RELATION]-()
		RETURN count(DISTINCT n) as nodeCount, count(DISTINCT r) as relationCount
	`

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"tenantId":   namespace.TenantID,
		"kbId":       namespace.KnowledgeBaseID,
		"entityType": entityType,
	})
	if err != nil {
		return nil, fmt.Errorf("查询实体类型统计失败: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		return &knowledge.GraphStats{
			NodeCount:     int64(getFloat64Value(record, "nodeCount")),
			RelationCount: int64(getFloat64Value(record, "relationCount")),
		}, nil
	}

	return &knowledge.GraphStats{}, nil
}
