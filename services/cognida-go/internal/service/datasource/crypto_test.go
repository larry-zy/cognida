package datasource

import (
	"errors"
	"testing"
)

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher("test-secret")
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	for _, plain := range []string{"", "p@ssw0rd!", "中文密码🔑", "very-long-" + string(make([]byte, 512))} {
		enc, err := c.Encrypt(plain)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", plain, err)
		}
		if enc == plain && plain != "" {
			t.Fatalf("密文不应等于明文")
		}
		got, err := c.Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != plain {
			t.Fatalf("round trip 不一致: got %q want %q", got, plain)
		}
	}
}

func TestCipherNonceRandomness(t *testing.T) {
	c, _ := NewCipher("test-secret")
	a, _ := c.Encrypt("same")
	b, _ := c.Encrypt("same")
	if a == b {
		t.Fatalf("相同明文两次加密不应产生相同密文（nonce 随机）")
	}
}

func TestCipherKeyMismatch(t *testing.T) {
	c1, _ := NewCipher("secret-one")
	c2, _ := NewCipher("secret-two")
	enc, _ := c1.Encrypt("password")
	if _, err := c2.Decrypt(enc); !errors.Is(err, ErrDecryptFailed) {
		t.Fatalf("密钥变更后解密应返回 ErrDecryptFailed, got %v", err)
	}
}

func TestCipherCorruptedCiphertext(t *testing.T) {
	c, _ := NewCipher("test-secret")
	for _, bad := range []string{"", "not-base64!!!", "aGVsbG8="} { // 最后一个是合法 base64 但太短/无效
		if _, err := c.Decrypt(bad); !errors.Is(err, ErrDecryptFailed) {
			t.Fatalf("Decrypt(%q) 应返回 ErrDecryptFailed, got %v", bad, err)
		}
	}
}

func TestNewCipherMissingKey(t *testing.T) {
	if _, err := NewCipher(""); !errors.Is(err, ErrSecretKeyMissing) {
		t.Fatalf("空密钥应返回 ErrSecretKeyMissing, got %v", err)
	}
	t.Setenv(SecretKeyEnv, "")
	if _, err := NewCipherFromEnv(); !errors.Is(err, ErrSecretKeyMissing) {
		t.Fatalf("环境变量缺失应返回 ErrSecretKeyMissing, got %v", err)
	}
	t.Setenv(SecretKeyEnv, "from-env")
	if _, err := NewCipherFromEnv(); err != nil {
		t.Fatalf("NewCipherFromEnv: %v", err)
	}
}
