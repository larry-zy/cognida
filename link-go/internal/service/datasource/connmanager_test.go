package datasource

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	model "link/internal/model/datasource"
)

// ========================================
// 测试用假 sql driver（Open 即成功，无真实连接）
// ========================================

var (
	fakeOpenCount    int64
	registerFakeOnce sync.Once
)

type fakeConn struct{}

func (fakeConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (fakeConn) Close() error                        { return nil }
func (fakeConn) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }

type fakeDrv struct{}

func (fakeDrv) Open(string) (driver.Conn, error) {
	atomic.AddInt64(&fakeOpenCount, 1)
	return fakeConn{}, nil
}

func registerFakeDriver() {
	registerFakeOnce.Do(func() { sql.Register("cm_fake", fakeDrv{}) })
}

// ========================================
// 测试用内存仓储
// ========================================

type memRepo struct {
	mu    sync.Mutex
	items map[string]*model.DataSource
}

func newMemRepo() *memRepo { return &memRepo{items: make(map[string]*model.DataSource)} }

func (r *memRepo) Create(_ context.Context, ds *model.DataSource) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *ds
	r.items[ds.ID] = &cp
	return nil
}

func (r *memRepo) Get(_ context.Context, id string, tenantID int64) (*model.DataSource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ds, ok := r.items[id]
	if !ok || ds.TenantID != tenantID {
		return nil, nil
	}
	cp := *ds
	return &cp, nil
}

func (r *memRepo) GetByName(_ context.Context, name string, tenantID int64) (*model.DataSource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ds := range r.items {
		if ds.Name == name && ds.TenantID == tenantID {
			cp := *ds
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memRepo) List(_ context.Context, filter model.ListFilter) ([]*model.DataSource, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*model.DataSource
	for _, ds := range r.items {
		if ds.TenantID == filter.TenantID {
			cp := *ds
			out = append(out, &cp)
		}
	}
	return out, int64(len(out)), nil
}

func (r *memRepo) Update(_ context.Context, ds *model.DataSource) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *ds
	r.items[ds.ID] = &cp
	return nil
}

func (r *memRepo) UpdateStatus(_ context.Context, id string, _ int64, status string, checkedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ds, ok := r.items[id]; ok {
		ds.Status = status
		ds.LastHealthCheckAt = &checkedAt
	}
	return nil
}

func (r *memRepo) Delete(_ context.Context, id string, _ int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, id)
	return nil
}

// ========================================
// 测试脚手架
// ========================================

func newTestManager(t *testing.T, repo model.Repository) *ConnectionManager {
	t.Helper()
	registerFakeDriver()
	cipher, err := NewCipher("cm-test-secret")
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	m := NewConnectionManager(repo, cipher, DefaultPoolOptions())
	m.openFunc = func(_, dsn string) (*sql.DB, error) { return sql.Open("cm_fake", dsn) }
	m.skipPing = true
	return m
}

