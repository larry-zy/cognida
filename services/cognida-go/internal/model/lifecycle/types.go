// Package lifecycle defines the durable reference and specialist pipeline contract.
package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const SchemaVersion = "1"

var (
	ErrNotFound  = errors.New("lifecycle record not found")
	ErrForbidden = errors.New("lifecycle access denied")
	ErrInvalid   = errors.New("invalid lifecycle request")
	ErrConflict  = errors.New("lifecycle state conflict")
	ErrExpired   = errors.New("lifecycle reference or approval expired")
)

type Principal struct {
	TenantID int64 `json:"tenant_id"`
	UserID   int64 `json:"user_id"`
}

func (p Principal) Validate() error {
	if p.TenantID <= 0 || p.UserID <= 0 {
		return ErrForbidden
	}
	return nil
}

type RefID struct {
	AssetID string `json:"asset_id"`
	Version int    `json:"version"`
}

func (r RefID) String() string { return r.AssetID + ":" + strconv.Itoa(r.Version) }
func (r RefID) Validate() error {
	if _, e := uuid.Parse(r.AssetID); e != nil || r.Version < 1 {
		return fmt.Errorf("%w: invalid asset reference", ErrInvalid)
	}
	return nil
}
func ParseRefID(value string) (RefID, error) {
	i := strings.LastIndex(value, ":")
	if i < 0 {
		return RefID{}, ErrInvalid
	}
	v, e := strconv.Atoi(value[i+1:])
	if e != nil {
		return RefID{}, ErrInvalid
	}
	r := RefID{AssetID: value[:i], Version: v}
	return r, r.Validate()
}

type Kind string

const (
	RawAsset       Kind = "raw_asset"
	CleanedAsset   Kind = "cleaned_asset"
	EnrichedAsset  Kind = "enriched_asset"
	PublishedAsset Kind = "published_asset"
	QualityReport  Kind = "quality_report"
	Insight        Kind = "insight"
	Evaluation     Kind = "evaluation"
	Result         Kind = "result"
)

type PolicySnapshot struct {
	Sensitivity string   `json:"sensitivity"`
	Scopes      []string `json:"scopes"`
	Approved    bool     `json:"approved"`
	Version     string   `json:"version"`
}
type Provenance struct {
	SourceType    string          `json:"source_type"`
	SourceID      string          `json:"source_id,omitempty"`
	URI           string          `json:"uri,omitempty"`
	Digest        string          `json:"digest"`
	RequestID     string          `json:"request_id,omitempty"`
	AccessRequest json.RawMessage `json:"access_request,omitempty"`
	SourceVersion string          `json:"source_version,omitempty"`
	SourceOwnerID int64           `json:"source_owner_id,omitempty"`
}
type DataAssetRef struct {
	RefID
	AssetRefID    string         `json:"asset_ref_id"`
	SchemaVersion string         `json:"schema_version"`
	TenantID      int64          `json:"tenant_id"`
	Kind          Kind           `json:"kind"`
	Status        string         `json:"status"`
	Name          string         `json:"name,omitempty"`
	ProducerID    string         `json:"producer_id"`
	Parents       []RefID        `json:"parents"`
	Policy        PolicySnapshot `json:"policy"`
	Provenance    Provenance     `json:"provenance"`
	CreatedAt     time.Time      `json:"created_at"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
	// ObjectKey is private repository metadata, never accepted from an API caller.
	ObjectKey string `json:"-"`
}
type ResultRef = DataAssetRef
type QualityReportRef = DataAssetRef
type InsightRef = DataAssetRef
type EvaluationRef = DataAssetRef

func (r DataAssetRef) Validate() error {
	if err := r.RefID.Validate(); err != nil {
		return err
	}
	if r.SchemaVersion != SchemaVersion || r.TenantID <= 0 || r.Kind == "" || r.ProducerID == "" || r.ObjectKey == "" {
		return ErrInvalid
	}
	return nil
}

type GateResult struct {
	Status       string   `json:"status"`
	Reason       string   `json:"reason"`
	EvidenceRefs []RefID  `json:"evidence_refs"`
	Score        *float64 `json:"score,omitempty"`
	Threshold    *float64 `json:"threshold,omitempty"`
}

const (
	GatePassed           = "passed"
	GateFailed           = "failed"
	GateQuarantined      = "quarantined"
	GateApprovalRequired = "approval_required"
)

type Output struct {
	Decision map[string]any `json:"decision,omitempty"`
	Refs     []DataAssetRef `json:"refs"`
	Summary  string         `json:"summary"`
	Gate     GateResult     `json:"gate"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
type AgentContract struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Purpose         string   `json:"purpose"`
	SchemaVersion   string   `json:"schema_version"`
	InputKinds      []Kind   `json:"input_kinds"`
	OutputKinds     []Kind   `json:"output_kinds"`
	InvocationModes []string `json:"invocation_modes"`
	RequiredScopes  []string `json:"required_scopes"`
	ToolAllowlist   []string `json:"tool_allowlist"`
	RiskClass       string   `json:"risk_class"`
	MaxIterations   int      `json:"max_iterations"`
	TokenBudget     int      `json:"token_budget"`
}

