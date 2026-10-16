// Package feedback defines the durable evidence and publication boundary for quality repair.
package feedback

import (
	l "cognida/internal/model/lifecycle"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalid   = errors.New("invalid feedback request")
	ErrForbidden = errors.New("feedback source unavailable or unauthorized")
	ErrNotFound  = errors.New("feedback record not found")
	ErrConflict  = errors.New("feedback version or state conflict")
	ErrGate      = errors.New("fixed validation gate has not passed")
	ErrBudget    = errors.New("feedback repair budget exhausted")
)

type Gate struct {
	Metric         string  `json:"metric"`
	HigherIsBetter bool    `json:"higher_is_better"`
	MinPairs       int     `json:"min_pairs"`
	MaxRegression  float64 `json:"max_regression"`
	MinImprovement float64 `json:"min_improvement"`
}
type Budget struct {
	MaxRounds      int   `json:"max_rounds"`
	MaxCalls       int   `json:"max_calls"`
	MaxCostMicros  int64 `json:"max_cost_micros"`
	TimeoutSeconds int   `json:"timeout_seconds"`
}
type Evidence struct {
	OutputRef          l.RefID   `json:"output_ref"`
	SourceRefs         []l.RefID `json:"source_refs"`
	EvaluationTaskID   string    `json:"evaluation_task_id,omitempty"`
	EvaluationResultID string    `json:"evaluation_result_id,omitempty"`
	Question           string    `json:"question"`
	Problem            string    `json:"problem"`
	Stage              string    `json:"stage"`
}
type Create struct {
	Title            string          `json:"title"`
	KnowledgeBaseID  string          `json:"knowledge_base_id"`
	BaselineRef      l.RefID         `json:"baseline_ref"`
	Evidence         Evidence        `json:"evidence"`
	DatasetVersionID string          `json:"dataset_version_id"`
	EvaluationConfig json.RawMessage `json:"evaluation_config"`
	Gate             Gate            `json:"gate"`
	Budget           Budget          `json:"budget"`
}
type Round struct {
	ID         string          `json:"id"`
	Key        string          `json:"idempotency_key"`
	Digest     string          `json:"digest"`
	Plan       json.RawMessage `json:"plan"`
	TaskID     string          `json:"task_id,omitempty"`
	Status     string          `json:"status"`
	Calls      int             `json:"reserved_calls"`
	CostMicros int64           `json:"reserved_cost_micros"`
	CreatedAt  time.Time       `json:"created_at"`
}
type Case struct {
	ID       string `json:"id"`
	TenantID int64  `json:"tenant_id"`
	UserID   int64  `json:"user_id"`
	Version  int    `json:"version"`
	Status   string `json:"status"`
	Create
	BaselineKnowledgeID string          `json:"baseline_knowledge_id"`
	BaselineLabels      []string        `json:"baseline_labels"`
	BaselineDigest      string          `json:"baseline_digest"`
	EvaluationSnapshot  json.RawMessage `json:"evaluation_snapshot"`
	ActiveVersionID     string          `json:"active_version_id"`
	ReservedCalls       int             `json:"reserved_calls"`
	ReservedCostMicros  int64           `json:"reserved_cost_micros"`
	Rounds              []Round         `json:"rounds"`
	Reservations        map[string]int  `json:"reservations"`
	Corrections         []Correction    `json:"corrections"`
	Deadline            time.Time       `json:"deadline"`
	Error               string          `json:"error,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}
type Correction struct {
	Ref    l.RefID `json:"ref"`
	Digest string  `json:"digest"`
	Key    string  `json:"idempotency_key"`
}
type CandidateInput struct {
	SourceRef l.RefID `json:"source_ref"`
	RoundID   string  `json:"round_id"`
	// Reviewed tagging result is optional; labels are resolved from its immutable review.
	LabelResultID      string `json:"label_result_id,omitempty"`
	LabelReviewVersion int    `json:"label_review_version,omitempty"`
}
type Publication struct {
	ID       string `json:"id"`
	CaseID   string `json:"case_id"`
	TenantID int64  `json:"tenant_id"`
	UserID   int64  `json:"user_id"`
	Version  int    `json:"version"`
	CandidateInput
	KnowledgeID           string          `json:"knowledge_id"`
	ContentDigest         string          `json:"content_digest"`
	Labels                []string        `json:"labels"`
	Status                string          `json:"status"`
	Projection            json.RawMessage `json:"projection,omitempty"`
	BaselineEvaluationID  string          `json:"baseline_evaluation_id,omitempty"`
	CandidateEvaluationID string          `json:"candidate_evaluation_id,omitempty"`
	Comparison            json.RawMessage `json:"comparison,omitempty"`
	ValidationDigest      string          `json:"validation_digest,omitempty"`
	ApprovedBy            int64           `json:"approved_by,omitempty"`
	ReviewComment         string          `json:"review_comment,omitempty"`
	Error                 string          `json:"error,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}
type Event struct {
	ID            string    `json:"id"`
	CaseID        string    `json:"case_id"`
	PublicationID string    `json:"publication_id,omitempty"`
	Action        string    `json:"action"`
	ActorID       int64     `json:"actor_id"`
	Detail        string    `json:"detail"`
	CreatedAt     time.Time `json:"created_at"`
}
type View struct {
	CaseID          string    `json:"case_id"`
	PublicationID   string    `json:"publication_id"`
	KnowledgeBaseID string    `json:"knowledge_base_id"`
	KnowledgeID     string    `json:"knowledge_id"`
	Digest          string    `json:"digest"`
	Labels          []string  `json:"labels,omitempty"`
	Deadline        time.Time `json:"deadline"`
}

func Digest(v any) string {
	raw, _ := json.Marshal(v)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

type Repository interface {
	CreateCase(context.Context, *Case, string, string) (*Case, error)
	GetCase(context.Context, l.Principal, string) (*Case, error)
	ListCases(context.Context, l.Principal, string, int) ([]Case, string, error)
	SaveCase(context.Context, *Case, int, Event) error
	CreatePublication(context.Context, *Publication, string, string) (*Publication, error)
	GetPublication(context.Context, l.Principal, string) (*Publication, error)
	ListPublications(context.Context, l.Principal, string) ([]Publication, error)
	SavePublication(context.Context, *Publication, int, Event) error
	// Commit locks the case and candidate together; the expected digest fences a stale validation.
	Commit(context.Context, l.Principal, string, string, int, int, string, string) error
	Events(context.Context, l.Principal, string) ([]Event, error)
	FindKnowledge(context.Context, int64, string) (*Case, *Publication, error)
	ScanCases(context.Context, string, int) ([]Case, string, error)
}
