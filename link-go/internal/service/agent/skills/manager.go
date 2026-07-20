// Package skills 提供 Skill 管理器
// 负责从目录加载和管理 Skill 生命周期
package skills

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// defaultWatchInterval 默认目录轮询间隔
const defaultWatchInterval = 3 * time.Second

// ========================================
// SkillManager Skill 管理器
// ========================================

// SkillManager Skill 管理器接口
type SkillManager interface {
	// LoadSkills 从目录加载所有 Skill
	LoadSkills(dir string) (*SkillLoadResult, error)

	// LoadSkill 从文件加载单个 Skill
	LoadSkill(path string) (*Skill, error)

	// UnloadSkill 卸载 Skill
	UnloadSkill(name string) error

	// ReloadSkill 重新加载 Skill
	ReloadSkill(name string) error

	// GetSkill 获取 Skill
	GetSkill(name string) (*Skill, bool)

	// ListSkills 列出所有 Skill
	ListSkills() []*Skill

	// ListSkillsInfo 列出所有 Skill 简要信息
	ListSkillsInfo() []*SkillInfo

	// SearchSkills 搜索 Skill
	SearchSkills(query string) []*SkillMatchResult

	// MatchSkillsForTask 根据任务匹配 Skill
	MatchSkillsForTask(task string, limit int) []*SkillMatchResult

	// GetRegistry 获取注册表
	GetRegistry() SkillRegistry

	// GetSkillSource 获取 Skill 来源路径
	GetSkillSource(name string) (string, bool)

	// Watch 监听目录变化（可选实现）
	Watch(dir string) error

	// StopWatch 停止监听
	StopWatch(dir string) error
}

// skillManager Skill 管理器实现
type skillManager struct {
	registry    SkillRegistry
	loader      SkillLoader
	sources     map[string]string // skill name -> source path
	watchers    map[string]chan struct{} // watched directory -> stop channel
	watchInterval time.Duration          // 轮询间隔（0 时用默认值）
	mu          sync.RWMutex
}

// NewSkillManager 创建 Skill 管理器
func NewSkillManager(opts ...ManagerOption) SkillManager {
	registry := NewSkillRegistry()
	loader := NewSkillLoader()

	m := &skillManager{
		registry: registry,
		loader:   loader,
		sources:  make(map[string]string),
		watchers: make(map[string]chan struct{}),
	}

	// 应用选项
	for _, opt := range opts {
		opt(m)
	}

	return m
}

// ManagerOption 管理器选项
type ManagerOption func(*skillManager)

// WithRegistry 使用自定义注册表
func WithRegistry(registry SkillRegistry) ManagerOption {
	return func(m *skillManager) {
		m.registry = registry
	}
}

// WithLoader 使用自定义加载器
func WithLoader(loader SkillLoader) ManagerOption {
	return func(m *skillManager) {
		m.loader = loader
	}
}

// WithWatchInterval 设置目录轮询间隔
func WithWatchInterval(interval time.Duration) ManagerOption {
	return func(m *skillManager) {
		m.watchInterval = interval
	}
}

// LoadSkills 从目录加载所有 Skill
func (m *skillManager) LoadSkills(dir string) (*SkillLoadResult, error) {
	// 解析绝对路径
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// 检查目录是否存在
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		// 目录不存在，返回空结果
		return &SkillLoadResult{
			Skills:  []*Skill{},
			Success: true,
			Count:   0,
		}, nil
	}

	// 使用加载器加载目录
	skills, errs := m.loader.(*skillLoader).LoadDir(absDir)

	// 注册到注册表
	var registeredSkills []*Skill
	var allErrors []string

	for _, loadErr := range errs {
		allErrors = append(allErrors, loadErr.Error())
	}

	for _, skill := range skills {
		if err := m.registry.Register(skill); err != nil {
			allErrors = append(allErrors, fmt.Sprintf("failed to register skill %s: %v", skill.Name, err))
			continue
		}

		// 记录来源路径
		m.mu.Lock()
		m.sources[skill.Name] = skill.Source
		m.mu.Unlock()

		registeredSkills = append(registeredSkills, skill)
	}

	result := &SkillLoadResult{
		Skills:  registeredSkills,
		Success: len(allErrors) == 0,
		Errors:  allErrors,
		Count:   len(registeredSkills),
	}

	log.Printf("[SkillManager] Loaded %d skills from %s", result.Count, absDir)

	return result, nil
}

// LoadSkill 从文件加载单个 Skill
func (m *skillManager) LoadSkill(path string) (*Skill, error) {
	// 解析绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// 使用加载器加载文件
	skill, err := m.loader.LoadFile(absPath)
	if err != nil {
		return nil, err
	}

	// 注册到注册表
	if err := m.registry.Register(skill); err != nil {
		return nil, err
	}

	// 记录来源路径
	m.mu.Lock()
	m.sources[skill.Name] = absPath
	m.mu.Unlock()

	log.Printf("[SkillManager] Loaded skill: %s from %s", skill.Name, absPath)

	return skill, nil
}

// UnloadSkill 卸载 Skill
func (m *skillManager) UnloadSkill(name string) error {
	// 从注册表注销
	if err := m.registry.Unregister(name); err != nil {
		return err
	}

	// 移除来源记录
	m.mu.Lock()
	delete(m.sources, name)
	m.mu.Unlock()

	log.Printf("[SkillManager] Unloaded skill: %s", name)

	return nil
}

