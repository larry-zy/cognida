// Package tools provides tool-related persistence models
package tools

import "time"

// Tool 工具持久化模型
type Tool struct {
	ID          int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID    int64     `json:"tenant_id" gorm:"not null;index:idx_tenant_id"`
	Name        string    `json:"name" gorm:"type:varchar(255);not null"`
	Type        string    `json:"type" gorm:"type:varchar(50);not null"` // search/database/http/custom
	Description string    `json:"description" gorm:"type:text"`
	Config      string    `json:"config" gorm:"type:json"` // JSON
	Enabled     bool      `json:"enabled" gorm:"default:true"`
	CreatedBy   *int64    `json:"created_by" gorm:"index"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// ToolExecution 工具执行记录持久化模型
type ToolExecution struct {
	ID           int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	MessageID    string    `json:"message_id" gorm:"not null;size:36;index:idx_message_id"`
	ToolID       int64     `json:"tool_id" gorm:"not null;index:idx_tool_id"`
	InputParams  string    `json:"input_params" gorm:"type:json"`           // JSON
	OutputData   string    `json:"output_data" gorm:"type:json"`            // JSON
	Status       string    `json:"status" gorm:"type:varchar(50);not null"` // success/failed/timeout
	DurationMs   int       `json:"duration_ms" gorm:"not null"`
	ErrorMessage string    `json:"error_message" gorm:"type:text"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
}
