package taskcontrol

import (
	"context"
	"encoding/json"
	"time"
)

type ReplanCommand struct {
	Snapshot       json.RawMessage
	IdempotencyKey string
	RequestHash    string
	Reason         string
}
type ReplanReceipt struct {
	Scope
	TaskID       string    `json:"task_id"`
	OperationKey string    `json:"-"`
	RequestHash  string    `json:"-"`
	FromVersion  int64     `json:"from_version"`
	PlanVersion  int64     `json:"plan_version"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}
type ReplanRepository interface {
	LookupReplan(context.Context, Scope, string, string, string) (*Plan, error)
	ReplanExecution(context.Context, Scope, string, int64, ReplanCommand) (*Plan, error)
}
