# Neo4j 图谱开发踩坑记录

## 1. Neo4j 关系类型不匹配导致查询不到关系

### 问题描述
调用 `GetNodeDetail` API 获取节点详情时，返回的关系列表为空。

### 根本原因
Cypher 查询中使用的关系类型与 Neo4j 实际存储的类型不一致：

```cypher
// 错误：使用 RELATION
OPTIONAL MATCH (n)-[r:RELATION]-(m:Entity)

// 正确：应该使用 RELATES_TO
OPTIONAL MATCH (n)-[r:RELATES_TO]-(m:Entity)
```

### 调试方法
使用专门的工具查看 Neo4j 中实际存储的数据结构：

```bash
go run cmd/check_relations/main.go
```

输出示例：
```
Relation: {Id:53 Type:RELATES_TO Props:map[...source:支付宝 target:天猫 type:关联 weight:8]}
```

从输出可以清楚看到：
- 关系类型是 `RELATES_TO` 而不是 `RELATION`
- 关系属性包含 `source`, `target`, `type`, `weight` 等字段

### 解决方案
统一使用 `RELATES_TO` 作为关系类型。建议：
1. 在代码中定义常量，避免硬编码
2. 添加单元测试验证查询语句

---

## 2. SearchNode 方法没有返回关系数据

### 问题描述
`GetNodeDetail` API 调用 `SearchNode` 方法，但返回的 `relations` 为空数组。

### 根本原因
`SearchNode` 方法的 Cypher 查询只返回节点，没有返回关系：

```cypher
// 只返回节点，没有关系
MATCH (n:Entity)
WHERE n.tenant_id = $tenant_id AND n.kb_id = $kb_id AND n.name IN $names
RETURN n.id, n.name, n.title, n.type  // ❌ 缺少关系
```

### 解决方案
使用 `OPTIONAL MATCH` 同时获取节点和相关关系：

```cypher
MATCH (n:Entity)
WHERE n.tenant_id = $tenant_id AND n.kb_id = $kb_id AND n.name IN $names
OPTIONAL MATCH (n)-[r:RELATES_TO]-(m:Entity)
RETURN n, r, m
```

解析结果时：
```go
// 获取节点
node := r.parseNodeFromRecord(record, "n")

// 获取相关节点
relatedNode := r.parseNodeFromRecord(record, "m")

// 获取关系
rel := r.parseRelationFromRecord(record, "r")
```

---

## 3. 关系 weight 字段返回 0

### 问题描述
前端展示的关系权重都是 0，但 Neo4j 中实际存储的值是正确的（如 weight:8）。

### 可能原因排查

#### 原因1：关系类型不匹配（本次问题所在）
见问题1，使用错误的关系类型导致查询不到关系，返回空值。

#### 原因2：解析逻辑错误
`parseRelationFromRecord` 方法正确实现了 weight 解析：

```go
if weight, ok := props["weight"].(float64); ok {
    rel.Weight = weight
}
if strength, ok := props["strength"].(float64); ok {
    rel.Strength = strength
}
```

#### 原因3：source/target 字段缺失
关系属性中 `source` 和 `target` 可能不存在（存储为节点名称而非关系属性），需要从节点中获取：

```go
// 尝试从关系属性获取
if src, ok := props["source"].(string); ok {
    rel.Source = src
}

// 如果没有，从节点推断
if rel.Source == "" && centerNode != nil {
    rel.Source = centerNode.Name
}
```

### 验证数据的方法
创建调试工具直接查看 Neo4j 原始数据：

```go
// cmd/check_relations/main.go
result, err := session.Run(ctx, `
    MATCH ()-[r:RELATES_TO]->()
    RETURN r
    ORDER BY r.id DESC
    LIMIT 5
`, nil)
```

---

## 4. 编译错误：GraphRepository 接口未完全实现

### 问题描述
```
*graphRepository does not implement interfaces.GraphRepository (missing method UpdateNode)
```

### 根本原因
接口定义了新方法但实现类未添加对应实现。

### 解决方案
在 `internal/application/repository/graph.go` 中添加缺失的方法：

