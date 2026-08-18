// Package trace 提供调用链追踪（span）的领域实体与仓储端口定义。
//
// 一次请求会展开成一棵 span 树：顶层 agent.chat 为根，其下挂工具调用、
// 子代理委派、编排等子 span。span 由 OpenTelemetry SDK 采集，经 MySQL
// SpanExporter 落到 trace_spans 表；本包只描述纯领域形态，不含持久化细节。
package trace

import (
	"context"
	"sort"
	"time"
)

// 状态码常量（映射 OTel codes.Code 的字符串形态）。
const (
	StatusUnset = "UNSET"
	StatusOK    = "OK"
	StatusError = "ERROR"
)

// Span 一条追踪跨度（领域实体）。
type Span struct {
	ID           int64     `json:"id"`
	TraceID      string    `json:"trace_id"`
	SpanID       string    `json:"span_id"`
	ParentSpanID string    `json:"parent_span_id"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	DurationMs   int64     `json:"duration_ms"`
	StatusCode   string    `json:"status_code"`
	StatusMsg    string    `json:"status_msg"`
	RequestID    string    `json:"request_id"`
	SessionID    string    `json:"session_id"`
	TenantID     *int64    `json:"tenant_id"`
	UserID       *int64    `json:"user_id"`
	AgentName    string    `json:"agent_name"`
	Attributes   string    `json:"attributes"` // JSON 对象文本
	Events       string    `json:"events"`     // JSON 数组文本
	CreatedAt    time.Time `json:"created_at"`
}

// IsError 该 span 是否以错误结束。
func (s *Span) IsError() bool { return s.StatusCode == StatusError }

// TraceSummary 一条 trace 的聚合摘要（列表视图，一行代表一整棵调用链）。
type TraceSummary struct {
	TraceID    string    `json:"trace_id"`
	RootName   string    `json:"root_name"`
	RequestID  string    `json:"request_id"`
	SessionID  string    `json:"session_id"`
	AgentName  string    `json:"agent_name"`
	TenantID   *int64    `json:"tenant_id"`
	UserID     *int64    `json:"user_id"`
	SpanCount  int       `json:"span_count"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	DurationMs int64     `json:"duration_ms"`
	HasError   bool      `json:"has_error"`
}

// SpanNode 跨度树节点（详情视图，递归结构）。
type SpanNode struct {
	Span     *Span       `json:"span"`
	Children []*SpanNode `json:"children"`
}

// Query trace 列表查询参数。
type Query struct {
	TenantID  *int64
	RequestID string
	SessionID string
	AgentName string
	TraceID   string
	OnlyError bool
	StartTime *time.Time
	EndTime   *time.Time

	Page     int
	PageSize int
}

// Validate 归一化分页参数。
func (q *Query) Validate() error {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 200 {
		q.PageSize = 20
	}
	return nil
}

// Repository trace 持久化端口（domain 只依赖此接口，实现在 infrastructure）。
type Repository interface {
	// SaveBatch 批量落库一组 span（由 SpanExporter 调用）。
	SaveBatch(ctx context.Context, spans []*Span) error
	// ListTraces 按 trace 聚合分页查询（每行一整棵调用链的摘要）。
	ListTraces(ctx context.Context, q *Query) ([]*TraceSummary, int64, error)
	// GetSpansByTraceID 取某条 trace 的全部 span（按开始时间升序）。
	GetSpansByTraceID(ctx context.Context, traceID string) ([]*Span, error)
	// HasRequestIDs 返回 ids 中确实在 trace_spans 出现过的 request_id（按租户过滤）。
	HasRequestIDs(ctx context.Context, tenantID int64, ids []string) (map[string]bool, error)
}

// BuildTree 把扁平 span 列表还原成森林（多根容错）：
//   - 以 span_id 建索引，按 parent_span_id 挂到父节点；
//   - 父不存在（跨进程断链或根 span）的节点作为森林的根；
//   - 同层子节点按 StartTime 升序，保证展示稳定。
func BuildTree(spans []*Span) []*SpanNode {
	nodes := make(map[string]*SpanNode, len(spans))
	for _, s := range spans {
		nodes[s.SpanID] = &SpanNode{Span: s}
	}

	var roots []*SpanNode
	for _, s := range spans {
		node := nodes[s.SpanID]
		if parent, ok := nodes[s.ParentSpanID]; ok && s.ParentSpanID != "" && s.ParentSpanID != s.SpanID {
			parent.Children = append(parent.Children, node)
		} else {
			roots = append(roots, node)
		}
	}

	var sortChildren func(ns []*SpanNode)
	sortChildren = func(ns []*SpanNode) {
		sort.SliceStable(ns, func(i, j int) bool {
			return ns[i].Span.StartTime.Before(ns[j].Span.StartTime)
		})
		for _, n := range ns {
			sortChildren(n.Children)
		}
	}
	sortChildren(roots)
	return roots
}
