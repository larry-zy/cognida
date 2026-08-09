// Package evaluation 提供评测系统的领域层定义，包括实体、值对象、仓储接口和领域错误。
//
// 领域模型：
//   - EvaluationTask: 评测任务实体，包含任务配置、状态和进度
//   - EvaluationResult: 评测结果实体，包含单个 QA 对的评测指标
//
// 仓储接口：
//   - EvaluationTaskRepository: 任务持久化接口
//   - EvaluationResultRepository: 结果持久化接口
//
// 领域错误：
//   - ErrTaskNotFound: 任务不存在
//   - ErrInvalidStatus: 无效状态转换
//   - ErrRepository: 仓储操作失败
package evaluation
