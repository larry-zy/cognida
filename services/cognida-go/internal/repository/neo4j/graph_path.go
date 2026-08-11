package neo4j

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"cognida/internal/model/knowledge"
)

// maxPathDepth 是变长路径查询 maxDepth 的硬上限（PF-2）。
// 变长 MATCH [*1..depth] 在密集图上随 depth 近似指数级膨胀，depth≥3 即有 CPU/内存 DoS 风险，
// 故对所有路径查询的 maxDepth 统一封顶，超出则截断到该上限（保持函数签名与调用方不变）。
const maxPathDepth = 5

// maxPathResults 是无 shortestPath 的变长路径查询（FindPathWithTypes）返回结果集的硬上限（PF-2），
// 防止密集图上枚举出海量路径全部物化进内存。
const maxPathResults = 50

// clampPathDepth 将 maxDepth 收敛到 (0, maxPathDepth]：非正值取默认 3，超上限则截断到 maxPathDepth。
func clampPathDepth(maxDepth int) int {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	if maxDepth > maxPathDepth {
		maxDepth = maxPathDepth
	}
	return maxDepth
}

// SearchPath 搜索路径
func (r *Neo4jRepository) SearchPath(ctx context.Context, namespace knowledge.NameSpace, startNode, endNode string, maxDepth int) ([]*knowledge.GraphData, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	maxDepth = clampPathDepth(maxDepth) // 封顶 maxDepth（PF-2）

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

		// 构建关系：N 个节点对应 N-1 条关系，relTypes[i] 连接 nodeNames[i]→nodeNames[i+1]。
		// start==end 时路径只有单节点、relTypes 为空，此处自然得到空关系切片，不再越界 panic。
		relations := make([]*knowledge.GraphRelation, 0, len(relTypes))
		for i := 0; i < len(relTypes) && i+1 < len(nodeNames); i++ {
			relations = append(relations, &knowledge.GraphRelation{
				Source: nodeNames[i],
				Target: nodeNames[i+1],
				Type:   relTypes[i],
			})
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
	opts.MaxDepth = clampPathDepth(opts.MaxDepth) // 封顶 maxDepth（PF-2）

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
		for _, n := range nodesData {
			if nodeMap, ok := n.(map[string]interface{}); ok {
				nodes = append(nodes, &knowledge.GraphNode{
					ID:         getValueAsString(nodeMap, "id"),
					Name:       getValueAsString(nodeMap, "name"),
					EntityType: getValueAsString(nodeMap, "entityType"),
				})
			}
		}

		relations := make([]*knowledge.GraphRelation, 0)
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
	opts.MaxDepth = clampPathDepth(opts.MaxDepth) // 封顶 maxDepth（PF-2）；LIMIT $k 已限定结果集

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
		for _, n := range nodesData {
			if nodeMap, ok := n.(map[string]interface{}); ok {
				nodes = append(nodes, &knowledge.GraphNode{
					ID:         getValueAsString(nodeMap, "id"),
					Name:       getValueAsString(nodeMap, "name"),
					EntityType: getValueAsString(nodeMap, "entityType"),
				})
			}
		}

		relations := make([]*knowledge.GraphRelation, 0)
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
	maxDepth = clampPathDepth(maxDepth) // 封顶 maxDepth（PF-2）

	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	// 变长非 shortestPath 查询在密集图上会枚举出海量路径，故封顶 maxDepth 并加硬 LIMIT（PF-2）。
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
		LIMIT $limit
	`, maxDepth)

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"start":    startNode,
		"end":      endNode,
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
		"types":    relationTypes,
		"limit":    maxPathResults,
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
		for _, n := range nodesData {
			if nodeMap, ok := n.(map[string]interface{}); ok {
				nodes = append(nodes, &knowledge.GraphNode{
					ID:         getValueAsString(nodeMap, "id"),
					Name:       getValueAsString(nodeMap, "name"),
					EntityType: getValueAsString(nodeMap, "entityType"),
				})
			}
		}

		relations := make([]*knowledge.GraphRelation, 0)
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
