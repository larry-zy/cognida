package huggingface

import (
	"context"
	"regexp"
	"time"

	l "cognida/internal/model/lifecycle"
)

const GovernedProfile = "huggingface.static-import-v1"
const GovernedCalls = 11 // metadata + card + four downloads + four decodes + asset registration

var governedDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var governedNode = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

// GovernedInvocation is supplied only by the trusted task runtime adapter.
// Deadline is the parent absolute deadline; the repository also bounds it by
// its own database clock plus TimeoutSeconds when the first reservation lands.
type GovernedInvocation struct {
	OperationKey   string    `json:"operation_key"`
	PayloadHash    string    `json:"payload_hash"`
	TaskID         string    `json:"task_id"`
	PlanVersion    int64     `json:"plan_version"`
	NodeID         string    `json:"node_id"`
	Request        Request   `json:"request"`
	Deadline       time.Time `json:"deadline"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	RequestID      string    `json:"request_id"`
}

func (in GovernedInvocation) Validate() error {
	if !governedDigest.MatchString(in.OperationKey) || !governedDigest.MatchString(in.PayloadHash) || in.TaskID == "" || len(in.TaskID) > 128 || in.PlanVersion < 1 || !governedNode.MatchString(in.NodeID) || in.Deadline.IsZero() || in.TimeoutSeconds < 1 || in.TimeoutSeconds > 120 || len(in.RequestID) > 256 || in.Request.Validate() != nil {
		return ErrInvalid
	}
	return nil
}

// GovernedRepository extends the independent synchronous import ledger. A
// claimed import is never reclaimed: uncertain reads expire rather than repeat
// an external download. CancelReservation creates a tombstone under the same
// unique operation binding, including when cancellation arrives first.
type GovernedRepository interface {
	Repository
	ReserveGoverned(context.Context, l.Principal, GovernedInvocation, bool) (*Import, error)
	LookupGoverned(context.Context, l.Principal, string, string) (*Import, error)
	ClaimGoverned(context.Context, l.Principal, string) (*Import, bool, error)
	FinishGoverned(context.Context, l.Principal, string, string, *Manifest, []l.RefID, string) (*Import, error)
	PendingGoverned(context.Context, int) ([]Import, error)
}
