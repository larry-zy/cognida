# Agent API 参考

## 核心 API

### Agent 接口

```go
type Agent interface {
    // Chat 处理消息并返回完整响应
    Chat(ctx context.Context, message string) (*Response, error)

    // Stream 流式处理消息
    Stream(ctx context.Context, message string) (<-chan *Chunk, error)

    // Name 返回Agent名称
    Name() string
}
```

### Response 结构

```go
type Response struct {
    // Content 响应文本内容
    Content string

    // ToolCalls 工具调用记录
    ToolCalls []*ToolCall

    // Metadata 元数据
    Metadata map[string]interface{}
}
```

### ToolCall 结构

```go
type ToolCall struct {
    // Name 工具名称
    Name string

    // Input 工具输入参数
    Input map[string]interface{}

    // Output 工具输出结果
    Output string

    // Error 执行错误
    Error error
}
```

### Chunk 结构

```go
type Chunk struct {
    // Content 部分内容
    Content string

    // Done 是否为最后一块
    Done bool

    // Metadata 元数据
    Metadata map[string]interface{}
}
```

## Builder API

### New 函数

```go
func New(chatModel model.ChatModel) *Builder
```

创建一个新的Builder实例。

### Builder 方法

#### Name

```go
func (b *Builder) Name(name string) *Builder
```
设置Agent名称。

#### Prompt

```go
func (b *Builder) Prompt(prompt string) *Builder
```
设置系统提示词。

#### WithToolModel

```go
func (b *Builder) WithToolModel(toolModel model.ToolCallingChatModel) *Builder
```
设置支持工具调用的模型。

#### Tools

```go
func (b *Builder) Tools(tools ...tool.BaseTool) *Builder
```
手动指定工具列表。

#### ToolsAutoSelect

```go
func (b *Builder) ToolsAutoSelect() *Builder
```
启用LLM自动工具选择。

#### WithRegistry

```go
func (b *Builder) WithRegistry(registry ToolRegistry) *Builder
```
设置工具注册表。

#### Before / After

```go
func (b *Builder) Before(hook BeforeHook) *Builder
func (b *Builder) After(hook AfterHook) *Builder
```
添加前置/后置钩子。

#### Middleware

```go
func (b *Builder) Middleware(mw ...Middleware) *Builder
```
添加中间件。

#### WithMaxIterations

```go
func (b *Builder) WithMaxIterations(maxIter int) *Builder
```
设置工具调用的最大迭代次数。

#### Build

```go
func (b *Builder) Build(ctx context.Context) (Agent, error)
```
构建Agent实例。

## 编排 API

### Sequential

```go
func Sequential(agents ...Agent) Agent
```
创建串行编排的Agent。

### Parallel

```go
func Parallel(agents ...Agent) Agent
```
创建并行编排的Agent。

### Conditional

```go
func Conditional(predicate func(string) bool, trueAgent, falseAgent Agent) Agent
func Branch(predicate func(string) bool, trueAgent, falseAgent Agent) Agent
```
创建条件路由的Agent。

### Loop

```go
func Loop(bodyAgent Agent, continuation func(*Response) bool) Agent
```
创建循环执行的Agent。

### Supervisor

```go
func Supervisor(coordinator Agent, workers ...Agent) Agent
func NamedSupervisor(coordinator Agent, workers map[string]Agent) Agent
```
创建主管模式的Agent。

### Func

```go
func Func(fn func(context.Context, string) (*Response, error)) Agent
```
从函数创建Agent。

## 中间件 API

### Middleware 接口

```go
type Middleware interface {
    // Before 请求前处理
    Before(ctx context.Context, message string) (context.Context, string, error)

    // After 响应后处理
    After(ctx context.Context, resp *Response) error
}
```

### 内置中间件

#### LoggingMiddleware

```go
func NewLoggingMiddleware(w io.Writer, prefix string) Middleware
```
创建日志中间件。

#### MetricsMiddleware

```go
func NewMetricsMiddleware() Middleware
```
创建指标中间件。

### 辅助函数

#### Chain

```go
func Chain(middlewares ...Middleware) []Middleware
```
组合多个中间件。

## 工具 API

### ToolRegistry 接口

```go
type ToolRegistry interface {
    GetTools() []tool.BaseTool
    GetToolsByNames(names []string) ([]tool.BaseTool, error)
    List() []string
}
```

### GetDefaultRegistry

```go
func GetDefaultRegistry() ToolRegistry
```
获取默认工具注册表。

## 类型定义

### BeforeHook

```go
type BeforeHook func(ctx context.Context, message string) (context.Context, string, error)
```

请求前钩子函数。

### AfterHook

```go
type AfterHook func(ctx context.Context, resp *Response) error
```

响应后钩子函数。
