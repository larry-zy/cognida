// Package config: JWT 密钥启动校验 + 非密配置（config.yaml）加载单元测试。
package config

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// repoConfigYAMLPath 入库 config.yaml 现与本测试包同目录（internal/config），
// go test 的 CWD 即包目录，故直接引用文件名。
const repoConfigYAMLPath = "config.yaml"

// TestBuildFileConfig_FallbackToDefaults 找不到 yaml 时回退到代码内默认值（不 fatal）。
func TestBuildFileConfig_FallbackToDefaults(t *testing.T) {
	// 指向不存在的文件，强制 ReadFile 失败 → 保持默认基线。
	t.Setenv("CONFIG_FILE", filepath.Join(t.TempDir(), "does-not-exist.yaml"))

	fc := buildFileConfig()
	want := defaultFileConfig()
	if !reflect.DeepEqual(fc, want) {
		t.Fatalf("缺失 yaml 时应回退到默认基线\n got=%+v\nwant=%+v", fc, want)
	}
}

// TestBuildFileConfig_RepoYAMLEqualsDefaults 行为等价核心断言：
// 入库 config.yaml 的每个值必须与代码内默认完全一致，
// 从而保证「无 env」时改造前后取到相同的值。
func TestBuildFileConfig_RepoYAMLEqualsDefaults(t *testing.T) {
	if _, err := os.Stat(repoConfigYAMLPath); err != nil {
		t.Fatalf("入库 config.yaml 不存在: %v", err)
	}
	t.Setenv("CONFIG_FILE", repoConfigYAMLPath)

	fc := buildFileConfig()
	want := defaultFileConfig()
	if !reflect.DeepEqual(fc, want) {
		t.Fatalf("config.yaml 值与代码默认不一致（行为不等价）\n yaml=%+v\ndefault=%+v", fc, want)
	}
}

// TestBuildFileConfig_YAMLBaseline yaml 中出现的 key 覆盖默认，未出现的保持默认。
func TestBuildFileConfig_YAMLBaseline(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "config.yaml")
	content := "" +
		"server:\n" +
		"  port: \"9090\"\n" +
		"redis:\n" +
		"  pool_size: 42\n"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatalf("写临时 yaml 失败: %v", err)
	}
	t.Setenv("CONFIG_FILE", tmp)

	fc := buildFileConfig()
	if fc.Server.Port != "9090" {
		t.Errorf("yaml 应覆盖 server.port: got %q want %q", fc.Server.Port, "9090")
	}
	if fc.Redis.PoolSize != 42 {
		t.Errorf("yaml 应覆盖 redis.pool_size: got %d want 42", fc.Redis.PoolSize)
	}
	// 未在 yaml 中出现的 key 保持代码默认
	if fc.Server.Host != "0.0.0.0" {
		t.Errorf("未覆盖的 server.host 应保持默认: got %q", fc.Server.Host)
	}
	if fc.Skill.Endpoint != "http://localhost:3100/mcp" {
		t.Errorf("未覆盖的 skill.endpoint 应保持默认: got %q", fc.Skill.Endpoint)
	}
}

// TestEnvOverridesYAML 环境变量优先级最高，覆盖 yaml 基线值。
func TestEnvOverridesYAML(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(tmp, []byte("server:\n  port: \"9090\"\n"), 0o644); err != nil {
		t.Fatalf("写临时 yaml 失败: %v", err)
	}
	t.Setenv("CONFIG_FILE", tmp)
	t.Setenv("SERVER_PORT", "7777") // env 覆盖 yaml

	if got := getEnv("SERVER_PORT", buildFileConfig().Server.Port); got != "7777" {
		t.Errorf("env 应覆盖 yaml: got %q want 7777", got)
	}
}

// TestSecretsNeverInYAML 硬性要求：入库 config.yaml 不得出现任何密钥/占位密钥值。
func TestSecretsNeverInYAML(t *testing.T) {
	data, err := os.ReadFile(repoConfigYAMLPath)
	if err != nil {
		t.Fatalf("读取 config.yaml 失败: %v", err)
	}
	// 逐行检查：只看键名/值内容中是否出现密钥类 token（注释行豁免，注释只是说明文档）。
	re := regexp.MustCompile(`(?i)(password|secret|token|api_?key)`)
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue // 跳过空行与整行注释
		}
		// 去掉行尾注释后再判定
		codePart := trimmed
		if idx := strings.Index(codePart, "#"); idx >= 0 {
			codePart = codePart[:idx]
		}
		if re.MatchString(codePart) {
			t.Errorf("config.yaml 第 %d 行疑似含密钥类字段: %q", i+1, line)
		}
	}
}

// TestFileConfigStructHasNoSecretFields 反射校验 FileConfig 不含任何密钥类 yaml 字段。
func TestFileConfigStructHasNoSecretFields(t *testing.T) {
	re := regexp.MustCompile(`(?i)(password|secret|token|api_?key)`)
	var walk func(rt reflect.Type)
	walk = func(rt reflect.Type) {
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			// 只校验 yaml tag（真正落入入库文件的键名）；Go 字段名不入库，豁免。
			tag := f.Tag.Get("yaml")
			if re.MatchString(tag) {
				t.Errorf("FileConfig 含疑似密钥 yaml 字段: %s (yaml:%q)", f.Name, tag)
			}
			if f.Type.Kind() == reflect.Struct {
				walk(f.Type)
			}
		}
	}
	walk(reflect.TypeOf(FileConfig{}))
}

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

// TestGetEnvAsInt_RejectsPartialParse 验证〔M8〕：非法/半数值环境变量整体回退默认值，
// 不再像旧 fmt.Sscanf 那样把 "12abc" 静默截断为 12。
func TestGetEnvAsInt_RejectsPartialParse(t *testing.T) {
	const key = "COGNIDA_TEST_INT"
	cases := []struct {
		val  string
		want int
	}{
		{"42", 42},    // 合法整串
		{"-7", -7},    // 合法负数
		{"12abc", 99}, // 半数值 → 回退
		{"abc", 99},   // 非数值 → 回退
		{"3.14", 99},  // 浮点 → 回退
		{" 5", 99},    // 前导空格 → 回退
		{"", 99},      // 空 → 回退（等同未设置）
	}
	for _, tc := range cases {
		t.Setenv(key, tc.val)
		if got := getEnvAsInt(key, 99); got != tc.want {
			t.Errorf("getEnvAsInt(%q) = %d, want %d", tc.val, got, tc.want)
		}
	}
}

// TestGetEnvAsInt64_RejectsPartialParse 同上，覆盖 int64 变体。
func TestGetEnvAsInt64_RejectsPartialParse(t *testing.T) {
	const key = "COGNIDA_TEST_INT64"
	t.Setenv(key, "9000000000") // 超 int32，验证 64 位解析
	if got := getEnvAsInt64(key, 1); got != 9000000000 {
		t.Errorf("getEnvAsInt64 合法值 = %d, want 9000000000", got)
	}
	t.Setenv(key, "900x")
	if got := getEnvAsInt64(key, 1); got != 1 {
		t.Errorf("getEnvAsInt64 半数值应回退默认值 1, got %d", got)
	}
}
