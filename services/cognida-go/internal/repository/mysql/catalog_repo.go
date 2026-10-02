package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	c "cognida/internal/model/catalog"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type catalogTableRow struct {
	ID                                string `gorm:"primaryKey"`
	TenantID                          int64
	DatasourceID, SchemaName, Comment string
	PhysicalName                      string `gorm:"column:table_name"`
	Tags                              []byte
	Deprecated                        bool
	Payload                           []byte
	LastSyncedAt                      time.Time
}

func (catalogTableRow) TableName() string { return "asset_table" }

type catalogColumnRow struct {
	ID                  string `gorm:"primaryKey"`
	TenantID            int64
	TableID, ColumnName string
	Deprecated          bool
	Payload             []byte
}

func (catalogColumnRow) TableName() string { return "asset_column" }

type glossaryRow struct {
	ID                 string `gorm:"primaryKey"`
	TenantID           int64
	Term, Description  string
	Synonyms, AssetIDs []byte
}

func (glossaryRow) TableName() string { return "business_glossary" }

type catalogRepository struct{ db *gorm.DB }

func NewCatalogRepository(db *gorm.DB) c.Repository { return &catalogRepository{db} }
func catalogJSON(v any) []byte                      { b, _ := json.Marshal(v); return b }
func catalogError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.ErrNotFound
	}
	return err
}
func catalogLimit(n int) int {
	if n <= 0 {
		return 20
	}
	if n > 100 {
		return 100
	}
	return n
}

