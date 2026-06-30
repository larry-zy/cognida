# Neo4j 图谱关系踩坑总结

## 问题 1: 前端显示 UUID 而不是节点名称

### 现象
关联关系详情显示的是节点 ID (UUID) 而不是节点名称

### 原因
`convertToVisData` 函数只设置了 `from`/`to`（vis-network 需要的节点 ID），没有保留 `source`/`target`（后端返回的节点名称）

```javascript
// 错误：只设置了 from/to
return {
  from: fromId,
  to: toId,
  // source 和 target 被遗漏
}
```

### 修复
保留原始的 `source`/`target` 字段供显示使用

```javascript
return {
  id: rel.id,
  from: fromId,    // vis-network使用的节点ID
  to: toId,
  // 保留原始的 source 和 target（节点名称），用于显示
  source: rel.source,
  target: rel.target
}
```

---

## 问题 2: 更新关系时 type 和 strength 没有生效

### 现象
更新请求返回成功，但数据库中值没变

### 原因
`UpdateRelation` 的 MATCH 条件包含 `kb_id`：

```cypher
MATCH ()-[r:RELATES_TO {id: $id, kb_id: $kb_id}]->()
```

旧数据中的关系没有 `kb_id` 属性，导致 MATCH 失败，更新没有执行。

### 修复
改为只用 `id` 匹配，同时 `SET r.kb_id` 确保新数据也有 kb_id：

```cypher
MATCH ()-[r:RELATES_TO {id: $id}]->()
SET r.type = $type
SET r.description = $description
SET r.strength = $strength
SET r.weight = $weight
SET r.kb_id = $kb_id
```

---

## 问题 3: 新增关系时 type 和 strength 为空

### 现象
新创建的关系 type 为空字符串，strength 为 0

### 原因
`AddRelation` 使用 `MERGE` + `ON CREATE SET`：

```cypher
MERGE (source)-[r:RELATES_TO {id: $id, kb_id: $kb_id}]->(target)
ON CREATE SET
    r.type = COALESCE($type, '关联'),
    r.strength = COALESCE($strength, 5.0)
-- 没有 ON MATCH SET
```

只有创建新关系时才设置属性，如果关系已存在（通过 MERGE 匹配到），属性不会更新。

### 修复
添加 `ON MATCH SET`：

```cypher
ON CREATE SET
    r.type = COALESCE($type, '关联'),
    r.strength = COALESCE($strength, 5.0),
    r.weight = COALESCE($weight, 5.0),
    r.kb_id = $kb_id
ON MATCH SET
    r.type = COALESCE($type, '关联'),
    r.strength = COALESCE($strength, 5.0),
    r.weight = COALESCE($weight, 5.0),
    r.kb_id = $kb_id
```

---

## 问题 4: 查询返回空值

### 现象
GetGraph 查询返回的关系 type/strength 为空

### 原因
Cypher 查询中属性访问没有使用别名：

```cypher
-- 错误：没有别名
RETURN r.type, r.strength

-- 正确：使用别名
RETURN r.type as type, r.strength as strength
```

### 修复
所有返回字段都加上 `as` 别名：

```cypher
RETURN r.id as rel_id,
       r.source as source,
       r.target as target,
       r.type as type,
       r.description as description,
       r.strength as strength,
       r.weight as weight
```

---

## 为什么一开始没搞对？

### 1. 对 Neo4j 的 MERGE 语义理解不足
`ON CREATE SET` 只在创建时执行，匹配到已有节点/关系时不会执行。需要同时加 `ON MATCH SET`。

### 2. 忽略了旧数据兼容性
旧数据没有 `kb_id` 属性，用 `{id: $id, kb_id: $kb_id}` 匹配会失败。

### 3. Cypher 返回键的命名规则
没有使用 `as` 别名时，`r.type` 在结果中的键可能是 `r.type` 而不是 `type`，导致 `record.Get("type")` 获取失败。

### 4. 调试信息不足
一开始没有打印完整的参数和返回值，无法定位问题在哪一层。

---

## 最佳实践

### 1. MERGE 操作
```cypher
-- 推荐写法：同时处理 CREATE 和 MATCH
MERGE (n:Label {id: $id})
ON CREATE SET n.prop = $val
ON MATCH SET n.prop = $val
```

### 2. 兼容旧数据
```cypher
-- 如果旧数据可能缺少某些属性，只用主键匹配
MATCH (n:Label {id: $id})
-- 同时设置/更新其他属性
SET n.new_prop = $val
```

### 3. 查询返回值
```cypher
-- 始终使用别名，确保键名可控
RETURN r.id as id, r.prop as prop
-- 而不是
RETURN r.id, r.prop
```

### 4. 调试日志
```go
// 记录输入参数
log.Printf("[Layer] Method START: param1=%q, param2=%f", param1, param2)

// 记录数据库返回
log.Printf("[Layer] Retrieved: field1=%s, field2=%s", field1, field2)
```
