// Package semantic 提供指标语义层仓储接口定义。
package semantic

import "context"

// Repository 是指标语义模型的持久化契约。
//
// 读多写少：查询主路径（NL2Semantics）以读为主（按租户/名称加载生效模型的
// ModelBundle）；建模控制台的写入不在本 change 范围，接口仅保留 Upsert 供建模侧接入。
type Repository interface {
	// GetActiveModel 按租户 + 逻辑名加载「生效」语义模型的完整 bundle；
	// 无生效模型返回 ErrModelNotFound，供调用方回退词法路径。
	GetActiveModel(ctx context.Context, tenantID int64, name string) (*ModelBundle, error)

	// ListActiveModels 列出租户下全部生效语义模型（仅模型头，不含构件），
	// 供意图识别时判断请求可能命中哪个语义域。
	ListActiveModels(ctx context.Context, tenantID int64) ([]*SemanticModel, error)

	// GetModelBundle 按模型 ID 加载完整 bundle（含全部构件）。
	GetModelBundle(ctx context.Context, modelID string) (*ModelBundle, error)

	// UpsertBundle 全量写入/更新一个语义模型及其构件（建模侧使用，事务内幂等）。
	UpsertBundle(ctx context.Context, bundle *ModelBundle) error

	// BumpVersion 递增模型版本（语义变更后调用，驱动 Verified Query 缓存失效）。
	BumpVersion(ctx context.Context, modelID string) (int, error)
}
