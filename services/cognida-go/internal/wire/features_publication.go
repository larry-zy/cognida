package wire

import (
	"context"
	"fmt"

	l "cognida/internal/model/lifecycle"
	"cognida/internal/repository/mysql"
	"cognida/internal/service/catalog"
	"cognida/internal/service/lifecycle"
	"gorm.io/gorm"
)

func featurePublicationValidator(db *gorm.DB, catalogService *catalog.Service) lifecycle.PublicationValidator {
	return func(ctx context.Context, p l.Principal, input l.Input, options map[string]any) error {
		if input.Kind == "text" {
			id, _ := options["knowledge_base_id"].(string)
			if id == "" {
				return fmt.Errorf("%w: knowledge_base_id required for text publication", l.ErrInvalid)
			}
			kb, err := mysql.NewKnowledgeBaseRepository(db, true).FindByIDForTenant(ctx, id, p.TenantID)
			if err != nil || kb == nil || kb.DeletedAt != nil || !kb.IsActive() {
				return l.ErrForbidden
			}
			return nil
		}
		if raw, exists := options["catalog_asset_id"]; exists && raw != "" {
			id, ok := raw.(string)
			if !ok {
				return l.ErrInvalid
			}
			asset, err := catalogService.Get(ctx, p.TenantID, id)
			if err != nil {
				return err
			}
			if asset.Deprecated {
				return l.ErrInvalid
			}
		}
		return nil
	}
}
