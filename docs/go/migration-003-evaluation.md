# Evaluation Database Migration

本文档描述评测系统数据库迁移的内容和执行方法。

## 迁移文件

**文件**: `migrations/003_evaluation_persistence.sql`

## 表结构

### 1. evaluation_tasks

评测任务主表，存储任务元数据和状态。

| 字段 | 类型 | 说明 | 索引 |
|------|------|------|------|
| id | string(36) | 任务ID (UUID) | PRIMARY |
| tenant_id | bigint | 租户ID | INDEX |
| user_id | bigint | 用户ID | INDEX |
| dataset_id | string(100) | 数据集ID | - |
| type | string(20) | 评测类型 | - |
| kb_id | string(100) | 知识库ID | - |
| agent_id | string(100) | Agent ID | - |
| model_id | string(100) | 模型ID | - |
| config | json | 评测配置 | - |
| status | string(20) | 任务状态 | INDEX |
| error_message | text | 错误信息 | - |
| total_count | int | 总QA数量 | - |
| success_count | int | 成功数量 | - |
| failure_count | int | 失败数量 | - |
| created_at | timestamp | 创建时间 | INDEX |
| updated_at | timestamp | 更新时间 | - |
| deleted_at | timestamp | 软删除时间 | INDEX |

**索引**:
- `idx_tenant_id` - 租户查询
- `idx_status` - 状态过滤
- `idx_tenant_status` - 租户+状态组合查询
- `idx_created_at` - 时间范围查询
- `idx_deleted_at` - 软删除过滤

### 2. evaluation_qa_results

QA级别评测结果表，存储每个QA对的详细指标。

| 字段 | 类型 | 说明 | 索引 |
|------|------|------|------|
| id | bigint | 自增ID | PRIMARY |
| task_id | string(36) | 任务ID | INDEX |
| question | text | 问题 | - |
| reference_answer | text | 参考答案 | - |
| generated_answer | text | 生成答案 | - |
| retrieved_pids | json | 检索到的文档ID | - |
| relevant_pids | json | 相关文档ID | - |
| success | boolean | 是否成功 | - |
| error | text | 错误信息 | - |
| precision | float | Precision指标 | - |
| recall | float | Recall指标 | - |
| ndcg | float | NDCG指标 | - |
| rr | float | RR指标 | - |
| rouge_1 | float | ROUGE-1 | - |
| rouge_2 | float | ROUGE-2 | - |
| rouge_l | float | ROUGE-L | - |
| bleu_1 | float | BLEU-1 | - |
| bleu_2 | float | BLEU-2 | - |
| bleu_4 | float | BLEU-4 | - |
| llm_score | float | LLM评分 | - |
| llm_reasoning | text | LLM推理过程 | - |
| semantic_similarity | float | 语义相似度 | - |
| created_at | timestamp | 创建时间 | - |

**外键**:
- `task_id` → `evaluation_tasks(id)` ON DELETE CASCADE

**索引**:
- `idx_task_id` - 任务结果查询

## 执行迁移

### 使用 golang-migrate

```bash
# 向上迁移（创建表）
migrate -path migrations -database "mysql://user:password@tcp(localhost:3306)/dbname" up

# 向下迁移（删除表）
migrate -path migrations -database "mysql://user:password@tcp(localhost:3306)/dbname" down 1
```

### 使用 MySQL 客户端

```bash
mysql -u root -p dbname < migrations/003_evaluation_persistence.sql
```

## 数据清理

### 删除所有评测数据

```sql
-- 删除QA结果（由于外键CASCADE，会先删除）
DELETE FROM evaluation_qa_results WHERE task_id IN (
    SELECT id FROM evaluation_tasks WHERE deleted_at IS NOT NULL
);

-- 软删除任务
UPDATE evaluation_tasks SET deleted_at = NOW() WHERE created_at < '2024-01-01';
```

### 硬删除已软删除的数据

```sql
-- 删除超过30天的软删除数据
DELETE FROM evaluation_qa_results 
WHERE task_id IN (SELECT id FROM evaluation_tasks WHERE deleted_at < NOW() - INTERVAL 30 DAY);

DELETE FROM evaluation_tasks 
WHERE deleted_at < NOW() - INTERVAL 30 DAY;
```

## 性能优化

### 分区表（可选）

对于大规模部署，建议按 `created_at` 对 `evaluation_tasks` 进行分区：

```sql
ALTER TABLE evaluation_tasks
PARTITION BY RANGE (YEAR(created_at)) (
    PARTITION p2023 VALUES LESS THAN (2024),
    PARTITION p2024 VALUES LESS THAN (2025),
    PARTITION p2025 VALUES LESS THAN (2026),
    PARTITION pmax VALUES LESS THAN MAXVALUE
);
```

### 批量插入优化

批量插入QA结果时使用事务和批量插入：

```go
func (r *EvaluationResultRepository) CreateBatch(ctx context.Context, results []*EvaluationResult) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // 分批插入，每批100条
        batchSize := 100
        for i := 0; i < len(results); i += batchSize {
            end := i + batchSize
            if end > len(results) {
                end = len(results)
            }
            batch := results[i:end]
            if err := tx.CreateInBatches(batch, batchSize).Error; err != nil {
                return err
            }
        }
        return nil
    })
}
```

## 监控查询

### 任务状态统计

```sql
SELECT 
    status,
    COUNT(*) as count,
    AVG(success_count) as avg_success,
    AVG(failure_count) as avg_failure
FROM evaluation_tasks
WHERE deleted_at IS NULL
GROUP BY status;
```

### 评测类型分布

```sql
SELECT 
    type,
    COUNT(*) as count,
    AVG(total_count) as avg_qa_count
FROM evaluation_tasks
WHERE deleted_at IS NULL
GROUP BY type;
```

### 每日任务统计

```sql
SELECT 
    DATE(created_at) as date,
    COUNT(*) as total_tasks,
    SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed
FROM evaluation_tasks
WHERE deleted_at IS NULL
GROUP BY DATE(created_at)
ORDER BY date DESC
LIMIT 30;
```

## 回滚计划

如需回滚此迁移：

```sql
-- 删除外键约束
ALTER TABLE evaluation_qa_results DROP FOREIGN KEY fk_evaluation_qa_results_task_id;

-- 删除表
DROP TABLE IF EXISTS evaluation_qa_results;
DROP TABLE IF EXISTS evaluation_tasks;
```
