// Package neo4j 提供 Graph 基础设施层 Neo4j 实现
package neo4j

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"link/internal/model/knowledge"
)

// ========================================
// Neo4j 仓储实现
// ========================================

// defaultSearchNodeLimit 是 SearchNodes 未显式指定 Limit 时的兜底上限，
// 用于防止未限制查询退化为整命名空间扫描返回全部节点的大结果集风险。
const defaultSearchNodeLimit = 100

// Neo4jRepository Neo4j 图谱仓储实现
type Neo4jRepository struct {
	driver   neo4j.DriverWithContext
	dbName   string
	cache    *graphCache
}

// graphCache 图谱缓存
type graphCache struct {
	nodes     map[string]*knowledge.GraphNode
	relations map[string]*knowledge.GraphRelation
	mu        sync.RWMutex
}

// NewNeo4jRepository 创建 Neo4j 仓储
func NewNeo4jRepository(uri, username, password, dbName string) (*Neo4jRepository, error) {
	driver, err := neo4j.NewDriverWithContext(
		uri,
		neo4j.BasicAuth(username, password, ""),
	)
	if err != nil {
		return nil, fmt.Errorf("创建 Neo4j 驱动失败: %w", err)
	}

	return NewNeo4jRepositoryFromDriver(driver, dbName)
}

// NewNeo4jRepositoryFromDriver 从现有驱动创建 Neo4j 仓储（用于依赖注入）
func NewNeo4jRepositoryFromDriver(driver neo4j.DriverWithContext, dbName string) (*Neo4jRepository, error) {
	repo := &Neo4jRepository{
		driver: driver,
		dbName: dbName,
		cache: &graphCache{
			nodes:     make(map[string]*knowledge.GraphNode),
			relations: make(map[string]*knowledge.GraphRelation),
		},
	}

	// Initialize indexes idempotently
	ctx := context.Background()
	if err := repo.InitIndexes(ctx); err != nil {
		return nil, fmt.Errorf("初始化索引失败: %w", err)
	}

	return repo, nil
}

// InitIndexes initialize indexes idempotently
func (r *Neo4jRepository) InitIndexes(ctx context.Context) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	// Tenant isolation indexes
	indexes := []struct {
		name       string
		label      string
		properties []string
		options    string
	}{
		{
			name:       "entity_tenant_kb",
			label:      "Entity",
			properties: []string{"tenant_id", "kb_id"},
			options:    "{unique: false}",
		},
		{
			name:       "entity_knowledge",
			label:      "Entity",
			properties: []string{"knowledge_id"},
			options:    "{unique: false}",
		},
		{
			name:       "entity_name_fulltext",
			label:      "Entity",
			properties: []string{"name"},
			options:    "{type: 'fulltext'}",
		},
		{
			name:       "relation_type_weight",
			label:      "RELATION",
			properties: []string{"type", "weight"},
			options:    "{unique: false}",
		},
		{
			name:       "community_kb",
			label:      "Community",
			properties: []string{"kb_id"},
			options:    "{unique: false}",
		},
		{
			name:       "community_pagerank",
			label:      "Community",
			properties: []string{"pagerank"},
			options:    "{unique: false}",
		},
		{
			name:       "entity_pagerank",
			label:      "Entity",
			properties: []string{"pagerank", "betweenness"},
			options:    "{unique: false}",
		},
	}

	for _, idx := range indexes {
		// Try to create index, ignore if exists
		// Use Neo4j 4.x compatible syntax (without OPTIONS clause)
		var cypher string
		if idx.label == "Entity" && len(idx.properties) == 1 && idx.properties[0] == "name" {
			// Regular index on name
			cypher = fmt.Sprintf("CREATE INDEX %s IF NOT EXISTS FOR (n:%s) ON (n.name)", idx.name, idx.label)
		} else {
			// Composite index
			cypher = fmt.Sprintf("CREATE INDEX %s IF NOT EXISTS FOR (n:%s)", idx.name, idx.label)
			if len(idx.properties) > 0 {
				props := strings.Join(idx.properties, ", n.")
				cypher += " ON (n." + props + ")"
			}
		}

		_, err := session.Run(ctx, cypher, nil)
		if err != nil {
			// Log but continue - index might already exist
			fmt.Printf("Warning: failed to create index %s: %v\n", idx.name, err)
		}
	}

	return nil
}

// Close 关闭连接
func (r *Neo4jRepository) Close(ctx context.Context) error {
	return r.driver.Close(ctx)
}

// ========================================
// 图谱操作实现
// ========================================

