// Package skills 提供 Skill 注册表
// 负责存储和检索 Skill 定义
package skills

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// ========================================
// SkillRegistry Skill 注册表接口
// ========================================

// SkillRegistry Skill 注册表接口
type SkillRegistry interface {
	// Register 注册 Skill
	Register(skill *Skill) error

	// Unregister 注销 Skill
	Unregister(name string) error

	// Get 获取 Skill
	Get(name string) (*Skill, bool)

	// List 列出所有 Skill
	List() []*Skill

	// ListByCategory 按分类列出 Skill
	ListByCategory(category string) []*Skill

	// Search 搜索 Skill（基于关键词匹配）
	Search(query string) []*SkillMatchResult

	// MatchForTask 根据任务描述匹配最相关的 Skill
	MatchForTask(task string, limit int) []*SkillMatchResult

	// GetInfo 获取 Skill 简要信息
	GetInfo(name string) (*SkillInfo, bool)

	// Size 返回 Skill 数量
	Size() int

	// Categories 返回所有分类
	Categories() []string
}

// skillRegistry Skill 注册表实现
type skillRegistry struct {
	mu     sync.RWMutex
	skills map[string]*Skill
	// 索引
	categoryIndex map[string][]string // category -> skill names
	tagIndex      map[string][]string // tag -> skill names
}

// NewSkillRegistry 创建 Skill 注册表
func NewSkillRegistry() SkillRegistry {
	return &skillRegistry{
		skills:        make(map[string]*Skill),
		categoryIndex: make(map[string][]string),
		tagIndex:      make(map[string][]string),
	}
}

// Register 注册 Skill
func (r *skillRegistry) Register(skill *Skill) error {
	if skill == nil {
		return fmt.Errorf("skill cannot be nil")
	}

	if !IsValidSkillName(skill.Name) {
		return fmt.Errorf("invalid skill name: %s", skill.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查是否已存在
	if _, exists := r.skills[skill.Name]; exists {
		return fmt.Errorf("skill already registered: %s", skill.Name)
	}

	// 设置加载时间
	skill.LoadedAt = time.Now()

	// 存储 Skill
	r.skills[skill.Name] = skill

	// 更新分类索引
	if skill.Category != "" {
		r.categoryIndex[skill.Category] = append(r.categoryIndex[skill.Category], skill.Name)
	}

	// 更新标签索引
	for _, tag := range skill.Tags {
		r.tagIndex[tag] = append(r.tagIndex[tag], skill.Name)
	}

	return nil
}

// Unregister 注销 Skill
func (r *skillRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	skill, exists := r.skills[name]
	if !exists {
		return fmt.Errorf("skill not found: %s", name)
	}

	// 从分类索引中移除
	if skill.Category != "" {
		names := r.categoryIndex[skill.Category]
		r.categoryIndex[skill.Category] = removeString(names, name)
		if len(r.categoryIndex[skill.Category]) == 0 {
			delete(r.categoryIndex, skill.Category)
		}
	}

	// 从标签索引中移除
	for _, tag := range skill.Tags {
		names := r.tagIndex[tag]
		r.tagIndex[tag] = removeString(names, name)
		if len(r.tagIndex[tag]) == 0 {
			delete(r.tagIndex, tag)
		}
	}

	// 删除 Skill
	delete(r.skills, name)

	return nil
}

// Get 获取 Skill
func (r *skillRegistry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, exists := r.skills[name]
	return skill, exists
}

// List 列出所有 Skill
func (r *skillRegistry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		skills = append(skills, skill)
	}

	// 按名称排序
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills
}

// ListByCategory 按分类列出 Skill
func (r *skillRegistry) ListByCategory(category string) []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names, exists := r.categoryIndex[category]
	if !exists {
		return nil
	}

	skills := make([]*Skill, 0, len(names))
	for _, name := range names {
		if skill, exists := r.skills[name]; exists {
			skills = append(skills, skill)
		}
	}

	return skills
}

// Search 搜索 Skill（基于关键词匹配）
func (r *skillRegistry) Search(query string) []*SkillMatchResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queryLower := strings.ToLower(query)
	terms := strings.Fields(queryLower)

	var results []*SkillMatchResult

	for _, skill := range r.skills {
		relevance, reason := r.calculateRelevance(skill, queryLower, terms)
		if relevance > 0 {
			results = append(results, &SkillMatchResult{
				Skill:     skill,
				Relevance: relevance,
				Reason:    reason,
			})
		}
	}

	// 按相关度排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})

	return results
}

// MatchForTask 根据任务描述匹配最相关的 Skill
func (r *skillRegistry) MatchForTask(task string, limit int) []*SkillMatchResult {
	results := r.Search(task)

	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

// GetInfo 获取 Skill 简要信息
func (r *skillRegistry) GetInfo(name string) (*SkillInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, exists := r.skills[name]
	if !exists {
		return nil, false
	}

	return &SkillInfo{
		Name:           skill.Name,
		Description:    skill.Description,
		WhenToUse:      skill.WhenToUse,
		Tags:           skill.Tags,
		Category:       skill.Category,
		IsPureGuidance: skill.IsPureGuidance(),
	}, true
}

// Size 返回 Skill 数量
func (r *skillRegistry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.skills)
}

// Categories 返回所有分类
func (r *skillRegistry) Categories() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	categories := make([]string, 0, len(r.categoryIndex))
	for category := range r.categoryIndex {
		categories = append(categories, category)
	}

	sort.Strings(categories)

	return categories
}

