# service-proto-contract Specification

## ADDED Requirements

### Requirement: proto 顶层单一源

系统 SHALL 以顶层 `proto/` 目录作为跨服务 proto 契约的唯一 source-of-truth，收纳 `analytics`/`docreader`/`evaluation`/`judge`/`quality` 等 `.proto`。系统 MUST NOT 在 `cognida-go/api/proto` 与 `cognida-python/proto` 各维护一份手抄 `.proto`。

#### Scenario: 单一源目录存在

- **WHEN** 检查仓库顶层
- **THEN** 存在 `proto/` 目录，包含全部跨服务 `.proto` 定义
- **AND** `cognida-go/api/proto` 与 `cognida-python/proto` 下 MUST NOT 再保留手抄的 `.proto` 源文件

### Requirement: buf generate 同产 Go 与 Python stub

系统 SHALL 用 `buf generate`（`buf.yaml` + `buf.gen.yaml`）从顶层 `proto/` 同时生成 Go 与 Python stub。生成 SHALL 由 `make proto` 封装。

#### Scenario: 生成 Go 与 Python stub

- **WHEN** 运行 `buf generate`（或 `make proto`）
- **THEN** 系统 SHALL 从 `proto/` 生成 Go stub 到 cognida-go 生成目录
- **AND** SHALL 同时生成 Python stub 到 cognida-python 生成目录

#### Scenario: buf 配置存在

- **WHEN** 检查仓库
- **THEN** 存在 `buf.yaml` 与 `buf.gen.yaml`
- **AND** `make proto` 目标 SHALL 调用 `buf generate`

### Requirement: CI 校验生成物一致性

系统 SHALL 在 CI 中校验生成物与提交一致：跑 `buf generate` 后若生成物与仓库现状有差异则失败，防止手改生成物或 proto/生成物 drift。

#### Scenario: 生成物 drift 时 CI 失败

- **WHEN** CI 运行 `buf generate` 后执行 `git diff --exit-code`
- **THEN** 若生成物与提交不一致，CI SHALL 失败
- **AND** 失败信息 SHALL 指向需重新生成的 stub

### Requirement: 跨服务统一错误码契约

系统 SHALL 在 proto 中定义统一错误码 `enum ErrorCode`，作为 Go 与 Python 跨服务通信的共享错误码契约，二者 SHALL 复用同一生成的枚举，MUST NOT 各自硬编码错误码字面量。

#### Scenario: 错误码定义于 proto

- **WHEN** 检查 `proto/` 契约
- **THEN** 存在 `enum ErrorCode` 定义跨服务错误码
- **AND** Go 与 Python 侧 SHALL 使用该枚举生成物，MUST NOT 各自维护错误码常量
