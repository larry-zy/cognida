# Evaluation API 文档

评测系统 API 提供任务创建、结果查询、进度跟踪等功能。

## 基础信息

**Base URL**: `/api/v1/evaluation`

**认证**: 需要 Bearer Token（通过 TenantID 和 UserID 进行多租户隔离）

---

## 端点列表

### 1. 创建评测任务

创建新的评测任务并加入执行队列。

**请求**:
```http
POST /api/v1/evaluation/tasks
Content-Type: application/json
Authorization: Bearer <token>
```

**请求体**:
```json
{
  "dataset_id": "ds-001",              // 必需，数据集ID
  "type": "rag",                       // 必需，评测类型: agent/rag/qa
  "kb_id": "kb-001",                   // RAG类型必需
  "agent_id": "agent-001",             // Agent类型必需
  "model_id": "model-001",             // 必需，模型ID
  "config": {                          // 可选，评测配置
    "top_k": 5,
    "graders": ["rouge", "bleu"]
  }
}
```

**响应** (201 Created):
```json
{
  "success": true,
  "data": {
    "task_id": "eval-a1b2c3d4",
    "type": "evaluation",
    "status": "pending",
    "dataset_id": "ds-001",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**错误响应**:
- `400` - 配置无效
- `404` - 数据集不存在
- `400` - 数据集类型不匹配

---

### 2. 获取任务状态

获取评测任务的详细状态和结果。

**请求**:
```http
GET /api/v1/evaluation/tasks/{task_id}
Authorization: Bearer <token>
```

**响应** (200 OK):
```json
{
  "success": true,
  "data": {
    "task_id": "eval-a1b2c3d4",
    "dataset_id": "ds-001",
    "type": "rag",
    "status": "completed",
    "total_count": 100,
    "success_count": 95,
    "failure_count": 5,
    "qa_results": [
      {
        "question": "What is AI?",
        "reference_answer": "Artificial Intelligence",
        "generated_answer": "AI is...",
        "rouge_1": 0.85,
        "rouge_2": 0.75,
        "bleu_1": 0.65
      }
    ],
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:05:00Z"
  }
}
```

---

### 3. 流式获取任务进度

通过 SSE (Server-Sent Events) 实时获取任务执行进度。

**请求**:
```http
GET /api/v1/evaluation/tasks/{task_id}/stream
Authorization: Bearer <token>
```

**响应事件**:
```
event: progress
data: {"stage":"generation","current":50,"total":100,"message":"Generating answers...","percentage":50}

event: done
data: {"message":"Evaluation completed"}
```

**阶段说明**:
- `loading` - 加载数据集
- `generation` - 生成答案
- `evaluation` - 计算指标
- `completed` - 完成
- `failed` - 失败

---

### 4. 获取评测结果

获取任务的所有评测结果详情。

**请求**:
```http
GET /api/v1/evaluation/tasks/{task_id}/results
Authorization: Bearer <token>
```

**响应** (200 OK):
```json
{
  "success": true,
  "data": {
    "task_id": "eval-a1b2c3d4",
    "dataset_id": "ds-001",
    "type": "rag",
    "status": "completed",
    "total_count": 100,
    "success_count": 95,
    "failure_count": 5,
    "qa_results": [...],
    "rouge_1": 0.82,
    "rouge_2": 0.74,
    "bleu_1": 0.63
  }
}
```

---

### 5. 列出评测任务

获取租户下的评测任务列表（支持分页和过滤）。

**请求**:
```http
GET /api/v1/evaluation/results?page=1&page_size=20&status=completed&type=rag
Authorization: Bearer <token>
```

**查询参数**:
| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| page | int | 否 | 页码，默认 1 |
| page_size | int | 否 | 每页数量，默认 20，最大 100 |
| status | string | 否 | 过滤状态: pending/running/completed/failed |
| type | string | 否 | 过滤类型: agent/rag/qa |

**响应** (200 OK):
```json
{
  "success": true,
  "data": {
    "items": [
      {
        "task_id": "eval-a1b2c3d4",
        "dataset_id": "ds-001",
        "type": "rag",
        "status": "completed",
        "total_count": 100,
        "success_count": 95,
        "failure_count": 5,
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:05:00Z"
      }
    ],
    "total": 42,
    "page": 1,
    "page_size": 20,
    "total_pages": 3
  }
}
```

---

### 6. 删除评测任务

软删除指定的评测任务（不能删除运行中的任务）。

**请求**:
```http
DELETE /api/v1/evaluation/tasks/{task_id}
Authorization: Bearer <token>
```

**响应** (200 OK):
```json
{
  "success": true,
  "data": {
    "message": "Task deleted successfully",
    "task_id": "eval-a1b2c3d4"
  }
}
```

**错误响应**:
- `404` - 任务不存在
- `400` - 任务正在运行，无法删除

---

### 7. 列出数据集

获取可用的评测数据集列表。

**请求**:
```http
GET /api/v1/evaluation/datasets?type=rag
Authorization: Bearer <token>
```

**查询参数**:
| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| type | string | 否 | 过滤类型: agent/rag/qa |

**响应** (200 OK):
```json
{
  "success": true,
  "data": [
    {
      "id": "ds-001",
      "name": "RAG Benchmark Dataset",
      "description": "Standard RAG evaluation dataset",
      "type": "file",
      "evaluation_type": "rag",
      "qa_count": 100,
      "modified_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

---

### 8. 获取数据集信息

获取指定数据集的详细信息。

**请求**:
```http
GET /api/v1/evaluation/datasets/{dataset_id}
Authorization: Bearer <token>
```

**响应** (200 OK):
```json
{
  "success": true,
  "data": {
    "id": "ds-001",
    "name": "RAG Benchmark Dataset",
    "description": "Standard RAG evaluation dataset",
    "type": "file",
    "evaluation_type": "rag",
    "qa_count": 100,
    "modified_at": "2024-01-01T00:00:00Z"
  }
}
```

---

## 错误码

| 错误码 | 说明 |
|--------|------|
| `DATASET_NOT_FOUND` | 数据集不存在 |
| `DATASET_TYPE_MISMATCH` | 数据集类型与评测类型不匹配 |
| `TASK_NOT_FOUND` | 任务不存在 |
| `INVALID_CONFIG` | 配置无效 |
| `TASK_RUNNING` | 任务正在运行 |

---

## 数据模型

### EvaluationTaskConfig

评测任务配置

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| dataset_id | string | 是 | 数据集ID |
| type | string | 是 | 评测类型 |
| kb_id | string | 条件 | RAG类型必需 |
| agent_id | string | 条件 | Agent类型必需 |
| model_id | string | 是 | 模型ID |
| config | object | 否 | 额外配置 |

### QAResult

QA评测结果

| 字段 | 类型 | 说明 |
|------|------|------|
| question | string | 问题 |
| reference_answer | string | 参考答案 |
| generated_answer | string | 生成答案 |
| rouge_1, rouge_2, rouge_l | float | ROUGE指标 |
| bleu_1, bleu_2, bleu_4 | float | BLEU指标 |
| llm_score | float | LLM评分 |
| semantic_similarity | float | 语义相似度 |
