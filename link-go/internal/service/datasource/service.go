// Package datasource 数据源应用服务：外部数据库注册表 CRUD、测试连接、schema 探查、
// 受管连接池（ConnectionManager）。密码只以 AES-GCM 密文落库，任何响应不含密码。
package datasource

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	model "link/internal/model/datasource"
)

// IDGenerator 生成数据源 ID（与 infrastructure/id.IDGenerator 同构，便于复用注入）
type IDGenerator interface {
	Generate() string
}

// ErrNameConflict 数据源名称在租户内冲突
var ErrNameConflict = errors.New("datasource: 数据源名称已存在")

// ErrNotFound 数据源不存在
var ErrNotFound = errors.New("datasource: 数据源不存在")

// Service 数据源应用服务
type Service struct {
	repo   model.Repository
	cipher *Cipher
	cm     *ConnectionManager
	idGen  IDGenerator
	// testTimeout 测试连接超时
	testTimeout time.Duration
}

// NewService 创建数据源服务
func NewService(repo model.Repository, cipher *Cipher, cm *ConnectionManager, idGen IDGenerator) *Service {
	return &Service{repo: repo, cipher: cipher, cm: cm, idGen: idGen, testTimeout: 5 * time.Second}
}

// ConnectionManager 暴露受管连接（供 agent 工具经 ConnectionProvider 消费）
func (s *Service) ConnectionManager() *ConnectionManager { return s.cm }

// ========================================
// 输入 / 输出 DTO
// ========================================

// SaveInput 创建/更新数据源输入。更新时 Password 为空表示保留原密码。
type SaveInput struct {
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	Host         string          `json:"host"`
	Port         int             `json:"port"`
	DatabaseName string          `json:"database_name"`
	Username     string          `json:"username"`
	Password     string          `json:"password"`
	Extra        json.RawMessage `json:"extra,omitempty"`
}

