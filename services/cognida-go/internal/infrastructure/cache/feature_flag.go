// Package cache provides feature flag support for semantic cache
package cache

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	domaincache "cognida/internal/model/cache"
)

// ========================================
// Cache Feature Flag
// ========================================

// FeatureFlag 缓存特性开关
type FeatureFlag struct {
	mu           sync.RWMutex
	globalEnabled bool
	agentFlags    map[string]bool // Agent 级别开关
	agentConfigs  map[string]*domaincache.AgentCacheConfig // Agent 级别完整配置
	updatedAt    time.Time
}

// NewFeatureFlag 创建特性开关
func NewFeatureFlag() *FeatureFlag {
	return &FeatureFlag{
		globalEnabled: false,
		agentFlags:    make(map[string]bool),
		agentConfigs:  make(map[string]*domaincache.AgentCacheConfig),
		updatedAt:    time.Now(),
	}
}

// IsEnabled 检查是否启用（全局 + Agent 级别）
func (ff *FeatureFlag) IsEnabled(agentType string) bool {
	ff.mu.RLock()
	defer ff.mu.RUnlock()

	// 全局关闭则全部关闭
	if !ff.globalEnabled {
		return false
	}

	// 检查 Agent 级别开关
	if enabled, ok := ff.agentFlags[agentType]; ok {
		return enabled
	}

	// 未配置的 Agent 默认关闭
	return false
}

// SetGlobal 设置全局开关
func (ff *FeatureFlag) SetGlobal(enabled bool) {
	ff.mu.Lock()
	defer ff.mu.Unlock()
	ff.globalEnabled = enabled
	ff.updatedAt = time.Now()
}

// SetAgent 设置 Agent 级别开关
func (ff *FeatureFlag) SetAgent(agentType string, enabled bool) {
	ff.mu.Lock()
	defer ff.mu.Unlock()
	ff.agentFlags[agentType] = enabled
	ff.updatedAt = time.Now()
}

// GetGlobal 获取全局开关状态
func (ff *FeatureFlag) GetGlobal() bool {
	ff.mu.RLock()
	defer ff.mu.RUnlock()
	return ff.globalEnabled
}

// GetAgent 获取 Agent 级别开关状态
func (ff *FeatureFlag) GetAgent(agentType string) (bool, bool) {
	ff.mu.RLock()
	defer ff.mu.RUnlock()
	enabled, ok := ff.agentFlags[agentType]
	return enabled, ok
}

// GetAll 获取所有开关状态
func (ff *FeatureFlag) GetAll() map[string]interface{} {
	ff.mu.RLock()
	defer ff.mu.RUnlock()

	result := make(map[string]interface{})
	result["global_enabled"] = ff.globalEnabled
	result["updated_at"] = ff.updatedAt

	agents := make(map[string]bool)
	for k, v := range ff.agentFlags {
		agents[k] = v
	}
	result["agents"] = agents

	return result
}

// IsGlobalEnabled 检查全局是否启用（测试兼容）
func (ff *FeatureFlag) IsGlobalEnabled() bool {
	return ff.GetGlobal()
}

// SetGlobalEnabled 设置全局开关（测试兼容）
func (ff *FeatureFlag) SetGlobalEnabled(enabled bool) {
	ff.SetGlobal(enabled)
}

// GetAgentConfig 获取 Agent 配置（测试兼容）
func (ff *FeatureFlag) GetAgentConfig(agentType string) (*domaincache.AgentCacheConfig, bool) {
	ff.mu.RLock()
	defer ff.mu.RUnlock()

	config, ok := ff.agentConfigs[agentType]
	return config, ok
}

// SetAgentConfig 设置 Agent 配置（测试兼容）
func (ff *FeatureFlag) SetAgentConfig(agentType string, config *domaincache.AgentCacheConfig) {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	if ff.agentConfigs == nil {
		ff.agentConfigs = make(map[string]*domaincache.AgentCacheConfig)
	}
	ff.agentConfigs[agentType] = config

	// 同步更新 agentFlags
	if config != nil {
		ff.agentFlags[agentType] = config.Enabled
	}

	ff.updatedAt = time.Now()
}

// RemoveAgent 移除 Agent 配置（测试兼容）
func (ff *FeatureFlag) RemoveAgent(agentType string) {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	delete(ff.agentFlags, agentType)
	delete(ff.agentConfigs, agentType)
	ff.updatedAt = time.Now()
}

// GetAllConfigs 获取所有配置（测试兼容）
type AllConfigs struct {
	GlobalEnabled bool
	Agents        map[string]*domaincache.AgentCacheConfig
}

func (ff *FeatureFlag) GetAllConfigs() *AllConfigs {
	ff.mu.RLock()
	defer ff.mu.RUnlock()

	configs := &AllConfigs{
		GlobalEnabled: ff.globalEnabled,
		Agents:        make(map[string]*domaincache.AgentCacheConfig),
	}

	for k, v := range ff.agentConfigs {
		configs.Agents[k] = v
	}

	return configs
}

