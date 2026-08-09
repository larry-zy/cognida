// Neo4j 索引创建脚本
// 用于生产环境的索引初始化
// 使用方法：cypher-shell -u neo4j -p password < create_neo4j_indexes.cypher

// ========================================
// 1. 租户隔离索引
// ========================================

// 节点租户隔离索引 (tenant_id + kb_id)
CREATE INDEX graph_entity_tenant_kb_id IF NOT EXISTS
FOR (n:Entity) ON (n.tenant_id, n.kb_id);

// 节点知识条目索引
CREATE INDEX graph_entity_knowledge_id IF NOT EXISTS
FOR (n:Entity) ON (n.knowledge_id);

// 关系租户隔离索引
CREATE INDEX graph_relation_tenant_kb_id IF NOT EXISTS
FOR ()-[r:RELATION]-() ON (r.tenant_id, r.kb_id);

// ========================================
// 2. 全文搜索索引
// ========================================

// 节点名称全文索引
CREATE FULLTEXT INDEX graph_entity_name_fulltext IF NOT EXISTS
FOR (n:Entity) ON EACH [n.name, n.title]
OPTIONS {
  indexConfig: {
    `fulltext.analyzer`: "standard",
    `fulltext.eventually_consistent`: true
  }
};

// ========================================
// 3. 关系索引
// ========================================

// 关系类型索引
CREATE INDEX graph_relation_type IF NOT EXISTS
FOR ()-[r:RELATION]-() ON (r.type);

// 关系权重索引
CREATE INDEX graph_relation_weight IF NOT EXISTS
FOR ()-[r:RELATION]-() ON (r.weight);

// 关系强度索引
CREATE INDEX graph_relation_strength IF NOT EXISTS
FOR ()-[r:RELATION]-() ON (r.strength);

// ========================================
// 4. 唯一约束
// ========================================

// 节点 ID 唯一约束
CREATE CONSTRAINT graph_entity_id_unique IF NOT EXISTS
FOR (n:Entity) REQUIRE n.id IS UNIQUE;

// ========================================
// 5. 分析结果索引
// ========================================

// 社区节点索引
CREATE INDEX graph_community_kb_id IF NOT EXISTS
FOR (n:Community) ON (n.kb_id);

// 社区ID索引
CREATE INDEX graph_community_id IF NOT EXISTS
FOR (n:Community) ON (n.community_id);

// 中心性分数索引 - PageRank
CREATE INDEX graph_entity_pagerank IF NOT EXISTS
FOR (n:Entity) ON (n.pagerank);

// 中心性分数索引 - Betweenness
CREATE INDEX graph_entity_betweenness IF NOT EXISTS
FOR (n:Entity) ON (n.betweenness);

// ========================================
// 6. 其他辅助索引
// ========================================

// 节点类型索引
CREATE INDEX graph_entity_type IF NOT EXISTS
FOR (n:Entity) ON (n.entity_type);

// Chunk ID 索引（用于关联查询）
CREATE INDEX graph_entity_chunk_id IF NOT EXISTS
FOR (n:Entity) ON (n.chunk_id);

// ========================================
// 索引验证查询
// ========================================

// 查看所有创建的索引
SHOW INDEXES;

// 查看全文索引
SHOW FULLTEXT INDEXES;

// 查看约束
SHOW CONSTRAINTS;
