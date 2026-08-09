// Package neo4j 提供 Graph 基础设施层 Neo4j 实现
package neo4j

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"cognida/internal/model/knowledge"
)

// ========================================
// Neo4j 仓储实现
// ========================================

// defaultSearchNodeLimit 是 SearchNodes 未显式指定 Limit 时的兜底上限，
// 用于防止未限制查询退化为整命名空间扫描返回全部节点的大结果集风险。
const defaultSearchNodeLimit = 100

// Neo4jRepository Neo4j 图谱仓储实现
type Neo4jRepository struct {
	driver neo4j.DriverWithContext
	dbName string
	cache  *graphCache
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

// CheckHealth 检查健康状态
func (r *Neo4jRepository) CheckHealth(ctx context.Context) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: r.dbName})
	defer session.Close(ctx)

	cypher := "RETURN 1"
	_, err := session.Run(ctx, cypher, nil)
	return err
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
