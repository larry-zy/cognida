package knowledge

import (
	agentctx "cognida/internal/model/agent"
	l "cognida/internal/model/lifecycle"
	d "cognida/internal/model/strategy"
	strategies "cognida/internal/service/strategy"
	"context"
	"encoding/json"
	"errors"
)

func DefaultRetrievalStrategy(context.Context, l.Principal) (json.RawMessage, error) {
	return json.RawMessage(`{"inherit_defaults":true}`), nil
}
func (c *RetrievalCapability) WithStrategies(s *strategies.Service) *RetrievalCapability {
	c.strategies = s
	return c
}
func (c *RetrievalCapability) strategyQuery(ctx context.Context, tenant int64, q GovernedQuery) (context.Context, GovernedQuery, error) {
	if c.strategies == nil {
		return ctx, q, nil
	}
	user, ok := agentctx.GetUserID(ctx)
	if !ok || user <= 0 {
		return ctx, q, d.ErrForbidden
	}
	p := l.Principal{TenantID: tenant, UserID: user}
	snap := strategies.FromContext(ctx)
	if snap != nil && snap.Kind != "retrieval" {
		return ctx, q, nil
	}
	if snap == nil {
		key := agentctx.MustGetRequestID(ctx)
		if key == "" {
			return ctx, q, d.ErrInvalid
		}
		var e error
		topK := q.TopK
		if topK <= 0 {
			topK = capDefaultMaxChunks
		}
		effective, _ := json.Marshal(d.RetrievalPolicy{TopK: topK, ScoreThreshold: q.MinScore, Rerank: q.EnableRerank})
		snap, e = c.strategies.Pin(ctx, p, "retrieval.default", d.Digest([]string{"retrieval", key}), effective)
		if errors.Is(e, d.ErrNotFound) {
			return ctx, q, nil
		}
		if e != nil {
			return ctx, q, e
		}
	}
	if e := c.strategies.ValidateSnapshot(ctx, p, snap); e != nil {
		return ctx, q, e
	}
	var policy d.RetrievalPolicy
	if json.Unmarshal(snap.EffectiveBody, &policy) != nil {
		return ctx, q, d.ErrInvalid
	}
	if policy.InheritDefaults {
		return strategies.WithSnapshot(ctx, snap), q, nil
	}
	q.TopK = policy.TopK
	q.MinScore = policy.ScoreThreshold
	q.EnableRerank = policy.Rerank
	return strategies.WithSnapshot(ctx, snap), q, nil
}

func (c *RetrievalCapability) validateStrategy(ctx context.Context, tenant int64) error {
	snap := strategies.FromContext(ctx)
	if snap == nil || snap.Kind != "retrieval" {
		return nil
	}
	if c.strategies == nil {
		return d.ErrUnavailable
	}
	user, ok := agentctx.GetUserID(ctx)
	if !ok || user <= 0 {
		return d.ErrForbidden
	}
	return c.strategies.ValidateSnapshot(ctx, l.Principal{TenantID: tenant, UserID: user}, snap)
}
