# Proposal: Evaluation System

## Why

当前 Link 项目缺少完整的评测系统，无法对 RAG、QA、Agent 等功能进行系统性的质量评估。需要构建一个支持多种评测类型、可扩展的评测框架，用于验证和优化系统性能。

## What Changes

- **新增评测系统模块**：支持 RAG、QA、Agent 三种评测类型
- **Go Worker + Redis 队列**：异步任务处理，支持持久化和重试
- **Python HTTP API**：提供批量指标计算服务（ROUGE、BLEU、LLM 裁判等）
- **SSE 实时推送**：前端通过轮询 Redis 获取评测进度
- **混合数据集存储**：公共数据集（文件系统）+ 用户数据集（数据库）
- **内嵌 Worker**：在 API 服务进程中运行，并发数=3，支持重试（最多 3 次）

## Capabilities

### New Capabilities

- `evaluation-tasks`: 评测任务管理（创建、查询、状态跟踪）
- `evaluation-worker`: 后台 Worker 执行（队列、并发控制、重试）
- `evaluation-executor`: 评测执行器（Agent/RAG/QA 三种模式）
- `evaluation-dataset`: 数据集加载（文件系统 + 数据库混合模式）
- `evaluation-metrics`: 指标计算服务（Python HTTP API）
- `evaluation-progress`: 进度推送（SSE + Redis 轮询）

### Modified Capabilities

*无*

## Impact

**新增依赖**：
- Redis（已有，用于任务队列和进度缓存）
- Python 评测服务（新增 FastAPI 服务）

**新增代码**：
- `link-go/internal/application/evaluation/`: Worker、执行器、Python 客户端
- `link-go/internal/infrastructure/redis/evaluation/`: Redis 队列和进度操作
- `link-python/services/evaluation/`: 指标计算 HTTP API

**新增 API**：
- `POST /api/v1/evaluation/tasks` - 创建评测任务
- `GET /api/v1/evaluation/tasks/{id}` - 查询任务状态
- `GET /api/v1/evaluation/tasks/{id}/stream` - SSE 进度推送
- `GET /api/v1/evaluation/tasks/{id}/results` - 获取评测结果

**新增数据表**（已有）：
- `evaluation_results` - 评测结果汇总
- `evaluation_qa_results` - QA 详细结果
