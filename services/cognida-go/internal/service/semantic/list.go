package semantic

import (
	model "cognida/internal/model/semantic"
	"context"
	"fmt"
)

func (s *Service) ListManaged(ctx context.Context, tenantID int64, status, cursor string, limit int) ([]*model.SemanticModel, string, error) {
	if status == "" {
		status = "active"
	}
	if status != "all" && status != "active" && status != "draft" && status != "deprecated" {
		return nil, "", fmt.Errorf("%w: invalid model status", ErrValidation)
	}
	repo, ok := s.repo.(model.ModelLister)
	if !ok {
		return nil, "", fmt.Errorf("model listing unavailable")
	}
	return repo.ListModels(ctx, tenantID, status, cursor, limit)
}
