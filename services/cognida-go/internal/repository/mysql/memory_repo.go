package mysql

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	d "cognida/internal/model/memory"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type personalMemoryRow struct {
	ID                                          string `gorm:"primaryKey"`
	TenantID, UserID                            int64
	Category, Content                           string
	Importance                                  float64
	Version                                     int64
	Source                                      string
	Embedding                                   []byte
	EmbeddingModel, IdempotencyKey, RequestHash string
	AccessCount                                 int
	LastAccessAt                                *time.Time
	CreatedAt, UpdatedAt                        time.Time
	DeletedAt                                   *time.Time
}

func (personalMemoryRow) TableName() string { return "user_memories" }

type personalMemoryRepository struct{ db *gorm.DB }

func NewLongTermMemoryRepository(db *gorm.DB) d.LongTermMemoryRepository {
	return &personalMemoryRepository{db: db}
}

func (r *personalMemoryRepository) scoped(ctx context.Context, scope d.Scope) (*gorm.DB, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	return r.db.WithContext(ctx).Model(&personalMemoryRow{}).Where("tenant_id = ? AND user_id = ?", scope.TenantID, scope.UserID), nil
}
func memoryRow(m *d.LongTermMemory) personalMemoryRow {
	var embedding []byte
	if len(m.Embedding) > 0 {
		embedding, _ = json.Marshal(m.Embedding)
	}
	return personalMemoryRow{ID: m.ID, TenantID: m.TenantID, UserID: m.UserID, Category: m.Category,
		Content: m.Content, Importance: m.Importance, Version: m.Version, Source: m.Source,
		Embedding: embedding, EmbeddingModel: m.EmbeddingModel, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}
func (r personalMemoryRow) domain() (*d.LongTermMemory, error) {
	m := &d.LongTermMemory{ID: r.ID, TenantID: r.TenantID, UserID: r.UserID, Category: r.Category,
		Content: r.Content, Importance: r.Importance, Version: r.Version, Source: r.Source,
		EmbeddingModel: r.EmbeddingModel, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		AccessCount: r.AccessCount, LastAccessAt: r.LastAccessAt, DeletedAt: r.DeletedAt}
	if len(r.Embedding) > 0 {
		if err := json.Unmarshal(r.Embedding, &m.Embedding); err != nil {
			return nil, err
		}
	}
	return m, nil
}
func memoryRepoError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return d.ErrNotFound
	}
	return err
}