// Sanitized 对外安全视图：不含密码（明文与密文均不含）
type Sanitized struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Type              string     `json:"type"`
	Host              string     `json:"host"`
	Port              int        `json:"port"`
	DatabaseName      string     `json:"database_name"`
	Username          string     `json:"username"`
	Status            string     `json:"status"`
	LastHealthCheckAt *time.Time `json:"last_health_check_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func sanitize(ds *model.DataSource) *Sanitized {
	return &Sanitized{
		ID:                ds.ID,
		Name:              ds.Name,
		Type:              string(ds.Type),
		Host:              ds.Host,
		Port:              ds.Port,
		DatabaseName:      ds.DatabaseName,
		Username:          ds.Username,
		Status:            ds.Status,
		LastHealthCheckAt: ds.LastHealthCheckAt,
		CreatedAt:         ds.CreatedAt,
		UpdatedAt:         ds.UpdatedAt,
	}
}

func (in *SaveInput) validate(isCreate bool) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("datasource: name 不能为空")
	}
	if _, err := DriverFor(model.Type(in.Type)); err != nil {
		return err
	}
	if strings.TrimSpace(in.Host) == "" {
		return errors.New("datasource: host 不能为空")
	}
	if in.Port <= 0 || in.Port > 65535 {
		return errors.New("datasource: port 非法")
	}
	if strings.TrimSpace(in.DatabaseName) == "" {
		return errors.New("datasource: database_name 不能为空")
	}
	if strings.TrimSpace(in.Username) == "" {
		return errors.New("datasource: username 不能为空")
	}
	if isCreate && in.Password == "" {
		return errors.New("datasource: 创建数据源必须提供密码")
	}
	return nil
}

// ========================================
// CRUD
// ========================================

// Create 注册数据源（密码加密落库）
func (s *Service) Create(ctx context.Context, tenantID, userID int64, in *SaveInput) (*Sanitized, error) {
	if err := in.validate(true); err != nil {
		return nil, err
	}
	if existing, err := s.repo.GetByName(ctx, in.Name, tenantID); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, ErrNameConflict
	}
	enc, err := s.cipher.Encrypt(in.Password)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	ds := &model.DataSource{
		ID:                s.idGen.Generate(),
		TenantID:          tenantID,
		Name:              in.Name,
		Type:              model.Type(in.Type),
		Host:              in.Host,
		Port:              in.Port,
		DatabaseName:      in.DatabaseName,
		Username:          in.Username,
		PasswordEncrypted: enc,
		Extra:             in.Extra,
		Status:            model.StatusActive,
		CreatedBy:         userID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.repo.Create(ctx, ds); err != nil {
		return nil, err
	}
	return sanitize(ds), nil
}

// Update 更新数据源；Password 为空保留原密码。任何字段变更都会推进 updated_at，
// ConnectionManager 据此失效重建连接池。
func (s *Service) Update(ctx context.Context, tenantID int64, id string, in *SaveInput) (*Sanitized, error) {
	if err := in.validate(false); err != nil {
		return nil, err
	}
	ds, err := s.repo.Get(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if ds == nil {
		return nil, ErrNotFound
	}
	if in.Name != ds.Name {
		if existing, err := s.repo.GetByName(ctx, in.Name, tenantID); err != nil {
			return nil, err
		} else if existing != nil && existing.ID != id {
			return nil, ErrNameConflict
		}
	}
	ds.Name = in.Name
	ds.Type = model.Type(in.Type)
	ds.Host = in.Host
	ds.Port = in.Port
	ds.DatabaseName = in.DatabaseName
	ds.Username = in.Username
	ds.Extra = in.Extra
	if in.Password != "" {
		enc, err := s.cipher.Encrypt(in.Password)
		if err != nil {
			return nil, err
		}
		ds.PasswordEncrypted = enc
		// 重录密码即恢复可用状态
		ds.Status = model.StatusActive
	}
	if err := s.repo.Update(ctx, ds); err != nil {
		return nil, err
	}
	s.cm.Invalidate(tenantID, id)
	return sanitize(ds), nil
}

// Delete 删除数据源并联动关闭其连接池
func (s *Service) Delete(ctx context.Context, tenantID int64, id string) error {
	ds, err := s.repo.Get(ctx, id, tenantID)
	if err != nil {
		return err
	}
	if ds == nil {
		return ErrNotFound
	}
	if err := s.repo.Delete(ctx, id, tenantID); err != nil {
		return err
	}
	s.cm.Invalidate(tenantID, id)
	return nil
}

// Get 数据源详情（安全视图）
func (s *Service) Get(ctx context.Context, tenantID int64, id string) (*Sanitized, error) {
	ds, err := s.repo.Get(ctx, id, tenantID)
	if err != nil {
		return nil, err
	}
	if ds == nil {
		return nil, ErrNotFound
	}
	return sanitize(ds), nil
}

// List 数据源列表（安全视图）
func (s *Service) List(ctx context.Context, filter model.ListFilter) ([]*Sanitized, int64, error) {
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*Sanitized, 0, len(items))
	for _, ds := range items {
		out = append(out, sanitize(ds))
	}
	return out, total, nil
}

// ========================================
// 测试连接
// ========================================

// TestInput 测试连接输入：优先用表单配置；DatasourceID 非空且 Password 为空时用已存凭证。
type TestInput struct {
	DatasourceID string `json:"datasource_id,omitempty"`
	SaveInput
}

// TestConnection 用临时连接测试连通性（不进缓存），错误已脱敏。
func (s *Service) TestConnection(ctx context.Context, tenantID int64, in *TestInput) error {
	cfg := &model.DataSource{
		Name:         in.Name,
		Type:         model.Type(in.Type),
		Host:         in.Host,
		Port:         in.Port,
		DatabaseName: in.DatabaseName,
		Username:     in.Username,
	}
	password := in.Password

	// 编辑场景：密码留空 → 用已存数据源的凭证补齐（其余字段以表单为准）
	if password == "" && in.DatasourceID != "" {
		stored, err := s.repo.Get(ctx, in.DatasourceID, tenantID)
		if err != nil {
			return err
		}
		if stored == nil {
			return ErrNotFound
		}
		password, err = s.cipher.Decrypt(stored.PasswordEncrypted)
		if err != nil {
			return err
		}
		if cfg.Host == "" { // 无表单时整体回退已存配置
			cfg = stored
		}
	}
	if password == "" {
		return errors.New("datasource: 测试连接需要密码")
	}

	drv, err := DriverFor(cfg.Type)
	if err != nil {
		return err
	}
	dsn, err := drv.BuildDSN(cfg, password)
	if err != nil {
		return err
	}
	db, err := sql.Open(drv.DriverName(), dsn)
	if err != nil {
		return sanitizeErr(err, password)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	pingCtx, cancel := context.WithTimeout(ctx, s.testTimeout)
	defer cancel()
	if err := drv.Ping(pingCtx, db); err != nil {
		return fmt.Errorf("连接失败: %w", sanitizeErr(err, password))
	}
	return nil
}

// ========================================
// schema 探查
// ========================================

// ListTables 外部数据源表清单（经受管连接池）
func (s *Service) ListTables(ctx context.Context, tenantID int64, id string) ([]TableInfo, error) {
	db, ds, err := s.cm.Acquire(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	drv, err := DriverFor(ds.Type)
	if err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return drv.ListTables(queryCtx, db, ds.DatabaseName)
}

// DescribeTable 外部数据源单表结构
func (s *Service) DescribeTable(ctx context.Context, tenantID int64, id, table string) (*TableSchema, error) {
	db, ds, err := s.cm.Acquire(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	drv, err := DriverFor(ds.Type)
	if err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return drv.DescribeTable(queryCtx, db, ds.DatabaseName, table)
}

// Acquire 实现 model.ConnectionProvider（转发给 ConnectionManager），
// 供 agent 工具按 database_id 路由查询。
func (s *Service) Acquire(ctx context.Context, tenantID int64, datasourceID string) (*sql.DB, *model.DataSource, error) {
	return s.cm.Acquire(ctx, tenantID, datasourceID)
}
