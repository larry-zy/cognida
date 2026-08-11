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

	// 批量写入 Cypher：每个 graphData 一次 UNWIND 节点 + 一次 UNWIND 关系（PF-3），
	// 取代逐节点/逐关系 tx.Run。语义与逐条版本保持完全一致：
	//   - MERGE 键、ON CREATE/ON MATCH 的 SET 字段完全相同；
	//   - chunks 仍为 Neo4j 原生 list，attributes/properties/chunk_ids 仍为 JSON 字符串〔GR-1〕；
	//   - 保留 per-graphData 边界（先建本批节点再建本批关系），避免跨批关系 MATCH 到后批节点，
	//     以严格复现原“节点先于关系、按 graphData 分组”的顺序语义。
	nodeCypher := `
		UNWIND $rows AS row
		MERGE (n:Entity {name: row.name, tenant_id: $tenantId, kb_id: $kbId})
		ON CREATE SET
			n.id = row.id,
			n.name = row.name,
			n.entity_type = row.entityType,
			n.tenant_id = $tenantId,
			n.kb_id = $kbId,
			n.chunks = row.chunks,
			n.attributes = row.attributes,
			n.properties = row.properties
		ON MATCH SET
			n.chunks = CASE WHEN row.chunks IS NOT NULL THEN row.chunks ELSE n.chunks END,
			n.attributes = CASE WHEN row.attributes IS NOT NULL THEN row.attributes ELSE n.attributes END,
			n.properties = CASE WHEN row.properties IS NOT NULL THEN row.properties ELSE n.properties END
	`

	relCypher := `
		UNWIND $rows AS row
		MATCH (s:Entity {name: row.source, tenant_id: $tenantId, kb_id: $kbId})
		MATCH (t:Entity {name: row.target, tenant_id: $tenantId, kb_id: $kbId})
		MERGE (s)-[r:RELATION {id: row.id, tenant_id: $tenantId, kb_id: $kbId}]->(t)
		ON CREATE SET
			r.type = row.type,
			r.strength = row.strength,
			r.weight = row.weight,
			r.chunk_ids = row.chunkIds,
			r.properties = row.properties
		ON MATCH SET
			r.chunk_ids = CASE WHEN row.chunkIds IS NOT NULL THEN row.chunkIds ELSE r.chunk_ids END,
			r.strength = (r.strength + row.strength) / 2
	`

	for _, graphData := range graphs {
		// 组装本批节点行（跳过无 id 的节点，与逐条版本一致）
		nodeRows := make([]map[string]interface{}, 0, len(graphData.Node))
		for _, node := range graphData.Node {
			if node.ID == "" {
				continue
			}
			attrsJSON, _ := json.Marshal(node.Attributes)
			propsJSON, _ := json.Marshal(node.Properties)
			nodeRows = append(nodeRows, map[string]interface{}{
				"id":         node.ID,
				"name":       node.Name,
				"entityType": node.EntityType,
				"chunks":     node.Chunks, // Neo4j 原生 list〔GR-1〕
				"attributes": string(attrsJSON),
				"properties": string(propsJSON),
			})
		}
		if len(nodeRows) > 0 {
			_, err := tx.Run(ctx, nodeCypher, map[string]interface{}{
				"rows":     nodeRows,
				"tenantId": namespace.TenantID,
				"kbId":     namespace.KnowledgeBaseID,
			})
			if err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("添加节点失败: %w", err)
			}
		}

		// 组装本批关系行（跳过无 id 的关系，与逐条版本一致）
		relRows := make([]map[string]interface{}, 0, len(graphData.Relation))
		for _, rel := range graphData.Relation {
			if rel.ID == "" {
				continue
			}
			chunkIDsJSON, _ := json.Marshal(rel.ChunkIDs)
			propsJSON, _ := json.Marshal(rel.Properties)
			relRows = append(relRows, map[string]interface{}{
				"id":         rel.ID,
				"source":     rel.Source,
				"target":     rel.Target,
				"type":       rel.Type,
				"strength":   rel.Strength,
				"weight":     rel.Weight,
				"chunkIds":   string(chunkIDsJSON),
				"properties": string(propsJSON),
			})
		}
		if len(relRows) > 0 {
			_, err := tx.Run(ctx, relCypher, map[string]interface{}{
				"rows":     relRows,
				"tenantId": namespace.TenantID,
				"kbId":     namespace.KnowledgeBaseID,
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

	propsJSON, _ := json.Marshal(node.Properties)

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"id":         node.ID,
		"name":       node.Name,
		"entityType": node.EntityType,
		"tenantId":   namespace.TenantID,
		"kbId":       namespace.KnowledgeBaseID,
		"chunks":     node.Chunks, // 存为 Neo4j 原生 list，与删除路径谓词一致〔GR-1〕
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

	// 写库前算好权重，使 DB 中的 weight 与返回值一致
	relation.Weight = calculateRelationWeight(relation)

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"id":         relation.ID,
		"source":     relation.Source,
		"target":     relation.Target,
		"type":       relation.Type,
		"strength":   relation.Strength,
		"weight":     relation.Weight,
		"chunkIds":   string(chunkIDsJSON),
		"properties": string(propsJSON),
		"tenantId":   namespace.TenantID,
		"kbId":       namespace.KnowledgeBaseID,
	})
	if err != nil {
		return nil, err
	}

	return relation, nil
}

