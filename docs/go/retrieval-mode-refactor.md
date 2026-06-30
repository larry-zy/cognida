# 知识库检索模式重构文档

## 概述

将知识库的检索模式从单选改为多选（标签选择）方式，向量检索为默认必选项。

## 设计原则

- **向量检索**：默认必选，不可取消勾选
- **BM25 关键词检索**：可选
- **图谱检索**：可选

## 前端实现

### 1. UI 改造

**文件**：`web/src/views/knowledge/KnowledgeBaseList.vue`

**原单选方式**：
```vue
<el-select v-model="formData.retrieval_mode">
  <el-option label="向量检索" value="vector" />
  <el-option label="BM25 关键词检索" value="bm25" />
  <el-option label="混合检索" value="hybrid" />
  <el-option label="图谱检索" value="graph" />
</el-select>
```

**现多选标签方式**：
```vue
<el-checkbox-group v-model="formData.retrieval_modes">
  <el-checkbox label="vector" :disabled="true">
    <el-tag type="primary" effect="plain">向量检索</el-tag>
    <span class="tag-tip">基于向量相似度检索（默认必选）</span>
  </el-checkbox>
  <el-checkbox label="bm25">
    <el-tag type="success" effect="plain">BM25 关键词检索</el-tag>
    <span class="tag-tip">基于关键词匹配检索</span>
  </el-checkbox>
  <el-checkbox label="graph">
    <el-tag type="warning" effect="plain">图谱检索</el-tag>
    <span class="tag-tip">基于知识图谱关系检索</span>
  </el-checkbox>
</el-checkbox-group>
```

### 2. 数据结构变更

```typescript
// 原单选
const formData = reactive<CreateKnowledgeBaseRequest>({
  retrieval_mode: 'vector',
  ...
})

// 现多选
const formData = reactive<CreateKnowledgeBaseRequest & {
  retrieval_modes: ['vector']
}>({
  retrieval_modes?: string[]
})
```

### 3. 保存时转换逻辑

```typescript
async function saveKnowledgeBase() {
  const request: any = {
    // 检索配置 - 将 retrieval_modes 数组转换为后端格式
    retrieval_mode: formData.retrieval_modes.includes('bm25') ? 'hybrid' : 'vector',
    graph_enabled: formData.retrieval_modes.includes('graph'),
    ...
  }
  ...
}
```

### 4. 加载时还原逻辑

```typescript
async function loadKnowledgeBaseData(id: string) {
  const res = await knowledgeApi.getDetail(id)
  const data = res.data

  // 检索配置 - 转换为多选数组
  const modes = ['vector']
  if (data.setting?.retrieval_mode?.includes('bm25') || data.setting?.graph_enabled) {
    if (data.setting?.retrieval_mode?.includes('bm25')) {
      modes.push('bm25')
    }
    if (data.setting?.graph_enabled) {
      modes.push('graph')
    }
  }
  formData.retrieval_modes = modes
}
```

## 后端实现

### 1. 类型定义

**文件**：`internal/types/kb.go`

#### CreateKnowledgeBaseRequest 新增字段

```go
type CreateKnowledgeBaseRequest struct {
    // ... 其他字段
    // 检索模式：支持前端多选（向量、BM25、图谱），后端自动转换
    RetrievalModes   []string       `json:"retrieval_modes,omitempty"`
    RetrievalMode    string         `json:"retrieval_mode,omitempty"` // 向后兼容
    // ... 其他字段
}
```

#### UpdateKnowledgeBaseRequest 新增字段

```go
type UpdateKnowledgeBaseRequest struct {
    // ... 其他字段
    // 检索模式：支持前端多选（向量、BM25、图谱），后端自动转换
    RetrievalModes   []string       `json:"retrieval_modes,omitempty"`
    RetrievalMode    *string        `json:"retrieval_mode,omitempty"` // 向后兼容
    // ... 其他字段
}
```

### 2. Handler 处理逻辑

**文件**：`internal/handler/knowledge_base.go`

#### Create 方法添加处理逻辑

```go
// 处理检索模式：如果前端传了 retrieval_modes 数组，则转换为后端字段
retrievalMode := "vector" // 默认为向量检索
graphEnabled := false

if len(req.RetrievalModes) > 0 {
    // 检查数组中包含哪些检索模式
    hasBM25 := false
    hasGraph := false
    for _, mode := range req.RetrievalModes {
        switch mode {
        case "vector":
            // 向量检索始终启用，是默认选项
        case "bm25":
            hasBM25 = true
        case "graph":
            hasGraph = true
        }
    }
    // 根据 selection 确定 retrieval_mode：如果有 BM25 则为 hybrid，否则为 vector
    if hasBM25 {
        retrievalMode = "hybrid"
    }
    graphEnabled = hasGraph
} else if req.RetrievalMode != "" {
    // 向后兼容：如果直接传了 retrieval_mode
    retrievalMode = req.RetrievalMode
    graphEnabled = req.GraphEnabled
}

setting := &types.KBSetting{
    KBID:                  kbID,
    RetrievalMode:       retrievalMode,
    SimilarityThreshold: req.SimilarityThreshold,
    TopK:                req.TopK,
    RerankEnabled:       req.RerankEnabled,
    GraphEnabled:        graphEnabled,
    // ...
}
```

