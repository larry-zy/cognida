package mysql

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	domain_datasource "cognida/internal/model/datasource"
)

// ========================================
// DataSourceModel 外部数据源模型
// ========================================

// DataSourceModel 外部数据源 GORM 模型。
// password_encrypted 存 AES-GCM 密文（base64），任何查询结果不得直接回给前端。
type DataSourceModel struct {
	ID                string     `gorm:"column:id;primaryKey;type:varchar(64)" json:"id"`
	TenantID          int64      `gorm:"column:tenant_id;not null;uniqueIndex:uk_ds_tenant_name" json:"tenant_id"`
	Name              string     `gorm:"column:name;not null;type:varchar(128);uniqueIndex:uk_ds_tenant_name" json:"name"`
	Description       string     `gorm:"column:description;type:varchar(512)" json:"description,omitempty"`
	Type              string     `gorm:"column:type;not null;type:varchar(32)" json:"type"`
	Host              string     `gorm:"column:host;not null;type:varchar(255)" json:"host"`
	Port              int        `gorm:"column:port;not null" json:"port"`
	DatabaseName      string     `gorm:"column:database_name;not null;type:varchar(128)" json:"database_name"`
	Username          string     `gorm:"column:username;not null;type:varchar(128)" json:"username"`
	PasswordEncrypted string     `gorm:"column:password_encrypted;not null;type:varchar(1024)" json:"-"`
	Extra             []byte     `gorm:"column:extra;type:json" json:"extra,omitempty"`
	Status            string     `gorm:"column:status;not null;type:varchar(32);default:active" json:"status"`
	LastHealthCheckAt *time.Time `gorm:"column:last_health_check_at" json:"last_health_check_at,omitempty"`
	CreatedBy         int64      `gorm:"column:created_by;not null" json:"created_by"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null" json:"updated_at"`
}

// TableName 指定表名
func (DataSourceModel) TableName() string {
	return "data_sources"
}

// toDomain 转换为领域实体
func (m *DataSourceModel) toDomain() *domain_datasource.DataSource {
	return &domain_datasource.DataSource{
		ID:                m.ID,
		TenantID:          m.TenantID,
		Name:              m.Name,
		Description:       m.Description,
		Type:              domain_datasource.Type(m.Type),
		Host:              m.Host,
		Port:              m.Port,
		DatabaseName:      m.DatabaseName,
		Username:          m.Username,
		PasswordEncrypted: m.PasswordEncrypted,
		Extra:             m.Extra,
		Status:            m.Status,
		LastHealthCheckAt: m.LastHealthCheckAt,
		CreatedBy:         m.CreatedBy,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}

// datasourceModelFromDomain 从领域实体构建模型
func datasourceModelFromDomain(d *domain_datasource.DataSource) *DataSourceModel {
	return &DataSourceModel{
		ID:                d.ID,
		TenantID:          d.TenantID,
		Name:              d.Name,
		Description:       d.Description,
		Type:              string(d.Type),
		Host:              d.Host,
		Port:              d.Port,
		DatabaseName:      d.DatabaseName,
		Username:          d.Username,
		PasswordEncrypted: d.PasswordEncrypted,
		Extra:             d.Extra,
		Status:            d.Status,
		LastHealthCheckAt: d.LastHealthCheckAt,
		CreatedBy:         d.CreatedBy,
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
	}
}

// ========================================
// dataSourceRepository 仓储实现
// ========================================

type dataSourceRepository struct {
	db *gorm.DB
}

// NewDataSourceRepository 创建数据源仓储
func NewDataSourceRepository(db *gorm.DB) domain_datasource.Repository {
	return &dataSourceRepository{db: db}
}

func (r *dataSourceRepository) Create(ctx context.Context, ds *domain_datasource.DataSource) error {
	now := time.Now()
	if ds.CreatedAt.IsZero() {
		ds.CreatedAt = now
	}
	if ds.UpdatedAt.IsZero() {
		ds.UpdatedAt = now
	}
	return r.db.WithContext(ctx).Create(datasourceModelFromDomain(ds)).Error
}

func (r *dataSourceRepository) Get(ctx context.Context, id string, tenantID int64) (*domain_datasource.DataSource, error) {
	var m DataSourceModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *dataSourceRepository) GetByName(ctx context.Context, name string, tenantID int64) (*domain_datasource.DataSource, error) {
	var m DataSourceModel
	err := r.db.WithContext(ctx).
		Where("name = ? AND tenant_id = ?", name, tenantID).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return m.toDomain(), nil
}

func (r *dataSourceRepository) List(ctx context.Context, filter domain_datasource.ListFilter) ([]*domain_datasource.DataSource, int64, error) {
	query := r.db.WithContext(ctx).Model(&DataSourceModel{}).
		Where("tenant_id = ?", filter.TenantID)
	if name := strings.TrimSpace(filter.Name); name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}
	if typ := strings.TrimSpace(filter.Type); typ != "" {
		query = query.Where("type = ?", typ)
	}
	if dbName := strings.TrimSpace(filter.DatabaseName); dbName != "" {
		query = query.Where("database_name LIKE ?", "%"+dbName+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	var models []DataSourceModel
	err := query.Order("created_at DESC").
		Limit(limit).Offset(filter.Offset).
		Find(&models).Error
	if err != nil {
		return nil, 0, err
	}

	items := make([]*domain_datasource.DataSource, 0, len(models))
	for i := range models {
		items = append(items, models[i].toDomain())
	}
	return items, total, nil
}

func (r *dataSourceRepository) ListAll(ctx context.Context) ([]*domain_datasource.DataSource, error) {
	var models []DataSourceModel
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*domain_datasource.DataSource, 0, len(models))
	for i := range models {
		items = append(items, models[i].toDomain())
	}
	return items, nil
}

func (r *dataSourceRepository) Update(ctx context.Context, ds *domain_datasource.DataSource) error {
	ds.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", ds.ID, ds.TenantID).
		Select("name", "description", "type", "host", "port", "database_name", "username",
			"password_encrypted", "extra", "status", "updated_at").
		Updates(datasourceModelFromDomain(ds)).Error
}

func (r *dataSourceRepository) UpdateStatus(ctx context.Context, id string, tenantID int64, status string, checkedAt time.Time) error {
	// 不更新 updated_at：updated_at 是连接配置的版本号（ConnectionManager 据此失效重建），
	// 健康检查不应触发连接池重建。
	return r.db.WithContext(ctx).Model(&DataSourceModel{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		UpdateColumns(map[string]interface{}{
			"status":               status,
			"last_health_check_at": checkedAt,
		}).Error
}

func (r *dataSourceRepository) Delete(ctx context.Context, id string, tenantID int64) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Delete(&DataSourceModel{}).Error
}
