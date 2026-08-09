package neo4j

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"cognida/internal/model/knowledge"
)

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
			"entityName":  member.EntityName,
			"communityId": member.CommunityID,
			"score":       member.MembershipScore,
			"tenantId":    namespace.TenantID,
			"kbId":        namespace.KnowledgeBaseID,
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
