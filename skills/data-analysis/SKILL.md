---
name: data-analysis
description: 专注于数据查询和分析的技能，通过 SQL 查询获取和分析数据。
when_to_use: 当任务需要查询数据库、分析数据、生成报表或执行数据统计时使用此技能。
category: data
tags:
  - sql
  - database
  - analytics
  - reporting
version: "1.0.0"
author: Link Team
allowed_tools:
  - sql_execute
  - get_schema
model: inherit
context_mode: inline
effort: 7
---

# Data Analysis Skill

此 Skill 提供数据查询和分析的系统化方法和最佳实践。

## 核心概念

数据分析是通过 SQL 查询从数据库中提取、处理和分析信息的过程：

1. **需求理解**：明确分析目标
2. **模式探索**：了解数据结构
3. **查询构造**：编写 SQL 查询
4. **结果验证**：检查数据质量
5. **洞察提取**：从数据中发现价值

## 可用工具

### get_schema - 获取数据库模式
用途：查看表结构、字段信息、关系

```json
{
  "tool": "get_schema",
  "parameters": {
    "table": "users"
  }
}
```

### sql_execute - 执行 SQL 查询
用途：执行 SELECT、聚合、联接等查询

```json
{
  "tool": "sql_execute",
  "parameters": {
    "query": "SELECT COUNT(*) FROM users WHERE status = 'active'"
  }
}
```

## 分析流程

### 标准分析流程

```
1. 理解业务问题
   ↓
2. 探索数据模式
   ↓
3. 构造查询
   ↓
4. 执行并验证
   ↓
5. 分析和解释
```

### 数据探索步骤

1. **了解表结构**
   - 使用 `get_schema` 查看表定义
   - 理解字段类型和约束
   - 识别关键索引

2. **检查数据质量**
   - 检查空值分布
   - 验证数据范围
   - 识别异常值

3. **探索数据分布**
   - 统计摘要
   - 分组聚合
   - 时间趋势

## 常见查询模式

### 基础查询

```sql
-- 统计记录数
SELECT COUNT(*) FROM table_name;

-- 去重计数
SELECT COUNT(DISTINCT column_name) FROM table_name;

-- 条件筛选
SELECT * FROM table_name WHERE condition;
```

### 聚合分析

```sql
-- 分组统计
SELECT category, COUNT(*), AVG(value)
FROM table_name
GROUP BY category;

-- 时间序列
SELECT DATE(created_at), COUNT(*)
FROM table_name
GROUP BY DATE(created_at)
ORDER BY created_at;
```

### 复杂分析

```sql
-- 窗口函数
SELECT
    name,
    sales,
    RANK() OVER (ORDER BY sales DESC) as rank
FROM sales_data;

-- CTE 递归查询
WITH RECURSIVE category_tree AS (
    SELECT id, name, parent_id
    FROM categories
    WHERE parent_id IS NULL
    UNION ALL
    SELECT c.id, c.name, c.parent_id
    FROM categories c
    JOIN category_tree ct ON c.parent_id = ct.id
)
SELECT * FROM category_tree;
```

## 最佳实践

### 查询优化

1. **使用索引**
   - 在 WHERE、JOIN、ORDER BY 字段上建立索引
   - 避免在索引列上使用函数

2. **限制结果集**
   - 使用 LIMIT 限制返回行数
   - 避免 SELECT *，只查询需要的列

3. **优化 JOIN**
   - 小表驱动大表
   - 确保连接字段有索引

### 数据验证

1. **检查空值**
   ```sql
   SELECT COUNT(*) FROM table WHERE column IS NULL;
   ```

2. **验证范围**
   ```sql
   SELECT MIN(value), MAX(value) FROM table;
   ```

3. **检查重复**
   ```sql
   SELECT column, COUNT(*)
   FROM table
   GROUP BY column
   HAVING COUNT(*) > 1;
   ```

### 安全考虑

1. **SQL 注入防护**
   - 使用参数化查询
   - 验证输入数据

2. **权限控制**
   - 遵循最小权限原则
   - 只查询授权数据

3. **性能监控**
   - 记录慢查询
   - 优化资源消耗

## 常见分析场景

### 用户行为分析

```sql
-- 用户活跃度
SELECT
    user_id,
    COUNT(*) as action_count,
    MAX(timestamp) as last_action
FROM user_actions
WHERE timestamp >= NOW() - INTERVAL '7 days'
GROUP BY user_id;
```

### 销售分析

```sql
-- 销售趋势
SELECT
    DATE(order_date) as date,
    SUM(amount) as total_sales,
    COUNT(*) as order_count
FROM orders
GROUP BY DATE(order_date)
ORDER BY date;
```

### 漏斗分析

```sql
-- 转化漏斗
SELECT
    step,
    COUNT(DISTINCT user_id) as users,
    COUNT(DISTINCT user_id) * 100.0 / LAG(COUNT(DISTINCT user_id)) OVER (ORDER BY step) as conversion_rate
FROM funnel_events
GROUP BY step
ORDER BY step;
```

## 输出格式

数据分析结果应包含：

```markdown
## 数据分析报告

### 分析目标
[描述分析目标和问题]

### 数据来源
- 表名：[表名]
- 时间范围：[时间范围]
- 记录数：[数量]

### 关键发现
1. **[发现标题]**
   - 数据：[支持数据]
   - 洞察：[业务洞察]

### 数据可视化
[表格或图表描述]

### 建议
[基于分析的业务建议]

### 附加信息
- 查询耗时：[时间]
- 数据质量：[评估]
```

## 注意事项

- 始终验证查询结果的正确性
- 注意大数据量查询的性能影响
- 保护敏感数据，遵守隐私政策
- 记录分析过程以便复现
- 对分析结论保持谨慎态度
