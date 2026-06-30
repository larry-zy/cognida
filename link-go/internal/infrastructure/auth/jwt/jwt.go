package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ========================================
// Claims 声明
// ========================================

// Claims JWT 声明
type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username,omitempty"`
	TenantID int64  `json:"tenant_id,omitempty"`
	jwt.RegisteredClaims
}

// ========================================
// 配置
// ========================================

// Config JWT 配置
type Config struct {
	Secret     string        // 密钥
	ExpireTime time.Duration // 过期时间
	Issuer     string        // 签发者
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Secret:     "your-secret-key-change-me",
		ExpireTime: 24 * time.Hour * 7, // 7 天
		Issuer:     "link",
	}
}

// ========================================
// JWT 管理器
// ========================================

// JWT JWT 管理器
type JWT struct {
	config *Config
}

// New 创建 JWT 管理器
func New(config *Config) *JWT {
	if config == nil {
		config = DefaultConfig()
	}
	return &JWT{
		config: config,
	}
}

// NewWithSecret 使用密钥创建 JWT 管理器
func NewWithSecret(secret string) *JWT {
	config := DefaultConfig()
	config.Secret = secret
	return New(config)
}

// GenerateToken 生成 Token
func (j *JWT) GenerateToken(userID int64, username string, tenantID int64) (string, error) {
	now := time.Now()
	expireTime := now.Add(j.config.ExpireTime)

	claims := Claims{
		UserID:   userID,
		Username: username,
		TenantID: tenantID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireTime),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    j.config.Issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(j.config.Secret))
}

// ParseToken 解析 Token
func (j *JWT) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(j.config.Secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// RefreshToken 刷新 Token
func (j *JWT) RefreshToken(tokenString string) (string, error) {
	claims, err := j.ParseToken(tokenString)
	if err != nil {
		return "", err
	}

	return j.GenerateToken(claims.UserID, claims.Username, claims.TenantID)
}

// ========================================
// 全局默认实例
// ========================================

var defaultJWT *JWT

// Init 初始化默认 JWT 实例
func Init(secret string, expireTime time.Duration) {
	defaultJWT = &JWT{
		config: &Config{
			Secret:     secret,
			ExpireTime: expireTime,
			Issuer:     "link",
		},
	}
}

// GenerateToken 生成 Token（使用默认实例）
func GenerateToken(userID int64, username string, tenantID int64) (string, error) {
	if defaultJWT == nil {
		defaultJWT = New(DefaultConfig())
	}
	return defaultJWT.GenerateToken(userID, username, tenantID)
}

// ParseToken 解析 Token（使用默认实例）
func ParseToken(tokenString string) (*Claims, error) {
	if defaultJWT == nil {
		defaultJWT = New(DefaultConfig())
	}
	return defaultJWT.ParseToken(tokenString)
}

// RefreshToken 刷新 Token（使用默认实例）
func RefreshToken(tokenString string) (string, error) {
	if defaultJWT == nil {
		defaultJWT = New(DefaultConfig())
	}
	return defaultJWT.RefreshToken(tokenString)
}

// ========================================
// 快捷方法
// ========================================

// Generate 生成 Token
func (j *JWT) Generate(userID int64) (string, error) {
	return j.GenerateToken(userID, "", 0)
}

// Parse 解析 Token
func (j *JWT) Parse(tokenString string) (int64, error) {
	claims, err := j.ParseToken(tokenString)
	if err != nil {
		return 0, err
	}
	return claims.UserID, nil
}

// GetUserID 从 Token 获取用户 ID
func GetUserID(tokenString string) (int64, error) {
	if defaultJWT == nil {
		defaultJWT = New(DefaultConfig())
	}
	return defaultJWT.Parse(tokenString)
}

// GetTenantID 从 Token 获取租户 ID
func GetTenantID(tokenString string) (int64, error) {
	claims, err := ParseToken(tokenString)
	if err != nil {
		return 0, err
	}
	return claims.TenantID, nil
}
