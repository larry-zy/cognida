# Python Evaluation Service

## Why

Go 服务中的评测系统存在多个生产级别的问题：
- 中文分词不正确（按字符分割而非词语）
- RAG 评测中上下文是假的（只传文档 ID 而非内容）
- 缺少 LLM-as-Judge 评估维度
- 缺少语义相似度计算

Python 拥有更成熟的 NLP 生态（jieba、sentence-transformers、rouge 等），更适合实现生产级评测。

## What Changes

在 Python 服务中实现完整的评测能力：
- **新增** `services.evaluation` 模块，包含评测服务实现
- **新增** gRPC 评测服务接口，支持流式返回进度和结果
- **新增** 本地评测数据集管理
- **新增** 多种内置评测指标：检索指标（Precision/Recall/NDCG/MRR/MAP）、生成指标（ROUGE/BLEU）、语义相似度
- **新增** 正确的中文分词支持
- **新增** 插件式评测器（Grader）系统，支持自定义指标和算法
- **新增** LLM-as-Judge 作为可组合的评测组件
- **新增** 灵活的评测策略支持（零样本生成、数据驱动生成）

Go 服务保持职责：
- 任务创建和管理
- 进度查询和结果存储
- 鉴权和路由

## Capabilities

### New Capabilities
- `evaluation-execution`: Python 执行评测任务并流式返回进度和最终结果
- `evaluation-metrics`: 检索指标（Precision/Recall/NDCG/MRR/MAP）、生成指标（ROUGE/BLEU）、语义相似度
- `evaluation-graders`: 插件式评测器系统，支持自定义指标、自定义算法、LLM-as-Judge 组件
- `evaluation-datasets`: 本地评测数据集存储和管理
- `evaluation-grpc`: gRPC 流式接口支持

### Modified Capabilities
- (无现有评测功能，纯新增)

## Impact

**代码影响：**
- 新增 `services/evaluation/` 目录及实现
- 新增 `proto/evaluation.proto` 定义
- 更新 `grpc/servicer.py` 添加评测服务

**API 影响：**
- 新增 gRPC `EvaluationService`，提供 `ExecuteEvaluation` 流式接口

**依赖影响：**
- 新增 Python 依赖：jieba（分词）、sentence-transformers（语义相似度）、rouge-chinese（ROUGE 指标）
- 新增 Python 依赖：importlib-metadata（插件发现）、pydantic（配置验证）
- 需要 Go 提供知识库查询接口（用于 RAG 检索）

**系统影响：**
- 评测数据集存储在 Python 服务本地
- Go 通过 gRPC 调用 Python 执行评测
- 评测结果由 Go 接收后存储到自己的数据库
