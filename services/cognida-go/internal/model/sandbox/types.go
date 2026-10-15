// Package sandbox defines execution contracts, not a working isolation runtime.
package sandbox

import (
	"context"
	"errors"
	"time"

	"cognida/internal/model/lifecycle"
)

const (
	SchemaVersion         = "1"
	ControlledDevelopment = "controlled_development"
	CandidateOnly         = "candidate"
	RegularFile           = "regular_file"
)

var (
	ErrInvalid     = errors.New("invalid sandbox contract")
	ErrForbidden   = errors.New("sandbox access denied")
	ErrUnavailable = errors.New("sandbox dependency unavailable")
	ErrUnsupported = errors.New("sandbox capability unsupported")
	ErrConflict    = errors.New("sandbox execution binding conflict")
)

// ResourceLimits are explicit hard budgets. Zero never means unlimited.
type ResourceLimits struct {
	CPUMillis      int64 `json:"cpu_millis"`
	MemoryBytes    int64 `json:"memory_bytes"`
	DiskBytes      int64 `json:"disk_bytes"`
	Processes      int64 `json:"processes"`
	TimeoutSeconds int64 `json:"timeout_seconds"`
	InputBytes     int64 `json:"input_bytes"`
	OutputBytes    int64 `json:"output_bytes"`
	OutputFiles    int64 `json:"output_files"`
	LogBytes       int64 `json:"log_bytes"`
}

// IsolationRequirements are trusted operator configuration. Their declaration
// is not evidence that a runtime actually enforces them; a future runner must
// verify all requirements before accepting work, without a host fallback.
type IsolationRequirements struct {
	NonRoot           bool
	ReadOnlyRoot      bool
	DropCapabilities  bool
	NoNewPrivileges   bool
	SyscallFilter     bool
	IsolatedWorkspace bool
	ReadOnlyInputs    bool
	NetworkDisabled   bool
}

// Profile is server-owned and versioned, never decoded from a tool request.
// This initial contract accepts controlled development only, not multi-tenant
// untrusted code. Approved must be explicitly set by trusted configuration.
type Profile struct {
	ID                string
	Version           int
	Approved          bool
	IsolationLevel    string
	Image             string // Fully qualified registry/repository@sha256:digest, allowlisted here.
	Isolation         IsolationRequirements
	MaxResources      ResourceLimits
	AllowedMediaTypes []string
}

// InputRef identifies immutable bytes; neither a URL nor a storage/host path is
// accepted. The authorization resolver must verify digest and size from storage
// metadata, including current source policy, rather than trusting these claims.
type InputRef struct {
	Name      string          `json:"name"`
	Ref       lifecycle.RefID `json:"ref"`
	SHA256    string          `json:"sha256"`
	SizeBytes int64           `json:"size_bytes"`
}

// SubmitRequest has no environment, credential, command, mount, network or
// image-source override. Code is an authorized immutable asset as well.
type SubmitRequest struct {
	SchemaVersion  string         `json:"schema_version"`
	ExecutionID    string         `json:"execution_id,omitempty"`
	IdempotencyKey string         `json:"idempotency_key"`
	ProfileID      string         `json:"profile_id"`
	ProfileVersion int            `json:"profile_version"`
	ImageSHA256    string         `json:"image_sha256"`
	Code           InputRef       `json:"code"`
	Inputs         []InputRef     `json:"inputs"`
	Resources      ResourceLimits `json:"resources"`
}

// InputMetadata is returned only by a trusted authorization adapter. A success
// means the principal may currently read this exact immutable asset version.
type InputMetadata struct {
	TenantID  int64
	Ref       lifecycle.RefID
	SHA256    string
	SizeBytes int64
}

type ExecutionRef struct {
	ID            string `json:"id"`
	TenantID      int64  `json:"tenant_id"`
	UserID        int64  `json:"user_id"`
	RequestSHA256 string `json:"request_sha256"`
}

// Binding must be stored atomically with submission by the future controller.
// The same owner/key may only refer to the same request digest, never overwrite
// another execution. Validation alone does not implement durable idempotency.
type Binding struct {
	ExecutionRef
	IdempotencyKey string `json:"idempotency_key"`
}

type State string

const (
	StateQueued            State = "queued"
	StateRunning           State = "running"
	StateSucceeded         State = "succeeded"
	StateFailed            State = "failed"
	StateCancelRequested   State = "cancel_requested"
	StateCancelled         State = "cancelled"
	StateTimedOut          State = "timed_out"
	StateReconcileRequired State = "reconcile_required"
)

type ResourceUsage struct {
	CPUMilliseconds     int64 `json:"cpu_milliseconds"`
	PeakMemoryBytes     int64 `json:"peak_memory_bytes"`
	DiskBytes           int64 `json:"disk_bytes"`
	ElapsedMilliseconds int64 `json:"elapsed_milliseconds"`
	LogBytes            int64 `json:"log_bytes"`
}

type Execution struct {
	Binding
	State     State         `json:"state"`
	Usage     ResourceUsage `json:"usage"`
	ErrorCode string        `json:"error_code,omitempty"` // No raw logs, input text or credentials.
	UpdatedAt time.Time     `json:"updated_at"`
}

type Artifact struct {
	Path       string `json:"path"` // Canonical path relative to the execution output directory.
	Kind       string `json:"kind"`
	MediaType  string `json:"media_type"`
	SizeBytes  int64  `json:"size_bytes"`
	SHA256     string `json:"sha256"`
	LinkTarget string `json:"link_target,omitempty"`
}

type Manifest struct {
	SchemaVersion string     `json:"schema_version"`
	ExecutionID   string     `json:"execution_id"`
	RequestSHA256 string     `json:"request_sha256"`
	Disposition   string     `json:"disposition"` // candidate only; never a publication receipt.
	Artifacts     []Artifact `json:"artifacts"`
}

// Runner is deliberately an interface only. Implementations MUST authorize
// every operation, verify runtime enforcement, persist Binding before starting,
// and compare owner/key/digest on replay. Caller-selected IDs are not authority.
// Cancel returns observed state: a cancellation request is not proof of process
// termination. Collect must reauthorize inputs and inspect real files securely
// (no-follow traversal, byte hashes, MIME and bounded archive inspection), then
// return candidate metadata. It must not publish or register assets merely
// because a manifest passed the pure metadata validator.
type Runner interface {
	Submit(context.Context, lifecycle.Principal, SubmitRequest) (Execution, error)
	Status(context.Context, lifecycle.Principal, ExecutionRef) (Execution, error)
	Cancel(context.Context, lifecycle.Principal, ExecutionRef) (Execution, error)
	Collect(context.Context, lifecycle.Principal, ExecutionRef) (Manifest, error)
}