// AddGraph 添加图谱数据 (with single transaction and rollback)
func (r *Neo4jRepository) AddGraph(ctx context.Context, namespace knowledge.NameSpace, graphs []*knowledge.GraphData) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	// Use transaction for atomicity
	tx, err := session.BeginTransaction(ctx)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	defer func() {
		// Rollback on panic
		if r := recover(); r != nil {
			tx.Rollback(ctx)
			panic(r)
		}
	}()

	for _, graphData := range graphs {
		// Add nodes with merge logic for duplicate names
		for _, node := range graphData.Node {
			if node.ID == "" {
				continue
			}

			// Node merge: if node with same name exists, merge chunks and attributes
			cypher := `
				MERGE (n:Entity {name: $name, tenant_id: $tenantId, kb_id: $kbId})
				ON CREATE SET
					n.id = $id,
					n.name = $name,
					n.entity_type = $entityType,
					n.tenant_id = $tenantId,
					n.kb_id = $kbId,
					n.chunks = $chunks,
					n.attributes = $attributes,
					n.properties = $properties
				ON MATCH SET
					n.chunks = CASE WHEN $chunks IS NOT NULL THEN $chunks ELSE n.chunks END,
					n.attributes = CASE WHEN $attributes IS NOT NULL THEN $attributes ELSE n.attributes END,
					n.properties = CASE WHEN $properties IS NOT NULL THEN $properties ELSE n.properties END
				RETURN n
			`

			chunksJSON, _ := json.Marshal(node.Chunks)
			attrsJSON, _ := json.Marshal(node.Attributes)
			propsJSON, _ := json.Marshal(node.Properties)

			_, err := tx.Run(ctx, cypher, map[string]interface{}{
				"id":          node.ID,
				"name":        node.Name,
				"entityType":  node.EntityType,
				"tenantId":    namespace.TenantID,
				"kbId":        namespace.KnowledgeBaseID,
				"chunks":      string(chunksJSON),
				"attributes":  string(attrsJSON),
				"properties":  string(propsJSON),
			})
			if err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("添加节点失败: %w", err)
			}
		}

		// Add relations
		for _, rel := range graphData.Relation {
			if rel.ID == "" {
				continue
			}

			cypher := `
				MATCH (s:Entity {name: $source, tenant_id: $tenantId, kb_id: $kbId})
				MATCH (t:Entity {name: $target, tenant_id: $tenantId, kb_id: $kbId})
				MERGE (s)-[r:RELATION {id: $id, tenant_id: $tenantId, kb_id: $kbId}]->(t)
				ON CREATE SET
					r.type = $type,
					r.strength = $strength,
					r.weight = $weight,
					r.chunk_ids = $chunkIds,
					r.properties = $properties
				ON MATCH SET
					r.chunk_ids = CASE WHEN $chunkIds IS NOT NULL THEN $chunkIds ELSE r.chunk_ids END,
					r.strength = (r.strength + $strength) / 2
				RETURN r
			`

			chunkIDsJSON, _ := json.Marshal(rel.ChunkIDs)
			propsJSON, _ := json.Marshal(rel.Properties)

			_, err := tx.Run(ctx, cypher, map[string]interface{}{
				"id":         rel.ID,
				"source":     rel.Source,
				"target":     rel.Target,
				"type":       rel.Type,
				"strength":   rel.Strength,
				"weight":     rel.Weight,
				"chunkIds":   string(chunkIDsJSON),
				"properties": string(propsJSON),
				"tenantId":   namespace.TenantID,
				"kbId":       namespace.KnowledgeBaseID,
			})
			if err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("添加关系失败: %w", err)
			}
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	// Update cache
	r.updateCache(namespace, graphs)

	return nil
}

// DeleteGraph 删除图谱数据
func (r *Neo4jRepository) DeleteGraph(ctx context.Context, namespaces []knowledge.NameSpace) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	for _, namespace := range namespaces {
		cypher := `
			MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
			DETACH DELETE n
		`

		_, err := session.Run(ctx, cypher, map[string]interface{}{
			"tenantId": namespace.TenantID,
			"kbId":     namespace.KnowledgeBaseID,
		})
		if err != nil {
			return fmt.Errorf("删除图谱失败: %w", err)
		}
	}

	// 清除缓存
	for _, namespace := range namespaces {
		key := namespace.String()
		r.cache.mu.Lock()
		delete(r.cache.nodes, key)
		delete(r.cache.relations, key)
		r.cache.mu.Unlock()
	}

	return nil
}

