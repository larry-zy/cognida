# 评测系统集成文档

## 目录

- [一、系统概述](#一系统概述)
- [二、架构设计](#二架构设计)
- [三、Python 评分器体系](#三python-评分器体系)
- [四、Go 端实现](#四go-端实现)
- [五、Python gRPC 接口](#五python-grpc-接口)
- [六、数据库设计](#六数据库设计)
- [七、Redis 队列设计](#七redis-队列设计)
- [八、接口设计](#八接口设计)
- [九、进度推送 (SSE)](#九进度推送-sse)
- [十、实现步骤](#十实现步骤)

---

## 一、系统概述

### 1.1 目标

构建一个 Go + Python 异构评测系统，实现：

- **Go 负责编排**：任务管理、知识库检索、LLM 生成
- **Python 负责评分**：指标计算、LLM 裁判
- **SSE 实时推送**：前端实时获取评测进度

### 1.2 技术栈

| 组件 | 技术栈 |
|------|--------|
| Go API | Gin + GORM |
| Python 服务 | gRPC + FastAPI |
| 数据持久化 | MySQL 8.0 |
| 任务队列 | Redis 3.2+ |
| 进度推送 | Server-Sent Events (SSE) |

### 1.3 目录结构

```
D:\link\
├── evaluation/
│   ├── datasets/              # 公共数据集目录
│   │   ├── sample_qa/         # QA 数据集
│   │   ├── sample_agent/      # Agent 数据集
│   │   └── sample_rag/        # RAG 数据集
│   └── schema.sql             # 数据库表结构
├── cognida-go/
│   ├── api/proto/
│   │   └── evaluation.proto   # gRPC 接口定义
│   └── internal/
│       ├── application/
│       │   └── evaluation/     # 评测应用层
│       ├── infrastructure/
│       │   ├── grpc/
│       │   │   └── evaluation/ # gRPC 客户端
│       │   └── persistence/
│       │       └── mysql/      # MySQL 仓储
│       └── interface/
│           └── http/
│               ├── handler/    # HTTP 处理器
│               └── router/      # 路由配置
└── cognida-python/
    ├── proto/
    │   └── evaluation.proto   # gRPC 接口定义（同步）
    └── services/
        └── evaluation/
            ├── graders/        # 评分器
            │   ├── base.py     # 基类
            │   ├── registry.py # 注册表
            │   └── builtin/    # 内置评分器
            │       ├── retrieval.py
            │       ├── generation.py
            │       ├── semantic.py
            │       ├── llm.py
            │       └── rule_based.py
            └── metrics/        # 指标计算
                ├── retrieval.py
                ├── generation.py
                └── llm_judge.py
```

---

## 二、架构设计

### 2.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              前端 (Vue)                                │
└───────────────────────────┬─────────────────────────────────────────────┘
                            │
        ┌───────────────────┴───────────────────┐
        ▼                                       ▼
┌───────────────────────┐           ┌───────────────────────┐
│  POST /evaluation     │           │  GET /stream          │
│      /tasks           │           │  (SSE 推送进度)        │
└───────────┬───────────┘           └───────────┬───────────┘
            │                                   │
            ▼                                   │
┌───────────────────────────────────────────────────────────────────────┐
│                              Go API Layer                             │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────┐  │
│  │ 创建任务         │  │ 查询任务状态     │  │ SSE 流式推送        │  │
│  │ (MySQL + Redis) │  │ (Redis 缓存)     │  │ (Redis Pub/Sub)     │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────────┘  │
└───────────────────────────────┬───────────────────────────────────────┘
                                │
                                ▼
┌───────────────────────────────────────────────────────────────────────┐
│                           Go Worker Pool                             │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │  从 Redis 队列获取任务 (BRPOP)                                 │   │
│  └─────────────────────────────┬──────────────────────────────────┘   │
│                                │                                       │
│  ┌─────────────────────────────▼──────────────────────────────────┐   │
│  │  任务执行流程                                                   │   │
│  │  ┌──────────────────────────────────────────────────────────┐  │   │
│  │  │ 1. 从 D:\link\evaluation\datasets 读取数据集              │  │   │
│  │  └───────────────────────────┬──────────────────────────────┘  │   │
│  │                              │                                  │   │
│  │  ┌───────────────────────────▼──────────────────────────────┐  │   │
│  │  │ 2. 并发执行：检索 → LLM 生成                             │  │   │
│  │  └───────────────────────────┬──────────────────────────────┘  │   │
│  │                              │                                  │   │
│  │  ┌───────────────────────────▼──────────────────────────────┐  │   │
│  │  │ 3. 收集所有 QA 对                                        │  │   │
│  │  └───────────────────────────┬──────────────────────────────┘  │   │
│  │                              │                                  │   │
│  │  ┌───────────────────────────▼──────────────────────────────┐  │   │
│  │  │ 4. 调用 Python gRPC 计算指标                              │  │   │
│  │  └───────────────────────────┬──────────────────────────────┘  │   │
│  │                              │                                  │   │
│  │  ┌───────────────────────────▼──────────────────────────────┐  │   │
│  │  │ 5. 保存结果到 MySQL，更新 Redis 进度                      │  │   │
│  │  └──────────────────────────────────────────────────────────┘  │   │
│  └────────────────────────────────────────────────────────────────┘   │
│                                                                       │
│  并发控制：Redis eval:count / eval:limit                            │
└───────────────────────────────┬───────────────────────────────────────┘
                                │ gRPC
                                ▼
┌───────────────────────────────────────────────────────────────────────┐
│                        Python Evaluation Service                      │
│  ┌────────────────────────────────────────────────────────────────┐   │
│  │  评分器注册表 (GraderRegistry)                                   │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐       │   │
│  │  │ retrieval│  │generation│  │ semantic │  │ llm_judge│       │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘       │   │
│  └────────────────────────────────────────────────────────────────┘   │
│                                                                       │
│  指标计算：                                                           │
│  - ROUGE、BLEU (生成质量)                                             │
│  - Precision、Recall、NDCG (检索质量)                                 │
│  - 语义相似度                                                         │
│  - LLM 裁判（多维度评分）                                              │
└───────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌───────────────────────────────────────────────────────────────────────┐
│                           持久化层                                   │
│  ┌─────────────────────────┐  ┌─────────────────────────────────┐   │
│  │  MySQL (任务 + 结果)      │  │  Redis (队列 + 进度缓存)        │   │
│  │  - judge_tasks           │  │  - eval:queue                   │   │
│  │  - judge_results         │  │  - eval:progress:{id}           │   │
│  │  - judge_sample_results  │  │  - eval:count                   │   │
│  └─────────────────────────┘  └─────────────────────────────────┘   │
└───────────────────────────────────────────────────────────────────────┘
```

### 2.2 职责划分

| 职责 | Go | Python | 说明 |
|------|-----|--------|------|
| HTTP API | ✅ | - | 统一接口、认证鉴权 |
| 任务管理 | ✅ | - | 创建、状态跟踪、结果存储 |
| 数据集读取 | ✅ | - | 从 `D:\link\evaluation\datasets` 读取 |
| 知识库检索 | ✅ | - | RAG 检索（已实现） |
| LLM 生成 | ✅ | - | 调用模型生成答案 |
| 指标计算 | - | ✅ | Rouge、BLEU、语义相似度 |
| LLM 裁判 | - | ✅ | 多维度评分 |
| 进度推送 | ✅ | - | SSE |
| 任务队列 | ✅ | - | Redis 队列 |

---

## 三、Python 评分器体系

### 3.1 评分器列表

| 评分器名称 | 指标 | 类型 | 说明 |
|-----------|------|------|------|
| **retrieval** | |||
| └─ `precision` | precision (0-100) | PERCENTAGE | 精确率@K |
| └─ `recall` | recall (0-100) | PERCENTAGE | 召回率@K |
| └─ `ndcg` | ndcg (0-100) | PERCENTAGE | NDCG@K |
| └─ `mrr` | mrr (0-100) | PERCENTAGE | 平均倒数排名 |
| └─ `map` | map (0-100) | PERCENTAGE | 平均精度 |
| **generation** | |||
| └─ `rouge` | rouge, rouge_1, rouge_2, rouge_l (0-100) | PERCENTAGE | ROUGE 指标 |
| └─ `bleu` | bleu_1, bleu_2, bleu_4 (0-100) | PERCENTAGE | BLEU 指标 |
| **semantic** | |||
| └─ `semantic` | similarity (0-100) | PERCENTAGE | 语义相似度 |
| **llm_judge** | |||
| └─ `llm_judge` | 多维度 (1-5) | SCORE | LLM 裁判 |

### 3.2 LLM 裁判维度

| 维度 | 说明 |
|------|------|
| `accuracy` | 准确性 - 答案是否正确 |
| `completeness` | 完整性 - 是否覆盖所有要点 |
| `relevance` | 相关性 - 是否切题 |
| `clarity` | 清晰度 - 表达是否清楚 |
| `helpfulness` | 有用性 - 是否解决问题 |
| `reasoning` | 推理能力 - 逻辑是否严密 |
| `faithfulness` | 忠实度 - 是否基于检索内容 |

### 3.3 评分器配置示例

```json
{
  "graders": ["rouge", "bleu", "llm_judge"],
  "grader_config": {
    "rouge": {
      "types": ["1", "2", "l"]
    },
    "bleu": {
      "n": [1, 2, 4]
    },
    "llm_judge": {
      "dimensions": ["accuracy", "completeness", "relevance"],
      "model": "gpt-4o-mini"
    }
  }
}
```

---

## 四、Go 端实现

### 4.1 目录结构

```
cognida-go/internal/
├── application/evaluation/
│   ├── service.go              # 评测服务
│   ├── python_adapter.go       # Python 适配器
│   ├── worker.go               # 后台 Worker
│   └── dto.go                  # 数据传输对象
├── infrastructure/
│   ├── grpc/evaluation/
│   │   └── client.go           # gRPC 客户端
│   └── persistence/mysql/
│       └── evaluation_repo.go  # MySQL 仓储
└── interface/http/handler/
    ├── evaluation_handler.go   # 评测处理器
    ├── evaluation_handler_python.go  # Python 集成处理器
    └── evaluation_handler_sse.go     # SSE 推送处理器
```

### 4.2 Worker 实现

```go
// 后台 Worker 结构
type EvaluationWorker struct {
    redis      *redis.Client
    db         *gorm.DB
    pythonEval *python_adapter.PythonEvaluationAdapter
    stopCh     chan struct{}
}

// Worker 主循环
func (w *EvaluationWorker) Run() {
    for {
        select {
        case <-w.stopCh:
            return
        default:
            // 检查并发限制
            if !w.acquireSlot() {
                time.Sleep(1 * time.Second)
                continue
            }

            // 从队列取任务
            taskID := w.redis.BRPop(context.Background(), "eval:queue", 1*time.Second)
            if taskID == "" {
                w.releaseSlot()
                continue
            }

            // 执行任务
            go w.executeTask(taskID)
        }
    }
}

// 执行单个任务
func (w *EvaluationWorker) executeTask(taskID string) {
    defer w.releaseSlot()

    // 1. 更新状态为处理中
    w.updateTaskStatus(taskID, "PROCESSING")

    // 2. 加载任务配置
    task, _ := w.getTask(taskID)

    // 3. 读取数据集
    dataset, _ := w.loadDataset(task.DatasetID)

    // 4. 执行评测（检索 + 生成）
    qaResults, _ := w.runEvaluation(dataset, task)

    // 5. 调用 Python 计算指标
    metrics, _ := w.pythonEval.ComputeMetrics(qaResults, task.Graders)

    // 6. 保存结果
    w.saveResults(taskID, metrics)

    // 7. 更新状态为完成
    w.updateTaskStatus(taskID, "COMPLETED")
    w.publishProgress(taskID, 100, "completed")
}
```

---

## 五、Python gRPC 接口

### 5.1 Proto 定义

```protobuf
syntax = "proto3";

package evaluation;

option go_package = "link/api/proto;evaluation";

service EvaluationService {
    // 批量计算指标
    rpc ComputeMetrics(ComputeRequest) returns (ComputeResponse);

    // 列出可用评分器
    rpc ListGraders(ListGradersRequest) returns (ListGradersResponse);

    // 列出可用数据集
    rpc ListDatasets(Empty) returns (ListDatasetsResponse);
}

// 计算请求
message ComputeRequest {
    repeated QAItem items = 1;
    repeated string graders = 2;
    map<string, string> grader_config = 3;
}

// QA 项
message QAItem {
    string query = 1;
    string response = 2;
    string reference = 3;
    repeated string context = 4;
    repeated string retrieved_ids = 5;  // 检索到的文档 ID
}

// 计算响应
message ComputeResponse {
    bool success = 1;
    string error = 2;
    repeated ItemMetrics items = 3;
    AggregateMetrics aggregate = 4;
}

// 单项指标
message ItemMetrics {
    int32 index = 1;
    map<string, double> metrics = 2;
}

// 聚合指标
message AggregateMetrics {
    map<string, double> metrics = 3;
}

// 评分器信息
message GraderInfo {
    string name = 1;
    string mode = 2;
    string description = 3;
    map<string, string> parameters = 4;
}

message ListGradersRequest {
    string mode = 1;  // 过滤模式
}

message ListGradersResponse {
    repeated GraderInfo graders = 1;
}

message Empty {}
```

### 5.2 Python 服务实现

```python
class EvaluationServicer(evaluation_pb2_grpc.EvaluationServiceServicer):
    def ComputeMetrics(self, request, context):
        """批量计算指标。"""
        try:
            # 获取评分器
            graders = self._get_graders(request.graders)

            results = []
            for item in request.items:
                item_metrics = {}

                # 逐个评分器计算
                for grader in graders:
                    result = await grader.aevaluate(
                        query=item.query,
                        response=item.response,
                        reference=item.reference,
                        context=item.context,
                    )
                    item_metrics.update(result.metrics)

                results.append(evaluation_pb2.ItemMetrics(
                    index=item.index,
                    metrics=item_metrics,
                ))

            # 计算聚合指标
            aggregate = self._compute_aggregate(results)

            return evaluation_pb2.ComputeResponse(
                success=True,
                items=results,
                aggregate=aggregate,
            )

        except Exception as e:
            return evaluation_pb2.ComputeResponse(
                success=False,
                error=str(e),
            )
```

---

## 六、数据库设计

### 6.1 表结构（已有）

| 表名 | 用途 |
|------|------|
| `judge_tasks` | 评测任务 |
| `judge_results` | 汇总结果 |
| `judge_sample_results` | 样本详情 |

### 6.2 任务状态流转

```
PENDING → PROCESSING → COMPLETED
                    ↓
                  FAILED
```

### 6.3 Go 仓储接口

```go
type EvaluationTaskRepository interface {
    Create(ctx context.Context, task *EvaluationTask) error
    FindByID(ctx context.Context, id string) (*EvaluationTask, error)
    UpdateStatus(ctx context.Context, id string, status string) error
    UpdateProgress(ctx context.Context, id string, progress int) error
}

type EvaluationResultRepository interface {
    SaveResult(ctx context.Context, result *EvaluationResult) error
    FindByTaskID(ctx context.Context, taskID string) (*EvaluationResult, error)
}
```

---

## 七、Redis 队列设计

### 7.1 数据结构

```
# 任务队列 (List)
eval:queue          -> [task_id1, task_id2, ...]

# 处理中任务 (Set)
eval:processing     -> {task_id1, task_id2}

# 并发控制
eval:limit          -> "3"
eval:count          -> "2"

# 进度缓存 (Hash，过期时间 1 小时)
eval:progress:{id}  -> {
  "stage": "evaluation",
  "current": 5,
  "total": 10,
  "message": "计算指标中..."
}

# 结果缓存 (String，过期时间 24 小时)
eval:result:{id}    -> {JSON 结果}
```

### 7.2 并发控制流程

```
1. Worker 启动时检查 eval:count
2. 如果 count >= limit，等待
3. 获取任务后 INCR eval:count
4. 任务完成后 DECR eval:count
5. 异常退出时监控 DECR eval:count
```

---

## 八、接口设计

### 8.1 HTTP API（前端调用）

#### 创建评测任务

```http
POST /api/v1/evaluation/tasks
Content-Type: application/json

{
  "dataset_id": "sample_qa/default",
  "knowledge_base_id": "kb-xxx",
  "model_id": "gpt-4",
  "graders": ["rouge", "bleu", "llm_judge"],
  "grader_config": {
    "rouge": {"types": ["1", "2", "l"]},
    "llm_judge": {
      "dimensions": ["accuracy", "completeness", "relevance"]
    }
  }
}

Response:
{
  "success": true,
  "data": {
    "task_id": "task-uuid",
    "status": "PENDING"
  }
}
```

#### 查询任务状态

```http
GET /api/v1/evaluation/tasks/{task_id}

Response:
{
  "success": true,
  "data": {
    "task_id": "task-uuid",
    "status": "PROCESSING",
    "progress": 45,
    "stage": "generation",
    "created_at": "2026-05-05T10:00:00Z"
  }
}
```

#### 获取结果

```http
GET /api/v1/evaluation/tasks/{task_id}/results

Response:
{
  "success": true,
  "data": {
    "task_id": "task-uuid",
    "status": "COMPLETED",
    "aggregate": {
      "rouge_1": 75.5,
      "bleu_4": 68.2,
      "llm_judge_accuracy": 4.2
    },
    "items": [...]
  }
}
```

#### 列出可用评分器

```http
GET /api/v1/evaluation/python/graders

Response:
{
  "success": true,
  "data": {
    "graders": [
      {
        "name": "rouge",
        "mode": "pointwise",
        "description": "ROUGE 评分器",
        "parameters": {
          "types": {
            "type": "list",
            "description": "ROUGE 类型",
            "default": ["1", "2", "l"]
          }
        }
      },
      ...
    ]
  }
}
```

---

## 九、进度推送 (SSE)

### 9.1 SSE 端点

```http
GET /api/v1/evaluation/tasks/{task_id}/stream
```

### 9.2 事件格式

```
# 进度更新
event: progress
data: {"stage":"retrieval","current":5,"total":10,"message":"检索中..."}

# 完成
event: complete
data: {"task_id":"xxx","status":"completed","results":{...}}

# 错误
event: error
data: {"task_id":"xxx","error":"检索失败"}
```

### 9.3 Go SSE 实现

```go
func (h *EvaluationHandler) StreamTask(c *gin.Context) {
    taskID := c.Param("id")

    // 设置 SSE 头
    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")

    // 订阅 Redis 频道
    pubsub := redis.Subscribe(ctx, "eval:progress:"+taskID)
    defer pubsub.Close()

    // 发送 SSE 事件
    for {
        msg, err := pubsub.ReceiveMessage(ctx)
        if err != nil {
            break
        }

        // 格式化为 SSE
        fmt.Fprintf(c.Writer, "event: progress\ndata: %s\n\n", msg.Payload)
        c.Writer.Flush()
    }
}
```

### 9.4 前端示例

```javascript
const eventSource = new EventSource(`/api/v1/evaluation/tasks/${taskId}/stream`);

eventSource.addEventListener('progress', (e) => {
  const data = JSON.parse(e.data);
  console.log(`${data.stage}: ${data.current}/${data.total}`);
});

eventSource.addEventListener('complete', (e) => {
  const data = JSON.parse(e.data);
  console.log('完成:', data.results);
  eventSource.close();
});

eventSource.addEventListener('error', (e) => {
  console.error('错误:', e.data);
  eventSource.close();
});
```

---

## 十、实现步骤

### 10.1 Phase 1: 基础设施

- [ ] 同步 `evaluation.proto` 到 Go 和 Python
- [ ] 生成 Go gRPC 代码
- [ ] 实现 Redis 队列操作封装
- [ ] 实现并发控制逻辑

### 10.2 Phase 2: Python 服务

- [ ] 实现 `ComputeMetrics` 接口
- [ ] 实现评分器注册表初始化
- [ ] 测试各评分器功能

### 10.3 Phase 3: Go 核心逻辑

- [ ] 实现数据集加载器
- [ ] 实现 Worker 主体
- [ ] 实现评测编排逻辑
- [ ] 实现 gRPC 客户端

### 10.4 Phase 4: 接口和推送

- [ ] 实现 HTTP API
- [ ] 实现 SSE 推送
- [ ] 实现进度缓存到 Redis

### 10.5 Phase 5: 联调测试

- [ ] 端到端测试
- [ ] 并发测试
- [ ] 异常处理测试

---

## 附录

### A. 环境配置

#### Redis 版本说明

| 功能 | 最低版本 | 当前环境 |
|------|----------|----------|
| LPUSH/BRPOP（队列） | 2.0 | ✅ 3.2.100 |
| ZSet（延迟队列） | 2.0 | ✅ 3.2.100 |
| Pub/Sub（进度推送） | 2.0 | ✅ 3.2.100 |
| Hash（进度缓存） | 2.0 | ✅ 3.2.100 |
| INCR/DECR（并发控制） | 2.0 | ✅ 3.2.100 |

**当前环境**：Redis 3.2.100 (Windows) 运行在 `localhost:6379`，**完全满足评测系统需求** ✅

> **注意**：Redis 3.2.100 是 Windows 版本的最后一个稳定版本，虽无官方维护但功能稳定。长期可考虑迁移到 Docker/云 Redis。

```yaml
# Go .env
EVALUATION_DATASET_PATH=D:\link\evaluation\datasets
PYTHON_EVALUATION_ENDPOINT=localhost:50051
REDIS_ADDR=localhost:6379
EVALUATION_MAX_CONCURRENT=3
EVALUATION_WORKERS=3
```

```yaml
# Python .env
GRPC_PORT=50051
LLM_PROVIDER=openai
LLM_MODEL=gpt-4o-mini
```

### B. 数据集格式示例

```json
// D:\link\evaluation\datasets\sample_qa\default.json
{
  "dataset_id": "default",
  "description": "默认 QA 数据集",
  "items": [
    {
      "index": 0,
      "query": "什么是 RAG？",
      "reference": "RAG 是检索增强生成",
      "context": ["相关文档1", "相关文档2"]
    }
  ]
}
```

### C. 错误处理

| 错误码 | 说明 | 处理方式 |
|--------|------|----------|
| `EVAL_001` | 数据集不存在 | 返回 404 |
| `EVAL_002` | Python 服务不可用 | 标记任务 FAILED，重试 |
| `EVAL_003` | 评分器不存在 | 返回 400 |
| `EVAL_004` | 并发超限 | 放回队列 |
