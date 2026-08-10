// Package graph provides graph infrastructure implementations
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"

	"cognida/internal/config"
	domain_knowledge "cognida/internal/model/knowledge"
	domainrag "cognida/internal/model/rag"
)

// ========================================
// LLM Graph Extractor
// ========================================

// LLMExtractor LLM 图谱提取器，负责调用 LLM 提取实体和关系
type LLMExtractor struct {
	llmChat domainrag.LLMChat
}

// NewLLMExtractor 创建 LLM 提取器
func NewLLMExtractor(llmChat domainrag.LLMChat) *LLMExtractor {
	return &LLMExtractor{
		llmChat: llmChat,
	}
}

// ExtractEntities 从文档中提取实体
func (e *LLMExtractor) ExtractEntities(
	ctx context.Context,
	chunkID, document, query string,
	mode domain_knowledge.ExtractionMode,
) ([]*domain_knowledge.GraphNode, error) {
	// 根据模式选择提示词模板
	var templateName string
	if mode == domain_knowledge.ExtractionModeQuery {
		templateName = "entity_extraction_query"
	} else {
		templateName = "entity_extraction"
	}

	promptTemplate, err := config.LoadPromptTemplate(templateName)
	if err != nil {
		return nil, fmt.Errorf("failed to load entity extraction template: %w", err)
	}

	// 替换占位符
	prompt := strings.Replace(promptTemplate, "{{document}}", document, 1)

	// 查询模式：添加查询信息
	if mode == domain_knowledge.ExtractionModeQuery && query != "" {
		prompt = strings.Replace(prompt, "{{query}}", query, 1)
	}

	log.Printf("[LLMExtractor] Entity extraction mode=%s prompt length: %d", mode, len(prompt))

	// 构建消息
	messages := []domainrag.LLMMessage{
		{Role: "user", Content: prompt},
	}

	// 调用 LLM
	response, err := e.llmChat.Chat(ctx, messages, &domainrag.ChatOptions{
		Temperature: 0.1,
		MaxTokens:   2000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate entity extraction: %w", err)
	}

	log.Printf("[LLMExtractor] Entity extraction raw response: %s", response.Content)

	// 清理响应内容
	cleanedContent := cleanJSONResponse(response.Content)
	log.Printf("[LLMExtractor] Entity extraction cleaned response: %s", cleanedContent)

	// 解析 JSON 响应
	var result struct {
		Nodes []*domain_knowledge.GraphNode `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(cleanedContent), &result); err != nil {
		return nil, fmt.Errorf("failed to parse entity extraction response: %w, content: %s", err, cleanedContent)
	}

	// 为每个节点添加 chunk_id
	for _, node := range result.Nodes {
		if node.ID == "" {
			node.ID = uuid.New().String()
		}
		if !contains(node.Chunks, chunkID) {
			node.Chunks = append(node.Chunks, chunkID)
		}
	}

	return result.Nodes, nil
}

// ExtractRelations 从文档和实体中提取关系
func (e *LLMExtractor) ExtractRelations(
	ctx context.Context,
	kbID, chunkID, document, query string,
	entities []*domain_knowledge.GraphNode,
	mode domain_knowledge.ExtractionMode,
) ([]*domain_knowledge.GraphRelation, error) {
	// 根据模式选择提示词模板
	var templateName string
	if mode == domain_knowledge.ExtractionModeQuery {
		templateName = "relation_extraction_query"
	} else {
		templateName = "relation_extraction"
	}

	promptTemplate, err := config.LoadPromptTemplate(templateName)
	if err != nil {
		return nil, fmt.Errorf("failed to load relation extraction template: %w", err)
	}

	// 构建实体列表 JSON（仅包含名称）
	entityNames := make([]string, 0, len(entities))
	for _, e := range entities {
		entityNames = append(entityNames, fmt.Sprintf(`"%s"`, e.Name))
	}
	entitiesList := fmt.Sprintf("[%s]", strings.Join(entityNames, ", "))

	// 替换占位符
	prompt := strings.Replace(promptTemplate, "{{entities}}", entitiesList, 1)
	prompt = strings.Replace(prompt, "{{document}}", document, 1)

	// 查询模式：添加查询信息
	if mode == domain_knowledge.ExtractionModeQuery && query != "" {
		prompt = strings.Replace(prompt, "{{query}}", query, 1)
	}

	log.Printf("[LLMExtractor] Relation extraction mode=%s prompt length: %d", mode, len(prompt))

	// 构建消息
	messages := []domainrag.LLMMessage{
		{Role: "user", Content: prompt},
	}

	// 调用 LLM
	response, err := e.llmChat.Chat(ctx, messages, &domainrag.ChatOptions{
		Temperature: 0.1,
		MaxTokens:   2000,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate relation extraction: %w", err)
	}

	log.Printf("[LLMExtractor] Relation extraction raw response: %s", response.Content)

	// 清理响应内容
	cleanedContent := cleanJSONResponse(response.Content)
	log.Printf("[LLMExtractor] Relation extraction cleaned response: %s", cleanedContent)

	// 解析 JSON 响应
	var result struct {
		Relations []*domain_knowledge.GraphRelation `json:"relations"`
	}
	if err := json.Unmarshal([]byte(cleanedContent), &result); err != nil {
		return nil, fmt.Errorf("failed to parse relation extraction response: %w, content: %s", err, cleanedContent)
	}

	// 验证 source 和 target 是否存在于实体列表中
	entitySet := make(map[string]bool)
	for _, entity := range entities {
		entitySet[entity.Name] = true
	}

	validRelations := make([]*domain_knowledge.GraphRelation, 0, len(result.Relations))
	for _, rel := range result.Relations {
		// 验证 source 和 target
		if !entitySet[rel.Source] {
			log.Printf("[LLMExtractor] Warning: relation source '%s' not found in entities", rel.Source)
			continue
		}
		if !entitySet[rel.Target] {
			log.Printf("[LLMExtractor] Warning: relation target '%s' not found in entities", rel.Target)
			continue
		}

		// 设置关系 ID
		if rel.ID == "" {
			rel.ID = uuid.New().String()
		}

		validRelations = append(validRelations, rel)
	}

	return validRelations, nil
}

// ExtractGraphJoint 联合提取实体和关系（单次 LLM 调用）
// 使用 graph_extraction.yaml 模板，一次调用同时提取实体和关系
func (e *LLMExtractor) ExtractGraphJoint(
	ctx context.Context,
	chunkID, document string,
	existingContext *domain_knowledge.ExtractionContext,
) ([]*domain_knowledge.GraphNode, []*domain_knowledge.GraphRelation, error) {
	promptTemplate, err := config.LoadPromptTemplate("graph_extraction")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load graph extraction template: %w", err)
	}

	// 替换文档占位符
	prompt := strings.Replace(promptTemplate, "{{document}}", document, 1)

	// 注入已有实体上下文（如果有）
	if existingContext != nil && len(existingContext.SampleEntities) > 0 {
		existingEntities := e.formatExistingEntities(existingContext)
		prompt = strings.Replace(prompt, "{{.existingEntities}}", existingEntities, 1)
	} else {
		// 移除可选的已有实体部分
		prompt = strings.Replace(prompt, "{{if .existingEntities}}", "", 1)
		prompt = strings.Replace(prompt, "{{.existingEntities}}", "", 1)
		prompt = strings.Replace(prompt, "{{end}}", "", 1)
	}

	log.Printf("[LLMExtractor] Joint extraction prompt length: %d", len(prompt))

	// 构建消息
	messages := []domainrag.LLMMessage{
		{Role: "user", Content: prompt},
	}

	// 调用 LLM
	response, err := e.llmChat.Chat(ctx, messages, &domainrag.ChatOptions{
		Temperature: 0.1,
		MaxTokens:   3000,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate joint extraction: %w", err)
	}

	log.Printf("[LLMExtractor] Joint extraction raw response: %s", response.Content)

	// 清理并解析响应
	cleanedContent := cleanJSONResponse(response.Content)
	log.Printf("[LLMExtractor] Joint extraction cleaned response: %s", cleanedContent)

	// 解析联合提取结果
	var result struct {
		Nodes     []*domain_knowledge.GraphNode     `json:"nodes"`
		Relations []*domain_knowledge.GraphRelation `json:"relations"`
	}
	if err := json.Unmarshal([]byte(cleanedContent), &result); err != nil {
		return nil, nil, fmt.Errorf("failed to parse joint extraction response: %w, content: %s", err, cleanedContent)
	}

	// 为每个节点添加 chunk_id 并分配 ID
	for _, node := range result.Nodes {
		if node.ID == "" {
			node.ID = uuid.New().String()
		}
		if !contains(node.Chunks, chunkID) {
			node.Chunks = append(node.Chunks, chunkID)
		}
	}

	// 验证并分配关系 ID
	entitySet := make(map[string]bool)
	for _, node := range result.Nodes {
		entitySet[node.Name] = true
	}

	validRelations := make([]*domain_knowledge.GraphRelation, 0, len(result.Relations))
	for _, rel := range result.Relations {
		// 验证 source 和 target
		if !entitySet[rel.Source] {
			log.Printf("[LLMExtractor] Warning: relation source '%s' not found in entities", rel.Source)
			continue
		}
		if !entitySet[rel.Target] {
			log.Printf("[LLMExtractor] Warning: relation target '%s' not found in entities", rel.Target)
			continue
		}

		// 验证关系类型
		if !isValidRelationType(rel.Type) {
			log.Printf("[LLMExtractor] Warning: invalid relation type '%s', using RELATED_TO", rel.Type)
			rel.Type = "RELATED_TO"
		}

		// 设置关系 ID
		if rel.ID == "" {
			rel.ID = uuid.New().String()
		}

		validRelations = append(validRelations, rel)
	}

	log.Printf("[LLMExtractor] Joint extraction result: %d nodes, %d relations", len(result.Nodes), len(validRelations))
	return result.Nodes, validRelations, nil
}

// ExtractGraphIncremental 增量提取实体和关系（基于已有图谱上下文）
// 使用 graph_incremental.yaml 模板，注入已有实体和关系上下文
func (e *LLMExtractor) ExtractGraphIncremental(
	ctx context.Context,
	chunkID, document string,
	existingContext *domain_knowledge.ExtractionContext,
) ([]*domain_knowledge.GraphNode, []*domain_knowledge.GraphRelation, error) {
	if existingContext == nil {
		// 如果没有上下文，回退到联合提取
		return e.ExtractGraphJoint(ctx, chunkID, document, nil)
	}

	promptTemplate, err := config.LoadPromptTemplate("graph_incremental")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load graph incremental template: %w", err)
	}

	// 格式化上下文信息
	entityTypeDist := e.formatEntityTypeDistribution(existingContext.EntityTypes)
	sampleEntities := e.formatSampleEntities(existingContext.SampleEntities)
	relationTypeDist := e.formatRelationTypeDistribution(existingContext.RelationTypes)

	// 替换占位符
	prompt := strings.Replace(promptTemplate, "{{document}}", document, 1)
	prompt = strings.Replace(prompt, "{{.entityTypeDistribution}}", entityTypeDist, 1)
	prompt = strings.Replace(prompt, "{{.sampleEntities}}", sampleEntities, 1)
	prompt = strings.Replace(prompt, "{{.relationTypeDistribution}}", relationTypeDist, 1)

	log.Printf("[LLMExtractor] Incremental extraction prompt length: %d", len(prompt))

	// 构建消息
	messages := []domainrag.LLMMessage{
		{Role: "user", Content: prompt},
	}

	// 调用 LLM
	response, err := e.llmChat.Chat(ctx, messages, &domainrag.ChatOptions{
		Temperature: 0.1,
		MaxTokens:   3000,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate incremental extraction: %w", err)
	}

	log.Printf("[LLMExtractor] Incremental extraction raw response: %s", response.Content)

	// 清理并解析响应
	cleanedContent := cleanJSONResponse(response.Content)
	log.Printf("[LLMExtractor] Incremental extraction cleaned response: %s", cleanedContent)

	// 解析增量提取结果
	var result struct {
		NewNodes     []*domain_knowledge.GraphNode     `json:"new_nodes"`
		UpdatedNodes []*domain_knowledge.GraphNode     `json:"updated_nodes"`
		NewRelations []*domain_knowledge.GraphRelation `json:"new_relations"`
	}
	if err := json.Unmarshal([]byte(cleanedContent), &result); err != nil {
		return nil, nil, fmt.Errorf("failed to parse incremental extraction response: %w, content: %s", err, cleanedContent)
	}

	// 处理新节点
	for _, node := range result.NewNodes {
		if node.ID == "" {
			node.ID = uuid.New().String()
		}
		if !contains(node.Chunks, chunkID) {
			node.Chunks = append(node.Chunks, chunkID)
		}
		// 验证实体类型
		if !isValidEntityType(node.EntityType) {
			log.Printf("[LLMExtractor] Warning: invalid entity type '%s', using Other", node.EntityType)
			node.EntityType = "Other"
		}
	}

	// 合并新节点和更新节点
	allNodes := append(result.NewNodes, result.UpdatedNodes...)

	// 验证关系
	validRelations := make([]*domain_knowledge.GraphRelation, 0, len(result.NewRelations))
	for _, rel := range result.NewRelations {
		// 验证关系类型
		if !isValidRelationType(rel.Type) {
			log.Printf("[LLMExtractor] Warning: invalid relation type '%s', using RELATED_TO", rel.Type)
			rel.Type = "RELATED_TO"
		}

		// 设置关系 ID
		if rel.ID == "" {
			rel.ID = uuid.New().String()
		}

		validRelations = append(validRelations, rel)
	}

	log.Printf("[LLMExtractor] Incremental extraction result: %d new nodes, %d updated nodes, %d new relations",
		len(result.NewNodes), len(result.UpdatedNodes), len(validRelations))

	return allNodes, validRelations, nil
}

// ========================================
// Helper Functions
// ========================================

// formatExistingEntities 格式化已有实体为提示词上下文
func (e *LLMExtractor) formatExistingEntities(ctx *domain_knowledge.ExtractionContext) string {
	if len(ctx.SampleEntities) == 0 {
		return "暂无已有实体"
	}

	var sb strings.Builder
	sb.WriteString("已有实体类型分布：\n")
	for entityType, count := range ctx.EntityTypes {
		sb.WriteString(fmt.Sprintf("- %s: %d\n", entityType, count))
	}
	sb.WriteString("\n高频实体示例：\n")
	for i, entity := range ctx.SampleEntities {
		if i >= 10 { // 限制显示数量
			break
		}
		sb.WriteString(fmt.Sprintf("- %s (%s): %v\n", entity.Name, entity.EntityType, entity.Attributes))
	}
	return sb.String()
}

// formatEntityTypeDistribution 格式化实体类型分布
func (e *LLMExtractor) formatEntityTypeDistribution(dist map[string]int) string {
	if len(dist) == 0 {
		return "暂无数据"
	}
	var parts []string
	for entityType, count := range dist {
		parts = append(parts, fmt.Sprintf("%s:%d", entityType, count))
	}
	return strings.Join(parts, ", ")
}

// formatSampleEntities 格式化示例实体
func (e *LLMExtractor) formatSampleEntities(samples []domain_knowledge.EntitySample) string {
	if len(samples) == 0 {
		return "暂无示例"
	}
	var entities []string
	for i, sample := range samples {
		if i >= 20 { // 限制数量
			break
		}
		entities = append(entities, fmt.Sprintf("%s(%s)", sample.Name, sample.EntityType))
	}
	return strings.Join(entities, ", ")
}

// formatRelationTypeDistribution 格式化关系类型分布
func (e *LLMExtractor) formatRelationTypeDistribution(dist map[string]int) string {
	if len(dist) == 0 {
		return "暂无数据"
	}
	var parts []string
	for relType, count := range dist {
		parts = append(parts, fmt.Sprintf("%s:%d", relType, count))
	}
	return strings.Join(parts, ", ")
}

// isValidRelationType 验证关系类型是否有效
func isValidRelationType(relationType string) bool {
	validTypes := map[string]bool{
		"CONTAINS":     true,
		"RELATED_TO":   true,
		"DEPENDS_ON":   true,
		"PART_OF":      true,
		"SIMILAR_TO":   true,
		"CAUSES":       true,
		"LOCATED_IN":   true,
		"BELONGS_TO":   true,
		"CONNECTED_TO": true,
		"PRECEDES":     true,
		"FOLLOWS":      true,
	}
	return validTypes[relationType]
}

// isValidEntityType 验证实体类型是否有效
func isValidEntityType(entityType string) bool {
	validTypes := map[string]bool{
		"Person":       true,
		"Organization": true,
		"Product":      true,
		"Technology":   true,
		"Concept":      true,
		"Document":     true,
		"Project":      true,
		"Location":     true,
		"Event":        true,
		"Time":         true,
		"Other":        true,
	}
	return validTypes[entityType]
}

// cleanJSONResponse 清理 LLM 响应中的 JSON 内容。
//
// LLM 经常在 JSON 前后附带说明文字或 markdown 代码块标记，例如：
//
//	Here's the result:
//	```json
//	{"nodes": []}
//	```
//
// 本函数定位响应中第一个 JSON 起始符（`{` 或 `[`）到对应的最后一个结束符
// （`}` 或 `]`），从而剥离任意前导/尾随文本与代码围栏，返回纯 JSON 串。
func cleanJSONResponse(response string) string {
	response = strings.TrimSpace(response)

	// 找到第一个 JSON 起始符：取 '{' 与 '[' 中位置更靠前的那个。
	objStart := strings.IndexByte(response, '{')
	arrStart := strings.IndexByte(response, '[')

	var start int
	var closeChar byte
	switch {
	case objStart >= 0 && (arrStart < 0 || objStart < arrStart):
		start, closeChar = objStart, '}'
	case arrStart >= 0:
		start, closeChar = arrStart, ']'
	default:
		// 未发现 JSON 结构，原样返回（交由上层解析报错）。
		return response
	}

	// 匹配最后一个对应的结束符，剥离尾随文本与代码围栏。
	end := strings.LastIndexByte(response, closeChar)
	if end > start {
		return strings.TrimSpace(response[start : end+1])
	}

	return strings.TrimSpace(response[start:])
}

// contains 检查字符串切片是否包含指定元素
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
