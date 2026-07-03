## ADDED Requirements

### Requirement: Schema 语义选表

系统 SHALL 从库中挑选与当前问题相关的表/列子集注入提示词，替代把全库 schema 一次性灌入。`get_schema` 在未指定表名时 MUST NOT 默认返回全库所有表，而是返回按相关度排序的候选子集。相关度实现为**词法匹配**：以关键词对全部表的描述卡（表名/表注释/列名/列注释）打分，表名命中权重更高。语义向量检索（Milvus）为后续升级项，本变更暂不引入；升级时本要求的不变量（禁止全库无界注入、受上限约束的回退）保持不变。

#### Scenario: 按问题相关度选表

- **WHEN** 用户问"各区域销售额"，库中有 300 张表
- **THEN** 系统 SHALL 通过词法相关度从全部表描述中返回与"区域/销售"相关的候选表子集
- **AND** SHALL NOT 把全部 300 张表的结构注入提示词

#### Scenario: 无相关表命中时回退

- **WHEN** 语义检索未命中任何相关表
- **THEN** 系统 SHALL 返回受上限约束的少量候选或明确的"未找到相关表"提示
- **AND** SHALL NOT 无上限地回退为全库注入

### Requirement: 观察压缩

系统 SHALL 在工具输出追加进对话历史前对"观察"（observation）做压缩：原始结果行以结果信封替代，冗长工具输出 MUST 被截断或摘要。压缩 SHALL 在 Agent 循环追加 ToolMessage 的环节统一施加。

#### Scenario: 工具输出以信封入历史

- **WHEN** `sql_execute` 返回大结果集
- **THEN** 追加进历史的 ToolMessage SHALL 是结果信封而非原始行
- **AND** 后续循环 SHALL 基于信封而非原始行进行推理

### Requirement: 历史窗口与摘要

系统 SHALL 对多轮对话历史施加窗口化与摘要策略，MUST NOT 每轮把全量历史逐字回灌 LLM。超出窗口的早期轮次 SHALL 被摘要为紧凑记忆，保留关键决策、result_id 与结论。

#### Scenario: 超窗历史被摘要

- **WHEN** 会话累计轮次超过配置窗口大小
- **THEN** 系统 SHALL 把最早的若干轮压缩为摘要
- **AND** 摘要 SHALL 保留仍被引用的 `result_id` 与关键结论

### Requirement: 分层提示词与 token 预算治理

系统 SHALL 以分层动态提示词组织上下文（系统层 / 能力与工具层 / skill playbook 层 / 会话记忆层），并 MUST 对每轮注入实施 token 预算治理。当注入内容超预算时，系统 SHALL 按优先级裁剪低优先层，MUST 保留系统约束与安全策略层。

#### Scenario: 超预算时按层裁剪

- **WHEN** 单轮待注入上下文超过 token 预算
- **THEN** 系统 SHALL 优先裁剪会话记忆/样本等低优先内容
- **AND** SHALL 保留系统约束、安全 scope 与当前工具契约

#### Scenario: 预算与 skill 注入协同

- **WHEN** 命中的 skill playbook 注入使上下文接近预算上限
- **THEN** 系统 SHALL 在 token 预算内协调 playbook 与历史摘要的占用
- **AND** SHALL NOT 因 skill 注入而挤掉安全策略层
