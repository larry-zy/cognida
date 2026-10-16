## Why

HF 和 SFT 已有独立页面、API 和工具，但缺少 Supervisor 持久任务适配器，无法作为同一个任务中可恢复、可取消、可追踪的节点。技术支持与故障排查需要把明确来源的知识和已有问答经过清洗、准备、审核，分别供知识检索和 SFT 使用。

## What Changes

- 注册固定版本、有界 HF 导入和基于 prepared 来源及已发布标准的 SFT 候选构建能力。
- 实现持久提交、查询、取消栅栏、截止时间、授权复查和真实产物绑定，复用既有独立服务。
- 补齐 Supervisor 参数合同、规划说明、任务页面、领域回执和结果导航。
- 在隔离环境用真实公开技术支持数据及明确故障资料完成页面任务、SFT 人工审核导出、知识库检索和 Agent 分析；保留逐阶段证据。

## Capabilities

### New Capabilities

- `supervisor-data-acquisition`: 固定 HF 来源作为可持久编排的数据采集根节点。
- `supervisor-sft-candidate`: 准备资产到 SFT 待审候选的持久编排与独立能力兼容。
- `technical-support-workflow`: 技术支持资料从来源到 SFT、知识库与 Agent 使用的可验证场景。

### Modified Capabilities

无。现有独立能力与人工审批权限保留。

## Impact

Go taskcontrol/HF/governance 服务、MySQL 版本迁移、领域 worker、Vue 任务及领域工作台、真实数据库/API/Playwright 测试和 README。不会启动模型训练或 RL；不绕过人工审核、来源授权、网络出口限制和导入限额。
