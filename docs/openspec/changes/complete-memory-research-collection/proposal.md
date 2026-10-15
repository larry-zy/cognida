## Why

当前长期记忆仅有接口，RAG 优化器和 DeepResearch 未形成可用产品链路；采集流水线存在发布目标缺失、分析目标不执行、容器计算服务接线遗漏。用户于 2026-10-14 授权补齐这四块，并确认包含工作台参数、报告目标和 Docker/MCP/认证。

## What Changes

- 实现按租户和用户隔离的持久长期记忆、显式写入/修改/遗忘、跨会话召回、管理页面及 Agent 接入。
- 将 HyDE、查询改写、多跳检索接入 Agent 工具并补齐图谱 Retriever 适配，沿用现有授权范围。
- 实现有界研究规划、多源证据收集、引用校验、综合报告与真实 SSE 进度，提供 DeepResearch 页面和 API。
- 补齐文本发布目标及专家/阶段参数；让分析/报告目标真正产生有来源的模型分析；修复容器中计算和 MCP 服务启动及共享认证。
- 完成版本迁移、单元/API/真实数据库集成、前端测试、独立 Review、文档同步与 main 提交。

## Capabilities

### New Capabilities
- `personal-long-term-memory`: 用户私有、可管理可遗忘的持久记忆及跨会话召回。
- `scoped-research-workflow`: 授权范围内的可引用多阶段研究工作流。
- `collection-publication-completion`: 参数完整、目标生效且可部署的采集发布分析链路。

### Modified Capabilities
- `rag-service`: 优化检索工具和底层图谱检索使用同一授权边界并返回真实来源。

## Impact

Go memory/agent/knowledge/lifecycle/governance、MySQL 成对迁移、Python lifecycle 和启动入口、Vue 工作台与新页面、Docker Compose、测试及运行文档。保留既有会话、采集任务、资产版本、审批和经验蒸馏；不自动更改生产策略，不引入未经授权的共享记忆。
