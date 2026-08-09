// Package graph LLM 提取器测试
package graph

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain_knowledge "cognida/internal/model/knowledge"
)

// ========================================
// Clean JSON Response Tests
// ========================================

func TestCleanJSONResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{
			name:     "Clean JSON",
			response: `{"nodes":[]}`,
			want:     `{"nodes":[]}`,
		},
		{
			name:     "JSON with markdown code block",
			response: "```json\n{\"nodes\":[]}\n```",
			want:     `{"nodes":[]}`,
		},
		{
			name:     "JSON with generic code block",
			response: "```\n{\"nodes\":[]}\n```",
			want:     `{"nodes":[]}`,
		},
		{
			name:     "JSON with prefix text",
			response: "结果：{\"nodes\":[]}",
			want:     `{"nodes":[]}`,
		},
		{
			name:     "JSON with trailing text",
			response: `{"nodes":[]}`,
			want:     `{"nodes":[]}`,
		},
		{
			name:     "Complex JSON with markdown",
			response: "Here's the result:\n```json\n{\"nodes\":[{\"id\":\"1\",\"name\":\"Test\"}],\"relations\":[]}\n```",
			want:     `{"nodes":[{"id":"1","name":"Test"}],"relations":[]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanJSONResponse(tt.response)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ========================================
// Validation Tests
// ========================================

func TestIsValidRelationType(t *testing.T) {
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

	tests := []struct {
		name    string
		relType string
		want    bool
	}{
		{"Valid CONTAINS", "CONTAINS", true},
		{"Valid RELATED_TO", "RELATED_TO", true},
		{"Valid DEPENDS_ON", "DEPENDS_ON", true},
		{"Invalid type", "INVALID_TYPE", false},
		{"Empty type", "", false},
		{"Lowercase", "contains", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validTypes[tt.relType]
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsValidEntityType(t *testing.T) {
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

	tests := []struct {
		name       string
		entityType string
		want       bool
	}{
		{"Valid Person", "Person", true},
		{"Valid Technology", "Technology", true},
		{"Valid Other", "Other", true},
		{"Invalid type", "InvalidType", false},
		{"Empty type", "", false},
		{"Lowercase", "person", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validTypes[tt.entityType]
			assert.Equal(t, tt.want, got)
		})
	}
}

// ========================================
// Context Formatting Tests
// ========================================

func TestFormatExistingEntities(t *testing.T) {
	extractor := &LLMExtractor{}

	ctx := &domain_knowledge.ExtractionContext{
		EntityTypes: map[string]int{
			"Technology":   10,
			"Organization": 5,
		},
		SampleEntities: []domain_knowledge.EntitySample{
			{Name: "Python", EntityType: "Technology", Attributes: []string{"language"}},
			{Name: "Django", EntityType: "Technology", Attributes: []string{"framework"}},
		},
	}

	result := extractor.formatExistingEntities(ctx)
	assert.Contains(t, result, "Technology: 10")
	assert.Contains(t, result, "Organization: 5")
	assert.Contains(t, result, "Python")
	assert.Contains(t, result, "Django")
}

func TestFormatEntityTypeDistribution(t *testing.T) {
	extractor := &LLMExtractor{}

	dist := map[string]int{
		"Technology":   10,
		"Organization": 5,
		"Concept":      3,
	}

	result := extractor.formatEntityTypeDistribution(dist)
	assert.Contains(t, result, "Technology:10")
	assert.Contains(t, result, "Organization:5")
	assert.Contains(t, result, "Concept:3")
}

func TestFormatSampleEntities(t *testing.T) {
	extractor := &LLMExtractor{}

	samples := []domain_knowledge.EntitySample{
		{Name: "Python", EntityType: "Technology", Attributes: []string{"language"}},
		{Name: "Django", EntityType: "Technology", Attributes: []string{"framework"}},
		{Name: "Google", EntityType: "Organization", Attributes: []string{"company"}},
	}

	result := extractor.formatSampleEntities(samples)
	assert.Contains(t, result, "Python(Technology)")
	assert.Contains(t, result, "Django(Technology)")
	assert.Contains(t, result, "Google(Organization)")
}

func TestFormatRelationTypeDistribution(t *testing.T) {
	extractor := &LLMExtractor{}

	dist := map[string]int{
		"RELATED_TO":  15,
		"DEPENDS_ON":  8,
		"CONTAINS":    5,
	}

	result := extractor.formatRelationTypeDistribution(dist)
	assert.Contains(t, result, "RELATED_TO:15")
	assert.Contains(t, result, "DEPENDS_ON:8")
	assert.Contains(t, result, "CONTAINS:5")
}

// ========================================
// Empty Context Tests
// ========================================

func TestFormatEmptyContext(t *testing.T) {
	extractor := &LLMExtractor{}

	t.Run("Empty entity types", func(t *testing.T) {
		result := extractor.formatEntityTypeDistribution(map[string]int{})
		assert.Equal(t, "暂无数据", result)
	})

	t.Run("Empty sample entities", func(t *testing.T) {
		result := extractor.formatSampleEntities([]domain_knowledge.EntitySample{})
		assert.Equal(t, "暂无示例", result)
	})

	t.Run("Empty relation types", func(t *testing.T) {
		result := extractor.formatRelationTypeDistribution(map[string]int{})
		assert.Equal(t, "暂无数据", result)
	})
}

// ========================================
// JSON Parsing Tests
// ========================================

func TestParseJointExtractionResponse(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantErr bool
		nodes   int
		rels    int
	}{
		{
			name:    "Valid response",
			jsonStr: `{"nodes":[{"id":"1","name":"Test","entity_type":"Technology","chunks":[]}],"relations":[{"id":"r1","source":"Test","target":"Other","type":"RELATED_TO"}]}`,
			wantErr: false,
			nodes:   1,
			rels:    1,
		},
		{
			name:    "Empty response",
			jsonStr: `{"nodes":[],"relations":[]}`,
			wantErr: false,
			nodes:   0,
			rels:    0,
		},
		{
			name:    "Invalid JSON",
			jsonStr: `{invalid json}`,
			wantErr: true,
		},
		{
			// encoding/json 不要求字段存在，缺失的 nodes 解析为空切片而非报错。
			name:    "Missing nodes field",
			jsonStr: `{"relations":[]}`,
			wantErr: false,
			nodes:   0,
			rels:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result struct {
				Nodes     []*domain_knowledge.GraphNode    `json:"nodes"`
				Relations []*domain_knowledge.GraphRelation `json:"relations"`
			}
			err := json.Unmarshal([]byte(tt.jsonStr), &result)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, result.Nodes, tt.nodes)
				assert.Len(t, result.Relations, tt.rels)
			}
		})
	}
}

func TestParseIncrementalExtractionResponse(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantErr bool
		newNodes    int
		updatedNodes int
		newRels     int
	}{
		{
			name:    "Valid response",
			jsonStr: `{"new_nodes":[{"id":"1","name":"NewEntity","entity_type":"Technology","chunks":[]}],"updated_nodes":[{"id":"existing_1","name":"OldEntity","entity_type":"Technology","new_attributes":["new"]}],"new_relations":[{"id":"r1","source":"NewEntity","target":"OldEntity","type":"RELATED_TO"}]}`,
			wantErr: false,
			newNodes:    1,
			updatedNodes: 1,
			newRels:     1,
		},
		{
			name:    "Empty response",
			jsonStr: `{"new_nodes":[],"updated_nodes":[],"new_relations":[]}`,
			wantErr: false,
			newNodes:    0,
			updatedNodes: 0,
			newRels:     0,
		},
		{
			name:    "Invalid JSON",
			jsonStr: `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result struct {
				NewNodes      []*domain_knowledge.GraphNode    `json:"new_nodes"`
				UpdatedNodes  []*domain_knowledge.GraphNode    `json:"updated_nodes"`
				NewRelations  []*domain_knowledge.GraphRelation `json:"new_relations"`
			}
			err := json.Unmarshal([]byte(tt.jsonStr), &result)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, result.NewNodes, tt.newNodes)
				assert.Len(t, result.UpdatedNodes, tt.updatedNodes)
				assert.Len(t, result.NewRelations, tt.newRels)
			}
		})
	}
}

