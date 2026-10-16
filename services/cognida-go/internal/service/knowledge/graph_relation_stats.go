package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	domain "cognida/internal/model/knowledge"
)

var (
	ErrGraphIdentityRequired      = errors.New("图谱统计需要有效的租户和用户身份")
	ErrGraphInvalidScope          = errors.New("知识库ID不能为空")
	ErrGraphKnowledgeBaseNotFound = errors.New("知识库不存在或不可用")
	ErrGraphForbidden             = errors.New("无权访问该知识库的图谱")
	ErrGraphStatsUnavailable      = errors.New("图谱服务未配置")
)

// SetKnowledgeBaseRepository supplies the authoritative scope/ownership lookup.
// Without it relation statistics fail closed, including for an empty graph.
func (s *GraphService) SetKnowledgeBaseRepository(repo domain.KnowledgeBaseRepository) {
	s.knowledgeBaseRepo = repo
}

func (s *GraphService) GetRelationStats(ctx context.Context, tenantID, userID int64, kbID string) (*RelationStatsDTO, error) {
	if err := s.authorizeGraphKnowledgeBase(ctx, tenantID, userID, kbID, false); err != nil {
		return nil, err
	}
	repo, ok := s.graphRepo.(domain.GraphRelationStatsRepository)
	if !ok {
		return nil, ErrGraphStatsUnavailable
	}
	stats, err := repo.GetRelationStats(ctx, domain.NameSpace{TenantID: strconv.FormatInt(tenantID, 10), KnowledgeBaseID: kbID})
	if err != nil {
		return nil, fmt.Errorf("查询图谱关系统计失败: %w", err)
	}
	if stats == nil {
		return nil, errors.New("图谱关系统计返回空结果")
	}
	dto := &RelationStatsDTO{
		TotalRelations:       stats.TotalRelations,
		AverageStrength:      stats.AverageStrength,
		AverageWeight:        stats.AverageWeight,
		AverageChunkCount:    stats.AverageChunkCount,
		StrengthDistribution: make(map[string]int, len(stats.StrengthDistribution)),
		TypeDistribution:     make(map[string]int, len(stats.TypeDistribution)),
	}
	for bucket, count := range stats.StrengthDistribution {
		dto.StrengthDistribution[strconv.Itoa(bucket)] = count
	}
	for kind, count := range stats.TypeDistribution {
		dto.TypeDistribution[kind] = count
	}
	return dto, nil
}

// AddAuthorizedRelation is the HTTP write boundary. Public sharing permits
// statistics reads, not graph mutation by a different owner.
func (s *GraphService) AddAuthorizedRelation(ctx context.Context, tenantID, userID int64, kbID string, relation *domain.GraphRelation) (*domain.GraphRelation, error) {
	if err := s.authorizeGraphKnowledgeBase(ctx, tenantID, userID, kbID, true); err != nil {
		return nil, err
	}
	return s.AddRelation(ctx, domain.NameSpace{TenantID: strconv.FormatInt(tenantID, 10), KnowledgeBaseID: kbID}, relation)
}

func (s *GraphService) authorizeGraphKnowledgeBase(ctx context.Context, tenantID, userID int64, kbID string, write bool) error {
	if tenantID <= 0 || userID <= 0 {
		return ErrGraphIdentityRequired
	}
	if strings.TrimSpace(kbID) == "" {
		return ErrGraphInvalidScope
	}
	if s == nil || s.knowledgeBaseRepo == nil || s.graphRepo == nil {
		return ErrGraphStatsUnavailable
	}
	kb, err := s.knowledgeBaseRepo.FindByIDForTenant(ctx, kbID, tenantID)
	if err != nil {
		// The existing KB repository exposes this exact not-found error; do not
		// collapse unknown DB failures into either an empty graph or a 404.
		if err.Error() == "知识库不存在" {
			return ErrGraphKnowledgeBaseNotFound
		}
		return fmt.Errorf("读取图谱知识库授权失败: %w", err)
	}
	if kb == nil || kb.ID != kbID || kb.TenantID != tenantID || kb.DeletedAt != nil || !kb.IsActive() {
		return ErrGraphKnowledgeBaseNotFound
	}
	if kb.UserID != userID && (write || !kb.IsPublic) {
		return ErrGraphForbidden
	}
	return nil
}
