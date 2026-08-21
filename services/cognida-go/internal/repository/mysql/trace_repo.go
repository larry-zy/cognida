package mysql

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domaintrace "cognida/internal/model/trace"
)

// onConflictSpanIDDoNothing：span_id 冲突则跳过（幂等导出，重复投递不报错）。
var onConflictSpanIDDoNothing = clause.OnConflict{
	Columns:   []clause.Column{{Name: "span_id"}},
	DoNothing: true,
}

// traceRepository 调用链追踪仓储。
type traceRepository struct {
	db *gorm.DB
}

// NewTraceRepository 创建 trace 仓储。
func NewTraceRepository(db *gorm.DB) domaintrace.Repository {
	return &traceRepository{db: db}
}

// SaveBatch 批量落库一组 span（幂等：span_id 唯一，冲突忽略——同一 span 只会 export 一次，
// 重试/重复投递不应报错）。
func (r *traceRepository) SaveBatch(ctx context.Context, spans []*domaintrace.Span) error {
	if len(spans) == 0 {
		return nil
	}
	models := make([]*TraceSpanModel, 0, len(spans))
	for _, s := range spans {
		models = append(models, TraceSpanModelFromDomain(s))
	}
	// OnConflict DoNothing：span_id 已存在则跳过，避免批量导出重试打断整批。
	err := r.db.WithContext(ctx).
		Clauses(onConflictSpanIDDoNothing).
		CreateInBatches(models, 100).Error
	if err != nil {
		return fmt.Errorf("批量保存 span 失败: %w", err)
	}
	return nil
}

// GetSpansByTraceID 取某条 trace 的全部 span（按开始时间升序，供构建树）。
func (r *traceRepository) GetSpansByTraceID(ctx context.Context, traceID string) ([]*domaintrace.Span, error) {
	if traceID == "" {
		return nil, nil
	}
	var models []*TraceSpanModel
	err := r.db.WithContext(ctx).
		Where("trace_id = ?", traceID).
		Order("start_time ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("查询 trace span 失败: %w", err)
	}
	out := make([]*domaintrace.Span, len(models))
	for i, m := range models {
		out[i] = m.ToDomain()
	}
	return out, nil
}

// ListTraces 按 trace_id 聚合分页查询：每行是一整棵调用链的摘要。
//   - 过滤（request_id/session_id/agent_name/tenant/时间/trace_id）走 span 级 WHERE；
//   - only_error（是否含错误 span）是聚合条件，走 HAVING；
//   - 根信息（root_name / duration）取根 span（parent_span_id=” ）的值，缺根则回落总跨度。
func (r *traceRepository) ListTraces(ctx context.Context, q *domaintrace.Query) ([]*domaintrace.TraceSummary, int64, error) {
	if q == nil {
		q = &domaintrace.Query{}
	}
	_ = q.Validate()

	applyWhere := func(db *gorm.DB) *gorm.DB {
		if q.TenantID != nil {
			db = db.Where("tenant_id = ?", *q.TenantID)
		}
		if q.RequestID != "" {
			db = db.Where("request_id = ?", q.RequestID)
		}
		if q.SessionID != "" {
			db = db.Where("session_id = ?", q.SessionID)
		}
		if q.AgentName != "" {
			db = db.Where("agent_name = ?", q.AgentName)
		}
		if q.TraceID != "" {
			db = db.Where("trace_id = ?", q.TraceID)
		}
		if q.StartTime != nil {
			db = db.Where("start_time >= ?", *q.StartTime)
		}
		if q.EndTime != nil {
			db = db.Where("start_time <= ?", *q.EndTime)
		}
		return db
	}

	// 总数：不同 trace_id 的个数。only_error 时需在聚合后计数，用子查询包裹。
	var total int64
	if q.OnlyError {
		sub := applyWhere(r.db.WithContext(ctx).Table("trace_spans")).
			Select("trace_id").
			Group("trace_id").
			Having("SUM(CASE WHEN status_code = ? THEN 1 ELSE 0 END) > 0", domaintrace.StatusError)
		if err := r.db.WithContext(ctx).Table("(?) AS t", sub).Count(&total).Error; err != nil {
			return nil, 0, fmt.Errorf("统计 trace 总数失败: %w", err)
		}
	} else {
		if err := applyWhere(r.db.WithContext(ctx).Table("trace_spans")).
			Distinct("trace_id").Count(&total).Error; err != nil {
			return nil, 0, fmt.Errorf("统计 trace 总数失败: %w", err)
		}
	}

	type aggRow struct {
		TraceID    string
		RootName   string
		RequestID  string
		SessionID  string
		AgentName  string
		TenantID   *int64
		UserID     *int64
		SpanCount  int
		StartTime  time.Time
		EndTime    time.Time
		DurationMs int64
		ErrCount   int64
	}

	list := applyWhere(r.db.WithContext(ctx).Table("trace_spans")).
		Select(`trace_id,
			MAX(CASE WHEN parent_span_id = '' THEN name END) AS root_name,
			MAX(request_id) AS request_id,
			MAX(session_id) AS session_id,
			MAX(agent_name) AS agent_name,
			MAX(tenant_id) AS tenant_id,
			MAX(user_id) AS user_id,
			COUNT(*) AS span_count,
			MIN(start_time) AS start_time,
			MAX(end_time) AS end_time,
			COALESCE(MAX(CASE WHEN parent_span_id = '' THEN duration_ms END), 0) AS duration_ms,
			SUM(CASE WHEN status_code = '` + domaintrace.StatusError + `' THEN 1 ELSE 0 END) AS err_count`).
		Group("trace_id")
	if q.OnlyError {
		list = list.Having("err_count > 0")
	}

	offset := (q.Page - 1) * q.PageSize
	var rows []aggRow
	if err := list.Order("start_time DESC").Offset(offset).Limit(q.PageSize).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("查询 trace 列表失败: %w", err)
	}

	out := make([]*domaintrace.TraceSummary, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		out = append(out, &domaintrace.TraceSummary{
			TraceID:    row.TraceID,
			RootName:   row.RootName,
			RequestID:  row.RequestID,
			SessionID:  row.SessionID,
			AgentName:  row.AgentName,
			TenantID:   row.TenantID,
			UserID:     row.UserID,
			SpanCount:  row.SpanCount,
			StartTime:  row.StartTime,
			EndTime:    row.EndTime,
			DurationMs: row.DurationMs,
			HasError:   row.ErrCount > 0,
		})
	}
	return out, total, nil
}

// HasRequestIDs 批量判断哪些 request_id 在本租户下有 span。
func (r *traceRepository) HasRequestIDs(ctx context.Context, tenantID int64, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	seen := make([]string, 0, len(ids))
	uniq := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := uniq[id]; ok {
			continue
		}
		uniq[id] = struct{}{}
		seen = append(seen, id)
	}
	if len(seen) == 0 {
		return out, nil
	}
	db := r.db.WithContext(ctx).Table("trace_spans").Select("DISTINCT request_id").Where("request_id IN ?", seen)
	if tenantID > 0 {
		db = db.Where("tenant_id = ?", tenantID)
	}
	var found []string
	if err := db.Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("查询 request_id 是否有调用链失败: %w", err)
	}
	for _, id := range found {
		out[id] = true
	}
	return out, nil
}
