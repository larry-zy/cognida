// Package agent 提供 Agent 协作上下文的 Go context 传递支持
package agent

import (
	"context"
	"sync"
)

// ========================================
// Context Keys - 用于在 context.Context 中传递协作上下文
// ========================================

// contextKey 是内部类型，防止外部包冲突。
// 注意必须携带可区分的值：空结构体的所有实例相等，若各 key 都是 contextKey{}，
// 后写入的 WithValue 会遮蔽先写入的（曾导致 TenantID 恒为 0、Result Store 归属键失效）。
type contextKey string

// 定义 context key 变量
var (
	// CollaborationCtxKey 协作上下文 key
	CollaborationCtxKey = contextKey("collaboration_ctx")
	// SessionIDKey 会话 ID key
	SessionIDKey = contextKey("session_id")
	// TenantIDKey 租户 ID key
	TenantIDKey = contextKey("tenant_id")
	// UserIDKey 用户 ID key
	UserIDKey = contextKey("user_id")
	// RequestIDKey 请求 ID key（用于链路追踪）
	RequestIDKey = contextKey("request_id")
	// ToolScopeKey 会话工具 scope key（read / write / etl，Phase 6 硬工具门）
	ToolScopeKey = contextKey("tool_scope")
	// AllowedKBIDsKey 用户在会话入口选定的知识库范围 key（[]string，空表示不限定=全部已启用知识库）
	AllowedKBIDsKey = contextKey("allowed_kb_ids")
	// GraphEnabledKey 用户是否开启图谱增强 key（bool）
	GraphEnabledKey = contextKey("graph_enabled")
	// KBScopeModeKey 知识库选择模式 key（manual/hybrid/auto，见 KBScopeMode* 常量）
	KBScopeModeKey = contextKey("kb_scope_mode")
	// RouteSelectionKey Agent 在会话内经 kb_route 声明的聚焦知识库 key（*RouteSelection 指针）
	RouteSelectionKey = contextKey("kb_route_selection")
	// DatasourceIDKey 会话选定的外部数据源 ID key（string，空表示当前业务库）
	DatasourceIDKey = contextKey("datasource_id")
	// RetrievalParamsKey 会话级检索参数 key（*RetrievalParams，用户在会话设置里持久化）
	RetrievalParamsKey = contextKey("retrieval_params")
)

// 知识库选择模式常量。
const (
	// KBScopeModeManual 手动：范围完全由用户勾选决定，AI 不参与选库（默认）。
	KBScopeModeManual = "manual"
	// KBScopeModeHybrid 结合：用户勾选定义候选池，AI 在池内经 kb_route 自选。
	KBScopeModeHybrid = "hybrid"
	// KBScopeModeAuto 智能：忽略用户勾选，AI 从租户全部已启用库中经 kb_route 自选。
	KBScopeModeAuto = "auto"
)

// ========================================
// CollaborationContext 传递辅助函数
// ========================================

// WithCollaborationContext 将协作上下文注入到 Go context 中
func WithCollaborationContext(ctx context.Context, cc *CollaborationContext) context.Context {
	return context.WithValue(ctx, CollaborationCtxKey, cc)
}

// GetCollaborationContext 从 Go context 中获取协作上下文
func GetCollaborationContext(ctx context.Context) (*CollaborationContext, bool) {
	cc, ok := ctx.Value(CollaborationCtxKey).(*CollaborationContext)
	return cc, ok
}

// MustGetCollaborationContext 获取协作上下文，不存在则返回 nil
// 与 GetCollaborationContext 的区别是只返回值，不返回 bool
func MustGetCollaborationContext(ctx context.Context) *CollaborationContext {
	cc, _ := GetCollaborationContext(ctx)
	return cc
}

// ========================================
// SessionID 传递辅助函数
// ========================================

// WithSessionID 将会话 ID 注入到 Go context 中
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, SessionIDKey, sessionID)
}

// GetSessionID 从 Go context 中获取会话 ID
func GetSessionID(ctx context.Context) (string, bool) {
	sessionID, ok := ctx.Value(SessionIDKey).(string)
	return sessionID, ok
}

// MustGetSessionID 获取会话 ID，不存在则返回空字符串
func MustGetSessionID(ctx context.Context) string {
	sessionID, _ := GetSessionID(ctx)
	return sessionID
}

// ========================================
// TenantID 传递辅助函数
// ========================================

// WithTenantID 将租户 ID 注入到 Go context 中
func WithTenantID(ctx context.Context, tenantID int64) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// GetTenantID 从 Go context 中获取租户 ID
func GetTenantID(ctx context.Context) (int64, bool) {
	tenantID, ok := ctx.Value(TenantIDKey).(int64)
	return tenantID, ok
}

// MustGetTenantID 获取租户 ID，不存在则返回 0
func MustGetTenantID(ctx context.Context) int64 {
	tenantID, _ := GetTenantID(ctx)
	return tenantID
}

