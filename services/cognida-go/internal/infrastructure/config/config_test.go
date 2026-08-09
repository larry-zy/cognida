// Package config: JWT 密钥启动校验单元测试。
package config

import (
	"strings"
	"testing"
)

// TestJWTValidate_MissingSecret 未配置密钥必须校验失败。
func TestJWTValidate_MissingSecret(t *testing.T) {
	cases := []*JWTConfig{
		nil,
		{Secret: ""},
	}
	for _, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("空 JWT_SECRET 应校验失败, config=%+v", c)
		}
	}
}

// TestJWTValidate_PlaceholderSecret 占位密钥必须校验失败。
func TestJWTValidate_PlaceholderSecret(t *testing.T) {
	for placeholder := range jwtPlaceholderSecrets {
		c := &JWTConfig{Secret: placeholder}
		if err := c.Validate(); err == nil {
			t.Errorf("占位密钥 %q 应校验失败", placeholder)
		}
	}
}

// TestJWTValidate_ShortSecret 长度不足的密钥必须校验失败。
func TestJWTValidate_ShortSecret(t *testing.T) {
	c := &JWTConfig{Secret: strings.Repeat("a", jwtSecretMinLen-1)}
	if err := c.Validate(); err == nil {
		t.Errorf("%d 字节密钥（< 最小 %d 字节）应校验失败", jwtSecretMinLen-1, jwtSecretMinLen)
	}
}

// TestJWTValidate_ValidSecret 合法密钥（≥32 字节随机值）校验通过。
func TestJWTValidate_ValidSecret(t *testing.T) {
	c := &JWTConfig{Secret: strings.Repeat("x9", jwtSecretMinLen/2)} // 32 字节非占位
	if err := c.Validate(); err != nil {
		t.Errorf("合法密钥应校验通过: %v", err)
	}
}