// GetGraph 获取完整图谱数据
func (r *Neo4jRepository) GetGraph(ctx context.Context, namespace knowledge.NameSpace) (*knowledge.GraphData, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	// 查询节点
	nodeCypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		RETURN n.id as id, n.name as name, n.entity_type as entityType,
		       n.chunks as chunks, n.properties as properties
		ORDER BY n.name
	`

	nodesResult, err := session.Run(ctx, nodeCypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("查询节点失败: %w", err)
	}

	nodes := make([]*knowledge.GraphNode, 0)
	for nodesResult.Next(ctx) {
		record := nodesResult.Record()

		id := getStringValue(record, "id")
		name := getStringValue(record, "name")
		entityType := getStringValue(record, "entityType")
		chunksStr := getStringValue(record, "chunks")
		propsStr := getStringValue(record, "properties")

		if id == "" {
			continue
		}

		var chunks []string
		json.Unmarshal([]byte(chunksStr), &chunks)

		var properties map[string]string
		json.Unmarshal([]byte(propsStr), &properties)

		nodes = append(nodes, &knowledge.GraphNode{
			ID:         id,
			Name:       name,
			EntityType: entityType,
			Chunks:     chunks,
			Properties: properties,
		})
	}

	// 查询关系
	relCypher := `
		MATCH (s:Entity {tenant_id: $tenantId, kb_id: $kbId})-[r:RELATION]->(t:Entity {tenant_id: $tenantId, kb_id: $kbId})
		RETURN r.id as id, s.name as source, t.name as target, r.type as type,
		       r.strength as strength, r.weight as weight, r.chunk_ids as chunkIds,
		       r.properties as properties
		ORDER BY r.weight DESC
	`

	relResult, err := session.Run(ctx, relCypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
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
		json.Unmarshal([]byte(chunkIdsStr), &chunkIds)

		var properties map[string]string
		json.Unmarshal([]byte(propsStr), &properties)

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
		`

		result, err := session.Run(ctx, cypher, map[string]interface{}{
			"name":     nodeName,
			"tenantId": namespace.TenantID,
			"kbId":     namespace.KnowledgeBaseID,
		})
		if err != nil {
			continue
		}

		for result.Next(ctx) {
			record := result.Record()

			id := getStringValue(record, "id")
			name := getStringValue(record, "name")
			entityType := getStringValue(record, "entityType")
			chunksStr := getStringValue(record, "chunks")
			propsStr := getStringValue(record, "properties")

			if id == "" || nodeSet[id] {
				continue
			}

			var chunks []string
			json.Unmarshal([]byte(chunksStr), &chunks)

			var properties map[string]string
			json.Unmarshal([]byte(propsStr), &properties)

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
		relCypher := `
			MATCH (s:Entity)-[r:RELATION]->(t:Entity)
			WHERE s.name IN $names AND t.name IN $names
			  AND s.tenant_id = $tenantId AND s.kb_id = $kbId
			  AND t.tenant_id = $tenantId AND t.kb_id = $kbId
			RETURN r.id as id, s.name as source, t.name as target, r.type as type,
			       r.strength as strength, r.weight as weight, r.chunk_ids as chunkIds,
			       r.properties as properties
		`

		relResult, err := session.Run(ctx, relCypher, map[string]interface{}{
			"names":    nodeNames,
			"tenantId": namespace.TenantID,
			"kbId":     namespace.KnowledgeBaseID,
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
				json.Unmarshal([]byte(chunkIdsStr), &chunkIds)

				var properties map[string]string
				json.Unmarshal([]byte(propsStr), &properties)

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
		chunksStr := getStringValue(record, "chunks")
		attrsStr := getStringValue(record, "attributes")
		propsStr := getStringValue(record, "properties")

		if id == "" {
			continue
		}

		var chunks []string
		json.Unmarshal([]byte(chunksStr), &chunks)

		var attributes []string
		json.Unmarshal([]byte(attrsStr), &attributes)

		var properties map[string]string
		json.Unmarshal([]byte(propsStr), &properties)

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

	cypher := `
		MATCH (n:Entity {name: $name, tenant_id: $tenantId, kb_id: $kbId})-[r:RELATION]->(m:Entity {tenant_id: $tenantId, kb_id: $kbId})
	`

	params := map[string]interface{}{
		"name":     nodeName,
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	}

	// Add relation type filter
	if len(opts.Types) > 0 {
		cypher += " WHERE r.type IN $types"
		params["types"] = opts.Types
	}

	// Add weight filter
	if opts.MinWeight > 0 {
		if len(opts.Types) > 0 {
			cypher += " AND r.weight >= $minWeight"
		} else {
			cypher += " WHERE r.weight >= $minWeight"
		}
		params["minWeight"] = opts.MinWeight
	}

	cypher += `
		RETURN m.id as id, m.name as name, m.entity_type as entityType, m.chunks as chunks,
		       r.id as relId, r.type as relType, r.strength as relStrength, r.weight as relWeight
		ORDER BY relWeight DESC
	`

	if opts.Limit > 0 {
		cypher += " LIMIT $limit"
	}
	if opts.Offset > 0 {
		cypher += " SKIP $offset"
	}
	params["limit"] = opts.Limit
	params["offset"] = opts.Offset

	result, err := session.Run(ctx, cypher, params)
	if err != nil {
		return nil, fmt.Errorf("查询邻居节点失败: %w", err)
	}

	neighbors := make([]*knowledge.GraphNode, 0)
	relations := make([]*knowledge.GraphRelation, 0)
	degree := 0

	for result.Next(ctx) {
		record := result.Record()
		degree++

		id := getStringValue(record, "id")
		name := getStringValue(record, "name")
		entityType := getStringValue(record, "entityType")
		chunksStr := getStringValue(record, "chunks")
		relID := getStringValue(record, "relId")
		relType := getStringValue(record, "relType")
		relStrength := getFloat64Value(record, "relStrength")
		relWeight := getFloat64Value(record, "relWeight")

		var chunks []string
		json.Unmarshal([]byte(chunksStr), &chunks)

		neighbors = append(neighbors, &knowledge.GraphNode{
			ID:         id,
			Name:       name,
			EntityType: entityType,
			Chunks:     chunks,
		})

		relations = append(relations, &knowledge.GraphRelation{
			ID:       relID,
			Source:   nodeName,
			Target:   name,
			Type:     relType,
			Strength: relStrength,
			Weight:   relWeight,
		})
	}

	return &knowledge.NodeQueryResult{
		Node:      &knowledge.GraphNode{Name: nodeName},
		Neighbors: neighbors,
		Relations: relations,
		Degree:    degree,
	}, nil
}

// SearchPath 搜索路径
func (r *Neo4jRepository) SearchPath(ctx context.Context, namespace knowledge.NameSpace, startNode, endNode string, maxDepth int) ([]*knowledge.GraphData, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	if maxDepth <= 0 {
		maxDepth = 3
	}

	cypher := fmt.Sprintf(`
		MATCH (start:Entity {name: $start, tenant_id: $tenantId, kb_id: $kbId}),
		      (end:Entity {name: $end, tenant_id: $tenantId, kb_id: $kbId})
		MATCH path = shortestPath((start)-[*1..%d]-(end))
		RETURN [node in nodes(path) | node.name] as nodeNames,
		       [rel in relationships(path) | rel.type] as relTypes,
		       [rel in relationships(path) | rel.weight] as relWeights
	`, maxDepth)

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"start":    startNode,
		"end":      endNode,
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("路径查询失败: %w", err)
	}

	paths := make([]*knowledge.GraphData, 0)

	for result.Next(ctx) {
		record := result.Record()

		nodeNames := getStringSliceValue(record, "nodeNames")
		relTypes := getStringSliceValue(record, "relTypes")

		if len(nodeNames) == 0 {
			continue
		}

		// 构建节点
		nodes := make([]*knowledge.GraphNode, len(nodeNames))
		for i, name := range nodeNames {
			nodes[i] = &knowledge.GraphNode{
				Name: name,
			}
		}

		// 构建关系
		relations := make([]*knowledge.GraphRelation, len(relTypes)-1)
		for i := 0; i < len(relTypes)-1; i++ {
			relations[i] = &knowledge.GraphRelation{
				Source: nodeNames[i],
				Target: nodeNames[i+1],
				Type:   relTypes[i],
			}
		}

		paths = append(paths, &knowledge.GraphData{
			Node:     nodes,
			Relation: relations,
		})
	}

	return paths, nil
}

// FindShortestPath find shortest path between nodes
func (r *Neo4jRepository) FindShortestPath(ctx context.Context, namespace knowledge.NameSpace, startNode, endNode string, opts *knowledge.PathQueryOptions) (*knowledge.PathQueryResult, error) {
	if opts == nil {
		opts = &knowledge.PathQueryOptions{MaxDepth: 3}
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 3
	}

	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := fmt.Sprintf(`
		MATCH (start:Entity {name: $start, tenant_id: $tenantId, kb_id: $kbId}),
		      (end:Entity {name: $end, tenant_id: $tenantId, kb_id: $kbId})
		MATCH path = shortestPath((start)-[*1..%d]-(end))
		RETURN [node in nodes(path) | {id: node.id, name: node.name, entityType: node.entity_type}] as nodes,
		       [rel in relationships(path) | {id: rel.id, type: rel.type, strength: rel.strength, weight: rel.weight}] as relations,
		       length(path) as length,
		       reduce(weight = 0.0, r IN relationships(path) | weight + coalesce(r.weight, 0.0)) as weight
	`, opts.MaxDepth)

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"start":    startNode,
		"end":      endNode,
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("最短路径查询失败: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()

		nodesData := getInterfaceSliceValue(record, "nodes")
		relationsData := getInterfaceSliceValue(record, "relations")
		length := int(getFloat64Value(record, "length"))
		weight := getFloat64Value(record, "weight")

		nodes := make([]*knowledge.GraphNode, 0)
		if nodesData != nil {
			for _, n := range nodesData {
				if nodeMap, ok := n.(map[string]interface{}); ok {
					nodes = append(nodes, &knowledge.GraphNode{
						ID:         getValueAsString(nodeMap, "id"),
						Name:       getValueAsString(nodeMap, "name"),
						EntityType: getValueAsString(nodeMap, "entityType"),
					})
				}
			}
		}

		relations := make([]*knowledge.GraphRelation, 0)
		if relationsData != nil {
			for _, r := range relationsData {
				if relMap, ok := r.(map[string]interface{}); ok {
					relations = append(relations, &knowledge.GraphRelation{
						ID:       getValueAsString(relMap, "id"),
						Type:     getValueAsString(relMap, "type"),
						Strength: getValueAsFloat64(relMap, "strength"),
						Weight:   getValueAsFloat64(relMap, "weight"),
					})
				}
			}
		}

		// Set source/target for relations
		for i, rel := range relations {
			if i < len(nodes)-1 {
				rel.Source = nodes[i].Name
				rel.Target = nodes[i+1].Name
			}
		}

		return &knowledge.PathQueryResult{
			Nodes:     nodes,
			Relations: relations,
			Length:    length,
			Weight:    weight,
		}, nil
	}

	return nil, fmt.Errorf("未找到路径")
}

// FindKShortestPaths find K shortest paths between nodes
func (r *Neo4jRepository) FindKShortestPaths(ctx context.Context, namespace knowledge.NameSpace, startNode, endNode string, k int, opts *knowledge.PathQueryOptions) ([]*knowledge.PathQueryResult, error) {
	if opts == nil {
		opts = &knowledge.PathQueryOptions{MaxDepth: 3, MaxResults: k}
	}
	if k <= 0 {
		k = 3
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 3
	}

	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := fmt.Sprintf(`
		MATCH (start:Entity {name: $start, tenant_id: $tenantId, kb_id: $kbId}),
		      (end:Entity {name: $end, tenant_id: $tenantId, kb_id: $kbId})
		MATCH path = (start)-[*1..%d]-(end)
		WITH path, reduce(weight = 0.0, r IN relationships(path) | weight + coalesce(r.weight, 0.0)) as totalWeight
		ORDER BY totalWeight ASC
		LIMIT $k
		RETURN [node in nodes(path) | {id: node.id, name: node.name, entityType: node.entity_type}] as nodes,
		       [rel in relationships(path) | {id: rel.id, type: rel.type, strength: rel.strength, weight: rel.weight}] as relations,
		       length(path) as length,
		       totalWeight as weight
	`, opts.MaxDepth)

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"start":    startNode,
		"end":      endNode,
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
		"k":        k,
	})
	if err != nil {
		return nil, fmt.Errorf("K短路径查询失败: %w", err)
	}

	paths := make([]*knowledge.PathQueryResult, 0)

	for result.Next(ctx) {
		record := result.Record()

		nodesData := getInterfaceSliceValue(record, "nodes")
		relationsData := getInterfaceSliceValue(record, "relations")
		length := int(getFloat64Value(record, "length"))
		weight := getFloat64Value(record, "weight")

		nodes := make([]*knowledge.GraphNode, 0)
		if nodesData != nil {
			for _, n := range nodesData {
				if nodeMap, ok := n.(map[string]interface{}); ok {
					nodes = append(nodes, &knowledge.GraphNode{
						ID:         getValueAsString(nodeMap, "id"),
						Name:       getValueAsString(nodeMap, "name"),
						EntityType: getValueAsString(nodeMap, "entityType"),
					})
				}
			}
		}

		relations := make([]*knowledge.GraphRelation, 0)
		if relationsData != nil {
			for _, r := range relationsData {
				if relMap, ok := r.(map[string]interface{}); ok {
					relations = append(relations, &knowledge.GraphRelation{
						ID:       getValueAsString(relMap, "id"),
						Type:     getValueAsString(relMap, "type"),
						Strength: getValueAsFloat64(relMap, "strength"),
						Weight:   getValueAsFloat64(relMap, "weight"),
					})
				}
			}
		}

		// Set source/target for relations
		for i, rel := range relations {
			if i < len(nodes)-1 {
				rel.Source = nodes[i].Name
				rel.Target = nodes[i+1].Name
			}
		}

		paths = append(paths, &knowledge.PathQueryResult{
			Nodes:     nodes,
			Relations: relations,
			Length:    length,
			Weight:    weight,
		})
	}

	return paths, nil
}

