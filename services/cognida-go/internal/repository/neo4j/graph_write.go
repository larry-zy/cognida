package neo4j

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"cognida/internal/model/knowledge"
)

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
			_ = tx.Rollback(ctx)
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
				"id":         node.ID,
				"name":       node.Name,
				"entityType": node.EntityType,
				"tenantId":   namespace.TenantID,
				"kbId":       namespace.KnowledgeBaseID,
				"chunks":     string(chunksJSON),
				"attributes": string(attrsJSON),
				"properties": string(propsJSON),
			})
			if err != nil {
				_ = tx.Rollback(ctx)
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
				_ = tx.Rollback(ctx)
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
