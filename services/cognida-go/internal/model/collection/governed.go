package collection

import (
	l "cognida/internal/model/lifecycle"
	tc "cognida/internal/model/taskcontrol"
	"context"
	"encoding/json"
	"strings"
	"time"
)

const SnapshotMode = "snapshot"
const SnapshotProfile = "collection-snapshot-v1"

// GovernedInvocation is immutable, trusted executor data, never an HTTP DTO.
type GovernedInvocation struct {
	OperationKey     string    `json:"operation_key"`
	PayloadHash      string    `json:"payload_hash"`
	Profile          string    `json:"profile"`
	URL              string    `json:"url"`
	MaxResponseBytes int64     `json:"max_response_bytes"`
	Deadline         time.Time `json:"deadline"`
	TimeoutSeconds   int       `json:"timeout_seconds"`
	CallsReserved    int       `json:"calls_reserved"`
}

func (in GovernedInvocation) Validate() error {
	if len(in.OperationKey) != 64 || len(in.PayloadHash) != 64 || strings.Trim(in.OperationKey+in.PayloadHash, "0123456789abcdef") != "" || in.Profile != SnapshotProfile || in.MaxResponseBytes < 1 || in.MaxResponseBytes > 2<<20 || in.Deadline.IsZero() || in.TimeoutSeconds < 1 || in.TimeoutSeconds > 120 || in.CallsReserved != 3 {
		return tc.ErrInvalid
	}
	return (&Source{ID: "source", TaskID: "task", URL: in.URL}).Validate()
}

type GovernedRepository interface {
	SubmitGoverned(context.Context, l.Principal, GovernedInvocation, bool) (*Run, error)
	FindGoverned(context.Context, l.Principal, string) (*Run, error)
	FinishGoverned(context.Context, l.Principal, string, RunStatus, []l.RefID, string) (*Run, error)
}

func SameGoverned(a, b GovernedInvocation) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