// UpdateNode 更新节点
func (r *Neo4jRepository) UpdateNode(ctx context.Context, namespace knowledge.NameSpace, node *knowledge.GraphNode) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := `
		MATCH (n:Entity {id: $id, tenant_id: $tenantId, kb_id: $kbId})
		SET n.name = $name,
		    n.entity_type = $entityType,
		    n.attributes = $attributes
		RETURN n
	`

	attrsJSON, _ := json.Marshal(node.Attributes)

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"id":         node.ID,
		"name":       node.Name,
		"entityType": node.EntityType,
		"tenantId":   namespace.TenantID,
		"kbId":       namespace.KnowledgeBaseID,
		"attributes": string(attrsJSON),
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

	return nil
}

// DeleteByChunkID 按ChunkID删除
func (r *Neo4jRepository) DeleteByChunkID(ctx context.Context, namespace knowledge.NameSpace, chunkID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	// toStringOrNull(n.chunks) IS NULL 便携排除 GR-1 前遗留的字符串型 chunks，
	// 避免列表谓词（$chunkId IN n.chunks）对字符串抛类型错误〔#2〕。
	cypher := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		WHERE toStringOrNull(n.chunks) IS NULL AND $chunkId IN n.chunks
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

// DeleteByChunkIDs 批量按 chunk_id 集合删除图谱数据：
//  1. 一次 UNWIND 从命中节点的 chunks 列表移除全部目标 chunk_id；
//  2. DETACH DELETE 由此 chunks 变空的孤儿节点（连同其关系）。
//
// 相比逐个 DeleteByChunkID：单次会话完成批量移除，且真正清除空节点，
// 避免删文档/重处理后残留 chunks=[] 的孤儿实体（KB-7）。作用域受
// {tenant_id, kb_id} 约束，不跨租户。
func (r *Neo4jRepository) DeleteByChunkIDs(ctx context.Context, namespace knowledge.NameSpace, chunkIDs []string) error {
	if len(chunkIDs) == 0 {
		return nil
	}
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	// 单次遍历：仅在「本次引用了目标 chunk 的节点」上移除目标 chunk，再删除因此变空的节点。
	// 关键修正〔#5〕：prune 的作用域严格限定为被 trim 命中的节点（WITH n 承接），绝不波及
	// kb 内其它恰好 chunks 已为空的历史节点——原实现无差别删除全 kb 空节点会误删无关实体。
	// toStringOrNull(n.chunks) IS NULL 便携排除 GR-1 前遗留的字符串型 chunks，避免列表谓词
	// 对字符串抛类型错误〔#2〕。
	op := `
		MATCH (n:Entity {tenant_id: $tenantId, kb_id: $kbId})
		WHERE toStringOrNull(n.chunks) IS NULL
		  AND ANY(c IN n.chunks WHERE c IN $chunkIds)
		SET n.chunks = [c IN n.chunks WHERE NOT c IN $chunkIds]
		WITH n
		WHERE size(n.chunks) = 0
		DETACH DELETE n
	`
	if _, err := session.Run(ctx, op, map[string]interface{}{
		"tenantId": namespace.TenantID,
		"kbId":     namespace.KnowledgeBaseID,
		"chunkIds": chunkIDs,
	}); err != nil {
		return fmt.Errorf("批量删除 chunk 图谱失败: %w", err)
	}
	return nil
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
		// 〔#1〕原实现以 chunks STARTS WITH $scopeId 全库扫描删除，存在两重缺陷：
		//   1. 跨租户：MATCH 不带 tenant_id/kb_id 约束，会命中任意租户的实体；
		//   2. 语义错误：chunk_id 与 knowledge_id 是相互独立的 UUID，前者不以后者为前缀，
		//      故谓词实为危险空操作；且一个 Entity 可被多个 knowledge 的 chunk 共享，
		//      整节点 DETACH DELETE 会误伤其它 knowledge 的数据。
		// 当前签名无 namespace 入参、且全库无任何调用方。按 knowledge 粒度删图请走 chunk
		// 作用域的 DeleteByChunkIDs（受 {tenant_id, kb_id} 约束，租户安全）。
		return fmt.Errorf("DeleteByScope 不支持 knowledge_id 作用域（跨租户且语义错误）：请改用 chunk 作用域的 DeleteByChunkIDs")
	default:
		return fmt.Errorf("不支持的scope类型: %s", scopeType)
	}

	_, err := session.Run(ctx, cypher, params)
	if err != nil {
		return fmt.Errorf("按scope删除失败: %w", err)
	}

	return nil
}
