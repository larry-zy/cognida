# GenUI 实现完成说明

## 已创建文件

### Domain 层
- `internal/domain/agent/genui/entity.go` - UI Schema 定义
- `internal/domain/agent/genui/generator.go` - UI 生成器

### Infrastructure 层
- `internal/infrastructure/search/web.go` - 联网搜索工具

### Application 层
- `internal/application/agent/genui/usecase.go` - GenUI 用例

### Interface 层
- `internal/interface/http/handler/genui_handler.go` - GenUI Handler

---

## 需要手动集成的部分

### 1. 更新 router.go

在 `internal/interface/http/router/router.go` 中：

**① 添加 Handler 字段** (已完成)
```go
type Router struct {
    // ... 其他字段
    genuiHandler *handler.GenUIHandler  // 已添加
}
```

**② 更新 NewRouter 参数** (已完成)
```go
func NewRouter(
    // ... 其他参数
    genuiHandler *handler.GenUIHandler,  // 已添加
    // ...
)
```

**③ 在 Setup() 中添加路由调用**
在认证路由组中添加：
```go
auth := api.Group("")
auth.Use(r.authMiddleware.Apply())
{
    // ... 其他路由
    r.setupEvaluationRoutes(auth)
    r.setupGenUIRoutes(auth)  // 添加这一行
}
```

**④ 添加 setupGenUIRoutes 函数**
```go
// setupGenUIRoutes 设置 GenUI 路由
func (r *Router) setupGenUIRoutes(api *gin.RouterGroup) {
    genui := api.Group("/genui")
    {
        genui.POST("", r.genuiHandler.Generate)
    }
}
```

### 2. 更新依赖注入

在 `cmd/server/main.go` 或类似的启动文件中：

```go
// 创建联网搜索器
webSearcher := search.NewTavilySearcher(os.Getenv("TAVILY_API_KEY"))
// 或者使用 Mock 测试：
// webSearcher := search.NewMockSearcher()

// 创建 GenUI 用例
genuiUseCase := genuiuc.NewUseCase(chatModel, webSearcher)

// 创建 GenUI Handler
genuiHandler := handler.NewGenUIHandler(genuiUseCase)

// 传递给 Router
router := router.NewRouter(
    // ... 其他 handlers
    genuiHandler,
    // ... 其他参数
)
```

---

## API 使用示例

### 请求
```bash
curl -X POST http://localhost:8080/api/v1/genui \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{
    "query": "分析一下 2024 年 AI 行业的发展趋势",
    "options": {
      "enable_web_search": true,
      "max_search_results": 5
    }
  }'
```

### 响应示例
```json
{
  "success": true,
  "data": {
    "type": "ui_response",
    "layout": "vertical",
    "components": [
      {
        "type": "markdown",
        "props": {
          "content": "# 2024 年 AI 行业分析\n\n基于联网搜索结果..."
        }
      },
      {
        "type": "card",
        "props": {
          "title": "核心数据",
          "items": [
            {"label": "市场规模", "value": "$500B", "trend": "+25%"},
            {"label": "增长率", "value": "35%", "trend": "up"}
          ]
        }
      },
      {
        "type": "chart",
        "props": {
          "chart_type": "line",
          "title": "增长趋势",
          "data": {
            "labels": ["Q1", "Q2", "Q3", "Q4"],
            "series": [
              {"name": "2024", "data": [100, 150, 200, 280]}
            ]
          }
        }
      }
    ],
    "metadata": {
      "timestamp": "2024-01-01T00:00:00Z",
      "sources": [
        {"type": "web", "url": "https://example.com", "title": "来源1"}
      ]
    }
  }
}
```

---

## 前端集成

前端根据 `components` 数组动态渲染组件：

```typescript
function renderUIComponent(component: UIComponent) {
  switch (component.type) {
    case 'markdown':
      return <Markdown content={component.props.content} />;
    case 'card':
      return <Card title={component.props.title} items={component.props.items} />;
    case 'chart':
      return <Chart type={component.props.chart_type} data={component.props.data} />;
    case 'data_table':
      return <DataTable columns={component.props.columns} rows={component.props.rows} />;
    // ... 其他组件
  }
}
```

---

## 环境变量

```bash
# Tavily API (可选，不配置则使用 Mock)
TAVILY_API_KEY=tvly-xxx
```
