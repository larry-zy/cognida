package memory

import "errors"

var (
	ErrInvalid      = errors.New("invalid memory request")
	ErrUnauthorized = errors.New("memory owner identity required")
	ErrNotFound     = errors.New("memory not found")
	ErrConflict     = errors.New("memory changed or idempotency key reused")
)

// Scope is mandatory at every persistence boundary. Neither field comes from request JSON.
type Scope struct{ TenantID, UserID int64 }

func (s Scope) Validate() error {
	if s.TenantID <= 0 || s.UserID <= 0 {
		return ErrUnauthorized
	}
	return nil
}

type WriteInput struct {
	Content    string  `json:"content" jsonschema:"required,description=用户明确要求保存的事实或偏好，最多4000字符"`
	Category   string  `json:"category" jsonschema:"required,description=fact（事实）或preference（偏好）"`
	Importance float64 `json:"importance" jsonschema:"description=重要性0到1，不是事实真实性或置信概率"`
}
type UpdateInput struct {
	WriteInput
	ExpectedVersion int64 `json:"expected_version" jsonschema:"required,description=当前记忆版本，避免覆盖并发修改"`
}
type ListFilter struct {
	Category, Query, Cursor string
	Limit                   int
}
type MemoryPage struct {
	Items      []*LongTermMemory `json:"items"`
	NextCursor string            `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
}
type RecallInput struct {
	Query    string `json:"query" jsonschema:"required,description=当前问题或希望回忆的内容"`
	Category string `json:"category,omitempty" jsonschema:"description=可选fact或preference"`
	Limit    int    `json:"limit" jsonschema:"description=结果数量，最多20条"`
}
type RecalledMemory struct {
	Memory *LongTermMemory `json:"memory"`
	Score  float64         `json:"score"`
	Method string          `json:"method"`
}
type RecallResult struct {
	Items          []RecalledMemory `json:"items"`
	Method         string           `json:"method"`
	CandidateLimit int              `json:"candidate_limit"`
	Truncated      bool             `json:"truncated"`
	Note           string           `json:"note"`
}
