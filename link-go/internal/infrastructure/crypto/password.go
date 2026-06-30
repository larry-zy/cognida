package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// ========================================
// Argon2id 密码哈希
// ========================================

// Argon2 配置参数
var (
	// 时间复杂度（迭代次数）
	timeCost = uint32(1)

	// 内存成本（KB）
	memoryCost = uint32(64 * 1024) // 64MB

	// 并行度
	parallelism = uint8(2)

	// 盐长度
	saltLength = 16

	// 密钥长度
	keyLength = uint32(32)
)

// HashPassword 哈希密码
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	// 生成随机盐
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// 使用 Argon2id 哈希
	hash := argon2.IDKey(
		[]byte(password),
		salt,
		timeCost,
		memoryCost,
		parallelism,
		keyLength,
	)

	// 编码为 base64
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	// 返回格式: $argon2id$v=19$m=65536,t=1,p=2$salt$hash
	encodedHash := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memoryCost,
		timeCost,
		parallelism,
		b64Salt,
		b64Hash,
	)

	return encodedHash, nil
}

// VerifyPassword 验证密码
func VerifyPassword(password, encodedHash string) (bool, error) {
	if password == "" || encodedHash == "" {
		return false, nil
	}

	// 解析哈希
	salt, hash, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}

	// 使用相同参数计算哈希
	otherHash := argon2.IDKey(
		[]byte(password),
		salt,
		timeCost,
		memoryCost,
		parallelism,
		keyLength,
	)

	// 常量时间比较，防止时序攻击
	if subtle.ConstantTimeCompare(hash, otherHash) == 1 {
		return true, nil
	}

	return false, nil
}

// decodeHash 解码哈希
func decodeHash(encodedHash string) (salt, hash []byte, err error) {
	params := splitHash(encodedHash)
	if len(params) != 6 {
		return nil, nil, fmt.Errorf("invalid hash format")
	}

	var version int
	_, err = fmt.Sscanf(params[1], "v=%d", &version)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid version: %w", err)
	}
	if version != argon2.Version {
		return nil, nil, fmt.Errorf("unsupported version: %d", version)
	}

	_, err = fmt.Sscanf(params[2], "m=%d", &memoryCost)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid memory cost: %w", err)
	}

	_, err = fmt.Sscanf(params[3], "t=%d", &timeCost)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid time cost: %w", err)
	}

	_, err = fmt.Sscanf(params[4], "p=%d", &parallelism)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid parallelism: %w", err)
	}

	salt, err = base64.RawStdEncoding.DecodeString(params[5])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid salt: %w", err)
	}

	hash, err = base64.RawStdEncoding.DecodeString(params[6])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid hash: %w", err)
	}

	return salt, hash, nil
}

// splitHash 分割哈希
func splitHash(s string) []string {
	var parts []string
	var start int
	inEscape := false

	for i, c := range s {
		switch c {
		case '$':
			if !inEscape {
				if i > start {
					parts = append(parts, s[start:i])
				}
				start = i + 1
			}
		case '\\':
			inEscape = !inEscape
		default:
			inEscape = false
		}
	}

	if start < len(s) {
		parts = append(parts, s[start:])
	}

	return parts
}

// ========================================
// 简单 bcrypt 兼容接口（可选）
// ========================================

// GeneratePassword 生成密码哈希（简单封装）
func GeneratePassword(password string) (string, error) {
	return HashPassword(password)
}

// CheckPassword 检查密码（简单封装）
func CheckPassword(password, hash string) bool {
	ok, _ := VerifyPassword(password, hash)
	return ok
}