func (c AgentContract) Accepts(kind Kind) bool {
	for _, k := range c.InputKinds {
		if k == kind {
			return true
		}
	}
	return false
}

type StageDefinition struct {
	ID           string   `json:"id"`
	AgentID      string   `json:"agent_id"`
	DependsOn    []string `json:"depends_on"`
	AllowedGates []string `json:"allowed_gates"`
	MaxAttempts  int      `json:"max_attempts"`
	Approval     bool     `json:"approval"`
}
type PipelineDefinition struct {
	ID        string            `json:"id"`
	Version   int               `json:"version"`
	Name      string            `json:"name"`
	Stages    []StageDefinition `json:"stages"`
	MaxStages int               `json:"max_stages"`
	Enabled   bool              `json:"enabled"`
}

func (d PipelineDefinition) Validate(contracts map[string]AgentContract) error {
	if d.ID == "" || d.Version < 1 || len(d.Stages) == 0 || len(d.Stages) > 32 || d.MaxStages < len(d.Stages) || d.MaxStages > 32 {
		return fmt.Errorf("%w: bounded versioned definition required", ErrInvalid)
	}
	stages := map[string]StageDefinition{}
	for _, s := range d.Stages {
		if s.ID == "" || s.MaxAttempts < 1 || s.MaxAttempts > 5 {
			return fmt.Errorf("%w: stage identity/retry", ErrInvalid)
		}
		if _, ok := stages[s.ID]; ok {
			return fmt.Errorf("%w: duplicate stage %s", ErrInvalid, s.ID)
		}
		if _, ok := contracts[s.AgentID]; !ok {
			return fmt.Errorf("%w: unknown agent %s", ErrInvalid, s.AgentID)
		}
		if contracts[s.AgentID].MaxIterations < 1 || contracts[s.AgentID].TokenBudget < 1 {
			return fmt.Errorf("%w: agent budget required", ErrInvalid)
		}
		for _, gate := range s.AllowedGates {
			if gate != GatePassed && gate != GateFailed {
				return fmt.Errorf("%w: unsafe gate transition", ErrInvalid)
			}
		}
		if len(s.DependsOn) > 0 && len(s.AllowedGates) == 0 {
			return fmt.Errorf("%w: gate policy required", ErrInvalid)
		}
		if contracts[s.AgentID].RiskClass == "write" && !s.Approval {
			return fmt.Errorf("%w: publication approval required", ErrInvalid)
		}
		stages[s.ID] = s
	}
	marks := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if marks[id] == 1 {
			return fmt.Errorf("%w: cyclic definition", ErrInvalid)
		}
		if marks[id] == 2 {
			return nil
		}
		marks[id] = 1
		s := stages[id]
		for _, parent := range s.DependsOn {
			p, ok := stages[parent]
			if !ok {
				return fmt.Errorf("%w: missing stage %s", ErrInvalid, parent)
			}
			compatible := false
			for _, kind := range contracts[p.AgentID].OutputKinds {
				if contracts[s.AgentID].Accepts(kind) {
					compatible = true
				}
			}
			if !compatible {
				return fmt.Errorf("%w: incompatible edge %s -> %s", ErrInvalid, parent, id)
			}
			if err := visit(parent); err != nil {
				return err
			}
		}
		marks[id] = 2
		return nil
	}
	for id := range stages {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

const (
	StatusWaiting     = "waiting"
	StatusQueued      = "queued"
	StatusRunning     = "running"
	StatusApproval    = "awaiting_approval"
	StatusSucceeded   = "succeeded"
	StatusFailed      = "failed"
	StatusCancelled   = "cancelled"
	StatusQuarantined = "quarantined"
)

func Terminal(status string) bool {
	return status == StatusSucceeded || status == StatusFailed || status == StatusCancelled || status == StatusQuarantined
}

type PipelineRun struct {
	ID              string         `json:"id"`
	TenantID        int64          `json:"tenant_id"`
	UserID          int64          `json:"user_id"`
	TemplateID      string         `json:"template_id"`
	TemplateVersion int            `json:"template_version"`
	Status          string         `json:"status"`
	InputRefs       []RefID        `json:"input_refs"`
	Options         map[string]any `json:"options"`
	IdempotencyKey  string         `json:"-"`
	RequestID       string         `json:"request_id"`
	Error           string         `json:"error,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}
type StageRun struct {
	ID               string         `json:"id"`
	RunID            string         `json:"run_id,omitempty"`
	TenantID         int64          `json:"tenant_id"`
	UserID           int64          `json:"user_id"`
	StageKey         string         `json:"stage_key"`
	AgentID          string         `json:"agent_id"`
	Status           string         `json:"status"`
	Attempt          int            `json:"attempt"`
	MaxAttempts      int            `json:"max_attempts"`
	InputRefs        []RefID        `json:"input_refs"`
	Options          map[string]any `json:"options"`
	Output           *Output        `json:"output,omitempty"`
	Error            string         `json:"error,omitempty"`
	IdempotencyKey   string         `json:"-"`
	RequestID        string         `json:"request_id"`
	ApprovalRequired bool           `json:"approval_required"`
	Approved         bool           `json:"approved"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}
type Invocation = StageRun
type Approval struct {
	ID         string     `json:"id"`
	TenantID   int64      `json:"tenant_id"`
	RunID      string     `json:"run_id,omitempty"`
	StageID    string     `json:"stage_id"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason"`
	ResolvedBy int64      `json:"resolved_by,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
type Event struct {
	ID        int64     `json:"id"`
	TenantID  int64     `json:"tenant_id"`
	RunID     string    `json:"run_id,omitempty"`
	StageID   string    `json:"stage_id,omitempty"`
	Type      string    `json:"type"`
	RequestID string    `json:"request_id"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}
type Input struct {
	Kind    string   `json:"kind"`
	Text    string   `json:"text,omitempty"`
	Columns []string `json:"columns,omitempty"`
	Rows    [][]any  `json:"rows,omitempty"`
	// ColumnTypes records declared SQL types while exact decimal/identifier
	// strings survive compute transports without floating-point conversion.
	ColumnTypes map[string]string `json:"column_types,omitempty"`
}

// UnmarshalJSON preserves table numbers through HTTP input and persisted snapshots.
func (i *Input) UnmarshalJSON(raw []byte) error {
	type plain Input
	var value plain
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*i = Input(value)
	return nil
}

func (i Input) Validate() error {
	switch i.Kind {
	case "text":
		if strings.TrimSpace(i.Text) == "" || utf8.RuneCountInString(i.Text) > 200000 {
			return ErrInvalid
		}
	case "table":
		if len(i.Columns) == 0 || len(i.Columns) > 200 || len(i.Rows) == 0 || len(i.Rows) > 10000 || len(i.Columns)*len(i.Rows) > 100000 {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for _, c := range i.Columns {
			if c == "" || utf8.RuneCountInString(c) > 200 || seen[c] {
				return ErrInvalid
			}
			seen[c] = true
		}
		for column, typ := range i.ColumnTypes {
			if !seen[column] || strings.TrimSpace(typ) == "" || utf8.RuneCountInString(typ) > 64 {
				return ErrInvalid
			}
		}
		for _, r := range i.Rows {
			if len(r) != len(i.Columns) {
				return ErrInvalid
			}
			for _, cell := range r {
				switch v := cell.(type) {
				case nil, bool, int, int64, int32, uint, uint64, uint32:
				case json.Number:
					number, e := v.Float64()
					if len(v) > 128 || !json.Valid([]byte(v)) || e != nil || math.IsNaN(number) || math.IsInf(number, 0) {
						return ErrInvalid
					}
				case float64:
					if math.IsNaN(v) || math.IsInf(v, 0) {
						return ErrInvalid
					}
				case float32:
					if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
						return ErrInvalid
					}
				case string:
					if utf8.RuneCountInString(v) > 20000 {
						return ErrInvalid
					}
				default:
					return ErrInvalid
				}
			}
		}
	default:
		return ErrInvalid
	}
	body, e := json.Marshal(i)
	if e != nil || len(body) > 4<<20 {
		return ErrInvalid
	}
	return nil
}

type StageRequest struct {
	Principal Principal
	Stage     StageRun
	Inputs    []DataAssetRef
	Payloads  []json.RawMessage
}
type Adapter interface {
	Run(context.Context, StageRequest) (Output, error)
}
type Repository interface {
	Transaction(context.Context, func(Repository) error) error
	SaveDefinition(context.Context, PipelineDefinition) error
	FindDefinition(context.Context, string, int) (PipelineDefinition, error)
	RecoverableRuns(context.Context, int) ([]PipelineRun, error)
	ExpiredApprovals(context.Context, time.Time, int) ([]Approval, error)
	ListAssets(context.Context, int64, Kind, string, int) ([]DataAssetRef, error)
	ListRunsPage(context.Context, int64, string, int) ([]PipelineRun, error)
	ListInvocationsPage(context.Context, int64, string, int) ([]StageRun, error)
	PutAsset(context.Context, DataAssetRef) error
	FindAsset(context.Context, int64, RefID) (DataAssetRef, error)
	CreateRun(context.Context, *PipelineRun, []StageRun) error
	FindRun(context.Context, int64, string, bool) (*PipelineRun, error)
	FindRunByKey(context.Context, int64, string) (*PipelineRun, error)
	ListRuns(context.Context, int64, int) ([]PipelineRun, error)
	UpdateRun(context.Context, *PipelineRun) error
	CreateStage(context.Context, *StageRun) error
	FindStage(context.Context, int64, string, bool) (*StageRun, error)
	FindStageByKey(context.Context, int64, string) (*StageRun, error)
	ListStages(context.Context, int64, string) ([]StageRun, error)
	// LockStages must be used inside a transaction after locking its parent run.
	LockStages(context.Context, int64, string) ([]StageRun, error)
	ListInvocations(context.Context, int64, int) ([]StageRun, error)
	PendingStages(context.Context, int) ([]StageRun, error)
	UpdateStage(context.Context, *StageRun) error
	CreateApproval(context.Context, *Approval) error
	FindApproval(context.Context, int64, string, bool) (*Approval, error)
	ListApprovals(context.Context, int64, string) ([]Approval, error)
	UpdateApproval(context.Context, *Approval) error
	AppendEvent(context.Context, *Event) error
	Events(context.Context, int64, string, int64) ([]Event, error)
}
