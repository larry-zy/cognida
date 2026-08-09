## MODIFIED Requirements

### Requirement: Model Instance Creation

The system SHALL create model instances from configurations. 由工厂创建的 chat / embedding / rerank 实例 SHALL 在返回给调用方之前经**弹性装饰器**包装（见 `llm-resilience`），使返回的实例天然具备重试与熔断能力。装饰对调用方透明——返回类型与既有创建契约、DTO 均不变。对 chat 模型，组合根 SHALL 在存在同租户、同类型的其余可用模型时装配**有序 fallback 链**（主模型为 `IsDefault`，其余按稳定顺序追加）；无备用模型时退化为单目标弹性（仅重试+熔断）。弹性能力 SHALL 可整体关闭（opt-out），关闭时退化为直连底层实例。

#### Scenario: Create chat model instance
- **WHEN** a chat model instance is requested
- **THEN** the use case validates tenant access
- **AND** validates model type is chat
- **AND** uses factory to create instance
- **AND** 工厂 SHALL 用弹性装饰器包装该实例后返回 the chat repository

#### Scenario: Create embedding model instance
- **WHEN** an embedding model instance is requested
- **THEN** the use case validates tenant access
- **AND** validates model type is embedding
- **AND** uses factory to create instance
- **AND** 工厂 SHALL 用弹性装饰器包装该实例后返回 the embedding repository

#### Scenario: Create rerank model instance
- **WHEN** a rerank model instance is requested
- **THEN** the use case validates tenant access
- **AND** validates model type is rerank
- **AND** uses factory to create instance
- **AND** 工厂 SHALL 用弹性装饰器包装该实例后返回 the rerank repository

#### Scenario: Chat 模型装配 fallback 链
- **WHEN** 请求创建 chat 模型实例，且该租户存在同类型的其余可用模型
- **THEN** 组合根 SHALL 以主模型（`IsDefault`）为首、其余按稳定顺序追加，构造有序 fallback 链
- **AND** 返回的实例在主模型不可用时 SHALL 自动降级到备用模型

#### Scenario: 无备用模型退化为单目标弹性
- **WHEN** 请求创建模型实例但无其余同类型可用模型
- **THEN** 返回的实例 SHALL 仅在该单一目标上做重试与熔断
- **AND** MUST NOT 因缺少备用模型而创建失败

#### Scenario: 弹性能力可关闭
- **WHEN** 弹性能力被配置为关闭（opt-out）
- **THEN** 工厂 SHALL 返回直连底层的实例
- **AND** 行为 SHALL 与未引入弹性装饰前等价