// ========================================
// 相关度计算
// ========================================

// calculateRelevance 计算 Skill 与查询的相关度
// 返回值：0-1 之间的分数，0 表示不相关
func (r *skillRegistry) calculateRelevance(skill *Skill, queryLower string, terms []string) (float64, string) {
	var score float64
	var reasons []string

	// 1. 名称精确匹配 (最高优先级)
	if skill.Name == queryLower {
		score += 1.0
		reasons = append(reasons, "name exact match")
	}

	// 2. 名称部分匹配
	if strings.Contains(skill.Name, queryLower) || strings.Contains(queryLower, strings.ToLower(skill.Name)) {
		score += 0.8
		reasons = append(reasons, "name partial match")
	}

	// 3. 描述匹配（检查每个词）
	descLower := strings.ToLower(skill.Description)
	descWords := strings.Fields(descLower)
	matchedWords := 0
	for _, term := range terms {
		for _, word := range descWords {
			if strings.Contains(word, term) || term == word {
				matchedWords++
				break
			}
		}
	}
	if matchedWords > 0 {
		matchRatio := float64(matchedWords) / float64(len(terms))
		score += matchRatio * 0.5
		reasons = append(reasons, fmt.Sprintf("description match (%d/%d words)", matchedWords, len(terms)))
	}

	// 4. when_to_use 匹配
	if skill.WhenToUse != "" {
		whenToLower := strings.ToLower(skill.WhenToUse)
		whenMatchCount := 0
		for _, term := range terms {
			if strings.Contains(whenToLower, term) {
				whenMatchCount++
			}
		}
		if whenMatchCount > 0 {
			score += float64(whenMatchCount) / float64(len(terms)) * 0.4
			reasons = append(reasons, fmt.Sprintf("when_to_use match (%d terms)", whenMatchCount))
		}
	}

	// 5. 标签匹配
	for _, tag := range skill.Tags {
		tagLower := strings.ToLower(tag)
		for _, term := range terms {
			if strings.Contains(tagLower, term) || term == tagLower {
				score += 0.2
				reasons = append(reasons, fmt.Sprintf("tag match: %s", tag))
				break
			}
		}
	}

	// 6. 分类匹配
	if skill.Category != "" {
		categoryLower := strings.ToLower(skill.Category)
		for _, term := range terms {
			if strings.Contains(categoryLower, term) || term == categoryLower {
				score += 0.15
				reasons = append(reasons, fmt.Sprintf("category match: %s", skill.Category))
				break
			}
		}
	}

	// 7. CJK 二元字（bigram）重叠
	// 中文等 CJK 文本无空格，strings.Fields 无法分词（整句坍缩为单一 term），上面基于词的
	// 匹配对中文查询几乎恒为 0。这里改用相邻 CJK 字符组成的 bigram 与 Skill 的
	// 名称/描述/使用时机/标签做子串重叠度量，使中文查询能真正命中中文 Skill 文案。
	if qb := cjkBigrams(queryLower); len(qb) > 0 {
		haystack := strings.ToLower(skill.Name) + " " + descLower + " " +
			strings.ToLower(skill.WhenToUse) + " " + strings.ToLower(strings.Join(skill.Tags, " "))
		matched := 0
		for _, bg := range qb {
			if strings.Contains(haystack, bg) {
				matched++
			}
		}
		if matched > 0 {
			// 长中文查询含大量填充词，纯 matched/len(qb) 覆盖率会被稀释到阈值以下。
			// 改取「覆盖率」与「饱和计数」的较大者：短聚焦查询靠覆盖率，长查询靠
			// 少量强内容 bigram（如 数据/查询/分析）命中即饱和——3 个强命中即视为强信号。
			coverage := float64(matched) / float64(len(qb))
			satCount := float64(matched) / 3.0
			if satCount > 1.0 {
				satCount = 1.0
			}
			signal := coverage
			if satCount > signal {
				signal = satCount
			}
			score += signal * 0.7
			reasons = append(reasons, fmt.Sprintf("cjk overlap (%d/%d bigrams)", matched, len(qb)))
		}
	}

	// 标准化分数到 0-1 范围
	if score > 1.0 {
		score = 1.0
	}

	reason := strings.Join(reasons, "; ")

	return score, reason
}

// ========================================
// 辅助函数
// ========================================

// isCJKRune 判定是否为 CJK 表意文字（含扩展区与兼容区），用于对无空格文本做 bigram 切分。
func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

// cjkBigrams 从文本中提取去重后的相邻 CJK 字符二元组（bigram）。
// 仅对连续 CJK 字符成对取样（遇到非 CJK 字符即断开），因此中英混排文本里的英文词
// 不会被误切；英文侧仍由基于空格分词的 term 逻辑覆盖。返回值已去重、全小写。
func cjkBigrams(s string) []string {
	runes := []rune(s)
	seen := make(map[string]struct{})
	var out []string
	for i := 0; i+1 < len(runes); i++ {
		if isCJKRune(runes[i]) && isCJKRune(runes[i+1]) {
			bg := string(runes[i : i+2])
			if _, dup := seen[bg]; dup {
				continue
			}
			seen[bg] = struct{}{}
			out = append(out, bg)
		}
	}
	return out
}

// removeString 从字符串切片中移除指定元素
func removeString(slice []string, s string) []string {
	for i, item := range slice {
		if item == s {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
