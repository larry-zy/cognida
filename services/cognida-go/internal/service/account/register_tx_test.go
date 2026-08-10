package account

import (
	"context"
	"errors"
	"testing"

	"cognida/internal/config"
	"cognida/internal/model/user"
)

// txCtxKey 测试用哨兵：fakeTxManager 把它写进 ctx，各 fake 仓储据此断言
// 自己确实在事务作用域内被调用（即 Register 的写入都跑在同一事务里）。
type txCtxKey struct{}

// fakeTxManager 记录事务被开启的次数，并透传 fn 的返回值：
// fn 返回非 nil 即代表真实事务会回滚。
type fakeTxManager struct {
	calls   int
	fnError error // 捕获 fn 的返回值，用于断言「失败即回滚」
}

func (m *fakeTxManager) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	m.calls++
	err := fn(context.WithValue(ctx, txCtxKey{}, true))
	m.fnError = err
	return err
}

// inTx 断言 ctx 携带事务哨兵。
func inTx(ctx context.Context) bool {
	v, _ := ctx.Value(txCtxKey{}).(bool)
	return v
}

// fakeUserRepo 仅覆盖 Register 用到的方法，其余经内嵌接口继承（未实现，调用即 panic）。
type fakeUserRepo struct {
	user.UserRepository
	t         *testing.T
	createErr error
	createHit bool
}

func (f *fakeUserRepo) FindByEmail(ctx context.Context, tenantID int64, email string) (*user.User, error) {
	if !inTx(ctx) {
		f.t.Error("FindByEmail 未在事务作用域内执行")
	}
	return nil, errors.New("用户不存在")
}

func (f *fakeUserRepo) FindByUsername(ctx context.Context, tenantID int64, username string) (*user.User, error) {
	if !inTx(ctx) {
		f.t.Error("FindByUsername 未在事务作用域内执行")
	}
	return nil, errors.New("用户不存在")
}

func (f *fakeUserRepo) Create(ctx context.Context, u *user.User) error {
	if !inTx(ctx) {
		f.t.Error("userRepo.Create 未在事务作用域内执行")
	}
	f.createHit = true
	if f.createErr != nil {
		return f.createErr
	}
	u.ID = 1
	return nil
}

// fakeRefreshRepo 仅覆盖 Create。
type fakeRefreshRepo struct {
	user.RefreshTokenRepository
	t         *testing.T
	createErr error
	createHit bool
}

func (f *fakeRefreshRepo) Create(ctx context.Context, token *user.RefreshToken) error {
	if !inTx(ctx) {
		f.t.Error("refreshTokenRepo.Create 未在事务作用域内执行")
	}
	f.createHit = true
	return f.createErr
}

// fakeTenantAdapter 直接返回固定租户 ID。
type fakeTenantAdapter struct {
	t         *testing.T
	createHit bool
}

func (f *fakeTenantAdapter) CreateTenant(ctx context.Context, req interface{}) (*TenantInfo, error) {
	if !inTx(ctx) {
		f.t.Error("createDefaultTenant 未在事务作用域内执行")
	}
	f.createHit = true
	return &TenantInfo{ID: 42}, nil
}

func newRegisterService(t *testing.T, tm *fakeTxManager, userRepo *fakeUserRepo, refreshRepo *fakeRefreshRepo, tenant *fakeTenantAdapter) *authService {
	return &authService{
		userRepo:         userRepo,
		refreshTokenRepo: refreshRepo,
		jwtConfig:        &config.JWTConfig{Secret: testSecret, RefreshTokenExpire: 3600, AccessTokenExpire: 900},
		tenantService:    tenant,
		txManager:        tm,
	}
}

// TestRegister_WrapsWritesInSingleTransaction 验证注册全过程（建租户→建用户→存令牌）
// 收敛在同一事务内：事务恰好开启一次，且每次仓储写入都携带事务上下文。
func TestRegister_WrapsWritesInSingleTransaction(t *testing.T) {
	tm := &fakeTxManager{}
	userRepo := &fakeUserRepo{t: t}
	refreshRepo := &fakeRefreshRepo{t: t}
	tenant := &fakeTenantAdapter{t: t}
	s := newRegisterService(t, tm, userRepo, refreshRepo, tenant)

	resp, err := s.Register(context.Background(), &RegisterRequest{
		Username: "alice",
		Email:    "alice@link.dev",
		Password: "pass1234",
	})
	if err != nil {
		t.Fatalf("Register 不应报错: %v", err)
	}
	if resp == nil || resp.AccessToken == "" {
		t.Fatal("Register 应返回带令牌的响应")
	}
	if tm.calls != 1 {
		t.Errorf("事务应恰好开启一次, got %d", tm.calls)
	}
	if tm.fnError != nil {
		t.Errorf("成功路径事务函数应返回 nil（提交）, got %v", tm.fnError)
	}
	if !tenant.createHit || !userRepo.createHit || !refreshRepo.createHit {
		t.Error("租户/用户/令牌三处写入都应被调用")
	}
}

// TestRegister_RollsBackOnRefreshTokenFailure 验证末步（存刷新令牌）失败时，
// Register 返回错误且事务函数返回非 nil——真实事务据此回滚，不留半写脏数据。
func TestRegister_RollsBackOnRefreshTokenFailure(t *testing.T) {
	tm := &fakeTxManager{}
	userRepo := &fakeUserRepo{t: t}
	refreshRepo := &fakeRefreshRepo{t: t, createErr: errors.New("db down")}
	tenant := &fakeTenantAdapter{t: t}
	s := newRegisterService(t, tm, userRepo, refreshRepo, tenant)

	_, err := s.Register(context.Background(), &RegisterRequest{
		Username: "bob",
		Email:    "bob@link.dev",
		Password: "pass1234",
	})
	if err == nil {
		t.Fatal("末步失败时 Register 应返回错误")
	}
	if tm.calls != 1 {
		t.Errorf("事务应恰好开启一次, got %d", tm.calls)
	}
	if tm.fnError == nil {
		t.Error("失败路径事务函数应返回非 nil（触发回滚）")
	}
	if !userRepo.createHit {
		t.Error("用户写入应已发生（随后应随事务回滚）")
	}
}
