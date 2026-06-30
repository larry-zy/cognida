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

	"link/internal/infrastructure/config"
)

var (
	// DB 数据库连接（全局单例）
	DB *gorm.DB
)

// ========================================
// GORM 租户 Scope
// ========================================

// TenantScope 租户过滤 Scope
func TenantScope(tenantID int64) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if tenantID == 0 {
			return db
		}
		return db.Where("tenant_id = ?", tenantID)
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
			db = db.Where("tenant_id = ?", tenantID)
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

// WithContext 返回带上下文的 DB
func (r *BaseRepository) WithContext(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// WithTenantScope 返回带租户过滤的 DB
func (r *BaseRepository) WithTenantScope(ctx context.Context, tenantID int64) *gorm.DB {
	db := r.db.WithContext(ctx)
	if r.tenantEnabled && tenantID > 0 {
		db = db.Scopes(TenantScope(tenantID))
	}
	return db
}

// WithTenantAndSoftDeleteScope 返回带租户过滤和软删除过滤的 DB
func (r *BaseRepository) WithTenantAndSoftDeleteScope(ctx context.Context, tenantID int64) *gorm.DB {
	db := r.db.WithContext(ctx)
	if r.tenantEnabled {
		db = db.Scopes(TenantWithSoftDeleteScope(tenantID))
	} else {
		db = db.Scopes(SoftDeleteScope())
	}
	return db
}

// GetTenantID 从上下文获取租户ID
func (r *BaseRepository) GetTenantID(ctx context.Context) int64 {
	if tenantID, ok := ctx.Value("tenant_id").(int64); ok {
		return tenantID
	}
	return 0
}

// MustGetTenantID 必须获取租户ID，否则panic
func (r *BaseRepository) MustGetTenantID(ctx context.Context) int64 {
	tenantID := r.GetTenantID(ctx)
	if tenantID == 0 && r.tenantEnabled {
		panic("tenant_id is required but not set in context")
	}
	return tenantID
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

	DB = db
	log.Printf("✅ 数据库连接成功 (GORM): %s@%s:%s/%s\n", cfg.User, cfg.Host, cfg.Port, cfg.Database)

	return db, nil
}

// CloseDatabase 关闭数据库连接
func CloseDatabase() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err == nil {
			return sqlDB.Close()
		}
	}
	return nil
}

// GetDB 获取 GORM 数据库连接
func GetDB() *gorm.DB {
	return DB
}
