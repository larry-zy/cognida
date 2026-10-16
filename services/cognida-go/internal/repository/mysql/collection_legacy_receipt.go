package mysql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Read-only reconciliation for pre-migration runs. Identity exactly matches
// DocumentProcessor's old content idempotency key, never creates/reprocesses it.
func (r *CollectionRepository) FindLegacyReceipt(ctx context.Context, tenant int64, kb, hash string) (string, error) {
	if tenant <= 0 || kb == "" || len(hash) != 64 {
		return "", nil
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", tenant, kb, hash)))
	id := uuid.NewSHA1(uuid.NameSpaceURL, digest[:]).String()
	var row struct{ ID string }
	err := r.getDB(ctx).Table("knowledge").Select("id").Where("id=? AND tenant_id=? AND knowledge_base_id=? AND file_hash=? AND parse_status=?", id, tenant, kb, hash, "completed").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return row.ID, err
}
