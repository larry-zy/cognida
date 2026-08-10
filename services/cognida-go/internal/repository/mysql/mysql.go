package mysql

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	_ "github.com/go-sql-driver/mysql"

	"cognida/internal/config"
)

// ========================================
// GORM 租户 Scope
// ========================================

// TenantScope 租户过滤 Scope。
// tenantID>0 时施加 tenant_id 过滤并打上"已受租户约束"标记（供 fail-closed 守卫放行）；
// tenantID==0 时不再静默放行全表——不打标记，交由 RegisterTenantGuard 决定拦截，
// 从而避免"忘了传 tenant 就查全表"的越权（多租户关闭时守卫未注册，行为不变）。
func TenantScope(tenantID int64) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if tenantID == 0 {
			return db
		}
		return markTenantScoped(db.Where("tenant_id = ?", tenantID))
	}
}

// GlobalScope 显式声明"跨租户/系统级"操作：不施加 tenant_id 过滤，但打上标记以放行守卫。
// 仅用于确实需要跨租户的合法场景（如按主键做管理级查找、后台统计）。
func GlobalScope() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return markTenantScoped(db)
	}
}

// SoftDeleteScope 软删除过滤 Scope
func SoftDeleteScope() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("deleted_at IS NULL")
	}
}

// TenantWithSoftDeleteScope 租户过滤 + 软删除过滤 Scope
func TenantWithSoftDeleteScope(tenantID int64) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		db = db.Where("deleted_at IS NULL")
		if tenantID > 0 {
			db = markTenantScoped(db.Where("tenant_id = ?", tenantID))
		}
		return db
	}
}

// ========================================
// 自定义 Logger (过滤 record not found 错误)
// ========================================

// customLogger 自定义日志记录器，过滤 "record not found" 错误
type customLogger struct {
	logger.Interface
}

// Error 重写 Error 方法，过滤 ErrRecordNotFound
func (l *customLogger) Error(ctx context.Context, msg string, opts ...interface{}) {
	// 检查是否是 ErrRecordNotFound 错误
	if len(opts) > 0 {
		if err, ok := opts[0].(error); ok && errors.Is(err, gorm.ErrRecordNotFound) {
			return // 忽略 record not found 错误
		}
	}
	l.Interface.Error(ctx, msg, opts...)
}

// ========================================
// 基础仓储 - 使用 GORM
// ========================================

// BaseRepository 基础仓储，提供通用的租户过滤和 GORM 操作
type BaseRepository struct {
	db            *gorm.DB
	tenantEnabled bool
}

// NewBaseRepository 创建基础仓储
func NewBaseRepository(db *gorm.DB, tenantEnabled bool) *BaseRepository {
	return &BaseRepository{
		db:            db,
		tenantEnabled: tenantEnabled,
	}
}

// GetDB 获取数据库连接
func (r *BaseRepository) GetDB() *gorm.DB {
	return r.db
}

// WithContext 返回带上下文的 DB。
// 若 ctx 中携带了上层开启的事务，则复用该事务，从而真正参与事务原子性。
func (r *BaseRepository) WithContext(ctx context.Context) *gorm.DB {
	return DBFromContext(ctx, r.db)
}

// WithTenantScope 返回带租户过滤的 DB
func (r *BaseRepository) WithTenantScope(ctx context.Context, tenantID int64) *gorm.DB {
	db := DBFromContext(ctx, r.db)
	if r.tenantEnabled && tenantID > 0 {
		db = db.Scopes(TenantScope(tenantID))
	}
	return db
}

// WithTenantAndSoftDeleteScope 返回带租户过滤和软删除过滤的 DB
func (r *BaseRepository) WithTenantAndSoftDeleteScope(ctx context.Context, tenantID int64) *gorm.DB {
	db := DBFromContext(ctx, r.db)
	if r.tenantEnabled {
		db = db.Scopes(TenantWithSoftDeleteScope(tenantID))
	} else {
		db = db.Scopes(SoftDeleteScope())
	}
	return db
}

// WithGlobalScope 返回声明为"跨租户/系统级"的 DB（不做租户过滤但放行 fail-closed 守卫）。
// 仅供确需跨租户的合法路径使用，等同于显式承担越权风险。
func (r *BaseRepository) WithGlobalScope(ctx context.Context) *gorm.DB {
	return DBFromContext(ctx, r.db).Scopes(GlobalScope())
}

// WithGlobalAndSoftDeleteScope 跨租户 + 软删除过滤。
func (r *BaseRepository) WithGlobalAndSoftDeleteScope(ctx context.Context) *gorm.DB {
	return DBFromContext(ctx, r.db).Scopes(GlobalScope(), SoftDeleteScope())
}

// InitGORMDatabase 初始化 GORM 连接
func InitGORMDatabase(cfg *config.DatabaseConfig, logLevel string) (*gorm.DB, error) {
	dsn := cfg.GetDSN()

	// 配置日志级别
	var gormLogLevel logger.LogLevel
	switch logLevel {
	case "silent":
		gormLogLevel = logger.Silent
	case "error":
		gormLogLevel = logger.Error
	case "warn":
		gormLogLevel = logger.Warn
	case "info":
		gormLogLevel = logger.Info
	default:
		gormLogLevel = logger.Info
	}

	// 打开数据库连接
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: &customLogger{
			Interface: logger.Default.LogMode(gormLogLevel),
		},
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true, // 使用单数表名
			NoLowerCase:   false,
		},
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("连接 GORM 数据库失败: %w", err)
	}

	// 获取底层 sql.DB
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取 sql.DB 失败: %w", err)
	}

	// 设置连接池
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 测试连接
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("GORM 数据库 ping 失败: %w", err)
	}

	log.Printf("✅ 数据库连接成功 (GORM): %s@%s:%s/%s\n", cfg.User, cfg.Host, cfg.Port, cfg.Database)

	return db, nil
}
