// Package prompt 是全项目提示词（Prompt）的集中注册中心。
//
// 设计要点：
//   - 每个业务一个 YAML 文件（templates/<business>.yaml），文件内为「key: 多行文本」的
//     扁平映射，一个 key 对应一段可复用的提示词。
//   - 全部 YAML 经 go:embed 在编译期内嵌进二进制，启动时（init）一次性解析并缓存，
//     零运行时路径依赖、随二进制分发，无需随部署携带配置目录。
//   - 业务侧用 MustGet 在包级变量或调用点取用；缺失 key 直接 panic，实现启动即失败
//     （fail-fast），把「提示词漏配」这类错误暴露在最早的时刻而非请求链路深处。
//
// 知识图谱抽取/查询改写用的模板结构不同（templates 列表而非扁平 key），单独放在本包的
// extraction_templates/ 目录，由 extraction.go 的 LoadExtractionTemplate 内嵌加载，与本
// registry 的 templates/*.yaml 互不干扰。
package prompt

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed templates/*.yaml
var templateFS embed.FS

// store: business(文件名去 .yaml) -> key -> 提示词正文。
var store map[string]map[string]string

func init() {
	loaded, err := load()
	if err != nil {
		// 内嵌资源解析失败属于不可恢复的编译期/启动期错误，直接 panic。
		panic(fmt.Sprintf("prompt: 加载内嵌提示词配置失败: %v", err))
	}
	store = loaded
}

// load 解析 templates/ 下所有 YAML，返回 business -> key -> content 的两级映射。
func load() (map[string]map[string]string, error) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("读取内嵌 templates 目录: %w", err)
	}

	out := make(map[string]map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := templateFS.ReadFile("templates/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("读取 %s: %w", e.Name(), err)
		}
		var m map[string]string
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("解析 %s: %w", e.Name(), err)
		}
		business := strings.TrimSuffix(e.Name(), ".yaml")
		out[business] = m
	}
	return out, nil
}

// Get 返回 (business, key) 对应的提示词及是否存在。
func Get(business, key string) (string, bool) {
	m, ok := store[business]
	if !ok {
		return "", false
	}
	v, ok := m[key]
	return v, ok
}

// MustGet 返回提示词，缺失则 panic。适合在包级变量初始化时取用，
// 使「提示词漏配」在程序启动阶段即暴露，而非潜伏到请求链路中。
func MustGet(business, key string) string {
	v, ok := Get(business, key)
	if !ok {
		panic(fmt.Sprintf("prompt: 缺少提示词 %q/%q（请检查 internal/prompt/templates/%s.yaml）", business, key, business))
	}
	return v
}

// Keys 返回某业务下所有 key（已排序），主要用于测试与诊断。
func Keys(business string) []string {
	m, ok := store[business]
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