```go
// UpdateNode 更新节点属性
func (r *graphRepository) UpdateNode(ctx context.Context, namespace types.NameSpace, node *types.GraphNode) error {
    // 实现更新逻辑
}

// UpdateRelation 更新关系属性
func (r *graphRepository) UpdateRelation(ctx context.Context, namespace types.NameSpace, relation *types.GraphRelation) error {
    // 实现更新逻辑
}
```

### 建议
1. 使用 IDE 的接口实现提示功能
2. 添加方法后立即编译验证

---

## 5. Handler 语法错误：缺少闭合括号

### 问题描述
```
internal\handler\graph.go:266:51: syntax error: unexpected { at end of statement
```

### 根本原因
`AddRelation` 函数缺少闭合括号 `}`：

```go
func (h *GraphHandler) AddRelation(c *gin.Context) {
    // ...
    c.JSON(200, gin.H{
        "message": "success",
        "data":    relation,
    })
}  // ❌ 缺少这个括号

// UpdateNode 更新节点属性
func (h *GraphHandler) UpdateNode(c *gin.Context) {
```

### 解决方案
检查函数结构，确保每个函数都有正确的闭合括号。

### 建议
使用支持语法高亮和括号匹配的编辑器（VS Code、GoLand 等）。

---

## 6. 图谱查询方式差异：knowledge_id vs KB标签

### 问题描述
用户反馈 API `/api/v1/knowledge-bases/4b856e03-.../graph` 之前能查出数据，修改后查不出数据。

### 根本原因
命名空间查询方式不一致：

```go
// 方式1：使用 knowledge_id 属性过滤
namespace.Knowledge = kbID  // 会使用 WHERE n.knowledge_id = $kb_id

// 方式2：使用 KB 标签过滤
namespace.Knowledge = ""  // 会使用标签 KB_4b856e03
```

### Neo4j 存储方式
节点使用标签而非属性进行知识库隔离：

```cypher
// 节点创建
MERGE (n:ENTITY:KB_4b856e03 {id: $id})

// 关系创建
MERGE (source)-[r:RELATES_TO {kb_id: $kb_id}]->(target)
```

### 解决方案
Handler 中不设置 `namespace.Knowledge`，使用标签查询：

```go
namespace := types.NameSpace{
    TenantID: strconv.FormatInt(tenantID, 10),
    KBID:     kbID,
    // 不设置 Knowledge，让 repository 使用标签查询
}
```

---

## 7. 关系类型字段初始化

### 问题描述
历史数据中没有 `type` 字段，导致前端显示为空。

### 解决方案
创建批量更新工具使用 LLM 推断关系类型：

```go
// cmd/infer_relation_types/main.go
func inferRelationType(description, source, target string) string {
    // 调用 LLM API
    // 返回：作者、别名、属于、拥有、时间、关联
}
```

更新 prompt 模板限制关系类型：
```yaml
# config/prompt_templates/relation_extraction.yaml
type: 关系类型，必须从上述关系类型列表中选择（"作者"、"别名"、"属于"、"拥有"、"时间"、"关联"）
```

---

## 调试工具集

### 检查关系数据
```bash
go run cmd/check_relations/main.go
```

### 检查节点数据
```bash
go run cmd/debug_relations/main.go
```

### 批量更新关系类型
```bash
go run cmd/infer_relation_types/main.go
```

---

## 最佳实践

### 1. 数据库操作
- 添加日志记录关键操作
- 使用 `OPTIONAL MATCH` 避免因缺失数据导致查询失败
- 检查 driver 是否初始化

### 2. Cypher 查询
- 明确指定关系类型（RELATES_TO）
- 使用参数化查询防止注入
- 使用 LIMIT 限制返回数量

### 3. 代码维护
- 在常量文件中定义关系类型
- 统一命名规范（驼峰命名 vs 下划线命名）
- 接口变更后立即更新实现

### 4. 问题排查流程
1. 先用调试工具查看原始数据
2. 检查 Cypher 查询语句是否正确
3. 验证解析逻辑是否匹配数据结构
4. 查看日志中的错误信息