// ========================================
// Config File Loading
// ========================================

// yamlAgentConfig 单个 Agent 的 YAML 配置。TTL 用字符串承载（如 "24h"），
// 因为 yaml.v3 会把 time.Duration 当作纳秒整数解析。
type yamlAgentConfig struct {
	Enabled   bool    `yaml:"enabled"`
	Threshold float32 `yaml:"threshold"`
	TTL       string  `yaml:"ttl"`
	TopK      int     `yaml:"top_k"`
}

// yamlCacheConfig 顶层缓存配置文件结构。
type yamlCacheConfig struct {
	Enabled   bool                       `yaml:"enabled"`
	Threshold float32                    `yaml:"threshold"`
	TTL       string                     `yaml:"ttl"`
	TopK      int                        `yaml:"top_k"`
	Agents    map[string]yamlAgentConfig `yaml:"agents"`
}

// parseTTL 解析时长字符串，空串返回 0（表示沿用默认值）。
func parseTTL(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid ttl %q: %w", s, err)
	}
	return d, nil
}

// LoadConfigFromFile 从 YAML 文件加载配置并应用到 FeatureFlag。
// 预期格式：
//
//	enabled: true
//	threshold: 0.85
//	ttl: 24h
//	top_k: 5
//	agents:
//	  rag_agent:
//	    enabled: true
//	    threshold: 0.90
//	    ttl: 24h
//	    top_k: 3
func LoadConfigFromFile(ff *FeatureFlag, configPath string) error {
	strategy, err := loadConfigFromFile(configPath)
	if err != nil {
		return err
	}

	ff.SetGlobal(strategy.Global.Enabled)
	for agentType, config := range strategy.Agents {
		cfg := config // 复制，避免取到循环变量地址
		ff.SetAgentConfig(agentType, &cfg)
	}

	return nil
}

// loadConfigFromFile 读取并解析 YAML 配置文件为领域策略对象。
func loadConfigFromFile(configPath string) (*domaincache.AgentCacheStrategy, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file failed: %w", err)
	}

	var raw yamlCacheConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse yaml config failed: %w", err)
	}

	globalTTL, err := parseTTL(raw.TTL)
	if err != nil {
		return nil, err
	}

	defaults := domaincache.AgentCacheConfig{
		Enabled:   raw.Enabled,
		Threshold: raw.Threshold,
		TTL:       globalTTL,
		TopK:      raw.TopK,
	}

	strategy := &domaincache.AgentCacheStrategy{
		Global: domaincache.SemanticCacheConfig{
			Enabled:   raw.Enabled,
			Threshold: raw.Threshold,
			TTL:       globalTTL,
			TopK:      raw.TopK,
		},
		Defaults: defaults,
		Agents:   make(map[string]domaincache.AgentCacheConfig, len(raw.Agents)),
	}

	for name, ac := range raw.Agents {
		ttl, err := parseTTL(ac.TTL)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", name, err)
		}
		// 未显式设置的字段沿用默认值
		threshold := ac.Threshold
		if threshold == 0 {
			threshold = defaults.Threshold
		}
		if ttl == 0 {
			ttl = defaults.TTL
		}
		topK := ac.TopK
		if topK == 0 {
			topK = defaults.TopK
		}
		strategy.Agents[name] = domaincache.AgentCacheConfig{
			Enabled:   ac.Enabled,
			Threshold: threshold,
			TTL:       ttl,
			TopK:      topK,
		}
	}

	return strategy, nil
}

// ========================================
// Hot Reload Configuration
// ========================================

// HotReloader 配置热加载器
type HotReloader struct {
	ff       *FeatureFlag
	strategy *domaincache.AgentCacheStrategy
	mu       sync.RWMutex
}

// NewHotReloader 创建热加载器
func NewHotReloader(strategy *domaincache.AgentCacheStrategy) *HotReloader {
	return &HotReloader{
		ff:       NewFeatureFlag(),
		strategy: strategy,
	}
}

// GetFeatureFlag 获取特性开关
func (hr *HotReloader) GetFeatureFlag() *FeatureFlag {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	return hr.ff
}

// ReloadFromStrategy 从策略重新加载配置
func (hr *HotReloader) ReloadFromStrategy(strategy *domaincache.AgentCacheStrategy) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	hr.strategy = strategy
	hr.ff.globalEnabled = strategy.Global.Enabled
	hr.ff.agentFlags = make(map[string]bool)

	for agentType, config := range strategy.Agents {
		hr.ff.agentFlags[agentType] = config.Enabled
	}

	// 更新时间
	hr.ff.updatedAt = time.Now()
}

// ToggleGlobal 切换全局开关
func (hr *HotReloader) ToggleGlobal(enabled bool) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	hr.ff.globalEnabled = enabled
	hr.ff.updatedAt = time.Now()
	hr.strategy.Global.Enabled = enabled

	return nil
}