func seedDataSource(t *testing.T, repo model.Repository, cipher *Cipher, id string) *model.DataSource {
	t.Helper()
	enc, err := cipher.Encrypt("secret-pass")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	ds := &model.DataSource{
		ID: id, TenantID: 1, Name: "ds-" + id, Type: model.TypeMySQL,
		Host: "127.0.0.1", Port: 3306, DatabaseName: "demo", Username: "u",
		PasswordEncrypted: enc, Status: model.StatusActive,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := repo.Create(context.Background(), ds); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return ds
}

// ========================================
// 用例
// ========================================

func TestAcquireLazyLoadAndReuse(t *testing.T) {
	repo := newMemRepo()
	m := newTestManager(t, repo)
	seedDataSource(t, repo, m.cipher, "ds1")

	before := atomic.LoadInt64(&fakeOpenCount)
	db1, ds, err := m.Acquire(context.Background(), 1, "ds1")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if ds.Name != "ds-ds1" {
		t.Fatalf("元数据不符: %+v", ds)
	}
	db2, _, err := m.Acquire(context.Background(), 1, "ds1")
	if err != nil {
		t.Fatalf("Acquire#2: %v", err)
	}
	if db1 != db2 {
		t.Fatalf("同版本应复用同一连接池")
	}
	// 两次 Acquire 只 open 一次（sql.Open 本身不建连接，此处数 driver.Open 不适用；数池对象即可）
	if m.poolCount() != 1 {
		t.Fatalf("poolCount = %d, want 1", m.poolCount())
	}
	_ = before
}

func TestAcquireVersionInvalidation(t *testing.T) {
	repo := newMemRepo()
	m := newTestManager(t, repo)
	ds := seedDataSource(t, repo, m.cipher, "ds1")

	db1, _, err := m.Acquire(context.Background(), 1, "ds1")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// 模拟配置更新：updated_at 前进
	ds.UpdatedAt = ds.UpdatedAt.Add(time.Second)
	if err := repo.Update(context.Background(), ds); err != nil {
		t.Fatalf("Update: %v", err)
	}

	db2, _, err := m.Acquire(context.Background(), 1, "ds1")
	if err != nil {
		t.Fatalf("Acquire after update: %v", err)
	}
	if db1 == db2 {
		t.Fatalf("updated_at 变更后应重建连接池")
	}
	if m.poolCount() != 1 {
		t.Fatalf("旧池应被替换, poolCount = %d", m.poolCount())
	}
}

func TestInvalidateClosesPool(t *testing.T) {
	repo := newMemRepo()
	m := newTestManager(t, repo)
	seedDataSource(t, repo, m.cipher, "ds1")

	if _, _, err := m.Acquire(context.Background(), 1, "ds1"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	m.Invalidate(1, "ds1")
	if m.poolCount() != 0 {
		t.Fatalf("Invalidate 后 poolCount = %d, want 0", m.poolCount())
	}
}

func TestIdleEviction(t *testing.T) {
	repo := newMemRepo()
	m := newTestManager(t, repo)
	seedDataSource(t, repo, m.cipher, "ds1")
	seedDataSource(t, repo, m.cipher, "ds2")

	now := time.Now()
	m.nowFunc = func() time.Time { return now }

	if _, _, err := m.Acquire(context.Background(), 1, "ds1"); err != nil {
		t.Fatalf("Acquire ds1: %v", err)
	}

	// 时间推进超过 IdleTTL 后触达另一个数据源，应回收 ds1 的池
	now = now.Add(m.opts.IdleTTL + time.Minute)
	if _, _, err := m.Acquire(context.Background(), 1, "ds2"); err != nil {
		t.Fatalf("Acquire ds2: %v", err)
	}
	if m.poolCount() != 1 {
		t.Fatalf("空闲池应被回收, poolCount = %d, want 1", m.poolCount())
	}
}

func TestAcquireNotFoundNoFallback(t *testing.T) {
	repo := newMemRepo()
	m := newTestManager(t, repo)

	db, _, err := m.Acquire(context.Background(), 1, "missing")
	if err == nil || db != nil {
		t.Fatalf("不存在的数据源必须报错且不回落, db=%v err=%v", db, err)
	}
	if !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("错误信息应指明不存在: %v", err)
	}
}

func TestAcquireTenantIsolation(t *testing.T) {
	repo := newMemRepo()
	m := newTestManager(t, repo)
	seedDataSource(t, repo, m.cipher, "ds1") // tenant 1

	if _, _, err := m.Acquire(context.Background(), 2, "ds1"); err == nil {
		t.Fatalf("跨租户 Acquire 必须失败")
	}
}

func TestAcquireDecryptFailureMarksNeedCredentials(t *testing.T) {
	repo := newMemRepo()
	m := newTestManager(t, repo)
	ds := seedDataSource(t, repo, m.cipher, "ds1")

	// 用另一把密钥的密文模拟密钥变更
	other, _ := NewCipher("another-secret")
	enc, _ := other.Encrypt("secret-pass")
	ds.PasswordEncrypted = enc
	_ = repo.Update(context.Background(), ds)

	if _, _, err := m.Acquire(context.Background(), 1, "ds1"); !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("解密失败应返回 ErrDecryptFailed, got %v", err)
	}
	got, _ := repo.Get(context.Background(), "ds1", 1)
	if got.Status != model.StatusNeedCredentials {
		t.Fatalf("解密失败应标记 need_credentials, got %q", got.Status)
	}
	// 标记后再次 Acquire 直接拒绝
	if _, _, err := m.Acquire(context.Background(), 1, "ds1"); err == nil {
		t.Fatalf("need_credentials 状态应拒绝 Acquire")
	}
}

func TestSanitizeErr(t *testing.T) {
	err := errors.New(`dial failed for dsn "u:my-pass@tcp(1.2.3.4:3306)/db"`)
	got := sanitizeErr(err, "my-pass").Error()
	if strings.Contains(got, "my-pass") {
		t.Fatalf("脱敏后不应包含密码: %s", got)
	}
	if !strings.Contains(got, "***") {
		t.Fatalf("密码应被替换为 ***: %s", got)
	}
}
