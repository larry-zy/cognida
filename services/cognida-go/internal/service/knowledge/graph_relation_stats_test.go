package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "cognida/internal/model/knowledge"
	"github.com/stretchr/testify/require"
)

type relationStatsKBRepo struct {
	domain.KnowledgeBaseRepository
	kb       *domain.KnowledgeBase
	err      error
	calls    int
	tenantID int64
	kbID     string
}

func (r *relationStatsKBRepo) FindByIDForTenant(_ context.Context, id string, tenant int64) (*domain.KnowledgeBase, error) {
	r.calls++
	r.kbID, r.tenantID = id, tenant
	return r.kb, r.err
}

type relationStatsGraphRepo struct {
	domain.GraphRepository
	stats *domain.RelationStatistics
	err   error
	calls int
	ns    domain.NameSpace
}

func (r *relationStatsGraphRepo) GetRelationStats(_ context.Context, ns domain.NameSpace) (*domain.RelationStatistics, error) {
	r.calls++
	r.ns = ns
	return r.stats, r.err
}

func TestGraphRelationStatsAuthorizesBeforeAggregation(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name           string
		tenant, user   int64
		id             string
		kb             *domain.KnowledgeBase
		kbErr, wantErr error
	}{
		{name: "missing tenant", user: 9, id: "kb", wantErr: ErrGraphIdentityRequired},
		{name: "missing user", tenant: 41, id: "kb", wantErr: ErrGraphIdentityRequired},
		{name: "negative identity", tenant: -1, user: 9, id: "kb", wantErr: ErrGraphIdentityRequired},
		{name: "missing kb", tenant: 41, user: 9, id: " ", wantErr: ErrGraphInvalidScope},
		{name: "not found", tenant: 41, user: 9, id: "kb", kbErr: errors.New("知识库不存在"), wantErr: ErrGraphKnowledgeBaseNotFound},
		{name: "nil kb", tenant: 41, user: 9, id: "kb", wantErr: ErrGraphKnowledgeBaseNotFound},
		{name: "cross tenant", tenant: 41, user: 9, id: "kb", kb: &domain.KnowledgeBase{ID: "kb", TenantID: 42, UserID: 9, Status: 1}, wantErr: ErrGraphKnowledgeBaseNotFound},
		{name: "mismatched kb", tenant: 41, user: 9, id: "kb", kb: &domain.KnowledgeBase{ID: "other", TenantID: 41, UserID: 9, Status: 1}, wantErr: ErrGraphKnowledgeBaseNotFound},
		{name: "private other owner", tenant: 41, user: 9, id: "kb", kb: &domain.KnowledgeBase{ID: "kb", TenantID: 41, UserID: 10, Status: 1}, wantErr: ErrGraphForbidden},
		{name: "inactive", tenant: 41, user: 9, id: "kb", kb: &domain.KnowledgeBase{ID: "kb", TenantID: 41, UserID: 9}, wantErr: ErrGraphKnowledgeBaseNotFound},
		{name: "deleted", tenant: 41, user: 9, id: "kb", kb: &domain.KnowledgeBase{ID: "kb", TenantID: 41, UserID: 9, Status: 1, DeletedAt: &now}, wantErr: ErrGraphKnowledgeBaseNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &relationStatsGraphRepo{}
			kb := &relationStatsKBRepo{kb: tc.kb, err: tc.kbErr}
			svc := NewGraphService(repo, nil)
			svc.SetKnowledgeBaseRepository(kb)
			stats, err := svc.GetRelationStats(context.Background(), tc.tenant, tc.user, tc.id)
			require.ErrorIs(t, err, tc.wantErr)
			require.Nil(t, stats)
			require.Zero(t, repo.calls)
			if errors.Is(tc.wantErr, ErrGraphIdentityRequired) || errors.Is(tc.wantErr, ErrGraphInvalidScope) {
				require.Zero(t, kb.calls)
			}
		})
	}
}

func TestGraphRelationStatsDTOAndErrors(t *testing.T) {
	for _, public := range []bool{false, true} {
		owner := int64(9)
		if public {
			owner = 10
		}
		repo := &relationStatsGraphRepo{stats: &domain.RelationStatistics{TotalRelations: 2, AverageStrength: 2.25, AverageWeight: 3, AverageChunkCount: 1.5, StrengthDistribution: map[int]int{2: 2}, TypeDistribution: map[string]int{"owns": 2}}}
		kb := &relationStatsKBRepo{kb: &domain.KnowledgeBase{ID: "kb", TenantID: 41, UserID: owner, Status: 1, IsPublic: public}}
		svc := NewGraphService(repo, nil)
		svc.SetKnowledgeBaseRepository(kb)
		stats, err := svc.GetRelationStats(context.Background(), 41, 9, "kb")
		require.NoError(t, err)
		require.Equal(t, 2, stats.TotalRelations)
		require.Equal(t, map[string]int{"2": 2}, stats.StrengthDistribution)
		require.Equal(t, 1.5, stats.AverageChunkCount)
		require.Equal(t, domain.NameSpace{TenantID: "41", KnowledgeBaseID: "kb"}, repo.ns)
		require.Equal(t, int64(41), kb.tenantID)
		require.Equal(t, "kb", kb.kbID)
		failure := errors.New("database unavailable")
		kb.err = failure
		_, err = svc.GetRelationStats(context.Background(), 41, 9, "kb")
		require.ErrorIs(t, err, failure)
		kb.err = nil
		repo.err = failure
		_, err = svc.GetRelationStats(context.Background(), 41, 9, "kb")
		require.ErrorIs(t, err, failure)
		repo.err = nil
		repo.stats = nil
		_, err = svc.GetRelationStats(context.Background(), 41, 9, "kb")
		require.Error(t, err)
		repo.stats = &domain.RelationStatistics{}
		stats, err = svc.GetRelationStats(context.Background(), 41, 9, "kb")
		require.NoError(t, err)
		require.NotNil(t, stats.StrengthDistribution)
		require.NotNil(t, stats.TypeDistribution)
	}
	svc := NewGraphService(nil, nil)
	_, err := svc.GetRelationStats(context.Background(), 41, 9, "kb")
	require.ErrorIs(t, err, ErrGraphStatsUnavailable)
	svc.SetKnowledgeBaseRepository(&relationStatsKBRepo{})
	_, err = svc.GetRelationStats(context.Background(), 41, 9, "kb")
	require.ErrorIs(t, err, ErrGraphStatsUnavailable)
}
