// Package neo4j 提供 Graph 基础设施层 Neo4j 实现
package neo4j

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ========================================
// Neo4j 仓储实现
// ========================================

// defaultSearchNodeLimit 是 SearchNodes 未显式指定 Limit 时的兜底上限，
// 用于防止未限制查询退化为整命名空间扫描返回全部节点的大结果集风险。
const defaultSearchNodeLimit = 100

// defaultGraphLoadLimit 是 GetGraph/SearchNode 等整图加载查询的兜底上限，
// 防止大知识库一次性把全部节点/关系物化进内存（PF-1 无界加载）。
const defaultGraphLoadLimit = 1000

// Neo4jRepository Neo4j 图谱仓储实现
type Neo4jRepository struct {
	driver neo4j.DriverWithContext
	dbName string
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
	}

	// Initialize indexes idempotently
	ctx := context.Background()
	if err := repo.InitIndexes(ctx); err != nil {
		return nil, fmt.Errorf("初始化索引失败: %w", err)
	}

	// 一次性归一化 GR-1 之前以 JSON 字符串存储的遗留 chunks 为 Neo4j 原生 list。
	// 非致命：读路径已兼容遗留字符串（getStringSliceValue），此处失败仅告警不阻断启动。
	if n, err := repo.NormalizeChunksToList(ctx); err != nil {
		fmt.Printf("Warning: 归一化遗留 chunks 失败（读路径仍兼容，可忽略）: %v\n", err)
	} else if n > 0 {
		fmt.Printf("[Neo4j] 已归一化遗留字符串 chunks 节点 %d 个为原生 list\n", n)
	}

	return repo, nil
}

// NormalizeChunksToList 将 GR-1 之前以 JSON 字符串形式存储的 n.chunks 一次性归一化为
// Neo4j 原生 list。存量库遗留节点的 chunks 形如字符串 "null" 或 "[\"c1\",\"c2\"]"，会令
// 新版删除谓词（$chunkId IN n.chunks 等列表操作）在该节点上抛类型错误。本方法幂等：仅命中
// chunks 为字符串类型的节点（toStringOrNull(n.chunks)=n.chunks 便携识别——对 list/其它类型
// 返回 null 故不命中），解析后写回原生 list；归一化后的节点不再命中，分批循环至清零。
func (r *Neo4jRepository) NormalizeChunksToList(ctx context.Context) (int, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	const batch = 500
	normalized := 0
	for {
		readCypher := `
			MATCH (n:Entity)
			WHERE n.chunks IS NOT NULL AND toStringOrNull(n.chunks) = n.chunks
			RETURN elementId(n) AS eid, n.chunks AS chunks
			LIMIT $batch
		`
		result, err := session.Run(ctx, readCypher, map[string]interface{}{"batch": batch})
		if err != nil {
			return normalized, fmt.Errorf("扫描遗留 chunks 失败: %w", err)
		}

		rows := make([]map[string]interface{}, 0, batch)
		for result.Next(ctx) {
			rec := result.Record()
			eid, _ := rec.Get("eid")
			raw := getStringValue(rec, "chunks")
			var list []string
			// "null" / 非法 JSON 均落到空 list（这些遗留节点本就无 chunk 关联）。
			if err := json.Unmarshal([]byte(raw), &list); err != nil || list == nil {
				list = []string{}
			}
			rows = append(rows, map[string]interface{}{"eid": eid, "chunks": list})
		}
		if err := result.Err(); err != nil {
			return normalized, fmt.Errorf("读取遗留 chunks 失败: %w", err)
		}
		if len(rows) == 0 {
			break
		}

		writeCypher := `
			UNWIND $rows AS row
			MATCH (n) WHERE elementId(n) = row.eid
			SET n.chunks = row.chunks
		`
		if _, err := session.Run(ctx, writeCypher, map[string]interface{}{"rows": rows}); err != nil {
			return normalized, fmt.Errorf("回写归一化 chunks 失败: %w", err)
		}
		normalized += len(rows)

		if len(rows) < batch {
			break
		}
	}
	return normalized, nil
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

// CheckHealth 检查健康状态
func (r *Neo4jRepository) CheckHealth(ctx context.Context) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := "RETURN 1"
	_, err := session.Run(ctx, cypher, nil)
	return err
}
