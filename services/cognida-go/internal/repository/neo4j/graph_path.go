package neo4j

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"cognida/internal/model/knowledge"
)

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
		"start":    startNode,
		"end":      endNode,
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
		"types":    relationTypes,
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
