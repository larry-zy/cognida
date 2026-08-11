package neo4j

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"cognida/internal/model/knowledge"
)

// GetGraph 获取完整图谱数据
func (r *Neo4jRepository) GetGraph(ctx context.Context, namespace knowledge.NameSpace) (*knowledge.GraphData, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	// 查询节点
	// 加有界 LIMIT（PF-1）：大知识库若无上限会一次性把全部节点物化进内存。
	// 无分页入参，统一用 defaultGraphLoadLimit 兜底。
	nodeCypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		RETURN n.id as id, n.name as name, n.entity_type as entityType,
		       n.chunks as chunks, n.properties as properties
		ORDER BY n.name
		LIMIT $limit
	`

	nodesResult, err := session.Run(ctx, nodeCypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
		"limit":    defaultGraphLoadLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("查询节点失败: %w", err)
	}

	nodes := make([]*knowledge.GraphNode, 0)
	nodeNames := make([]string, 0)
	for nodesResult.Next(ctx) {
		record := nodesResult.Record()

		id := getStringValue(record, "id")
		name := getStringValue(record, "name")
		entityType := getStringValue(record, "entityType")
		chunks := getStringSliceValue(record, "chunks") // chunks 为 Neo4j 原生 list〔GR-1〕
		propsStr := getStringValue(record, "properties")

		if id == "" {
			continue
		}

		var properties map[string]string
		_ = json.Unmarshal([]byte(propsStr), &properties) // 解析失败保持零值

		nodes = append(nodes, &knowledge.GraphNode{
			ID:         id,
			Name:       name,
			EntityType: entityType,
			Chunks:     chunks,
			Properties: properties,
		})
		nodeNames = append(nodeNames, name)
	}

	// 节点集为空则无需查关系，直接返回（避免空 IN 列表全表扫描）。
	if len(nodeNames) == 0 {
		return &knowledge.GraphData{Node: nodes, Relation: make([]*knowledge.GraphRelation, 0)}, nil
	}

	// 查询关系
	// 关键修正〔#4〕：节点与关系各自独立 LIMIT 会让返回的边引用到未在节点集中的实体，
	// 前端据此物化出「幽灵节点/悬空边」。此处把关系两端约束在已返回的节点名集合内
	// （s.name/t.name IN $names），保证每条边的两端都在 nodes 中；仍加 LIMIT 兜底防边集放大。
	relCypher := `
		MATCH (s:Entity {tenant_id: $tenantId, kb_id: $kbId})-[r:RELATION]->(t:Entity {tenant_id: $tenantId, kb_id: $kbId})
		WHERE s.name IN $names AND t.name IN $names
		RETURN r.id as id, s.name as source, t.name as target, r.type as type,
		       r.strength as strength, r.weight as weight, r.chunk_ids as chunkIds,
		       r.properties as properties
		ORDER BY r.weight DESC
		LIMIT $limit
	`

	relResult, err := session.Run(ctx, relCypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
		"names":    nodeNames,
		"limit":    defaultGraphLoadLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("查询关系失败: %w", err)
	}

	relations := make([]*knowledge.GraphRelation, 0)
	for relResult.Next(ctx) {
		record := relResult.Record()

		id := getStringValue(record, "id")
		source := getStringValue(record, "source")
		target := getStringValue(record, "target")
		relType := getStringValue(record, "type")
		strength := getFloat64Value(record, "strength")
		weight := getFloat64Value(record, "weight")
		chunkIdsStr := getStringValue(record, "chunkIds")
		propsStr := getStringValue(record, "properties")

		if id == "" || source == "" || target == "" {
			continue
		}

		var chunkIds []string
		_ = json.Unmarshal([]byte(chunkIdsStr), &chunkIds) // 解析失败保持零值

		var properties map[string]string
		_ = json.Unmarshal([]byte(propsStr), &properties) // 解析失败保持零值

		relations = append(relations, &knowledge.GraphRelation{
			ID:         id,
			Source:     source,
			Target:     target,
			Type:       relType,
			Strength:   strength,
			Weight:     weight,
			ChunkIDs:   chunkIds,
			Properties: properties,
		})
	}

	return &knowledge.GraphData{
		Node:     nodes,
		Relation: relations,
	}, nil
}

// SearchNode 搜索节点
func (r *Neo4jRepository) SearchNode(ctx context.Context, namespace knowledge.NameSpace, nodeNames []string) (*knowledge.GraphData, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	nodes := make([]*knowledge.GraphNode, 0)
	nodeSet := make(map[string]bool)

	// 批量查询节点
	for _, nodeName := range nodeNames {
		cypher := `
			MATCH (n:Entity {name: $name, tenant_id: $tenantId, kb_id: $kbId})
			RETURN n.id as id, n.name as name, n.entity_type as entityType,
			       n.chunks as chunks, n.properties as properties
			LIMIT $limit
		`

		result, err := session.Run(ctx, cypher, map[string]interface{}{
			"name":     nodeName,
			"tenantId": namespace.TenantID,
			"kbId":     namespace.KnowledgeBaseID,
			"limit":    defaultGraphLoadLimit,
		})
		if err != nil {
			continue
		}

		for result.Next(ctx) {
			record := result.Record()

			id := getStringValue(record, "id")
			name := getStringValue(record, "name")
			entityType := getStringValue(record, "entityType")
			chunks := getStringSliceValue(record, "chunks") // chunks 为 Neo4j 原生 list〔GR-1〕
			propsStr := getStringValue(record, "properties")

			if id == "" || nodeSet[id] {
				continue
			}

			var properties map[string]string
			_ = json.Unmarshal([]byte(propsStr), &properties) // 解析失败保持零值

			nodes = append(nodes, &knowledge.GraphNode{
				ID:         id,
				Name:       name,
				EntityType: entityType,
				Chunks:     chunks,
				Properties: properties,
			})
			nodeSet[id] = true
		}
	}

	// 查询这些节点之间的关系
	relations := make([]*knowledge.GraphRelation, 0)
	if len(nodes) > 0 {
		// 加有界 LIMIT（PF-1）：命中节点间的关系数上界为 O(命中数²)，加上限兜底防止内存放大。
		relCypher := `
			MATCH (s:Entity)-[r:RELATION]->(t:Entity)
			WHERE s.name IN $names AND t.name IN $names
			  AND s.tenant_id = $tenantId AND s.kb_id = $kbId
			  AND t.tenant_id = $tenantId AND t.kb_id = $kbId
			RETURN r.id as id, s.name as source, t.name as target, r.type as type,
			       r.strength as strength, r.weight as weight, r.chunk_ids as chunkIds,
			       r.properties as properties
			LIMIT $limit
		`

		relResult, err := session.Run(ctx, relCypher, map[string]interface{}{
			"names":    nodeNames,
			"tenantId": namespace.TenantID,
			"kbId":     namespace.KnowledgeBaseID,
			"limit":    defaultGraphLoadLimit,
		})
		if err == nil {
			for relResult.Next(ctx) {
				record := relResult.Record()

				id := getStringValue(record, "id")
				source := getStringValue(record, "source")
				target := getStringValue(record, "target")
				relType := getStringValue(record, "type")
				strength := getFloat64Value(record, "strength")
				weight := getFloat64Value(record, "weight")
				chunkIdsStr := getStringValue(record, "chunkIds")
				propsStr := getStringValue(record, "properties")

				if id == "" || source == "" || target == "" {
					continue
				}

				var chunkIds []string
				_ = json.Unmarshal([]byte(chunkIdsStr), &chunkIds) // 解析失败保持零值

				var properties map[string]string
				_ = json.Unmarshal([]byte(propsStr), &properties) // 解析失败保持零值

				relations = append(relations, &knowledge.GraphRelation{
					ID:         id,
					Source:     source,
					Target:     target,
					Type:       relType,
					Strength:   strength,
					Weight:     weight,
					ChunkIDs:   chunkIds,
					Properties: properties,
				})
			}
		}
	}

	return &knowledge.GraphData{
		Node:     nodes,
		Relation: relations,
	}, nil
}

// SearchNodeV2 search nodes (improved - returns node names directly)
func (r *Neo4jRepository) SearchNodeV2(ctx context.Context, namespace knowledge.NameSpace, nodeNames []string) (*knowledge.GraphData, error) {
	// Use the same implementation as SearchNode for now
	return r.SearchNode(ctx, namespace, nodeNames)
}

// SearchNodes search nodes with full-text index support
func (r *Neo4jRepository) SearchNodes(ctx context.Context, namespace knowledge.NameSpace, query string, opts *knowledge.NodeQueryOptions) ([]*knowledge.GraphNode, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	if opts == nil {
		opts = &knowledge.NodeQueryOptions{}
	}

	// 兜底默认上限：当调用方未显式指定 Limit（opts 非空但 Limit<=0）时，
	// 若不限制将退化为整命名空间扫描并返回全部匹配节点，存在大结果集风险。
	// 统一在此处兜底，与 MySQL 图存储 SearchNodes 的默认上限语义保持一致。
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultSearchNodeLimit
	}

	// Use simple name matching for compatibility (full-text index might not be available)
	cypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		WHERE n.name CONTAINS $query
	`

	// Add entity type filter if specified
	if len(opts.EntityTypes) > 0 {
		cypher += " AND n.entity_type IN $entityTypes"
	}

	cypher += `
		RETURN n.id as id, n.name as name, n.entity_type as entityType,
		       n.chunks as chunks, n.attributes as attributes, n.properties as properties
		ORDER BY n.name
	`

	// Cypher 要求 SKIP 在 LIMIT 之前；LIMIT 现在总是存在以保证结果集有界。
	if opts.Offset > 0 {
		cypher += " SKIP $offset"
	}
	cypher += " LIMIT $limit"

	params := map[string]interface{}{
		"query":    query,
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
		"limit":    limit,
		"offset":   opts.Offset,
	}

	if len(opts.EntityTypes) > 0 {
		params["entityTypes"] = opts.EntityTypes
	}

	result, err := session.Run(ctx, cypher, params)
	if err != nil {
		return nil, fmt.Errorf("全文搜索失败: %w", err)
	}

	nodes := make([]*knowledge.GraphNode, 0)
	for result.Next(ctx) {
		record := result.Record()

		id := getStringValue(record, "id")
		name := getStringValue(record, "name")
		entityType := getStringValue(record, "entityType")
		chunks := getStringSliceValue(record, "chunks") // chunks 为 Neo4j 原生 list〔GR-1〕
		attrsStr := getStringValue(record, "attributes")
		propsStr := getStringValue(record, "properties")

		if id == "" {
			continue
		}

		var attributes []string
		_ = json.Unmarshal([]byte(attrsStr), &attributes) // 解析失败保持零值

		var properties map[string]string
		_ = json.Unmarshal([]byte(propsStr), &properties) // 解析失败保持零值

		nodes = append(nodes, &knowledge.GraphNode{
			ID:         id,
			Name:       name,
			EntityType: entityType,
			Attributes: attributes,
			Chunks:     chunks,
			Properties: properties,
		})
	}

	return nodes, nil
}

