## ADDED Requirements

### Requirement: 结构化错误分级

Data Agent 的数据工具在执行失败时 SHALL 把错误归入一个结构化 `error_kind`，并作为可修复观察的一部分回灌 LLM，MUST NOT 仅回灌裸底层 driver 错误文本。`error_kind` 枚举 SHALL 至少覆盖：`syntax`、`unknown_column`、`unknown_table`、`timeout`、`permission`、`transient`、`other`。每条可修复观察 SHALL 携带 `retriable`（布尔）标识该错误是否值得在修正后重试。

#### Scenario: 列名错误被归类为 unknown_column

- **WHEN** `sql_execute` 因引用不存在的列而失败
- **THEN** 回灌 LLM 的观察 SHALL 含 `error_kind = "unknown_column"`
- **AND** SHALL 含 `retriable = true`

#### Scenario: 语法错误被归类为 syntax

- **WHEN** `sql_execute` 因 SQL 语法错误而失败
- **THEN** 回灌 LLM 的观察 SHALL 含 `error_kind = "syntax"` 且 `retriable = true`

#### Scenario: 未识别错误归入 other

- **WHEN** 底层错误无法匹配任何已知分类
- **THEN** 观察 SHALL 含 `error_kind = "other"`
- **AND** SHALL 保留一段脱敏后的原始错误摘要供 LLM 判断

### Requirement: Schema-grounded 修复线索

对 `unknown_column` 与 `unknown_table` 两类错误，工具 SHALL 从错误信息中解析出触发错误的标识符，并附上同源 schema 的可用候选清单作为 `hint`，使 LLM 能在一次重试内完成修正。`unknown_column` 的 `hint` SHALL 包含相关表的可用列名；`unknown_table` 的 `hint` SHALL 包含可用表名候选。当无法解析标识符或无法获取 schema 时，工具 SHALL 降级为不带候选清单的通用提示，MUST NOT 因线索缺失而使整个工具调用崩溃。

#### Scenario: 列名错误附可用列清单

- **WHEN** `sql_execute` 因 `unknown_column` 失败且能定位相关表
- **THEN** 观察的 `hint` SHALL 列出该表的可用列名
- **AND** LLM 得以据此改写 SQL 后重试

#### Scenario: 表名错误附可用表候选

- **WHEN** `sql_execute` 因 `unknown_table` 失败
- **THEN** 观察的 `hint` SHALL 列出可用表名候选

#### Scenario: schema 不可得时安全降级

- **WHEN** 触发 `unknown_column`/`unknown_table` 但 schema 检索失败
- **THEN** 工具 SHALL 返回不含候选清单的通用 `hint`
- **AND** MUST NOT panic 或吞掉错误上下文

### Requirement: 瞬时错工具内退避重试

对归类为 `transient`（如连接抖动、死锁、可重试超时）的错误，工具 SHALL 在自身内部以有限次数 + 退避策略自动重试，MUST NOT 为每次瞬时抖动都触发一轮 LLM 生成。仅当重试预算耗尽后，工具才 SHALL 把 `transient` 错误作为可修复观察上抛给 LLM。工具内重试次数 SHALL 有明确上限以防止级联放大。

#### Scenario: 瞬时错自动重试后成功

- **WHEN** 查询首次因瞬时错失败、重试后成功
- **THEN** 工具 SHALL 返回成功结果信封
- **AND** LLM SHALL NOT 观察到中间的瞬时失败

#### Scenario: 瞬时错重试耗尽后上抛

- **WHEN** 瞬时错在达到工具内重试上限后仍失败
- **THEN** 工具 SHALL 回灌 `error_kind = "transient"` 的可修复观察
- **AND** SHALL 标注已耗尽自动重试预算

### Requirement: 重复失败护栏与动态重规划

ReAct 编排循环 SHALL 追踪连续失败的工具调用，并在同一工具/同一错误签名连续失败达到阈值时，注入一条「换策略」重规划提示，引导 LLM 改变方法（换表/换口径/反问用户）而非盲目重复同一失败调用。当注入重规划提示后仍持续失败达到二次上限时，循环 SHALL 优雅收尾（wind-down）给出诚实的部分结论，MUST NOT 把全部 `maxIter` 预算耗尽在同一失败签名上。

#### Scenario: 连续同错触发重规划提示

- **WHEN** 同一工具以相同错误签名连续失败达到阈值
- **THEN** 循环 SHALL 注入换策略重规划提示
- **AND** 该提示 SHALL 提示改变方法而非重复原调用

#### Scenario: 重规划后仍失败则优雅收尾

- **WHEN** 注入重规划提示后错误仍持续达到二次上限
- **THEN** 循环 SHALL 提前 wind-down 收尾
- **AND** 结论 SHALL 诚实说明未能完成及原因，MUST NOT 假装成功

#### Scenario: 成功调用重置失败计数

- **WHEN** 某工具在若干失败后返回一次成功观察
- **THEN** 该工具的连续失败计数 SHALL 归零