// ToggleAgent 切换 Agent 级别开关
func (hr *HotReloader) ToggleAgent(agentType string, enabled bool) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	hr.ff.agentFlags[agentType] = enabled
	hr.ff.updatedAt = time.Now()

	// 更新策略中的配置
	if config, ok := hr.strategy.Agents[agentType]; ok {
		config.Enabled = enabled
		hr.strategy.Agents[agentType] = config
	} else {
		// 创建新配置
		hr.strategy.Agents[agentType] = domaincache.AgentCacheConfig{
			Enabled:  enabled,
			Threshold: hr.strategy.Defaults.Threshold,
			TTL:      hr.strategy.Defaults.TTL,
			TopK:     hr.strategy.Defaults.TopK,
		}
	}

	return nil
}

// GetStrategy 获取当前策略
func (hr *HotReloader) GetStrategy() *domaincache.AgentCacheStrategy {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	return hr.strategy
}

// FeatureFlagReloaderAdapter 让 *HotReloader 适配 model/cache.FeatureFlagReloader 端口。
// Go 无协变返回，HotReloader.GetFeatureFlag 返回 *FeatureFlag（具体类型），
// 而端口要求返回 FeatureFlagView（接口），故需此适配层转换返回类型。
type FeatureFlagReloaderAdapter struct {
	*HotReloader
}

// NewFeatureFlagReloader 将 *HotReloader 包装为领域端口 FeatureFlagReloader。
func NewFeatureFlagReloader(hr *HotReloader) domaincache.FeatureFlagReloader {
	return &FeatureFlagReloaderAdapter{HotReloader: hr}
}

// GetFeatureFlag 返回领域端口视图（*FeatureFlag 自动满足 FeatureFlagView）。
func (a *FeatureFlagReloaderAdapter) GetFeatureFlag() domaincache.FeatureFlagView {
	return a.HotReloader.GetFeatureFlag()
}

// ========================================
// Watcher for Configuration Changes
// ========================================

// ConfigWatcher 配置观察者接口
type ConfigWatcher interface {
	Watch(ctx context.Context, onChange func(*domaincache.AgentCacheStrategy)) error
}

// FileConfigWatcher 文件配置观察者
type FileConfigWatcher struct {
	configPath string
	interval   time.Duration
}

// NewFileConfigWatcher 创建文件配置观察者
func NewFileConfigWatcher(configPath string, interval time.Duration) *FileConfigWatcher {
	return &FileConfigWatcher{
		configPath: configPath,
		interval:   interval,
	}
}

// Watch 监听配置文件变化
func (w *FileConfigWatcher) Watch(ctx context.Context, onChange func(*domaincache.AgentCacheStrategy)) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	var lastModTime time.Time

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// 检查文件修改时间
			info, err := os.Stat(w.configPath)
			if err != nil {
				continue
			}

			if info.ModTime().After(lastModTime) {
				lastModTime = info.ModTime()

				// 重新加载配置
				strategy, err := loadConfigFromFile(w.configPath)
				if err != nil {
					continue
				}

				// 触发变更回调
				onChange(strategy)
			}
		}
	}
}

// ========================================
// Admin API for Runtime Control
// ========================================

// RuntimeConfig 运行时配置
type RuntimeConfig struct {
	FeatureFlag *FeatureFlag `json:"feature_flag"`
	Strategy    *domaincache.AgentCacheStrategy `json:"strategy"`
}

// GetRuntimeConfig 获取运行时配置
func (hr *HotReloader) GetRuntimeConfig() *RuntimeConfig {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	return &RuntimeConfig{
		FeatureFlag: hr.ff,
		Strategy:    hr.strategy,
	}
}

// ApplyRuntimeConfig 应用运行时配置
func (hr *HotReloader) ApplyRuntimeConfig(cfg *RuntimeConfig) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if cfg.FeatureFlag != nil {
		hr.ff = cfg.FeatureFlag
	}

	if cfg.Strategy != nil {
		hr.strategy = cfg.Strategy
	}

	return nil
}

// ========================================
// Metrics
// ========================================

// CacheConfigMetrics 缓存配置指标。
// 类型已下沉至 model/cache（领域端口），此处保留别名供 infrastructure 内部使用。
type CacheConfigMetrics = domaincache.CacheConfigMetrics

// GetMetrics 获取配置指标
func (ff *FeatureFlag) GetMetrics() *CacheConfigMetrics {
	ff.mu.RLock()
	defer ff.mu.RUnlock()

	metrics := &CacheConfigMetrics{
		GlobalEnabled:  ff.globalEnabled,
		AgentCount:     len(ff.agentFlags),
		EnabledAgents:  make([]string, 0),
		DisabledAgents: make([]string, 0),
		LastUpdateTime: ff.updatedAt,
	}

	for agent, enabled := range ff.agentFlags {
		if enabled {
			metrics.EnabledAgents = append(metrics.EnabledAgents, agent)
		} else {
			metrics.DisabledAgents = append(metrics.DisabledAgents, agent)
		}
	}

	return metrics
}