// GetNeighbors get neighbor nodes with filters
func (r *Neo4jRepository) GetNeighbors(ctx context.Context, namespace knowledge.NameSpace, nodeName string, opts *knowledge.RelationQueryOptions) (*knowledge.NodeQueryResult, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	if opts == nil {
		opts = &knowledge.RelationQueryOptions{Limit: 100}
	}

	// 中心节点必存在；关系用 OPTIONAL MATCH 双向匹配，孤立节点也能返回中心本身。
	cypher := `
		MATCH (n:Entity {name: $name, tenant_id: $tenantId, kb_id: $kbId})
		OPTIONAL MATCH (n)-[r:RELATION]-(m:Entity {tenant_id: $tenantId, kb_id: $kbId})
	`

	params := map[string]interface{}{
		"name":     nodeName,
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	}

	// 过滤条件需允许 r 为 NULL（保留中心节点），故整体以 (r IS NULL OR (...)) 包裹
	filters := make([]string, 0, 2)
	if len(opts.Types) > 0 {
		filters = append(filters, "r.type IN $types")
		params["types"] = opts.Types
	}
	if opts.MinWeight > 0 {
		filters = append(filters, "r.weight >= $minWeight")
		params["minWeight"] = opts.MinWeight
	}
	if len(filters) > 0 {
		cypher += " WHERE r IS NULL OR (" + strings.Join(filters, " AND ") + ")"
	}

	cypher += `
		RETURN n.id as centerId, n.entity_type as centerType, n.chunks as centerChunks, n.attributes as centerAttrs,
		       m.id as id, m.name as name, m.entity_type as entityType, m.chunks as chunks,
		       r.id as relId, r.type as relType, r.strength as relStrength, r.weight as relWeight, r.chunk_ids as relChunks,
		       startNode(r).name as relSource, endNode(r).name as relTarget
		ORDER BY relWeight DESC
	`

	// Cypher 要求 SKIP 在 LIMIT 之前，顺序不能颠倒
	if opts.Offset > 0 {
		cypher += " SKIP $offset"
		params["offset"] = opts.Offset
	}
	if opts.Limit > 0 {
		cypher += " LIMIT $limit"
		params["limit"] = opts.Limit
	}

	result, err := session.Run(ctx, cypher, params)
	if err != nil {
		return nil, fmt.Errorf("查询邻居节点失败: %w", err)
	}

	center := &knowledge.GraphNode{Name: nodeName}
	centerLoaded := false
	neighbors := make([]*knowledge.GraphNode, 0)
	relations := make([]*knowledge.GraphRelation, 0)
	degree := 0

	for result.Next(ctx) {
		record := result.Record()

		// 中心节点属性（每行相同，取一次）
		if !centerLoaded {
			center.ID = getStringValue(record, "centerId")
			center.EntityType = getStringValue(record, "centerType")
			center.Chunks = getStringSliceValue(record, "centerChunks") // chunks 为 Neo4j 原生 list〔GR-1〕
			var centerAttrs []string
			_ = json.Unmarshal([]byte(getStringValue(record, "centerAttrs")), &centerAttrs) // 解析失败保持零值
			center.Attributes = centerAttrs
			centerLoaded = true
		}

		relID := getStringValue(record, "relId")
		if relID == "" {
			// 孤立节点：无关系行
			continue
		}
		degree++

		id := getStringValue(record, "id")
		name := getStringValue(record, "name")
		entityType := getStringValue(record, "entityType")
		relType := getStringValue(record, "relType")
		relStrength := getFloat64Value(record, "relStrength")
		relWeight := getFloat64Value(record, "relWeight")
		relSource := getStringValue(record, "relSource")
		relTarget := getStringValue(record, "relTarget")

		chunks := getStringSliceValue(record, "chunks") // chunks 为 Neo4j 原生 list〔GR-1〕

		var relChunks []string
		_ = json.Unmarshal([]byte(getStringValue(record, "relChunks")), &relChunks) // 解析失败保持零值

		neighbors = append(neighbors, &knowledge.GraphNode{
			ID:         id,
			Name:       name,
			EntityType: entityType,
			Chunks:     chunks,
		})

		relations = append(relations, &knowledge.GraphRelation{
			ID:       relID,
			Source:   relSource,
			Target:   relTarget,
			Type:     relType,
			Strength: relStrength,
			Weight:   relWeight,
			ChunkIDs: relChunks,
		})
	}

	return &knowledge.NodeQueryResult{
		Node:      center,
		Neighbors: neighbors,
		Relations: relations,
		Degree:    degree,
	}, nil
}