func (r *personalMemoryRepository) Create(ctx context.Context, scope d.Scope, m *d.LongTermMemory, key, hash string) (*d.LongTermMemory, error) {
	q, err := r.scoped(ctx, scope)
	if err != nil {
		return nil, err
	}
	if m == nil || m.ID == "" || key == "" || hash == "" {
		return nil, d.ErrInvalid
	}
	// Keys are opaque and case-sensitive, regardless of the table's content collation.
	// Store their digest so "Save-A" and "save-a" cannot alias in a _ci database.
	keyDigest := sha256.Sum256([]byte(key))
	key = hex.EncodeToString(keyDigest[:])
	row := memoryRow(m)
	row.TenantID, row.UserID, row.IdempotencyKey, row.RequestHash = scope.TenantID, scope.UserID, key, hash
	// The unique scope/key index also serializes concurrent retries. A replay never overwrites content.
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return nil, err
	}
	var stored personalMemoryRow
	if err := q.Where("idempotency_key = ?", key).First(&stored).Error; err != nil {
		return nil, memoryRepoError(err)
	}
	if stored.RequestHash != hash || stored.DeletedAt != nil {
		return nil, d.ErrConflict
	}
	return stored.domain()
}
func (r *personalMemoryRepository) Get(ctx context.Context, scope d.Scope, id string) (*d.LongTermMemory, error) {
	q, err := r.scoped(ctx, scope)
	if err != nil {
		return nil, err
	}
	var row personalMemoryRow
	if err := q.Where("id = ? AND deleted_at IS NULL", id).First(&row).Error; err != nil {
		return nil, memoryRepoError(err)
	}
	return row.domain()
}
func memoryCursor(id string) string { return base64.RawURLEncoding.EncodeToString([]byte(id)) }
func memoryCursorID(cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", d.ErrInvalid
	}
	id := string(b)
	if _, err := uuid.Parse(id); err != nil {
		return "", d.ErrInvalid
	}
	return id, nil
}
func (r *personalMemoryRepository) List(ctx context.Context, scope d.Scope, f d.ListFilter) (*d.MemoryPage, error) {
	q, err := r.scoped(ctx, scope)
	if err != nil {
		return nil, err
	}
	id, err := memoryCursorID(f.Cursor)
	if err != nil {
		return nil, err
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	q = q.Where("deleted_at IS NULL")
	if id != "" {
		q = q.Where("id > ?", id)
	}
	if f.Category != "" {
		q = q.Where("category = ?", f.Category)
	}
	if f.Query != "" {
		literal := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(f.Query)
		q = q.Where("content LIKE ? ESCAPE '!'", "%"+literal+"%")
	}
	var rows []personalMemoryRow
	if err := q.Order("id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := &d.MemoryPage{Items: []*d.LongTermMemory{}, HasMore: len(rows) > limit}
	if out.HasMore {
		rows = rows[:limit]
	}
	for _, row := range rows {
		m, err := row.domain()
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, m)
	}
	if out.HasMore {
		out.NextCursor = memoryCursor(rows[len(rows)-1].ID)
	}
	return out, nil
}
func (r *personalMemoryRepository) Candidates(ctx context.Context, scope d.Scope, category string, limit int) ([]*d.LongTermMemory, bool, error) {
	q, err := r.scoped(ctx, scope)
	if err != nil {
		return nil, false, err
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	q = q.Where("deleted_at IS NULL")
	if category != "" {
		q = q.Where("category = ?", category)
	}
	var rows []personalMemoryRow
	if err := q.Order("updated_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, false, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	out := make([]*d.LongTermMemory, 0, len(rows))
	for _, row := range rows {
		m, err := row.domain()
		if err != nil {
			return nil, false, err
		}
		out = append(out, m)
	}
	return out, more, nil
}
func (r *personalMemoryRepository) Update(ctx context.Context, scope d.Scope, m *d.LongTermMemory, expected int64) (*d.LongTermMemory, error) {
	if m == nil || expected < 1 {
		return nil, d.ErrInvalid
	}
	q, err := r.scoped(ctx, scope)
	if err != nil {
		return nil, err
	}
	row := memoryRow(m)
	result := q.Where("id = ? AND deleted_at IS NULL AND version = ?", m.ID, expected).Updates(map[string]any{
		"content": row.Content, "category": row.Category, "importance": row.Importance,
		"embedding": row.Embedding, "embedding_model": row.EmbeddingModel, "updated_at": row.UpdatedAt,
		"version": gorm.Expr("version + 1"),
	})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		if _, err := r.Get(ctx, scope, m.ID); err != nil {
			return nil, err
		}
		return nil, d.ErrConflict
	}
	return r.Get(ctx, scope, m.ID)
}
func (r *personalMemoryRepository) Delete(ctx context.Context, scope d.Scope, id string, expected int64) error {
	if expected < 1 {
		return d.ErrInvalid
	}
	q, err := r.scoped(ctx, scope)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	result := q.Where("id = ? AND deleted_at IS NULL AND version = ?", id, expected).Updates(map[string]any{
		"content": "", "embedding": nil, "embedding_model": "", "deleted_at": now, "updated_at": now,
		"version": gorm.Expr("version + 1"), "importance": 0, "access_count": 0, "last_access_at": nil,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	q, err = r.scoped(ctx, scope)
	if err != nil {
		return err
	}
	var row personalMemoryRow
	if err := q.Where("id = ?", id).First(&row).Error; err != nil {
		return memoryRepoError(err)
	}
	if row.DeletedAt != nil {
		return nil
	}
	return d.ErrConflict
}
