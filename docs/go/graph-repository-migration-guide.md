# 图谱仓储迁移指南

## 概述

本文档描述如何从旧的图谱仓储实现迁移到新的统一图谱仓储。

## 变更摘要

### 旧实现

- 位置：`internal/infrastructure/persistence/neo4j/retriever/repository.go`
- 接口：`Neo4jGraphRepository` (已标记为 DEPRECATED)
- 特点：
  - 动态节点标签
  - 按命名空间划分的独立标签
  - 批量删除方法 `DeleteNodes()`, `DeleteRelations()`

### 新实现

- 位置：`internal/infrastructure/persistence/neo4j/graph_repo.go`
- 接口：`GraphRepository` (统一接口)
- 特点：
  - 固定节点标签 `Entity`
  - 固定关系类型 `RELATION`
  - 租户隔离通过属性实现
  - 丰富的查询和统计方法

## 迁移步骤

### 1. 更新依赖注入

**旧代码**：
```go
func ProvideGraphRepository(db *gorm.DB) domain_knowledge.GraphRepository {
    return mysql.NewGraphMetaRepository(db)
}
```

**新代码**：
```go
func ProvideGraphRepository(neo4jConfig *config.Neo4jConfig) domain_knowledge.GraphRepository {
    if neo4jConfig == nil || neo4jConfig.URI == "" {
        return mysql.NewGraphMetaRepository(nil)
    }

    dbName := neo4jConfig.DatabaseName
    if dbName == "" {
        dbName = "neo4j"
    }
    repo, err := neo4j.NewNeo4jRepository(
        neo4jConfig.URI,
        neo4jConfig.Username,
        neo4jConfig.Password,
        dbName,
    )
    if err != nil {
        return mysql.NewGraphMetaRepository(nil)
    }
    return repo
}
```

### 2. 更新配置

添加 Neo4j 配置到环境变量：

```bash
# .env
NEO4J_URI=bolt://localhost:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=your_password
NEO4J_DATABASE=neo4j
NEO4J_MAX_POOL_SIZE=50
```

### 3. 数据迁移

如果现有数据使用旧的节点标签格式，需要进行数据迁移：

```cypher
// 迁移旧的动态标签节点到统一格式
MATCH (n)
WHERE n:TenantID_KBID OR ANY(label IN labels(n) WHERE label CONTAINS '_')
CALL {
  WITH n
  SET n:Entity
  REMOVE n:TenantID_KBID
  // 保留其他属性
} IN TRANSACTIONS OF 1000 ROWS
```

### 4. 批量操作迁移

旧仓储的批量删除方法需要改为循环调用：

**旧代码**：
```go
err := repo.DeleteNodes(ctx, namespace, nodeIDs)
err := repo.DeleteRelations(ctx, namespace, relationIDs)
```

**新代码**：
```go
// 批量删除节点
for _, nodeID := range nodeIDs {
    if err := repo.DeleteNode(ctx, namespace, nodeID); err != nil {
        log.Printf("Failed to delete node %s: %v", nodeID, err)
    }
}

// 批量删除关系
for _, relID := range relationIDs {
    if err := repo.DeleteRelation(ctx, namespace, relID); err != nil {
        log.Printf("Failed to delete relation %s: %v", relID, err)
    }
}
```

## API 变更对照表

### 基础操作

| 旧方法 | 新方法 | 说明 |
|--------|--------|------|
| `AddGraph()` | `AddGraph()` | 保持不变 |
| `DeleteGraph()` | `DeleteGraph()` | 保持不变 |
| `GetGraph()` | `GetGraph()` | 保持不变 |

### 查询操作

| 旧方法 | 新方法 | 说明 |
|--------|--------|------|
| `SearchNode()` | `SearchNodes()` | 支持更多过滤选项 |
| `SearchNodeV2()` | `SearchNodes()` | 合并到统一方法 |
| `SearchPath()` | `FindShortestPath()` | 更明确的方法名 |
| - | `FindKShortestPaths()` | 新增 K 路径查找 |
| - | `FindPathWithTypes()` | 新增类型约束路径 |
| - | `GetNeighbors()` | 新增邻居查询 |
| - | `GetNodeCommunity()` | 新增社区查询 |

