## ADDED Requirements

### Requirement: 从 HuggingFace 导入评测数据集

系统 SHALL 支持将 HuggingFace（或其他外部来源）数据集转换为本项目评测数据集格式并导入。转换器 SHALL 将源数据映射为评测样本字段，并落地为可导入的产物。

#### Scenario: 通用 QA 数据集转换
- **WHEN** 对一个包含问题/答案列的 HF 数据集执行转换
- **THEN** 每条记录映射为评测样本的 `question` 与 `reference_answer`
- **AND** 产物为标准评测数据集格式（DB 记录或 JSONL）

#### Scenario: Agent/工具调用基准转换
- **WHEN** 对一个含期望工具调用/期望步骤的 Agent 基准数据集执行转换
- **THEN** 每条记录除 `question`/`reference_answer` 外，附带期望工具与期望步骤字段
- **AND** 这些字段可供 Agent 评测的 tool_selection / trajectory 指标使用

#### Scenario: 字段缺失的容错
- **WHEN** 源数据集缺少必需的问题或答案列
- **THEN** 转换器跳过该条并记录告警计数
- **AND** 不因单条脏数据中断整批导入

### Requirement: 数据集样本支持 Agent 评测字段

评测数据集样本模型 SHALL 支持 Agent 评测所需的可选字段（期望工具调用列表、期望步骤/轨迹），且对 QA/RAG 样本保持向后兼容（字段为空即可）。

#### Scenario: Agent 样本携带期望工具
- **WHEN** 保存一条 Agent 评测样本
- **THEN** 样本可持久化期望工具调用与期望步骤
- **AND** 读取时可完整还原这些字段

#### Scenario: QA 样本无 Agent 字段
- **WHEN** 保存一条不含 Agent 字段的 QA 样本
- **THEN** 系统正常保存，Agent 相关字段为空
- **AND** 现有 QA/RAG 评测流程不受影响

### Requirement: seed 脚本一键导入

系统 SHALL 提供 seed 脚本，将预选的 HF 数据集下载、转换并写入评测数据集表（`evaluation_datasets` 与样本表）。脚本 SHALL 幂等，可重复执行不产生重复数据集。

#### Scenario: 首次执行 seed
- **WHEN** 运行评测数据集 seed 脚本
- **THEN** 预选数据集被写入 `evaluation_datasets` 及其样本表
- **AND** 每个数据集标注其评测类型（agent / qa）

#### Scenario: 重复执行 seed 幂等
- **WHEN** 再次运行同一 seed 脚本
- **THEN** 已存在的数据集被更新或跳过而非重复插入
- **AND** 样本数量与首次一致

### Requirement: 前端上传导入路径

系统 SHALL 保留经数据集管理页上传 JSONL 导入的路径。HF 转换产物 JSONL SHALL 与该上传通道的样本格式兼容。

#### Scenario: 上传转换产物
- **WHEN** 用户在数据集管理页上传 HF 转换生成的 JSONL
- **THEN** 系统按现有样本格式解析并创建数据集
- **AND** 导入后的数据集可被评测任务选用
