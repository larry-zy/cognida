package cache

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var (
	ErrCleanupInvalid     = errors.New("invalid scoped cache cleanup request")
	ErrCleanupIncomplete  = errors.New("cache cleanup enumeration is incomplete")
	ErrCleanupUnsupported = errors.New("cache repositories do not support safe cleanup")
	ErrCacheOwnership     = errors.New("cache identity or ownership mismatch")
	ErrCacheWriteFenced   = errors.New("cache ID is fenced for orphan cleanup; use a new ID")
	ErrCleanupBusy        = errors.New("cache cleanup fence is already held")
	ErrCleanupReconcile   = errors.New("cache cleanup outcome requires reconciliation")
)

// Cleanup is an explicit tenant operation, never an all-tenants sweep. A
// complete bounded enumeration and content preflight precede any mutation.
// Grace defaults to one minute (greater than the existing 30-second async
// writer budget), but is not a cross-store transaction: the Redis absent fence
// prevents late content publication. Unknown or abandoned active fences never
// expire automatically; they require operator reconciliation after in-flight
// writes/deletes have been stopped. This API does not silently clear them.
type OrphanCleanupRequest struct {
	TenantID    int64
	GracePeriod time.Duration
	MaxVectors  int
	BatchSize   int
}

type OrphanCleanupResult struct {
	Scanned    int
	Preserved  int
	Candidates int
	Deleted    int64
}

type CacheVectorIdentity struct {
	CacheID   string `json:"cache_id"`
	TenantID  int64  `json:"tenant_id,string"`
	AgentType string `json:"agent_type"`
	CreatedAt int64  `json:"created_at,string"`
}

var cleanupID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
var cleanupAgent = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`)

func (v CacheVectorIdentity) Validate() error {
	if v.TenantID < 1 || !cleanupID.MatchString(v.CacheID) || !cleanupAgent.MatchString(v.AgentType) || v.CreatedAt < 1 {
		return ErrCacheOwnership
	}
	return nil
}

type CacheCleanupFence struct {
	Identity CacheVectorIdentity `json:"identity"`
	Token    string              `json:"token"`
	State    string              `json:"state"`
}

// NotApplied is set only for a local rejection or a definite server rejection.
// Transport failures must remain unknown: retrying them is not proof of absence.
type CacheDeleteError struct {
	Cause      error
	NotApplied bool
}

func (e *CacheDeleteError) Error() string { return "cache vector deletion failed" }
func (e *CacheDeleteError) Unwrap() error { return e.Cause }

type CacheCleanupVectorRepository interface {
	ListCleanupCandidates(context.Context, int64, int64, int, int) ([]CacheVectorIdentity, error)
	DeleteCleanupCandidate(context.Context, CacheVectorIdentity) (int64, error)
}

type CacheCleanupContentRepository interface {
	// Fences are persistent and must also be fully enumerated: an unknown
	// deletion can remove the vector while its acknowledgement is lost.
	ListCleanupFences(context.Context, int64, int, int) ([]CacheCleanupFence, error)
	// Present inspects the primary hash, not an expiring secondary index.
	CleanupContentPresent(context.Context, CacheVectorIdentity) (bool, error)
	// Claim returns nil when content appeared since preflight. The absent check
	// and fence creation are atomic with SetContent's fence check and write.
	ClaimCleanup(context.Context, CacheVectorIdentity) (*CacheCleanupFence, error)
	// Finish releases only an acknowledged successful deletion. Definite failure
	// remains retryable; unknown outcomes retain an operator-visible fence.
	FinishCleanup(context.Context, CacheCleanupFence, string) error
}
