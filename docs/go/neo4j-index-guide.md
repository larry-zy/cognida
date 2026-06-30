# Neo4j 索引创建文档

## 概述

统一图谱仓储使用 Neo4j 作为图数据库存储。为了提高查询性能和数据一致性，需要创建适当的索引。

## 索引设计

### 1. 租户隔离索引

租户隔离是系统安全的核心，所有查询必须按租户过滤。

```cypher
// 节点租户隔离索引 (tenant_id + kb_id)
CREATE INDEX graph_entity_tenant_kb_id IF NOT EXISTS
FOR (n:Entity) ON (n.tenant_id, n.kb_id)

// 节点知识条目索引
CREATE INDEX graph_entity_knowledge_id IF NOT EXISTS
FOR (n:Entity) ON (n.knowledge_id)
```

### 2. 全文搜索索引

支持节点的快速全文搜索。

```cypher
// 节点名称全文索引
CREATE FULLTEXT INDEX graph_entity_name_fulltext IF NOT EXISTS
FOR (n:Entity) ON EACH [n.name, n.title]
OPTIONS {
  indexConfig: {
    `fulltext.analyzer`: "standard",
    `fulltext.eventually_consistent`: true
  }
}
```

### 3. 关系索引

支持按关系类型和权重过滤。

```cypher
// 关系类型索引
CREATE INDEX graph_relation_type IF NOT EXISTS
FOR ()-[r:RELATION]-() ON (r.type)

// 关系权重索引
CREATE INDEX graph_relation_weight IF NOT EXISTS
FOR ()-[r:RELATION]-() ON (r.weight)
```

### 4. 分析结果索引

支持社区检测和中心性分析结果查询。

```cypher
// 社区节点索引
CREATE INDEX graph_community_kb_id IF NOT EXISTS
FOR (n:Community) ON (n.kb_id)

// 中心性分数索引
CREATE INDEX graph_entity_pagerank IF NOT EXISTS
FOR (n:Entity) ON (n.pagerank)

CREATE INDEX graph_entity_betweenness IF NOT EXISTS
FOR (n:Entity) ON (n.betweenness)
```

## 索引创建流程

### 自动创建

仓储初始化时会自动调用 `InitIndexes()` 方法，该方法具有幂等性，可以多次调用而不会报错。

```go
repo, err := neo4j.NewNeo4jRepository(uri, username, password, dbName)
// InitIndexes() 已在构造函数中调用
```

### 手动创建

如果需要手动创建索引，可以执行以下 Cypher 命令：

```bash
# 连接到 Neo4j
cypher-shell -u neo4j -p password

# 执行索引创建
source scripts/create_neo4j_indexes.cypher
```

## 索引维护

### 查看现有索引

```cypher
SHOW INDEXES
```

### 删除索引

```cypher
DROP INDEX graph_entity_tenant_kb_id IF EXISTS
```

### 重建索引

```cypher
// 先删除
DROP INDEX graph_entity_tenant_kb_id IF EXISTS

// 再创建
CREATE INDEX graph_entity_tenant_kb_id
FOR (n:Entity) ON (n.tenant_id, n.kb_id)
```

## 性能优化

### 索引选择策略

1. **租户隔离查询**：使用 `tenant_id + kb_id` 复合索引
2. **全文搜索**：使用全文索引进行名称搜索
3. **关系过滤**：使用 `type` 和 `weight` 索引
4. **分析查询**：使用社区和中心性索引

### 查询优化示例

```cypher
// 使用索引的查询
MATCH (n:Entity)
WHERE n.tenant_id = $tenant_id AND n.kb_id = $kb_id
RETURN n

// 使用全文索引的查询
CALL db.index.fulltext.queryNodes('graph_entity_name_fulltext', $search_term)
YIELD node, score
RETURN node, score
ORDER BY score DESC
LIMIT 10
```

## 监控

### 索引使用情况

```cypher
// 查看索引统计
CALL db.indexStats()

// 查看查询计划
EXPLAIN
MATCH (n:Entity)
WHERE n.tenant_id = $tenant_id AND n.kb_id = $kb_id
RETURN n
```

### 性能指标

- 索引命中率：查询使用索引的比例
- 索引大小：索引占用的存储空间
- 查询延迟：查询响应时间

## 故障排查

### 索引未生效

**症状**：查询速度慢，EXPLAIN 显示 NodeByLabelScan

**解决**：
1. 确认索引已创建：`SHOW INDEXES`
2. 确认查询条件匹配索引字段
3. 使用 `PROFILE` 分析查询计划

### 全文索引不工作

**症状**：全文搜索返回空结果

**解决**：
1. 确认全文索引已创建：`SHOW FULLTEXT INDEXES`
2. 确认搜索语法正确：`CALL db.index.fulltext.queryNodes(...)`
3. 检查分析器配置

### 索引创建失败

**症状**：`InitIndexes()` 返回错误

**解决**：
1. 检查 Neo4j 版本兼容性
2. 检查磁盘空间
3. 检查权限设置
