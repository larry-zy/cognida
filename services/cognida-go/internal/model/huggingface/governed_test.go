package huggingface

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestGovernedInvocationBounds(t *testing.T) {
	base := GovernedInvocation{OperationKey: strings.Repeat("a", 64), PayloadHash: strings.Repeat("b", 64), TaskID: "task", PlanVersion: 1, NodeID: "import", Request: Request{RepoID: "org/data", Revision: strings.Repeat("c", 40), Config: "default", Split: "train", Files: []string{"file.jsonl"}, FieldMapping: FieldMapping{TextField: "text"}, MaxRows: 1, MaxBytes: 1024}, Deadline: time.Now().Add(time.Minute), TimeoutSeconds: 60}
	require.NoError(t, base.Validate())
	for _, mutate := range []func(*GovernedInvocation){func(in *GovernedInvocation) { in.OperationKey = "x" }, func(in *GovernedInvocation) { in.PayloadHash = "" }, func(in *GovernedInvocation) { in.TimeoutSeconds = 121 }, func(in *GovernedInvocation) { in.Deadline = time.Time{} }, func(in *GovernedInvocation) { in.NodeID = "bad/node" }, func(in *GovernedInvocation) { in.Request.Revision = "main" }} {
		in := base
		mutate(&in)
		require.ErrorIs(t, in.Validate(), ErrInvalid)
	}
}