#### Update 方法添加处理逻辑

```go
// 处理检索模式
if len(req.RetrievalModes) > 0 {
    // 检查数组中包含哪些检索模式
    hasBM25 := false
    hasGraph := false
    for _, mode := range req.RetrievalModes {
        switch mode {
        case "vector":
            // 向量检索始终启用
        case "bm25":
            hasBM25 = true
        case "graph":
            hasGraph = true
        }
    }
    // 根据 selection 确定 retrieval_mode：如果有 BM25 则为 hybrid，否则为 vector
    retrievalMode := "vector"
    if hasBM25 {
        retrievalMode = "hybrid"
    }
    setting.RetrievalMode = retrievalMode
    setting.GraphEnabled = hasGraph
} else {
    // 向后兼容：使用单独的字段
    if req.RetrievalMode != nil {
        setting.RetrievalMode = req.RetrievalMode
    }
    if req.GraphEnabled != nil {
        setting.GraphEnabled = req.GraphEnabled
    }
}
```

#### GetDetail 方法添加 retrieval_modes 响应

```go
func (h *KnowledgeBaseHandler) GetDetail(c *gin.Context) {
    id := c.Param("id")
    kb, err := h.kbBaseService.FindByIDWithSettings(c.Request.Context(), id)
    // ...

    // 构造响应数据，添加 retrieval_modes 数组供前端使用
    responseData := gin.H{
        "code":    0,
        "message": "success",
        "data":    kb,
    }

    // 如果有设置，添加 retrieval_modes 数组
    if kb.Setting != nil {
        retrievalModes := []string{"vector"}
        if kb.Setting.RetrievalMode == "hybrid" {
            retrievalModes = append(retrievalModes, "bm25")
        }
        if kb.Setting.GraphEnabled {
            retrievalModes = append(retrievalModes, "graph")
        }
        responseData["data"] = gin.H{
            // ... 其他字段
            "setting":       kb.Setting,
            "retrieval_modes": retrievalModes,
        }
    }

    c.JSON(200, responseData)
}
```

## 数据转换映射表

| 前端 retrieval_modes | 后端 retrieval_mode | 后端 graph_enabled |
|---------------------|---------------------|-------------------|
| `["vector"]` | `"vector"` | `false` |
| `["vector", "bm25"]` | `"hybrid"` | `false` |
| `["vector", "graph"]` | `"vector"` | `true` |
| `["vector", "bm25", "graph"]` | `"hybrid"` | `true` |

## 数据库存储

### kb_settings 表

```sql
CREATE TABLE `kb_settings` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `kb_id` varchar(36) NOT NULL,
  `retrieval_mode` varchar(20) DEFAULT 'vector',  -- 存储为 vector/hybrid
  `graph_enabled` tinyint(1) DEFAULT 0,          -- 存储是否启用图谱
  ...
  PRIMARY KEY (`id`),
  UNIQUE INDEX `uk_kb_id`(`kb_id`)
);
```

## API 变更

### POST /api/v1/knowledge-bases (创建)

**请求体**：
```json
{
  "name": "示例知识库",
  "retrieval_modes": ["vector", "bm25", "graph"]
}
```

**后端处理**：
- retrieval_mode → "hybrid"
- graph_enabled → true

### PUT /api/v1/knowledge-bases/:id (更新)

**请求体**：
```json
{
  "name": "更新名称",
  "retrieval_modes": ["vector", "graph"]
}
```

**后端处理**：
- retrieval_mode → "vector"
- graph_enabled → true

### GET /api/v1/knowledge-bases/:id (详情)

**响应体**：
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "kb-id",
    "name": "示例知识库",
    "setting": {
      "retrieval_mode": "hybrid",
      "graph_enabled": true,
      ...
    },
    "retrieval_modes": ["vector", "bm25", "graph"]
  }
}
```

## 兼容性说明

1. **向后兼容**：保留 `retrieval_mode` 和 `graph_enabled` 字段支持
2. **前端优先**：如果传了 `retrieval_modes` 数组，优先使用该字段
3. **降级处理**：如果 `retrieval_modes` 为空，使用 `retrieval_mode` 和 `graph_enabled` 单独字段