// ========================================
// UserID 传递辅助函数
// ========================================

// WithUserID 将用户 ID 注入到 Go context 中
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetUserID 从 Go context 中获取用户 ID
func GetUserID(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(UserIDKey).(int64)
	return userID, ok
}

// MustGetUserID 获取用户 ID，不存在则返回 0
func MustGetUserID(ctx context.Context) int64 {
	userID, _ := GetUserID(ctx)
	return userID
}

// ========================================
// RequestID 传递辅助函数（链路追踪）
// ========================================

// WithRequestID 将请求 ID 注入到 Go context 中
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID 从 Go context 中获取请求 ID
func GetRequestID(ctx context.Context) (string, bool) {
	requestID, ok := ctx.Value(RequestIDKey).(string)
	return requestID, ok
}

// MustGetRequestID 获取请求 ID，不存在则返回空字符串
func MustGetRequestID(ctx context.Context) string {
	requestID, _ := GetRequestID(ctx)
	return requestID
}

// ========================================
// ToolScope 传递辅助函数（Phase 6 硬工具门）
// ========================================

// WithToolScope 将会话工具 scope（read / write / etl）注入到 Go context 中。
// scope 由入口（handler / 委派方）授予，与 skill 策略共同构成工具放行的必要条件。
func WithToolScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, ToolScopeKey, scope)
}

// GetToolScope 从 Go context 中获取会话工具 scope
func GetToolScope(ctx context.Context) (string, bool) {
	scope, ok := ctx.Value(ToolScopeKey).(string)
	return scope, ok
}

// MustGetToolScope 获取会话工具 scope，不存在则返回空字符串（由策略层按最小权限兜底）
func MustGetToolScope(ctx context.Context) string {
	scope, _ := GetToolScope(ctx)
	return scope
}

// ========================================
// AllowedKBIDs 传递辅助函数（用户选定的知识库范围）
// ========================================

// WithAllowedKBIDs 将用户选定的知识库范围注入到 Go context 中。
// 该范围由入口 handler 从请求 kb_ids 授予，工具层据此做 scope 强制（求交/拒绝越权）。
// 传入空切片视为“不限定”，工具层按“全部已启用知识库”处理。
func WithAllowedKBIDs(ctx context.Context, kbIDs []string) context.Context {
	return context.WithValue(ctx, AllowedKBIDsKey, kbIDs)
}

// GetAllowedKBIDs 从 Go context 中获取用户选定的知识库范围。
// 第二个返回值表示是否显式设置过（区分“未设置”与“设置为空”）。
func GetAllowedKBIDs(ctx context.Context) ([]string, bool) {
	kbIDs, ok := ctx.Value(AllowedKBIDsKey).([]string)
	return kbIDs, ok
}

// MustGetAllowedKBIDs 获取知识库范围，未设置则返回 nil
func MustGetAllowedKBIDs(ctx context.Context) []string {
	kbIDs, _ := GetAllowedKBIDs(ctx)
	return kbIDs
}

// ========================================
// GraphEnabled 传递辅助函数（图谱增强开关）
// ========================================

// WithGraphEnabled 将图谱增强开关注入到 Go context 中
func WithGraphEnabled(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, GraphEnabledKey, enabled)
}

// GetGraphEnabled 从 Go context 中获取图谱增强开关
func GetGraphEnabled(ctx context.Context) (bool, bool) {
	enabled, ok := ctx.Value(GraphEnabledKey).(bool)
	return enabled, ok
}

// IsGraphEnabled 图谱增强是否开启，未设置默认 false
func IsGraphEnabled(ctx context.Context) bool {
	enabled, _ := GetGraphEnabled(ctx)
	return enabled
}

// ========================================
// KBScopeMode 传递辅助函数（知识库选择模式）
// ========================================

// WithKBScopeMode 将知识库选择模式（manual/hybrid/auto）注入到 Go context 中。
// 非法值由 MustGetKBScopeMode 兜底为 manual。
func WithKBScopeMode(ctx context.Context, mode string) context.Context {
	return context.WithValue(ctx, KBScopeModeKey, mode)
}

// GetKBScopeMode 从 Go context 中获取知识库选择模式
func GetKBScopeMode(ctx context.Context) (string, bool) {
	mode, ok := ctx.Value(KBScopeModeKey).(string)
	return mode, ok
}

// MustGetKBScopeMode 获取知识库选择模式，未设置或非法值默认 manual（向后兼容）。
func MustGetKBScopeMode(ctx context.Context) string {
	switch mode, _ := GetKBScopeMode(ctx); mode {
	case KBScopeModeHybrid:
		return KBScopeModeHybrid
	case KBScopeModeAuto:
		return KBScopeModeAuto
	default:
		return KBScopeModeManual
	}
}

// ========================================
// DatasourceID 传递辅助函数（会话数据源上下文）
// ========================================