// FindPathWithTypes find path with relation type constraints
func (r *Neo4jRepository) FindPathWithTypes(ctx context.Context, namespace knowledge.NameSpace, startNode, endNode string, relationTypes []string, maxDepth int) ([]*knowledge.PathQueryResult, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}

	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := fmt.Sprintf(`
		MATCH (start:Entity {name: $start, tenant_id: $tenantId, kb_id: $kbId}),
		      (end:Entity {name: $end, tenant_id: $tenantId, kb_id: $kbId})
		MATCH path = (start)-[r*1..%d]-(end)
		WHERE ALL(rel IN relationships(path) WHERE rel.type IN $types)
		RETURN [node in nodes(path) | {id: node.id, name: node.name, entityType: node.entity_type}] as nodes,
		       [rel in relationships(path) | {id: rel.id, type: rel.type, strength: rel.strength, weight: rel.weight}] as relations,
		       length(path) as length,
		       reduce(weight = 0.0, r IN relationships(path) | weight + coalesce(r.weight, 0.0)) as weight
		ORDER BY weight ASC
	`, maxDepth)

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"start":        startNode,
		"end":          endNode,
		"tenantId":     namespace.TenantID,
		"kbId":         namespace.KnowledgeBaseID,
		"types":        relationTypes,
	})
	if err != nil {
		return nil, fmt.Errorf("约束路径查询失败: %w", err)
	}

	paths := make([]*knowledge.PathQueryResult, 0)

	for result.Next(ctx) {
		record := result.Record()

		nodesData := getInterfaceSliceValue(record, "nodes")
		relationsData := getInterfaceSliceValue(record, "relations")
		length := int(getFloat64Value(record, "length"))
		weight := getFloat64Value(record, "weight")

		nodes := make([]*knowledge.GraphNode, 0)
		if nodesData != nil {
			for _, n := range nodesData {
				if nodeMap, ok := n.(map[string]interface{}); ok {
					nodes = append(nodes, &knowledge.GraphNode{
						ID:         getValueAsString(nodeMap, "id"),
						Name:       getValueAsString(nodeMap, "name"),
						EntityType: getValueAsString(nodeMap, "entityType"),
					})
				}
			}
		}

		relations := make([]*knowledge.GraphRelation, 0)
		if relationsData != nil {
			for _, r := range relationsData {
				if relMap, ok := r.(map[string]interface{}); ok {
					relations = append(relations, &knowledge.GraphRelation{
						ID:       getValueAsString(relMap, "id"),
						Type:     getValueAsString(relMap, "type"),
						Strength: getValueAsFloat64(relMap, "strength"),
						Weight:   getValueAsFloat64(relMap, "weight"),
					})
				}
			}
		}

		// Set source/target for relations
		for i, rel := range relations {
			if i < len(nodes)-1 {
				rel.Source = nodes[i].Name
				rel.Target = nodes[i+1].Name
			}
		}

		paths = append(paths, &knowledge.PathQueryResult{
			Nodes:     nodes,
			Relations: relations,
			Length:    length,
			Weight:    weight,
		})
	}

	return paths, nil
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
		json.Unmarshal([]byte(labelsStr), &labels)

		return &knowledge.Community{
			ID:         getStringValue(record, "id"),
		 KnowledgeBaseID:       getStringValue(record, "kbId"),
			Name:       getStringValue(record, "name"),
			Size:       int(getFloat64Value(record, "size")),
			Modularity: getFloat64Value(record, "modularity"),
			Labels:     labels,
		}, nil
	}

	return nil, fmt.Errorf("节点不属于任何社区")
}

