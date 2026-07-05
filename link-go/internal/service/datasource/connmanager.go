package datasource

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	model "link/internal/model/datasource"
)

// PoolOptions 外部数据源连接池参数。外部库不是自家资源，默认取保守值。
type PoolOptions struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
	// IdleTTL 整个池子多久未被 Acquire 后回收（关池释放句柄）
	IdleTTL time.Duration
	// PingTimeout 新建池后的连通性检查超时
	PingTimeout time.Duration
}

// DefaultPoolOptions 默认保守池参数
func DefaultPoolOptions() PoolOptions {
	return PoolOptions{
		MaxOpenConns:    4,
		MaxIdleConns:    2,
		ConnMaxIdleTime: 5 * time.Minute,
		ConnMaxLifetime: 30 * time.Minute,
		IdleTTL:         10 * time.Minute,
		PingTimeout:     5 * time.Second,
	}
}

type poolEntry struct {
	db       *sql.DB
	version  time.Time // 数据源 updated_at：配置变更后失效重建
	lastUsed time.Time
}

// ConnectionManager 外部数据源连接管理器：datasource_id → *sql.DB 懒加载缓存。
// 实现 model.ConnectionProvider。
type ConnectionManager struct {
	repo   model.Repository
	cipher *Cipher
	opts   PoolOptions

	mu    sync.Mutex
	pools map[string]*poolEntry // key: tenantID:datasourceID

	// openFunc 可注入以便单元测试不建真实连接；生产默认 sql.Open
	openFunc func(driverName, dsn string) (*sql.DB, error)
	// nowFunc 可注入以便测试空闲回收
	nowFunc func() time.Time
	// skipPing 测试用：跳过新建池后的 Ping
	skipPing bool
}

// NewConnectionManager 创建连接管理器
func NewConnectionManager(repo model.Repository, cipher *Cipher, opts PoolOptions) *ConnectionManager {
	return &ConnectionManager{
		repo:     repo,
		cipher:   cipher,
		opts:     opts,
		pools:    make(map[string]*poolEntry),
		openFunc: sql.Open,
		nowFunc:  time.Now,
	}
}

func poolKey(tenantID int64, datasourceID string) string {
	return fmt.Sprintf("%d:%s", tenantID, datasourceID)
}

// Acquire 返回数据源连接池与元数据（实现 model.ConnectionProvider）。
// 每次调用都会读一次注册表：既做存在性/租户校验，也拿 updated_at 做版本比对。
func (m *ConnectionManager) Acquire(ctx context.Context, tenantID int64, datasourceID string) (*sql.DB, *model.DataSource, error) {
	ds, err := m.repo.Get(ctx, datasourceID, tenantID)
	if err != nil {
		return nil, nil, fmt.Errorf("datasource: 查询数据源失败: %w", err)
	}
	if ds == nil {
		return nil, nil, fmt.Errorf("datasource: 数据源 %q 不存在", datasourceID)
	}
	if ds.Status == model.StatusNeedCredentials {
		return nil, nil, fmt.Errorf("datasource: 数据源 %q 凭证失效，请重新录入密码", ds.Name)
	}

	key := poolKey(tenantID, datasourceID)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.evictIdleLocked()

	if e, ok := m.pools[key]; ok {
		if e.version.Equal(ds.UpdatedAt) {
			e.lastUsed = m.nowFunc()
			return e.db, ds, nil
		}
		// 配置已变更：关旧池重建
		_ = e.db.Close()
		delete(m.pools, key)
	}

	db, err := m.openLocked(ctx, ds)
	if err != nil {
		return nil, nil, err
	}
	m.pools[key] = &poolEntry{db: db, version: ds.UpdatedAt, lastUsed: m.nowFunc()}
	return db, ds, nil
}

// openLocked 解密凭证、构建 DSN、开池并（可选）Ping。调用方须持锁。
func (m *ConnectionManager) openLocked(ctx context.Context, ds *model.DataSource) (*sql.DB, error) {
	drv, err := DriverFor(ds.Type)
	if err != nil {
		return nil, err
	}
	password, err := m.cipher.Decrypt(ds.PasswordEncrypted)
	if err != nil {
		// 解密失败标记需重录（尽力而为，不覆盖主错误）
		_ = m.repo.UpdateStatus(ctx, ds.ID, ds.TenantID, model.StatusNeedCredentials, m.nowFunc())
		return nil, err
	}
	dsn, err := drv.BuildDSN(ds, password)
	if err != nil {
		return nil, err
	}
	db, err := m.openFunc(drv.DriverName(), dsn)
	if err != nil {
		return nil, fmt.Errorf("datasource: 打开连接失败: %w", sanitizeErr(err, password))
	}
	db.SetMaxOpenConns(m.opts.MaxOpenConns)
	db.SetMaxIdleConns(m.opts.MaxIdleConns)
	db.SetConnMaxIdleTime(m.opts.ConnMaxIdleTime)
	db.SetConnMaxLifetime(m.opts.ConnMaxLifetime)

	if !m.skipPing {
		pingCtx, cancel := context.WithTimeout(ctx, m.opts.PingTimeout)
		defer cancel()
		if err := drv.Ping(pingCtx, db); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("datasource: 连接 %q 失败: %w", ds.Name, sanitizeErr(err, password))
		}
	}
	return db, nil
}

// Invalidate 使指定数据源的缓存池失效（更新/删除数据源时联动调用）
func (m *ConnectionManager) Invalidate(tenantID int64, datasourceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := poolKey(tenantID, datasourceID)
	if e, ok := m.pools[key]; ok {
		_ = e.db.Close()
		delete(m.pools, key)
	}
}

// Close 关闭全部缓存池（进程退出时调用）
func (m *ConnectionManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, e := range m.pools {
		_ = e.db.Close()
		delete(m.pools, key)
	}
}

// evictIdleLocked 回收超过 IdleTTL 未使用的池。调用方须持锁。
func (m *ConnectionManager) evictIdleLocked() {
	if m.opts.IdleTTL <= 0 {
		return
	}
	cutoff := m.nowFunc().Add(-m.opts.IdleTTL)
	for key, e := range m.pools {
		if e.lastUsed.Before(cutoff) {
			_ = e.db.Close()
			delete(m.pools, key)
		}
	}
}

// poolCount 测试用：当前缓存池数量
func (m *ConnectionManager) poolCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pools)
}