// ReloadSkill 重新加载 Skill
func (m *skillManager) ReloadSkill(name string) error {
	// 获取来源路径
	m.mu.RLock()
	source, exists := m.sources[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("skill source not found: %s", name)
	}

	// 卸载旧 Skill
	if err := m.UnloadSkill(name); err != nil {
		return err
	}

	// 重新加载
	_, err := m.LoadSkill(source)
	return err
}

// GetSkill 获取 Skill
func (m *skillManager) GetSkill(name string) (*Skill, bool) {
	return m.registry.Get(name)
}

// ListSkills 列出所有 Skill
func (m *skillManager) ListSkills() []*Skill {
	return m.registry.List()
}

// ListSkillsInfo 列出所有 Skill 简要信息
func (m *skillManager) ListSkillsInfo() []*SkillInfo {
	skills := m.registry.List()
	infos := make([]*SkillInfo, 0, len(skills))

	for _, skill := range skills {
		infos = append(infos, &SkillInfo{
			Name:          skill.Name,
			Description:   skill.Description,
			WhenToUse:     skill.WhenToUse,
			Tags:          skill.Tags,
			Category:      skill.Category,
			IsPureGuidance: skill.IsPureGuidance(),
		})
	}

	return infos
}

// SearchSkills 搜索 Skill
func (m *skillManager) SearchSkills(query string) []*SkillMatchResult {
	return m.registry.Search(query)
}

// MatchSkillsForTask 根据任务匹配 Skill
func (m *skillManager) MatchSkillsForTask(task string, limit int) []*SkillMatchResult {
	return m.registry.MatchForTask(task, limit)
}

// GetRegistry 获取注册表
func (m *skillManager) GetRegistry() SkillRegistry {
	return m.registry
}

// GetSkillSource 获取 Skill 来源路径
func (m *skillManager) GetSkillSource(name string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	source, exists := m.sources[name]
	return source, exists
}

// Watch 监听目录变化并在检测到改动时自动重新加载 Skill。
// 采用轮询（扫描文件 mtime）实现，避免引入额外的文件系统监听依赖。
func (m *skillManager) Watch(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	m.mu.Lock()
	if _, exists := m.watchers[absDir]; exists {
		m.mu.Unlock()
		return fmt.Errorf("directory already being watched: %s", absDir)
	}
	interval := m.watchInterval
	if interval <= 0 {
		interval = defaultWatchInterval
	}
	stop := make(chan struct{})
	m.watchers[absDir] = stop
	m.mu.Unlock()

	log.Printf("[SkillManager] Watching directory: %s (interval=%s)", absDir, interval)

	// 同步捕获基线指纹，避免 goroutine 启动前发生的改动被当成基线而漏加载。
	baseline := dirFingerprint(absDir)
	go m.watchLoop(absDir, interval, baseline, stop)

	return nil
}

// StopWatch 停止监听
func (m *skillManager) StopWatch(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	m.mu.Lock()
	stop, exists := m.watchers[absDir]
	if exists {
		delete(m.watchers, absDir)
	}
	m.mu.Unlock()

	if !exists {
		return fmt.Errorf("directory not being watched: %s", absDir)
	}

	close(stop)
	log.Printf("[SkillManager] Stopped watching: %s", absDir)

	return nil
}

// watchLoop 周期性扫描目录指纹，发现变化则重新加载 Skill。
func (m *skillManager) watchLoop(dir string, interval time.Duration, baseline string, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	last := baseline

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			current := dirFingerprint(dir)
			if current == last {
				continue
			}
			last = current
			if _, err := m.LoadSkills(dir); err != nil {
				log.Printf("[SkillManager] Reload on change failed for %s: %v", dir, err)
			} else {
				log.Printf("[SkillManager] Reloaded skills after change in %s", dir)
			}
		}
	}
}

// dirFingerprint 计算目录内容指纹，用文件路径+大小+修改时间聚合，
// 任意文件的新增/删除/修改都会导致指纹变化。
func dirFingerprint(dir string) string {
	var sb strings.Builder
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		fmt.Fprintf(&sb, "%s|%d|%d;", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return sb.String()
}

// ========================================
// 全局 Skill 管理器
// ========================================

var (
	globalManager     SkillManager
	globalManagerOnce sync.Once
)

// GetGlobalManager 获取全局 Skill 管理器
func GetGlobalManager() SkillManager {
	globalManagerOnce.Do(func() {
		globalManager = NewSkillManager()
	})
	return globalManager
}

// InitializeGlobalSkills 初始化全局 Skills
// 从默认目录加载所有 Skill
func InitializeGlobalSkills(skillDir string) (*SkillLoadResult, error) {
	manager := GetGlobalManager()
	return manager.LoadSkills(skillDir)
}

// GetSkill 获取 Skill（全局便捷方法）
func GetSkill(name string) (*Skill, bool) {
	return GetGlobalManager().GetSkill(name)
}

// ListAllSkills 列出所有 Skill（全局便捷方法）
func ListAllSkills() []*Skill {
	return GetGlobalManager().ListSkills()
}

// SearchAllSkills 搜索所有 Skill（全局便捷方法）
func SearchAllSkills(query string) []*SkillMatchResult {
	return GetGlobalManager().SearchSkills(query)
}

// MatchSkillForTask 根据任务匹配 Skill（全局便捷方法）
func MatchSkillForTask(task string, limit int) []*SkillMatchResult {
	return GetGlobalManager().MatchSkillsForTask(task, limit)
}