// GetNodeCommunity get community membership for a node
func (r *Neo4jRepository) GetNodeCommunity(ctx context.Context, namespace knowledge.NameSpace, nodeName string) (*knowledge.Community, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (n:Entity {name: $name, tenant_id: $tenantId, kb_id: $kbId})-[r:MEMBER_OF]->(c:Community {kb_id: $kbId})
		RETURN c.id as id, c.kb_id as kbId, c.name as name, c.size as size,
		       c.modularity as modularity, c.labels as labels
	`

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"name":     nodeName,
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询社区失败: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()

		labelsStr := getStringValue(record, "labels")
		var labels []string
		_ = json.Unmarshal([]byte(labelsStr), &labels) // 解析失败保持零值

		return &knowledge.Community{
			ID:              getStringValue(record, "id"),
			KnowledgeBaseID: getStringValue(record, "kbId"),
			Name:            getStringValue(record, "name"),
			Size:            int(getFloat64Value(record, "size")),
			Modularity:      getFloat64Value(record, "modularity"),
			Labels:          labels,
		}, nil
	}

	return nil, fmt.Errorf("节点不属于任何社区")
}

// GetEntityContext get existing entities for incremental extraction
func (r *Neo4jRepository) GetEntityContext(ctx context.Context, namespace knowledge.NameSpace) (*knowledge.ExtractionContext, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	// Get entity type distribution
	typeCypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		RETURN n.entity_type as type, count(n) as count
	`

	typeResult, err := session.Run(ctx, typeCypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询实体类型分布失败: %w", err)
	}

	entityTypes := make(map[string]int)
	for typeResult.Next(ctx) {
		record := typeResult.Record()
		entityType := getStringValue(record, "type")
		count := int(getFloat64Value(record, "count"))
		entityTypes[entityType] = count
	}

	// Get relation type distribution
	relCypher := `
		MATCH (s:Entity {tenant_id: $tenantId, kb_id: $kbId})-[r:RELATION]->(t:Entity {tenant_id: $tenantId, kb_id: $kbId})
		RETURN r.type as type, count(r) as count
	`

	relResult, err := session.Run(ctx, relCypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询关系类型分布失败: %w", err)
	}

	relationTypes := make(map[string]int)
	for relResult.Next(ctx) {
		record := relResult.Record()
		relType := getStringValue(record, "type")
		count := int(getFloat64Value(record, "count"))
		relationTypes[relType] = count
	}

	// Get existing entity names (limit to top 100 for context)
	entityCypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		RETURN DISTINCT n.name as name, n.entity_type as entityType
		LIMIT 100
	`

	entityResult, err := session.Run(ctx, entityCypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询实体列表失败: %w", err)
	}

	existingEntities := make([]string, 0)
	sampleEntities := make([]knowledge.EntitySample, 0)
	for entityResult.Next(ctx) {
		record := entityResult.Record()
		name := getStringValue(record, "name")
		entityType := getStringValue(record, "entityType")

		if name != "" {
			existingEntities = append(existingEntities, name)
			sampleEntities = append(sampleEntities, knowledge.EntitySample{
				Name:       name,
				EntityType: entityType,
			})
		}
	}

	return &knowledge.ExtractionContext{
		KnowledgeBaseID:  namespace.KnowledgeBaseID,
		ExistingEntities: existingEntities,
		EntityTypes:      entityTypes,
		RelationTypes:    relationTypes,
		SampleEntities:   sampleEntities,
	}, nil
}
