# datasource-management-ui Specification

## ADDED Requirements

### Requirement: 数据源管理页

前端 SHALL 提供数据源管理视图：列表展示（名称/类型/主机/状态）、新建、编辑、删除数据源。表单 SHALL 内置"测试连接"操作并在保存前可用；编辑时密码输入框留空表示不修改。页面 SHALL 使用自研 `Ui*` 组件而非直接使用 Element Plus。

#### Scenario: 新建数据源前先测试连接

- **WHEN** 用户填写完数据源表单并点击"测试连接"
- **THEN** 前端 SHALL 调用测试连接接口并展示成功/失败原因
- **AND** 测试结果 SHALL NOT 阻塞用户选择继续保存

#### Scenario: 列表展示连接状态

- **WHEN** 用户打开数据源管理页
- **THEN** 页面 SHALL 展示各数据源的名称、类型、主机与最近状态

#### Scenario: 编辑时密码留空

- **WHEN** 用户编辑数据源且不填密码直接保存
- **THEN** 前端 SHALL 不提交密码字段，后端保留原密码

### Requirement: Data Agent 数据源选择器

Data Agent 视图 SHALL 提供数据源选择器，默认选中"当前库"（不传 `datasource_id`）；选择外部数据源后，该会话的数据问答请求 SHALL 透传对应 `datasource_id`。

#### Scenario: 默认当前库

- **WHEN** 用户未选择任何外部数据源发起数据问答
- **THEN** 请求 SHALL 不携带 `datasource_id`，行为与现状一致

#### Scenario: 选择外部数据源后透传

- **WHEN** 用户在选择器中选中某已注册数据源并提问
- **THEN** `streamDataChat` 请求 SHALL 携带该 `datasource_id`
- **AND** 本会话后续提问 SHALL 沿用该选择直至用户变更