// WithDatasourceID 将会话选定的外部数据源 ID 注入到 Go context 中。
// 查询类工具（get_schema/sql_execute）在请求未显式传 database_id 时以此为默认值；
// 空字符串等同未设置（当前业务库，向后兼容）。
func WithDatasourceID(ctx context.Context, datasourceID string) context.Context {
	return context.WithValue(ctx, DatasourceIDKey, datasourceID)
}

// GetDatasourceID 从 Go context 中获取会话数据源 ID
func GetDatasourceID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(DatasourceIDKey).(string)
	return id, ok
}

// MustGetDatasourceID 获取会话数据源 ID，未设置则返回空字符串（当前业务库）
func MustGetDatasourceID(ctx context.Context) string {
	id, _ := GetDatasourceID(ctx)
	return id
}

// ========================================
// RetrievalParams 传递辅助函数（会话级检索参数治理）
// ========================================

// RetrievalParams 承载用户在会话设置里持久化的检索治理参数，经入口 handler 从
// RetrievalSetting 加载并注入 ctx；检索适配器读取后覆盖 LLM 经工具参数给出的即兴取值。
// 全部字段用指针以区分「未设置」与「显式设为零值」——nil 表示不覆盖，回退工具参数/系统默认。
type RetrievalParams struct {
	// TopK 会话级返回片段数（覆盖工具 top_k）
	TopK *int
	// MinScore 会话级最小相似度阈值（覆盖工具 min_score）
	MinScore *float64
	// RerankEnabled 会话级重排开关（覆盖工具 enable_rerank）
	RerankEnabled *bool
	// Alpha 混合检索向量/关键词权重（工具参数未暴露，仅会话级可控）
	Alpha *float64
}

// WithRetrievalParams 将会话级检索参数注入到 Go context 中（nil 不注入）。
func WithRetrievalParams(ctx context.Context, p *RetrievalParams) context.Context {
	if p == nil {
		return ctx
	}
	return context.WithValue(ctx, RetrievalParamsKey, p)
}

// GetRetrievalParams 从 Go context 中获取会话级检索参数。
func GetRetrievalParams(ctx context.Context) (*RetrievalParams, bool) {
	p, ok := ctx.Value(RetrievalParamsKey).(*RetrievalParams)
	return p, ok
}

// ========================================
// RouteSelection 传递辅助函数（Agent kb_route 聚焦选择）
// ========================================

// RouteSelection 承载 Agent 在单次会话内经 kb_route 声明的聚焦知识库范围。
// 由入口以指针注入 ctx；kb_route 工具写入、检索适配器读取（跨工具调用共享同一指针）。
// eino ReAct 顺序执行工具，理论上无并发写；仍以 mutex 兜底防御。
type RouteSelection struct {
	mu  sync.Mutex
	ids []string
}

// NewRouteSelection 创建一个空的路由选择 holder
func NewRouteSelection() *RouteSelection {
	return &RouteSelection{}
}

// Set 记录 Agent 声明的聚焦知识库 ID 集合（覆盖式）。
func (r *RouteSelection) Set(ids []string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids = append(r.ids[:0:0], ids...) // 拷贝，避免外部切片别名
}

// Get 返回 Agent 已声明的聚焦知识库 ID 集合的副本；未声明则为 nil。
func (r *RouteSelection) Get() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.ids) == 0 {
		return nil
	}
	return append([]string(nil), r.ids...)
}

// WithRouteSelection 将路由选择 holder 注入到 Go context 中。
func WithRouteSelection(ctx context.Context, sel *RouteSelection) context.Context {
	return context.WithValue(ctx, RouteSelectionKey, sel)
}

// GetRouteSelection 从 Go context 中获取路由选择 holder。
func GetRouteSelection(ctx context.Context) (*RouteSelection, bool) {
	sel, ok := ctx.Value(RouteSelectionKey).(*RouteSelection)
	return sel, ok
}

// ========================================
// 组合辅助函数
// ========================================

// WithAgentContext 将所有 Agent 相关的上下文注入到 Go context 中
func WithAgentContext(ctx context.Context, sessionID string, tenantID int64, userID int64, cc *CollaborationContext) context.Context {
	ctx = WithSessionID(ctx, sessionID)
	ctx = WithTenantID(ctx, tenantID)
	ctx = WithUserID(ctx, userID)
	if cc != nil {
		ctx = WithCollaborationContext(ctx, cc)
	}
	return ctx
}

// NewAgentContext 创建新的 Agent 上下文
// 这是一个便捷函数，用于创建包含所有必要信息的 context
func NewAgentContext(ctx context.Context, sessionID string, tenantID int64, userID int64, originalQuery string) context.Context {
	cc := NewCollaborationContext(sessionID, tenantID, originalQuery)
	cc.UserID = userID
	return WithAgentContext(ctx, sessionID, tenantID, userID, cc)
}