// Snapshot atomically applies a complete source snapshot, retaining governance and
// tombstones. A failed schema probe never reaches this method.
func (r *catalogRepository) Snapshot(ctx context.Context, tenant int64, ds string, tables []c.AssetTable) (*c.SyncResult, error) {
	out := &c.SyncResult{Warnings: []string{}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source DataSourceModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=?", ds, tenant).First(&source).Error; err != nil {
			return catalogError(err)
		}
		var old []catalogTableRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND datasource_id=?", tenant, ds).Find(&old).Error; err != nil {
			return err
		}
		known := map[string]catalogTableRow{}
		for _, v := range old {
			known[v.SchemaName+"\x00"+v.PhysicalName] = v
		}
		seen := map[string]bool{}
		for _, t := range tables {
			key := t.Schema + "\x00" + t.TableName
			if seen[key] {
				return c.ErrInvalid
			}
			seen[key] = true
			t.TenantID = tenant
			t.DatasourceID = ds
			t.Deprecated = false
			if prev, ok := known[key]; ok {
				var p c.AssetTable
				if err := json.Unmarshal(prev.Payload, &p); err != nil {
					return err
				}
				t.ID = prev.ID
				t.Governance = p.Governance
				out.Updated++
			} else {
				t.ID = uuid.NewString()
				out.Created++
			}
			columns := t.Columns
			t.Columns = nil
			if t.Tags == nil {
				t.Tags = []string{}
			}
			row := catalogTableRow{ID: t.ID, TenantID: tenant, DatasourceID: ds, SchemaName: t.Schema, PhysicalName: t.TableName, Comment: t.Comment, Tags: catalogJSON(t.Tags), Payload: catalogJSON(t), LastSyncedAt: t.LastSyncedAt}
			if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error; err != nil {
				return err
			}
			var previous []catalogColumnRow
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND table_id=?", tenant, t.ID).Find(&previous).Error; err != nil {
				return err
			}
			byName := map[string]catalogColumnRow{}
			for _, v := range previous {
				byName[v.ColumnName] = v
			}
			seenCols := map[string]bool{}
			for _, col := range columns {
				if seenCols[col.Name] {
					return c.ErrInvalid
				}
				seenCols[col.Name] = true
				col.TenantID = tenant
				col.TableID = t.ID
				col.Deprecated = false
				if prev, ok := byName[col.Name]; ok {
					var p c.AssetColumn
					if err := json.Unmarshal(prev.Payload, &p); err != nil {
						return err
					}
					col.ID = prev.ID
					col.Governance = p.Governance
				} else {
					col.ID = uuid.NewString()
				}
				cr := catalogColumnRow{ID: col.ID, TenantID: tenant, TableID: t.ID, ColumnName: col.Name, Payload: catalogJSON(col)}
				if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&cr).Error; err != nil {
					return err
				}
				out.Columns++
			}
			for name, col := range byName {
				if !seenCols[name] && !col.Deprecated {
					if err := tx.Model(&catalogColumnRow{}).Where("id=? AND tenant_id=?", col.ID, tenant).Update("deprecated", true).Error; err != nil {
						return err
					}
					out.Deprecated++
				}
			}
			out.Tables++
		}
		for key, t := range known {
			if !seen[key] && !t.Deprecated {
				if err := tx.Model(&catalogTableRow{}).Where("id=? AND tenant_id=?", t.ID, tenant).Update("deprecated", true).Error; err != nil {
					return err
				}
				if err := tx.Model(&catalogColumnRow{}).Where("table_id=? AND tenant_id=?", t.ID, tenant).Update("deprecated", true).Error; err != nil {
					return err
				}
				out.Deprecated++
			}
		}
		return nil
	})
	return out, err
}
func (r *catalogRepository) List(ctx context.Context, tenant int64, f c.Filter) (*c.Page[c.AssetTable], error) {
	q := r.db.WithContext(ctx).Model(&catalogTableRow{}).Where("asset_table.tenant_id=?", tenant)
	if !f.IncludeDeprecated {
		q = q.Where("deprecated = ?", false)
	}
	if f.DatasourceID != "" {
		q = q.Where("datasource_id=?", f.DatasourceID)
	}
	if f.Tag != "" {
		q = q.Where("JSON_CONTAINS(tags, ?)", string(catalogJSON(f.Tag)))
	}
	if f.Cursor != "" {
		q = q.Where("id > ?", f.Cursor)
	}
	if s := strings.TrimSpace(f.Query); s != "" {
		like := "%" + s + "%"
		q = q.Where(`(table_name LIKE ? OR comment LIKE ? OR EXISTS (SELECT 1 FROM business_glossary g WHERE g.tenant_id=asset_table.tenant_id AND (g.term LIKE ? OR JSON_SEARCH(g.synonyms,'one',?) IS NOT NULL) AND (JSON_CONTAINS(g.asset_ids,JSON_QUOTE(asset_table.id)) OR EXISTS (SELECT 1 FROM asset_column ac WHERE ac.table_id=asset_table.id AND ac.tenant_id=asset_table.tenant_id AND JSON_CONTAINS(g.asset_ids,JSON_QUOTE(ac.id))))))`, like, like, like, like)
	}
	var rows []catalogTableRow
	limit := catalogLimit(f.Limit)
	if err := q.Order("id").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := &c.Page[c.AssetTable]{Items: []c.AssetTable{}, HasMore: len(rows) > limit}
	if out.HasMore {
		rows = rows[:limit]
		out.NextCursor = rows[limit-1].ID
	}
	for _, v := range rows {
		var t c.AssetTable
		if err := json.Unmarshal(v.Payload, &t); err != nil {
			return nil, err
		}
		t.Deprecated = v.Deprecated
		if f.Query != "" {
			t.MatchSource = "glossary"
			if strings.Contains(strings.ToLower(t.TableName+" "+t.Comment), strings.ToLower(f.Query)) {
				t.MatchSource = "metadata"
			}
		}
		out.Items = append(out.Items, t)
	}
	return out, nil
}
func (r *catalogRepository) Get(ctx context.Context, tenant int64, id string) (*c.AssetTable, error) {
	var row catalogTableRow
	if err := r.db.WithContext(ctx).Where("id=? AND tenant_id=?", id, tenant).First(&row).Error; err != nil {
		return nil, catalogError(err)
	}
	var t c.AssetTable
	if err := json.Unmarshal(row.Payload, &t); err != nil {
		return nil, err
	}
	t.Deprecated = row.Deprecated
	t.Columns = []c.AssetColumn{}
	var cols []catalogColumnRow
	if err := r.db.WithContext(ctx).Where("table_id=? AND tenant_id=?", id, tenant).Order("column_name").Find(&cols).Error; err != nil {
		return nil, err
	}
	for _, v := range cols {
		var col c.AssetColumn
		if err := json.Unmarshal(v.Payload, &col); err != nil {
			return nil, err
		}
		col.Deprecated = v.Deprecated
		t.Columns = append(t.Columns, col)
	}
	return &t, nil
}
func (r *catalogRepository) UpdateGovernance(ctx context.Context, tenant int64, id string, g c.Governance) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row catalogTableRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=?", id, tenant).First(&row).Error
		if err == nil {
			var t c.AssetTable
			if err := json.Unmarshal(row.Payload, &t); err != nil {
				return err
			}
			t.Governance = g
			return tx.Model(&catalogTableRow{}).Where("id=? AND tenant_id=?", id, tenant).Updates(map[string]any{"payload": catalogJSON(t), "tags": catalogJSON(g.Tags)}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var col catalogColumnRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=?", id, tenant).First(&col).Error; err != nil {
			return catalogError(err)
		}
		var v c.AssetColumn
		if err := json.Unmarshal(col.Payload, &v); err != nil {
			return err
		}
		v.Governance = g
		return tx.Model(&catalogColumnRow{}).Where("id=? AND tenant_id=?", id, tenant).Update("payload", catalogJSON(v)).Error
	})
}
func (r *catalogRepository) Glossaries(ctx context.Context, tenant int64, f c.Filter) (*c.Page[c.Glossary], error) {
	q := r.db.WithContext(ctx).Where("tenant_id=?", tenant)
	if f.Cursor != "" {
		q = q.Where("id>?", f.Cursor)
	}
	if f.Query != "" {
		q = q.Where("term LIKE ?", "%"+f.Query+"%")
	}
	var rows []glossaryRow
	limit := catalogLimit(f.Limit)
	if err := q.Order("id").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := &c.Page[c.Glossary]{Items: []c.Glossary{}, HasMore: len(rows) > limit}
	if out.HasMore {
		rows = rows[:limit]
		out.NextCursor = rows[limit-1].ID
	}
	for _, v := range rows {
		g := c.Glossary{ID: v.ID, TenantID: tenant, Term: v.Term, Description: v.Description}
		if err := json.Unmarshal(v.Synonyms, &g.Synonyms); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(v.AssetIDs, &g.AssetIDs); err != nil {
			return nil, err
		}
		out.Items = append(out.Items, g)
	}
	return out, nil
}
func (r *catalogRepository) SaveGlossary(ctx context.Context, tenant int64, g c.Glossary) (*c.Glossary, error) {
	g.TenantID = tenant
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, id := range g.AssetIDs {
			var count int64
			if err := tx.Model(&catalogTableRow{}).Where("id=? AND tenant_id=?", id, tenant).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				if err := tx.Model(&catalogColumnRow{}).Where("id=? AND tenant_id=?", id, tenant).Count(&count).Error; err != nil {
					return err
				}
			}
			if count == 0 {
				return c.ErrNotFound
			}
		}
		create := g.ID == ""
		if create {
			g.ID = uuid.NewString()
		} else {
			var row glossaryRow
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_id=?", g.ID, tenant).First(&row).Error; err != nil {
				return catalogError(err)
			}
		}
		row := glossaryRow{ID: g.ID, TenantID: tenant, Term: g.Term, Description: g.Description, Synonyms: catalogJSON(g.Synonyms), AssetIDs: catalogJSON(g.AssetIDs)}
		if create {
			return tx.Create(&row).Error
		}
		return tx.Model(&glossaryRow{}).Where("id=? AND tenant_id=?", g.ID, tenant).Select("term", "description", "synonyms", "asset_ids").Updates(&row).Error
	})
	return &g, err
}
func (r *catalogRepository) DeleteGlossary(ctx context.Context, tenant int64, id string) error {
	result := r.db.WithContext(ctx).Where("id=? AND tenant_id=?", id, tenant).Delete(&glossaryRow{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return c.ErrNotFound
	}
	return nil
}
func (r *catalogRepository) ResolveCoordinate(ctx context.Context, tenant int64, ds, schema, table, column string) (string, error) {
	var row catalogTableRow
	if err := r.db.WithContext(ctx).Where("tenant_id=? AND datasource_id=? AND schema_name=? AND table_name=?", tenant, ds, schema, table).First(&row).Error; err != nil {
		return "", catalogError(err)
	}
	if column == "" {
		return row.ID, nil
	}
	var col catalogColumnRow
	if err := r.db.WithContext(ctx).Where("tenant_id=? AND table_id=? AND column_name=?", tenant, row.ID, column).First(&col).Error; err != nil {
		return "", catalogError(err)
	}
	return col.ID, nil
}
