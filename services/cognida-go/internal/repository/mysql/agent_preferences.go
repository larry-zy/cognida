package mysql

import (
	d "cognida/internal/model/agent"
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type agentPreferencesRow struct {
	TenantID   int64 `gorm:"primaryKey"`
	UserID     int64 `gorm:"primaryKey"`
	Version    int64
	ConfigJSON []byte `gorm:"column:config_json;type:json"`
	UpdatedAt  time.Time
}

func (agentPreferencesRow) TableName() string { return "agent_preferences" }

type agentPreferenceUpdateRow struct {
	TenantID     int64  `gorm:"primaryKey"`
	UserID       int64  `gorm:"primaryKey"`
	OperationKey string `gorm:"primaryKey"`
	RequestHash  string
	ResultJSON   []byte `gorm:"column:result_json;type:json"`
	CreatedAt    time.Time
}

func (agentPreferenceUpdateRow) TableName() string { return "agent_preference_updates" }

type agentPreferencesRepository struct{ db *gorm.DB }

func NewAgentPreferencesRepository(db *gorm.DB) d.PreferencesRepository {
	return &agentPreferencesRepository{db}
}
func decodePreferences(body []byte) (d.Preferences, error) {
	var p d.Preferences
	if err := json.Unmarshal(body, &p); err != nil {
		return p, err
	}
	return p, p.Validate()
}
func (r *agentPreferencesRepository) Read(ctx context.Context, scope d.ConfigScope) (d.Preferences, error) {
	if err := scope.Validate(); err != nil {
		return d.Preferences{}, err
	}
	var row agentPreferencesRow
	err := r.db.WithContext(ctx).Where("tenant_id=? AND user_id=?", scope.TenantID, scope.UserID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return d.Preferences{}, nil
	}
	if err != nil {
		return d.Preferences{}, err
	}
	p, err := decodePreferences(row.ConfigJSON)
	if err == nil && p.Version != row.Version {
		return p, d.ErrConfigInvalid
	}
	return p, err
}
func (r *agentPreferencesRepository) Update(ctx context.Context, scope d.ConfigScope, expected int64, key, hash string, next d.Preferences) (out d.Preferences, err error) {
	if err = scope.Validate(); err != nil {
		return
	}
	if expected < 0 || len(key) != 64 || len(hash) != 64 {
		err = d.ErrConfigInvalid
		return
	}
	if err = next.Validate(); err != nil {
		return
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		empty, _ := json.Marshal(d.Preferences{})
		// Even the first update serializes on the same scope row. Rollback removes this
		// insertion if CAS or validation fails; no successful response precedes commit.
		row := agentPreferencesRow{TenantID: scope.TenantID, UserID: scope.UserID, ConfigJSON: empty, UpdatedAt: time.Now().UTC()}
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
			return e
		}
		q := tx.Where("tenant_id=? AND user_id=?", scope.TenantID, scope.UserID)
		if e := q.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row).Error; e != nil {
			return e
		}
		var prior agentPreferenceUpdateRow
		e := tx.Where("tenant_id=? AND user_id=? AND operation_key=?", scope.TenantID, scope.UserID, key).First(&prior).Error
		if e == nil {
			if prior.RequestHash != hash {
				return d.ErrConfigConflict
			}
			out, e = decodePreferences(prior.ResultJSON)
			return e
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if row.Version != expected {
			return d.ErrConfigConflict
		}
		next.Version = expected + 1
		body, e := json.Marshal(next)
		if e != nil {
			return e
		}
		if e = tx.Model(&agentPreferencesRow{}).Where("tenant_id=? AND user_id=? AND version=?", scope.TenantID, scope.UserID, expected).Updates(map[string]any{"version": next.Version, "config_json": body, "updated_at": time.Now().UTC()}).Error; e != nil {
			return e
		}
		update := agentPreferenceUpdateRow{TenantID: scope.TenantID, UserID: scope.UserID, OperationKey: key, RequestHash: hash, ResultJSON: body, CreatedAt: time.Now().UTC()}
		if e = tx.Create(&update).Error; e != nil {
			return e
		}
		out = next
		return nil
	})
	return
}
