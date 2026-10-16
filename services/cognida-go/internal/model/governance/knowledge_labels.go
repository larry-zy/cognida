package governance

import (
	l "cognida/internal/model/lifecycle"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// KnowledgeLabelApplication is an atomic receipt for one approved result revision.
// Labels and the relational retrieval index are committed in the same transaction.
type KnowledgeLabelApplication struct {
	ID            string    `json:"id"`
	TenantID      int64     `json:"tenant_id"`
	UserID        int64     `json:"user_id"`
	KnowledgeID   string    `json:"knowledge_id"`
	ResultID      string    `json:"result_id"`
	ReviewVersion int       `json:"review_version"`
	SourceRefID   string    `json:"source_ref_id"`
	ContentDigest string    `json:"content_digest"`
	SchemeDigest  string    `json:"scheme_digest"`
	Labels        []string  `json:"labels"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}
type ApplyKnowledgeLabels struct {
	ResultID        string `json:"result_id"`
	ExpectedVersion int    `json:"expected_version"`
	KnowledgeID     string `json:"knowledge_id"`
}

// KnowledgeLabelTarget is a read-only resolution of the current approved
// revision. Applying labels repeats the same authorization and content checks.
type KnowledgeLabelTarget struct {
	ResultID      string `json:"result_id"`
	ReviewVersion int    `json:"review_version"`
	KnowledgeID   string `json:"knowledge_id"`
	SourceRefID   string `json:"source_ref_id"`
	ContentDigest string `json:"content_digest"`
}

// PreparedKnowledgeLabels is constructed only after source authorization.
type PreparedKnowledgeLabels struct {
	ApplyKnowledgeLabels
	SourceRefID, Content, SchemeDigest, ReviewDigest, IdempotencyKey string
	Labels                                                           []string
}
type KnowledgeLabelRepository interface {
	ApplyApproved(context.Context, l.Principal, PreparedKnowledgeLabels) (*KnowledgeLabelApplication, error)
	Labels(context.Context, l.Principal, string) ([]string, error)
	FilterKnowledgeLabels(context.Context, int64, []string, []string) (map[string]bool, error)
}

func LabelContentDigest(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
