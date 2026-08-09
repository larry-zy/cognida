# agent-core Specification

## ADDED Requirements

### Requirement: eino_agent 执行主干去上帝对象化

`eino_agent` 的执行 SHALL 收敛为单一执行主干，MUST NOT 把 memory/tool/streaming 三个正交维度以笛卡尔积展开成多个 `chatWith*`/`streamWith*` 变体。当前 `chatWithMemory`/`chatWithMemoryAndTools`/`chatWithMemoryOnly`/`chatWithTools`/`chatWithoutTools`/`streamWithTools`/`streamWithoutTools` 等变体 SHALL 被消除。

#### Scenario: 单一执行主干

- **WHEN** 检查 `service/agent/framework/eino_agent.go`
- **THEN** 存在单一执行主干入口
- **AND** MUST NOT 存在 `chatWithMemoryAndTools`/`chatWithMemoryOnly`/`chatWithTools`/`chatWithoutTools`/`streamWithTools`/`streamWithoutTools` 等按维度展开的重复变体

### Requirement: memory/tool-loop/streaming 组件可插拔

memory、tool-loop、streaming 三个维度 SHALL 抽为可插拔组件（策略/组合），由执行主干组合调用，使新增一个维度的行为不再乘出新变体。

#### Scenario: 三维度独立可插拔

- **WHEN** 检查 eino_agent 执行组件划分
- **THEN** memory 上下文构建、tool 执行循环、streaming 输出 SHALL 各为独立组件
- **AND** 执行主干 SHALL 通过组合这些组件承载不同请求，MUST NOT 为组合逐个复制主干代码

#### Scenario: 拆解保持行为一致

- **WHEN** 对 memory×tool×streaming 各组合执行同一请求
- **THEN** 拆解后的执行主干 SHALL 产出与拆解前一致的响应/流式行为
- **AND** 组合矩阵回归测试 SHALL 全部通过