// ========================================
// Edge Case Tests
// ========================================

func TestContains(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		item  string
		want  bool
	}{
		{
			name:  "Item exists",
			slice: []string{"a", "b", "c"},
			item:  "b",
			want:  true,
		},
		{
			name:  "Item does not exist",
			slice: []string{"a", "b", "c"},
			item:  "d",
			want:  false,
		},
		{
			name:  "Empty slice",
			slice: []string{},
			item:  "a",
			want:  false,
		},
		{
			name:  "Nil slice",
			slice: nil,
			item:  "a",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.slice, tt.item)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractionContextEdgeCases(t *testing.T) {
	t.Run("Nil context", func(t *testing.T) {
		extractor := &LLMExtractor{}
		result := extractor.formatEntityTypeDistribution(nil)
		assert.Equal(t, "暂无数据", result)
	})

	t.Run("Large sample entities truncated", func(t *testing.T) {
		extractor := &LLMExtractor{}
		samples := make([]domain_knowledge.EntitySample, 25)
		for i := 0; i < 25; i++ {
			samples[i] = domain_knowledge.EntitySample{
				Name:       "Entity" + strconv.Itoa(i),
				EntityType: "Technology",
			}
		}

		result := extractor.formatSampleEntities(samples)
		// Should only show first 20
		assert.Contains(t, result, "Entity0")
		assert.Contains(t, result, "Entity19")
	})
}