// CheckHealth 检查健康状态
func (r *Neo4jRepository) CheckHealth(ctx context.Context) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := "RETURN 1"
	_, err := session.Run(ctx, cypher, nil)
	return err
}

// UpdateNode 更新节点
func (r *Neo4jRepository) UpdateNode(ctx context.Context, namespace knowledge.NameSpace, node *knowledge.GraphNode) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (n:Entity {id: $id, tenant_id: $tenantId, kb_id: $kbId})
		SET n.properties = $properties
		RETURN n
	`

	propsJSON, _ := json.Marshal(node.Properties)

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"id":         node.ID,
		"tenantId":   namespace.TenantID,
		"kbId":       namespace.KnowledgeBaseID,
		"properties": string(propsJSON),
	})

	return err
}

// AddRelation 添加关系
func (r *Neo4jRepository) AddRelation(ctx context.Context, namespace knowledge.NameSpace, relation *knowledge.GraphRelation) (*knowledge.GraphRelation, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (s:Entity {name: $source, tenant_id: $tenantId, kb_id: $kbId})
		MATCH (t:Entity {name: $target, tenant_id: $tenantId, kb_id: $kbId})
		MERGE (s)-[r:RELATION {id: $id}]->(t)
		SET r.type = $type,
		    r.strength = $strength,
		    r.weight = $weight,
		    r.chunk_ids = $chunkIds,
		    r.properties = $properties
		RETURN r
	`

	chunkIDsJSON, _ := json.Marshal(relation.ChunkIDs)
	propsJSON, _ := json.Marshal(relation.Properties)

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"id":         relation.ID,
		"source":     relation.Source,
		"target":     relation.Target,
		"type":       relation.Type,
		"strength":   relation.Strength,
		"weight":     0, // 将被计算后更新
		"chunkIds":   string(chunkIDsJSON),
		"properties": string(propsJSON),
		"tenantId":   namespace.TenantID,
		"kbId":       namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, err
	}

	// 计算权重
	relation.Weight = calculateRelationWeight(relation)

	return relation, nil
}

