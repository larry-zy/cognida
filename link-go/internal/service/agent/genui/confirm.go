package genui

import "fmt"

// ConfirmActionEndpoint 是危险操作确认卡片回调的 resume 端点（Phase 5 任务 6.3）。
// Confirm 组件确认时前端向该端点 POST {pending_action_id, token, session_id}，
// 由 handler 校验 token 后提交暂停中的事务（follow-up resume）。
const ConfirmActionEndpoint = "/api/v1/agent/operations/confirm"

// ConfirmInput 危险操作确认卡片的装配原料，来自 sql_mutate 的 pending_confirm 结果。
// 与其余 A2UI 路径一致：UI 契约在 Go 端拼装，前端只渲染。
type ConfirmInput struct {
	Surface         string // surface 标识（handler 生成，一次暂停一个）
	Target          string // 目标表
	RowsAffected    int64  // 事务内 dry-run 评估的影响行数
	Message         string // 工具返回的暂停说明（含阈值与有效期）
	PendingActionID string // 待确认操作凭据 ID
	ConfirmToken    string // 一次性确认 token（resume 必须携带）
	SessionID       string // 归属会话（owner key 的 session 分量）
}

// ConfirmCompose 把 sql_mutate 的危险操作暂停装配成确认卡片 surface：
// Callout 说明 + Confirm 交互组件。确认动作携 pending_action_id + token +
// session_id 回调 ConfirmActionEndpoint；取消则让 pending action 自然过期。
func ConfirmCompose(in ConfirmInput) *UISpec {
	spec := &UISpec{
		Surface: in.Surface,
		Title:   "危险操作待确认",
		Catalog: Catalog,
		GenMode: GenModeTemplate,
		// DataModel 仅承载元信息（无行集）；数字来自工具真实 dry-run 结果。
		DataModel: &DataModel{Meta: map[string]interface{}{
			"pending_action_id": in.PendingActionID,
			"target":            in.Target,
			"rows_affected":     in.RowsAffected,
		}},
	}

	spec.Components = []Component{
		{
			ID:   "notice",
			Type: CompCallout,
			Props: map[string]interface{}{
				"title": fmt.Sprintf("写操作已暂停：预计影响 %d 行", in.RowsAffected),
				"text":  in.Message,
				"tone":  "warning",
			},
		},
		{
			ID:   "confirm",
			Type: CompConfirm,
			Props: map[string]interface{}{
				"title":             fmt.Sprintf("确认对表 %s 执行该写操作？", in.Target),
				"text":              fmt.Sprintf("预计影响 %d 行，确认后立即提交且不可撤销；不确认将在有效期后自动失效。", in.RowsAffected),
				// 续跑三元组同时在顶层 props 暴露：通用渲染器 A2UINode.fireConfirm 读
				// 顶层 prop('pending_action_id'/'token'/'session_id')；action.params 保留
				// 声明式回调契约（endpoint/method）。两处一致，避免 token 落空。
				"pending_action_id": in.PendingActionID,
				"token":             in.ConfirmToken,
				"session_id":        in.SessionID,
				"confirmLabel":      "确认执行",
				"cancelLabel":       "取消",
				// 确认回调契约：POST endpoint + params（follow-up resume）
				"action": map[string]interface{}{
					"name":     "confirm_operation",
					"method":   "POST",
					"endpoint": ConfirmActionEndpoint,
					"params": map[string]interface{}{
						"pending_action_id": in.PendingActionID,
						"token":             in.ConfirmToken,
						"session_id":        in.SessionID,
					},
				},
			},
		},
		{ID: RootID, Type: CompColumn, Children: []string{"notice", "confirm"}},
	}
	return spec
}
