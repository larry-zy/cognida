// Package mysql 提供调用链追踪（trace_spans）的 MySQL 持久化实现。
package mysql

import (
	"time"

	domaintrace "cognida/internal/model/trace"
)

// TraceSpanModel 对应 trace_spans 表，承载 OTel span 的持久化细节。
//
// 索引取向：按 trace_id 聚合成树（详情）、按 request_id/session_id 关联审计与会话、
// 按 agent_name/start_time 做列表过滤与排序。attributes/events 以 JSON 列原样留存。
type TraceSpanModel struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	TraceID      string    `gorm:"type:varchar(32);not null;index:idx_trace_id"`
	SpanID       string    `gorm:"type:varchar(16);not null;uniqueIndex:uk_span_id"`
	ParentSpanID string    `gorm:"type:varchar(16);index:idx_parent_span"`
	Name         string    `gorm:"type:varchar(255)"`
	Kind         string    `gorm:"type:varchar(24)"`
	StartTime    time.Time `gorm:"index:idx_start_time"`
	EndTime      time.Time
	DurationMs   int64  `gorm:"comment:耗时ms"`
	StatusCode   string `gorm:"type:varchar(8);index:idx_status_code"`
	StatusMsg    string `gorm:"type:varchar(512)"`
	RequestID    string `gorm:"type:varchar(64);index:idx_trace_request"`
	SessionID    string `gorm:"type:varchar(64);index:idx_trace_session"`
	TenantID     *int64 `gorm:"index:idx_trace_tenant"`
	UserID       *int64
	AgentName    string    `gorm:"type:varchar(128);index:idx_trace_agent"`
	Attributes   string    `gorm:"type:json"`
	Events       string    `gorm:"type:json"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

// TableName 指定表名。
func (TraceSpanModel) TableName() string {
	return "trace_spans"
}

// ToDomain 转换为领域实体。
func (m *TraceSpanModel) ToDomain() *domaintrace.Span {
	return &domaintrace.Span{
		ID:           m.ID,
		TraceID:      m.TraceID,
		SpanID:       m.SpanID,
		ParentSpanID: m.ParentSpanID,
		Name:         m.Name,
		Kind:         m.Kind,
		StartTime:    m.StartTime,
		EndTime:      m.EndTime,
		DurationMs:   m.DurationMs,
		StatusCode:   m.StatusCode,
		StatusMsg:    m.StatusMsg,
		RequestID:    m.RequestID,
		SessionID:    m.SessionID,
		TenantID:     m.TenantID,
		UserID:       m.UserID,
		AgentName:    m.AgentName,
		Attributes:   m.Attributes,
		Events:       m.Events,
		CreatedAt:    m.CreatedAt,
	}
}

// TraceSpanModelFromDomain 从领域实体构造持久化模型。
func TraceSpanModelFromDomain(s *domaintrace.Span) *TraceSpanModel {
	return &TraceSpanModel{
		ID:           s.ID,
		TraceID:      s.TraceID,
		SpanID:       s.SpanID,
		ParentSpanID: s.ParentSpanID,
		Name:         s.Name,
		Kind:         s.Kind,
		StartTime:    s.StartTime,
		EndTime:      s.EndTime,
		DurationMs:   s.DurationMs,
		StatusCode:   s.StatusCode,
		StatusMsg:    s.StatusMsg,
		RequestID:    s.RequestID,
		SessionID:    s.SessionID,
		TenantID:     s.TenantID,
		UserID:       s.UserID,
		AgentName:    s.AgentName,
		Attributes:   s.Attributes,
		Events:       s.Events,
	}
}