// AddNode 添加节点
func (r *Neo4jRepository) AddNode(ctx context.Context, namespace knowledge.NameSpace, node *knowledge.GraphNode) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MERGE (n:Entity {id: $id, tenant_id: $tenantId, kb_id: $kbId})
		SET n.name = $name,
		    n.entity_type = $entityType,
		    n.chunks = $chunks,
		    n.properties = $properties
		RETURN n
	`

	chunksJSON, _ := json.Marshal(node.Chunks)
	propsJSON, _ := json.Marshal(node.Properties)

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"id":         node.ID,
		"name":       node.Name,
		"entityType": node.EntityType,
		"tenantId":   namespace.TenantID,
		"kbId":       namespace.KnowledgeBaseID,
		"chunks":     string(chunksJSON),
		"properties": string(propsJSON),
	})

	return err
}

// UpdateRelation 更新关系
func (r *Neo4jRepository) UpdateRelation(ctx context.Context, namespace knowledge.NameSpace, relation *knowledge.GraphRelation) (*knowledge.GraphRelation, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (:Entity)-[r:RELATION {id: $id}]->(:Entity)
		SET r.strength = $strength,
		    r.type = $type,
		    r.weight = $weight,
		    r.properties = $properties
		RETURN r
	`

	propsJSON, _ := json.Marshal(relation.Properties)

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"id":         relation.ID,
		"strength":   relation.Strength,
		"type":       relation.Type,
		"weight":     relation.Weight,
		"properties": string(propsJSON),
	})

	if err != nil {
		return nil, err
	}

	return relation, nil
}