### 统计操作

| 旧方法 | 新方法 | 说明 |
|--------|--------|------|
| `GetGraphStats()` | `GetGraphStats()` | 返回更详细的信息 |
| - | `GetDegreeStats()` | 新增度统计 |
| - | `GetTypeDistribution()` | 新增类型分布 |
| - | `GetDensityMetrics()` | 新增密度指标 |
| - | `GetCentralitySummaries()` | 新增中心性统计 |
| - | `GetCommunityStats()` | 新增社区统计 |

### 删除操作

| 旧方法 | 新方法 | 说明 |
|--------|--------|------|
| `DeleteNode()` | `DeleteNode()` | 保持不变 |
| `DeleteRelation()` | `DeleteRelation()` | 保持不变 |
| `DeleteNodes()` | 循环调用 `DeleteNode()` | 需要应用层循环 |
| `DeleteRelations()` | 循环调用 `DeleteRelation()` | 需要应用层循环 |
| `DeleteByChunkID()` | `DeleteByChunkID()` | 保持不变 |
| `DeleteByChunkIDs()` | 循环调用 `DeleteByChunkID()` | 需要应用层循环 |

## 新功能使用指南

### 1. 节点搜索（带过滤）

```go
opts := &knowledge.NodeQueryOptions{
    EntityTypes: []string{"Technology", "Organization"},
    Limit:       10,
}

nodes, err := repo.SearchNodes(ctx, namespace, "Python", opts)
```

### 2. 邻居查询

```go
opts := &knowledge.RelationQueryOptions{
    RelationTypes: []string{"DEPENDS_ON", "RELATED_TO"},
    MinWeight:     0.5,
    MaxDepth:      2,
}

result, err := repo.GetNeighbors(ctx, namespace, "Python", opts)
```

### 3. 增量提取上下文

```go
context, err := repo.GetEntityContext(ctx, namespace)
// context 包含：
// - ExistingEntities: 已存在的实体名称
// - EntityTypes: 实体类型分布
// - RelationTypes: 关系类型分布
// - SampleEntities: 高频实体示例
```

### 4. 图谱统计

```go
// 基础统计
stats, err := repo.GetGraphStats(ctx, namespace)

// 度统计
degreeStats, err := repo.GetDegreeStats(ctx, namespace)

// 类型分布
typeDist, err := repo.GetTypeDistribution(ctx, namespace)

// 密度指标
density, err := repo.GetDensityMetrics(ctx, namespace)

// 中心性统计
centrality, err := repo.GetCentralitySummaries(ctx, namespace, "pagerank")
```

## 测试迁移

### 单元测试更新

更新测试以使用新的接口：

```go
// 旧测试
mockRepo := &MockNeo4jGraphRepository{}

// 新测试
mockRepo := &mockGraphRepository{}
```

### 集成测试更新

确保集成测试覆盖新功能：

1. 租户隔离查询
2. 全文搜索
3. 增量提取上下文
4. 统计方法
5. 分析结果存储

## 回滚计划

如果迁移出现问题，可以回滚到旧实现：

1. 恢复旧的 `retriever/repository.go` 文件
2. 恢复旧的依赖注入配置
3. 重新部署服务

## 支持与问题

如遇到迁移问题，请检查：

1. Neo4j 版本兼容性（推荐 4.4+）
2. 索引是否正确创建
3. 租户隔离属性是否正确设置
4. 查询是否使用正确的过滤器

## 参考资料

- [Neo4j 索引文档](./neo4j-index-guide.md)
- [图谱提取文档](./graph-extraction-guide.md)
- [Domain 层设计](../internal/domain/CLAUDE.md)
