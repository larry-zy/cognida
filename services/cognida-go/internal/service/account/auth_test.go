package account

import (
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"cognida/internal/config"
)

// testSecret 32 字节非占位密钥，满足 JWTConfig 校验下限。
var testSecret = strings.Repeat("x9", 16)

// signToken 用给定声明签发 HS256 token，供 ValidateToken 用例复用。
func signToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("签发 token 失败: %v", err)
	}
	return s
}

func newTestAuthService() *authService {
	return &authService{jwtConfig: &config.JWTConfig{Secret: testSecret}}
}

// TestValidateToken_MissingUsername 缺 username 的合法 token 不得 panic，
// 应正常返回并把 Username 留空——这是此前 500 崩溃（裸类型断言）的回归防护。
func TestValidateToken_MissingUsername(t *testing.T) {
	s := newTestAuthService()
	token := signToken(t, jwt.MapClaims{
		"user_id":    float64(1),
		"tenant_id":  float64(1),
		"token_type": "access",
	})

	claims, err := s.ValidateToken(token)
	if err != nil {
		t.Fatalf("缺 username 的合法 token 不应报错: %v", err)
	}
	if claims.UserID != 1 {
		t.Errorf("UserID = %d, 期望 1", claims.UserID)
	}
	if claims.Username != "" {
		t.Errorf("Username = %q, 期望空串", claims.Username)
	}
	if claims.TenantID != 1 {
		t.Errorf("TenantID = %d, 期望 1", claims.TenantID)
	}
}

// TestValidateToken_MissingUserID 缺 user_id（身份主键）应返回错误而非 panic，
// 上游中间件据此回 401。
func TestValidateToken_MissingUserID(t *testing.T) {
	s := newTestAuthService()
	token := signToken(t, jwt.MapClaims{"username": "dev"})

	if _, err := s.ValidateToken(token); err == nil {
		t.Fatal("缺 user_id 应返回错误")
	}
}

// TestValidateToken_WrongClaimType 声明类型不符（如 username 为数字）不得 panic，
// 该字段按缺省处理，核心身份仍可解析。
func TestValidateToken_WrongClaimType(t *testing.T) {
	s := newTestAuthService()
	token := signToken(t, jwt.MapClaims{
		"user_id":  float64(7),
		"username": float64(123), // 类型错误
		"email":    true,         // 类型错误
	})

	claims, err := s.ValidateToken(token)
	if err != nil {
		t.Fatalf("类型不符不应报错: %v", err)
	}
	if claims.UserID != 7 {
		t.Errorf("UserID = %d, 期望 7", claims.UserID)
	}
	if claims.Username != "" || claims.Email != "" {
		t.Errorf("类型不符字段应留空, got Username=%q Email=%q", claims.Username, claims.Email)
	}
}

// TestValidateToken_Valid 完整合法 token 各字段正确解析。
func TestValidateToken_Valid(t *testing.T) {
	s := newTestAuthService()
	token := signToken(t, jwt.MapClaims{
		"user_id":    float64(42),
		"username":   "alice",
		"email":      "alice@link.dev",
		"token_type": "access",
		"tenant_id":  float64(3),
	})

	claims, err := s.ValidateToken(token)
	if err != nil {
		t.Fatalf("合法 token 不应报错: %v", err)
	}
	if claims.UserID != 42 || claims.Username != "alice" ||
		claims.Email != "alice@link.dev" || claims.TokenType != "access" || claims.TenantID != 3 {
		t.Errorf("解析结果不符: %+v", claims)
	}
}