// DeleteNode 删除节点
func (r *Neo4jRepository) DeleteNode(ctx context.Context, namespace knowledge.NameSpace, nodeID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (n:Entity {id: $id, tenant_id: $tenantId, kb_id: $kbId})
		DETACH DELETE n
	`

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"id":       nodeID,
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})

	return err
}

// DeleteRelation 删除关系
func (r *Neo4jRepository) DeleteRelation(ctx context.Context, namespace knowledge.NameSpace, relationID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (:Entity)-[r:RELATION {id: $id}]->(:Entity)
		DELETE r
	`

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"id": relationID,
	})

	return err
}

// DeleteByChunkID 按ChunkID删除
func (r *Neo4jRepository) DeleteByChunkID(ctx context.Context, namespace knowledge.NameSpace, chunkID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		WHERE $chunkId IN n.chunks
		SET n.chunks = [c IN n.chunks WHERE c <> $chunkId]
		RETURN n
	`

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
		"chunkId":  chunkID,
	})

	// 删除没有chunk的节点关系
	cypher2 := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})-[r:RELATION]->(m:Entity {tenant_id: $tenantId, kb_id: $kbId})
		WHERE n.chunks = [] OR m.chunks = []
		DELETE r
	`

	_, err2 := session.Run(ctx, cypher2, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
	})

	if err != nil {
		return err
	}
	return err2
}

// DeleteByKnowledgeID 按KnowledgeID删除
func (r *Neo4jRepository) DeleteByKnowledgeID(ctx context.Context, namespace knowledge.NameSpace, knowledgeID string) error {
	// 实现类似 DeleteByChunkID，但基于知识条目ID
	// 这里简化处理，假设chunkID包含knowledgeID
	return r.DeleteByChunkID(ctx, namespace, knowledgeID)
}

// DeleteByScope delete by scope (kb_id or knowledge_id)
func (r *Neo4jRepository) DeleteByScope(ctx context.Context, scopeType string, scopeID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	var cypher string
	params := make(map[string]interface{})

	switch scopeType {
	case "kb_id":
		cypher = `
			MATCH (n:Entity {kb_id: $scopeId})
			DETACH DELETE n
		`
		params["scopeId"] = scopeID
	case "knowledge_id":
		// For knowledge_id, we need to filter by chunks that contain the knowledge_id
		cypher = `
			MATCH (n:Entity)
			WHERE ANY(chunk IN n.chunks WHERE chunk STARTS WITH $scopeId)
			DETACH DELETE n
		`
		params["scopeId"] = scopeID
	default:
		return fmt.Errorf("不支持的scope类型: %s", scopeType)
	}

	_, err := session.Run(ctx, cypher, params)
	if err != nil {
		return fmt.Errorf("按scope删除失败: %w", err)
	}

	return nil
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
	 KnowledgeBaseID:            namespace.KnowledgeBaseID,
		ExistingEntities: existingEntities,
		EntityTypes:     entityTypes,
		RelationTypes:   relationTypes,
		SampleEntities:  sampleEntities,
	}, nil
}

// ========================================
// Statistics Operations
// ========================================

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
		RETURN avg(degree) as avgDegree, max(degree) as maxDegree, min(degree) as minDegree,
		       count(CASE WHEN degree = 0 THEN 1 END) as isolatedNodes,
		       count(CASE WHEN degree > 2 * avg(degree) THEN 1 END) as highDegreeNodes
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
		EntityTypeDistribution:     entityDist,
		RelationTypeDistribution:   relDist,
		TopEntityTypes:             topEntities,
		TopRelationTypes:           topRelations,
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
			Density:              getFloat64Value(record, "density"),
			ComponentCount:       1, // Simplified - would require more complex query
			LargestComponentSize: nodeCount, // Simplified
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
		RETURN avg(n.%s) as avgScore, max(n.%s) as maxScore, min(n.%s) as minScore,
		       n.id as nodeId, n.name as nodeName, n.%s as score
		ORDER BY score DESC
		LIMIT 10
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
			CommunityCount:        int(getFloat64Value(record, "communityCount")),
			Modularity:            getFloat64Value(record, "modularity"),
			AverageCommunitySize:  getFloat64Value(record, "avgSize"),
			LargestCommunitySize:  int(getFloat64Value(record, "maxSize")),
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

// ========================================
// Analytics Result Storage
// ========================================

