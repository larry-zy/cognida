package mysql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	d "cognida/internal/model/governance"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type featureValueRow struct {
	TenantID    int64     `gorm:"primaryKey"`
	FeatureID   string    `gorm:"primaryKey"`
	Version     int       `gorm:"primaryKey"`
	EntityID    string    `gorm:"primaryKey"`
	ObservedAt  time.Time `gorm:"primaryKey"`
	Value       []byte
	ValueExact  []byte
	SourceRefID string
	AvailableAt time.Time
}

func (featureValueRow) TableName() string { return "feature_values" }

type featureReceiptRow struct {
	ID                            string `gorm:"primaryKey"`
	TenantID, UserID              int64
	IdempotencyKey, RequestDigest string
	Payload                       []byte
	AvailableAt                   time.Time
}

func (featureReceiptRow) TableName() string { return "feature_materializations" }
func featureDatabaseNow(tx *gorm.DB) (time.Time, error) {
	var micros int64
	err := tx.Raw("SELECT CAST(UNIX_TIMESTAMP(NOW(6))*1000000 AS SIGNED)").Scan(&micros).Error
	return time.UnixMicro(micros).UTC(), err
}
func writeFeatureValues(tx *gorm.DB, tenant int64, values []d.FeatureValue, available time.Time) error {
	if tenant <= 0 || len(values) > 1000 {
		return d.ErrInvalid
	}
	for _, v := range values {
		if _, e := uuid.Parse(v.FeatureID); e != nil || v.Version < 1 || v.EntityID == "" || len(v.EntityID) > 128 || v.Timestamp.IsZero() || len(v.Value) == 0 || len(v.Value) > 65536 || !json.Valid(v.Value) {
			return d.ErrInvalid
		}
		row := featureValueRow{TenantID: tenant, FeatureID: v.FeatureID, Version: v.Version, EntityID: v.EntityID, ObservedAt: v.Timestamp.UTC().Truncate(time.Microsecond), Value: v.Value, ValueExact: v.Value, SourceRefID: v.SourceRefID, AvailableAt: available}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		var old featureValueRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND feature_id=? AND version=? AND entity_id=? AND observed_at=?", tenant, v.FeatureID, v.Version, v.EntityID, row.ObservedAt).Take(&old).Error; err != nil {
			return err
		}
		exact := old.ValueExact
		if len(exact) == 0 {
			exact = old.Value
		}
		if !d.JSONEqual(exact, v.Value) || old.SourceRefID != v.SourceRefID {
			return d.ErrConflict
		}
	}
	return nil
}
func (r *governanceRepository) FeatureWrite(ctx context.Context, tenant int64, values []d.FeatureValue) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := featureDatabaseNow(tx)
		if err != nil {
			return err
		}
		return writeFeatureValues(tx, tenant, values, now)
	})
}
func (r *governanceRepository) FeatureQuery(ctx context.Context, tenant int64, entities []string, features []d.FeatureValue, at time.Time) ([]d.FeatureValue, error) {
	if tenant <= 0 || len(entities)*len(features) > 1000 {
		return nil, d.ErrInvalid
	}
	out := []d.FeatureValue{}
	for _, feature := range features {
		for _, entity := range entities {
			var row featureValueRow
			err := r.db.WithContext(ctx).Where("tenant_id=? AND feature_id=? AND version=? AND entity_id=? AND observed_at<=? AND available_at<=?", tenant, feature.FeatureID, feature.Version, entity, at, at).Order("observed_at DESC").Take(&row).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			exact := row.ValueExact
			if len(exact) == 0 {
				exact = row.Value
			}
			out = append(out, d.FeatureValue{FeatureID: row.FeatureID, Version: row.Version, EntityID: row.EntityID, Timestamp: row.ObservedAt, AvailableAt: row.AvailableAt, Value: exact, ValueExact: string(exact), SourceRefID: row.SourceRefID})
		}
	}
	return out, nil
}
func decodeFeatureReceipt(row featureReceiptRow) (*d.FeatureMaterialization, error) {
	var receipt d.FeatureMaterialization
	if json.Unmarshal(row.Payload, &receipt) != nil || receipt.ReceiptID != row.ID {
		return nil, d.ErrConflict
	}
	receipt.AvailableAt = row.AvailableAt.UTC()
	return &receipt, nil
}
func (r *governanceRepository) FeatureReceipt(ctx context.Context, tenant, user int64, key string) (*d.FeatureMaterialization, error) {
	if tenant <= 0 || user <= 0 || key == "" || len(key) > 100 {
		return nil, d.ErrInvalid
	}
	var row featureReceiptRow
	err := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND idempotency_key=?", tenant, user, key).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decodeFeatureReceipt(row)
}
func (r *governanceRepository) FeatureReceiptByID(ctx context.Context, tenant, user int64, id string) (*d.FeatureMaterialization, error) {
	if tenant <= 0 || user <= 0 {
		return nil, d.ErrInvalid
	}
	var row featureReceiptRow
	err := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=? AND id=?", tenant, user, id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return decodeFeatureReceipt(row)
}
func (r *governanceRepository) FeatureMaterialize(ctx context.Context, tenant, user int64, key string, receipt d.FeatureMaterialization, values []d.FeatureValue) (*d.FeatureMaterialization, error) {
	if strings.HasPrefix(key, "governed:") {
		return nil, d.ErrInvalid
	}
	return r.featureMaterialize(ctx, tenant, user, key, "", receipt, values)
}
func (r *governanceRepository) FeatureMaterializeGoverned(ctx context.Context, tenant, user int64, operationID string, receipt d.FeatureMaterialization, values []d.FeatureValue) (*d.FeatureMaterialization, error) {
	if _, e := uuid.Parse(operationID); e != nil {
		return nil, d.ErrInvalid
	}
	return r.featureMaterialize(ctx, tenant, user, "governed:"+operationID, operationID, receipt, values)
}
func (r *governanceRepository) featureMaterialize(ctx context.Context, tenant, user int64, key, operationID string, receipt d.FeatureMaterialization, values []d.FeatureValue) (out *d.FeatureMaterialization, err error) {
	if tenant <= 0 || user <= 0 || key == "" || len(key) > 100 || receipt.Count != len(values) || len(values) > 1000 || receipt.OutputRefID == "" || len(receipt.DefinitionDigest) != 64 || len(receipt.SourceDigest) != 64 {
		return nil, d.ErrInvalid
	}
	if _, e := uuid.Parse(receipt.ReceiptID); e != nil {
		return nil, d.ErrInvalid
	}
	receipt.AvailableAt = time.Time{}
	copied := append([]d.FeatureValue(nil), values...)
	for i := range copied {
		copied[i].AvailableAt = time.Time{}
		copied[i].ValueExact = string(copied[i].Value)
	}
	canonical, e := json.Marshal(struct {
		Receipt d.FeatureMaterialization
		Values  []d.FeatureValue
	}{receipt, copied})
	if e != nil {
		return nil, d.ErrInvalid
	}
	sum := sha256.Sum256(canonical)
	digest := hex.EncodeToString(sum[:])
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, e := featureDatabaseNow(tx)
		if e != nil {
			return e
		}
		if operationID != "" {
			var op governanceRow
			if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=? AND user_id=? AND kind=?", operationID, tenant, user, d.GovernedKind).Take(&op).Error; e != nil {
				return d.ErrForbidden
			}
			var state d.GovernedExecution
			if json.Unmarshal(op.Payload, &state) != nil || state.Invocation.Profile != d.FeatureProfile || state.Invocation.FeatureID != receipt.FeatureID || state.Invocation.FeatureVersion != receipt.Version || state.Invocation.InputRef.String() != receipt.SourceRefID {
				return d.ErrConflict
			}
			var old featureReceiptRow
			find := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND user_id=? AND idempotency_key=?", tenant, user, key).Take(&old).Error
			if find == nil {
				if old.RequestDigest != digest {
					return d.ErrConflict
				}
				out, e = decodeFeatureReceipt(old)
				return e
			}
			if !errors.Is(find, gorm.ErrRecordNotFound) {
				return find
			}
			now, e = featureDatabaseNow(tx)
			if e != nil {
				return e
			}
			if op.Status != "running" || !state.Invocation.Deadline.After(now) {
				return d.ErrConflict
			}
		}
		receipt.AvailableAt = now
		body, e := json.Marshal(receipt)
		if e != nil {
			return e
		}
		row := featureReceiptRow{ID: receipt.ReceiptID, TenantID: tenant, UserID: user, IdempotencyKey: key, RequestDigest: digest, Payload: body, AvailableAt: now}
		if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
			return e
		}
		var stored featureReceiptRow
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND user_id=? AND idempotency_key=?", tenant, user, key).Take(&stored).Error; e != nil {
			return e
		}
		if stored.RequestDigest != digest {
			return d.ErrConflict
		}
		if e = writeFeatureValues(tx, tenant, values, stored.AvailableAt); e != nil {
			return e
		}
		out, e = decodeFeatureReceipt(stored)
		return e
	})
	return
}
