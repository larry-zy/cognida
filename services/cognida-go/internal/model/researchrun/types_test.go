package researchrun

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestResearchSubmissionBounds(t *testing.T) {
	valid := Submission{OperationKey: strings.Repeat("a", 64), PayloadHash: strings.Repeat("b", 64), Profile: Profile, Request: Request{Query: "已确认的研究问题", KBIDs: []string{"kb"}}, Deadline: time.Now().Add(time.Minute), TimeoutSeconds: 120, MaxCalls: MaxCalls}
	require.NoError(t, valid.Validate())
	for name, mutate := range map[string]func(*Submission){"implicit scope": func(s *Submission) { s.Request.KBIDs = nil }, "duplicate scope": func(s *Submission) { s.Request.KBIDs = []string{"kb", "kb"} }, "unsafe text": func(s *Submission) { s.Request.Query = "a\x00b" }, "oversized question": func(s *Submission) { s.Request.Query = strings.Repeat("问", 4001) }, "unknown profile": func(s *Submission) { s.Profile = "unbounded" }, "short budget": func(s *Submission) { s.MaxCalls = 7 }, "monetary guarantee": func(s *Submission) { s.MaxCostMicros = 1 }, "invalid envelope": func(s *Submission) { s.PayloadHash = strings.Repeat("z", 64) }} {
		t.Run(name, func(t *testing.T) { v := valid; mutate(&v); require.Error(t, v.Validate()) })
	}
}