// StoreCommunity store community detection results
func (r *Neo4jRepository) StoreCommunity(ctx context.Context, namespace knowledge.NameSpace, community *knowledge.Community) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	labelsJSON, _ := json.Marshal(community.Labels)
	propsJSON, _ := json.Marshal(community.Properties)

	cypher := `
		MERGE (c:Community {id: $id, kb_id: $kbId})
		SET c.name = $name,
		    c.size = $size,
		    c.modularity = $modularity,
		    c.labels = $labels,
		    c.properties = $properties
		RETURN c
	`

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"id":         community.ID,
		"kbId":       namespace.KnowledgeBaseID,
		"name":       community.Name,
		"size":       community.Size,
		"modularity": community.Modularity,
		"labels":     string(labelsJSON),
		"properties": string(propsJSON),
	})

	return err
}

// StoreCommunityMembers store community member relationships
func (r *Neo4jRepository) StoreCommunityMembers(ctx context.Context, namespace knowledge.NameSpace, members []*knowledge.CommunityMember) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	for _, member := range members {
		cypher := `
			MATCH (e:Entity {name: $entityName, tenant_id: $tenantId, kb_id: $kbId})
			MATCH (c:Community {id: $communityId, kb_id: $kbId})
			MERGE (e)-[r:MEMBER_OF]->(c)
			SET r.membership_score = $score
			RETURN r
		`

		_, err := session.Run(ctx, cypher, map[string]interface{}{
			"entityName":       member.EntityName,
			"communityId":      member.CommunityID,
			"score":            member.MembershipScore,
			"tenantId":         namespace.TenantID,
			"kbId":             namespace.KnowledgeBaseID,
		})
		if err != nil {
			return fmt.Errorf("存储社区成员失败: %w", err)
		}
	}

	return nil
}

// UpdateCentralityScores update centrality scores for entity nodes
func (r *Neo4jRepository) UpdateCentralityScores(ctx context.Context, namespace knowledge.NameSpace, scores map[string]float64, scoreType string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	tx, err := session.BeginTransaction(ctx)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback(ctx)
			panic(r)
		}
	}()

	var propName string
	switch scoreType {
	case "pagerank":
		propName = "pagerank"
	case "betweenness":
		propName = "betweenness"
	default:
		return fmt.Errorf("不支持的分数类型: %s", scoreType)
	}

	for nodeID, score := range scores {
		cypher := fmt.Sprintf(`
			MATCH (n:Entity {id: $nodeId, tenant_id: $tenantId, kb_id: $kbId})
			SET n.%s = $score
			RETURN n
		`, propName)

		_, err := tx.Run(ctx, cypher, map[string]interface{}{
			"nodeId":   nodeID,
			"score":    score,
			"tenantId": namespace.TenantID,
			"kbId":     namespace.KnowledgeBaseID,
		})
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("更新中心性分数失败: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// ========================================
// 缓存操作
// ========================================

func (r *Neo4jRepository) updateCache(namespace knowledge.NameSpace, graphs []*knowledge.GraphData) {
	key := namespace.String()
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()

	for _, graphData := range graphs {
		for _, node := range graphData.Node {
			r.cache.nodes[key+":"+node.Name] = node
		}
		for _, rel := range graphData.Relation {
			r.cache.relations[key+":"+rel.GetKey()] = rel
		}
	}
}

// ========================================
// 辅助函数
// ========================================

// getValueAsString get string value from map
func getValueAsString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok && val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// getValueAsFloat64 get float64 value from map
func getValueAsFloat64(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok && val != nil {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0
}

// getStringValue 从 Record 中安全地获取字符串值
func getStringValue(record *neo4j.Record, key string) string {
	if val, ok := record.Get(key); ok && val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// getFloat64Value 从 Record 中安全地获取 float64 值
func getFloat64Value(record *neo4j.Record, key string) float64 {
	if val, ok := record.Get(key); ok && val != nil {
		switch v := val.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		}
	}
	return 0
}

// getStringSliceValue 从 Record 中安全地获取字符串切片值
func getStringSliceValue(record *neo4j.Record, key string) []string {
	if val, ok := record.Get(key); ok && val != nil {
		switch v := val.(type) {
		case []string:
			return v
		case []interface{}:
			result := make([]string, 0, len(v))
			for _, item := range v {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return nil
}

// getInterfaceSliceValue 从 Record 中安全地获取接口切片值
func getInterfaceSliceValue(record *neo4j.Record, key string) []interface{} {
	if val, ok := record.Get(key); ok && val != nil {
		if slice, ok := val.([]interface{}); ok {
			return slice
		}
	}
	return nil
}

// calculateRelationWeight 计算关系权重
func calculateRelationWeight(rel *knowledge.GraphRelation) float64 {
	// 简单的权重计算：基于强度和共同chunk数
	chunkFactor := float64(len(rel.ChunkIDs)) / 10.0
	if chunkFactor > 1.0 {
		chunkFactor = 1.0
	}
	return rel.Strength * (0.7 + 0.3*chunkFactor)
}
