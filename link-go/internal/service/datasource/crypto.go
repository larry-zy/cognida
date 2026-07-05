package datasource

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

// SecretKeyEnv 凭证加密密钥环境变量名。
// 密钥经 SHA-256 派生为 AES-256 key；密钥变更后旧密文不可解，
// 对应数据源应标记 need_credentials 要求重录密码。
const SecretKeyEnv = "DATASOURCE_SECRET_KEY"

// ErrSecretKeyMissing 未配置加密密钥
var ErrSecretKeyMissing = errors.New("datasource: " + SecretKeyEnv + " 未配置")

// ErrDecryptFailed 解密失败（密钥变更或密文损坏）
var ErrDecryptFailed = errors.New("datasource: 凭证解密失败（密钥可能已变更，请重新录入密码）")

// Cipher 数据源凭证 AES-GCM 加解密器。密文格式：base64(nonce + ciphertext)。
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher 用给定 secret 构建加解密器（secret 经 SHA-256 派生 AES-256 key）
func NewCipher(secret string) (*Cipher, error) {
	if secret == "" {
		return nil, ErrSecretKeyMissing
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("datasource: 初始化加密器失败: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("datasource: 初始化 GCM 失败: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// NewCipherFromEnv 从 DATASOURCE_SECRET_KEY 环境变量构建加解密器
func NewCipherFromEnv() (*Cipher, error) {
	return NewCipher(os.Getenv(SecretKeyEnv))
}

// Encrypt 加密明文密码，返回 base64(nonce+ciphertext)
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("datasource: 生成 nonce 失败: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密密文；密钥不匹配或密文损坏时返回 ErrDecryptFailed
func (c *Cipher) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrDecryptFailed
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", ErrDecryptFailed
	}
	plain, err := c.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", ErrDecryptFailed
	}
	return string(plain), nil
}
