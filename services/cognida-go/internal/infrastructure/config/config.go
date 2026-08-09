package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
	"cognida/internal/model/common"
)

// ========================================
// 非密配置文件（config.yaml）加载
// ========================================
//
// 加载优先级：代码内兜底默认 < config.yaml（非密真源）< 环境变量（覆盖，且是密钥唯一来源）。
// FileConfig 只包含【非密】字段（有 yaml key）；密钥字段没有 yaml key，只从 env 读。

// FileConfig 映射 config.yaml 的非密配置。所有字段均为非密项。
type FileConfig struct {
	Database struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		User     string `yaml:"user"`
		Database string `yaml:"database"`
	} `yaml:"database"`
	Milvus struct {
		Host string `yaml:"host"`
	} `yaml:"milvus"`
	Neo4j struct {
		URI         string `yaml:"uri"`
		Username    string `yaml:"username"`
		MaxPoolSize int    `yaml:"max_pool_size"`
	} `yaml:"neo4j"`
	JWT struct {
		AccessTokenExpire  int `yaml:"access_expire"`
		RefreshTokenExpire int `yaml:"refresh_expire"`
	} `yaml:"jwt"`
	Tenant struct {
		EnableMultiTenant       bool  `yaml:"enable_multi_tenant"`
		EnableCrossTenantAccess bool  `yaml:"enable_cross_tenant_access"`
		DefaultStorageQuota     int64 `yaml:"default_storage_quota"`
	} `yaml:"tenant"`
	Chat struct {
		Source    string `yaml:"source"`
		BaseURL   string `yaml:"base_url"`
		ModelName string `yaml:"model_name"`
		Provider  string `yaml:"provider"`
	} `yaml:"chat"`
	Search struct {
		APIEndpoint string `yaml:"api_endpoint"`
	} `yaml:"search"`
	Embedding struct {
		Provider string `yaml:"provider"`
		Model    string `yaml:"model"`
		BaseURL  string `yaml:"base_url"`
	} `yaml:"embedding"`
	Server struct {
		Port string `yaml:"port"`
		Mode string `yaml:"mode"`
		Host string `yaml:"host"`
	} `yaml:"server"`
	PythonGrpc struct {
		Enabled bool   `yaml:"enabled"`
		Target  string `yaml:"target"`
		Timeout int    `yaml:"timeout"`
	} `yaml:"python_grpc"`
	Redis struct {
		Addr         string `yaml:"addr"`
		DB           int    `yaml:"db"`
		PoolSize     int    `yaml:"pool_size"`
		MinIdleConns int    `yaml:"min_idle_conns"`
		MaxRetries   int    `yaml:"max_retries"`
	} `yaml:"redis"`
	TaskQueue struct {
		DequeueTimeout int `yaml:"dequeue_timeout"`
	} `yaml:"task_queue"`
	Worker struct {
		Concurrency     int  `yaml:"concurrency"`
		ShutdownTimeout int  `yaml:"shutdown_timeout"`
		Enabled         bool `yaml:"enabled"`
	} `yaml:"worker"`
	Evaluation struct {
		MaxConcurrent       int    `yaml:"max_concurrent"`
		WorkerEnabled       bool   `yaml:"worker_enabled"`
		PythonEndpoint      string `yaml:"python_endpoint"`
		DefaultTimeout      int    `yaml:"default_timeout"`
		AgentTimeout        int    `yaml:"agent_timeout"`
		MaxRetries          int    `yaml:"max_retries"`
		ProgressCacheExpiry int    `yaml:"progress_cache_expiry"`
	} `yaml:"evaluation"`
	Skill struct {
		Enabled    bool   `yaml:"enabled"`
		Endpoint   string `yaml:"endpoint"`
		Timeout    int    `yaml:"timeout"`
		CacheTTL   int    `yaml:"cache_ttl"`
		MaxRetries int    `yaml:"max_retries"`
	} `yaml:"skill"`
	Upload struct {
		UploadDir string `yaml:"upload_dir"`
	} `yaml:"upload"`
	Collaboration struct {
		DefaultMode           string `yaml:"default_mode"`
		MaxDelegateDepth      int    `yaml:"max_delegate_depth"`
		EnableCyclicDetection bool   `yaml:"enable_cyclic_detection"`
	} `yaml:"collaboration"`
	SemanticCache struct {
		Enabled   bool    `yaml:"enabled"`
		Threshold float32 `yaml:"threshold"`
		TTL       string  `yaml:"ttl"`
		TopK      int     `yaml:"top_k"`
	} `yaml:"semantic_cache"`
}

// defaultFileConfig 返回与历史代码内兜底默认完全一致的基线。
// config.yaml 会在此基线上做覆盖：未出现的 key 保持这里的默认值，
// 从而保证「无 yaml、无 env」时行为与改造前等价。
func defaultFileConfig() *FileConfig {
	fc := &FileConfig{}
	fc.Database.Host = "localhost"
	fc.Database.Port = "3306"
	fc.Database.User = "root"
	fc.Database.Database = "cognida"
	fc.Milvus.Host = ""
	fc.Neo4j.URI = "bolt://localhost:7687"
	fc.Neo4j.Username = "neo4j"
	fc.Neo4j.MaxPoolSize = 50
	fc.JWT.AccessTokenExpire = 86400
	fc.JWT.RefreshTokenExpire = 604800
	fc.Tenant.EnableMultiTenant = false
	fc.Tenant.EnableCrossTenantAccess = false
	fc.Tenant.DefaultStorageQuota = 10 * 1024 * 1024 * 1024
	fc.Chat.Source = string(common.ModelSourceRemote)
	fc.Chat.BaseURL = "https://api.deepseek.com/v1"
	fc.Chat.ModelName = "deepseek-chat"
	fc.Chat.Provider = "deepseek"
	fc.Search.APIEndpoint = "https://metaso.cn/api/v1/search"
	fc.Embedding.Provider = "dashscope"
	fc.Embedding.Model = "text-embedding-v3"
	fc.Embedding.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1/embeddings"
	fc.Server.Port = "8080"
	fc.Server.Mode = "debug"
	fc.Server.Host = "0.0.0.0"
	fc.PythonGrpc.Enabled = true
	fc.PythonGrpc.Target = "localhost:50051"
	fc.PythonGrpc.Timeout = 30
	fc.Redis.Addr = "localhost:6379"
	fc.Redis.DB = 0
	fc.Redis.PoolSize = 10
	fc.Redis.MinIdleConns = 2
	fc.Redis.MaxRetries = 3
	fc.TaskQueue.DequeueTimeout = 30
	fc.Worker.Concurrency = 4
	fc.Worker.ShutdownTimeout = 30
	fc.Worker.Enabled = false
	fc.Evaluation.MaxConcurrent = 3
	fc.Evaluation.WorkerEnabled = true
	fc.Evaluation.PythonEndpoint = "http://localhost:18888"
	fc.Evaluation.DefaultTimeout = 30
	fc.Evaluation.AgentTimeout = 180
	fc.Evaluation.MaxRetries = 3
	fc.Evaluation.ProgressCacheExpiry = 3600
	fc.Skill.Enabled = false
	fc.Skill.Endpoint = "http://localhost:3100/mcp"
	fc.Skill.Timeout = 30
	fc.Skill.CacheTTL = 60
	fc.Skill.MaxRetries = 3
	fc.Upload.UploadDir = "../../var/uploads"
	fc.Collaboration.DefaultMode = "summary"
	fc.Collaboration.MaxDelegateDepth = 10
	fc.Collaboration.EnableCyclicDetection = true
	fc.SemanticCache.Enabled = false
	fc.SemanticCache.Threshold = 0.85
	fc.SemanticCache.TTL = "24h"
	fc.SemanticCache.TopK = 5
	return fc
}

// resolveConfigFilePath 解析 config.yaml 路径（对齐 skills 加载的 CWD 兜底风格）：
// 优先 CONFIG_FILE 环境变量，否则依次尝试候选相对路径。找不到返回空串（不 fatal）。
func resolveConfigFilePath() string {
	if p := os.Getenv("CONFIG_FILE"); p != "" {
		return p
	}
	candidates := []string{
		"./config/config.yaml",
		"../config/config.yaml",
		"../../config/config.yaml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// buildFileConfig 以代码内默认为基线，叠加 config.yaml 中出现的 key（若文件存在）。
// 文件缺失/解析失败时静默回退到默认值，保证行为等价、绝不 fatal。
func buildFileConfig() *FileConfig {
	fc := defaultFileConfig()
	if path := resolveConfigFilePath(); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			_ = yaml.Unmarshal(data, fc) // 仅覆盖文件中出现的字段
		}
	}
	return fc
}

var (
	fileConfigOnce  sync.Once
	fileConfigCache *FileConfig
)

// loadFileConfig 返回进程内缓存的非密配置基线（首次加载后复用，避免重复读文件）。
func loadFileConfig() *FileConfig {
	fileConfigOnce.Do(func() {
		fileConfigCache = buildFileConfig()
	})
	return fileConfigCache
}

// ========================================
// 默认值常量
// ========================================

// DefaultTenantID 默认租户ID（用于开发环境）
const DefaultTenantID = int64(1)

// DefaultUserID 默认用户ID（用于开发环境）
const DefaultUserID = int64(1)

// DefaultPageSize 默认分页大小
const DefaultPageSize = 20

// MaxPageSize 最大分页大小
const MaxPageSize = 100

// DefaultSessionTimeout 默认会话超时时间
const DefaultSessionTimeout = 30 * 60 // 30分钟

// ========================================
// Helper Functions
// ========================================

// GetTenantIDWithDefault 获取租户ID，如果为0则返回默认值
func GetTenantIDWithDefault(tenantID int64) int64 {
	if tenantID == 0 {
		return DefaultTenantID
	}
	return tenantID
}

// GetUserIDWithDefault 获取用户ID，如果为0则返回默认值
func GetUserIDWithDefault(userID int64) int64 {
	if userID == 0 {
		return DefaultUserID
	}
	return userID
}

// NormalizePageSize 规范化分页大小
func NormalizePageSize(pageSize int) int {
	if pageSize <= 0 {
		return DefaultPageSize
	}
	if pageSize > MaxPageSize {
		return MaxPageSize
	}
	return pageSize
}

// NormalizePage 规范化页码
func NormalizePage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

// MilvusConfig Milvus配置
type MilvusConfig struct {
	Host  string
	Token string
}

// Neo4jConfig Neo4j图数据库配置
type Neo4jConfig struct {
	URI          string // Neo4j连接URI，如: bolt://localhost:7687
	Username     string // 用户名，默认: neo4j
	Password     string // 密码
	DatabaseName string // 数据库名称，默认: neo4j
	MaxPoolSize  int    // 连接池最大连接数
}

// JWTConfig JWT配置
type JWTConfig struct {
	Secret             string
	AccessTokenExpire  int
	RefreshTokenExpire int
}

// jwtSecretMinLen JWT 密钥最小长度（字节）。HS256 下短密钥可被离线爆破后自签合法 token。
const jwtSecretMinLen = 32

// jwtPlaceholderSecrets 已知占位密钥，出现即视为未配置
var jwtPlaceholderSecrets = map[string]bool{
	"your-secret-key":                           true,
	"your-secret-key-change-me":                 true,
	"your-secret-key-change-this-in-production": true,
}

// Validate 校验 JWT 密钥强度（fail-closed）：
// 缺失、等于占位符、长度不足 32 字节时返回错误，启动装配处应据此 log.Fatal。
func (c *JWTConfig) Validate() error {
	if c == nil || c.Secret == "" {
		return fmt.Errorf("JWT_SECRET 未配置：请在环境变量中设置至少 %d 字节的随机密钥（如 openssl rand -hex 32）", jwtSecretMinLen)
	}
	if jwtPlaceholderSecrets[c.Secret] {
		return fmt.Errorf("JWT_SECRET 为占位符，禁止使用：请替换为至少 %d 字节的随机密钥", jwtSecretMinLen)
	}
	if len(c.Secret) < jwtSecretMinLen {
		return fmt.Errorf("JWT_SECRET 长度不足：%d 字节 < 最小 %d 字节", len(c.Secret), jwtSecretMinLen)
	}
	return nil
}

// ChatConfig 聊天配置
type ChatConfig struct {
	Source    common.ModelSource // 模型源: local/remote
	BaseURL   string             // API Base URL
	ModelName string             // 模型名称
	APIKey    string             // API密钥
	Provider  string             // Provider: openai, aliwen, deepseek等
}

// SearchConfig 搜索配置
type SearchConfig struct {
	MetasoAPIKey string // Metaso 搜索 API Key
	APIEndpoint  string // 搜索 API 端点
}

// EmbeddingConfig Embedding 配置
type EmbeddingConfig struct {
	Provider string // 提供商: dashscope, openai, etc
	APIKey   string // API 密钥
	Model    string // 模型名称
	BaseURL  string // API Base URL
}

// TenantConfig 租户配置
type TenantConfig struct {
	EnableMultiTenant       bool  // 是否启用多租户
	EnableCrossTenantAccess bool  // 是否启用跨租户访问
	DefaultStorageQuota     int64 // 默认存储配额 (bytes)
}

// ServerConfig HTTP服务配置
type ServerConfig struct {
	Port string // HTTP服务端口
	Mode string // 运行模式: debug/release
	Host string // 监听地址
}

// PythonGrpcConfig Python gRPC 服务配置
type PythonGrpcConfig struct {
	Enabled bool   // 是否启用 Python gRPC 服务
	Target  string // gRPC 服务地址，如: localhost:50051
	Timeout int    // 连接超时时间（秒）
}

// RedisConfig Redis配置
type RedisConfig struct {
	Addr         string // Redis 地址，如 localhost:6379
	Password     string // 密码
	DB           int    // 数据库索引 (0-15)
	PoolSize     int    // 连接池大小
	MinIdleConns int    // 最小空闲连接
	MaxRetries   int    // 最大重试次数
}

// TaskQueueConfig 任务队列配置
type TaskQueueConfig struct {
	DequeueTimeout int // 队列消费超时时间（秒），默认 30
}

// EvaluationConfig 评测系统配置
type EvaluationConfig struct {
	MaxConcurrent       int    // 最大并发任务数，默认 3
	WorkerEnabled       bool   // 是否启用 Worker，默认 true
	PythonEndpoint      string // Python 评测服务地址，独立评测 FastAPI（:18888），非基础服务 :8000
	DefaultTimeout      int    // 默认单 QA 超时时间（秒），默认 30
	AgentTimeout        int    // Agent 单条评测超时时间（秒），默认 180——Agent 一问含多轮工具调用（get_schema→sql_execute→data_analysis→render_ui），60s 会误杀对比/图表等复杂题
	MaxRetries          int    // 最大重试次数，默认 3
	ProgressCacheExpiry int    // 进度缓存过期时间（秒），默认 3600
}

// SkillConfig Skill 系统配置
type SkillConfig struct {
	Enabled    bool   // 是否启用 Skill 系统
	Endpoint   string // MCP Server 端点，须指向 Python MCP 服务，如: http://localhost:3100/mcp（非 Go 自身 :8080）
	Timeout    int    // 默认超时时间（秒），默认 30
	CacheTTL   int    // Skill 列表缓存有效期（秒），默认 60
	MaxRetries int    // 最大重试次数，默认 3
}

// UploadConfig 文件上传配置
type UploadConfig struct {
	UploadDir string // 文件上传目录
}

// WorkerConfig 任务处理器配置
type WorkerConfig struct {
	Concurrency     int  // 并发处理任务数，默认 4
	ShutdownTimeout int  // 优雅关闭超时时间（秒），默认 30
	Enabled         bool // 是否启用 Worker，默认 false
}

// CollaborationConfig Agent 协作配置
type CollaborationConfig struct {
	// DefaultMode 默认上下文模式: none, summary, recent, full, isolated
	DefaultMode string `yaml:"default_mode" env:"COLLAB_DEFAULT_MODE"`

	// MaxDelegateDepth 最大委派深度，防止无限循环
	MaxDelegateDepth int `yaml:"max_delegate_depth" env:"COLLAB_MAX_DELEGATE_DEPTH"`

	// EnableCyclicDetection 是否启用循环检测
	EnableCyclicDetection bool `yaml:"enable_cyclic_detection" env:"COLLAB_ENABLE_CYCLIC_DETECTION"`

	// Scenarios 不同场景的配置覆盖
	Scenarios map[string]ScenarioConfig `yaml:"scenarios"`
}

// SemanticCacheConfig 语义缓存配置
type SemanticCacheConfig struct {
	// Enabled 是否启用语义缓存
	Enabled bool `yaml:"enabled" env:"SEMANTIC_CACHE_ENABLED"`

	// Threshold 相似度阈值 (0-1)
	Threshold float32 `yaml:"threshold" env:"SEMANTIC_CACHE_THRESHOLD"`

	// TTL 缓存过期时间
	TTL string `yaml:"ttl" env:"SEMANTIC_CACHE_TTL"`

	// TopK 检索数量
	TopK int `yaml:"top_k" env:"SEMANTIC_CACHE_TOP_K"`

	// Agents 各 Agent 类型的独立配置
	Agents map[string]AgentCacheConfigOverride `yaml:"agents"`
}

// AgentCacheConfigOverride Agent 缓存配置覆盖
type AgentCacheConfigOverride struct {
	Enabled   bool    `yaml:"enabled"`
	Threshold float32 `yaml:"threshold"`
	TTL       string  `yaml:"ttl"`
	TopK      int     `yaml:"top_k"`
}

// ScenarioConfig 场景配置
type ScenarioConfig struct {
	// Description 场景描述
	Description string `yaml:"description"`

	// AgentTypes 该场景下的 Agent 类型列表
	AgentTypes []string `yaml:"agent_types"`

	// ContextMode 该场景使用的上下文模式
	ContextMode string `yaml:"context_mode"`

	// ContextLimit 上下文消息数量限制（用于 recent 模式）
	ContextLimit int `yaml:"context_limit,omitempty"`
}

// ContextModeRecallStrategy 上下文模式召回策略配置
type ContextModeRecallStrategy struct {
	// None 无上下文模式
	None struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"none"`

	// Summary 摘要模式
	Summary struct {
		Enabled        bool `yaml:"enabled"`
		SummaryTokens  int  `yaml:"summary_tokens"`  // 摘要最大 token 数
		RecentMessages int  `yaml:"recent_messages"` // 附加的最近消息数
	} `yaml:"summary"`

	// Recent 最近消息模式
	Recent struct {
		Enabled bool `yaml:"enabled"`
		Limit   int  `yaml:"limit"` // 最近消息数量
	} `yaml:"recent"`

	// Full 完整历史模式
	Full struct {
		Enabled         bool `yaml:"enabled"`
		MaxHistoryDepth int  `yaml:"max_history_depth"` // 最大历史深度
	} `yaml:"full"`

	// Isolated 隔离模式
	Isolated struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"isolated"`
}

// Config 总配置
type Config struct {
	Database      *DatabaseConfig
	Milvus        *MilvusConfig
	Neo4j         *Neo4jConfig
	JWT           *JWTConfig
	Tenant        *TenantConfig
	Chat          *ChatConfig
	Search        *SearchConfig
	Embedding     *EmbeddingConfig
	Server        *ServerConfig
	PythonGrpc    *PythonGrpcConfig
	Redis         *RedisConfig
	TaskQueue     *TaskQueueConfig
	Worker        *WorkerConfig
	Collaboration *CollaborationConfig // Agent 协作配置
	SemanticCache *SemanticCacheConfig // 语义缓存配置
	Evaluation    *EvaluationConfig    // 评测系统配置
	Skill         *SkillConfig         // Skill 系统配置
	Upload        *UploadConfig        // 文件上传配置
}

// LoadDatabaseConfig 从环境变量加载数据库配置
func LoadDatabaseConfig() *DatabaseConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &DatabaseConfig{
		Host:     getEnv("DB_HOST", fc.Database.Host),
		Port:     getEnv("DB_PORT", fc.Database.Port),
		User:     getEnv("DB_USER", fc.Database.User),
		Password: getEnv("DB_PASSWORD", ""), // 密钥：仅 env
		Database: getEnv("DB_NAME", fc.Database.Database),
	}
}

// LoadMilvusConfig 从环境变量加载Milvus配置
func LoadMilvusConfig() *MilvusConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &MilvusConfig{
		Host:  getEnv("MILVUS_HOST", fc.Milvus.Host),
		Token: getEnv("MILVUS_TOKEN", ""), // 密钥：仅 env
	}
}

// LoadNeo4jConfig 从环境变量加载Neo4j配置
func LoadNeo4jConfig() *Neo4jConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &Neo4jConfig{
		URI:         getEnv("NEO4J_URI", fc.Neo4j.URI),
		Username:    getEnv("NEO4J_USERNAME", fc.Neo4j.Username),
		Password:    getEnv("NEO4J_PASSWORD", ""), // 密钥：仅 env
		MaxPoolSize: getEnvAsInt("NEO4J_MAX_POOL_SIZE", fc.Neo4j.MaxPoolSize),
	}
}

// LoadJWTConfig 从环境变量加载JWT配置
func LoadJWTConfig() *JWTConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &JWTConfig{
		// 无默认密钥：缺失由 Validate() 在启动装配处 fail-closed（密钥：仅 env）
		Secret:             getEnv("JWT_SECRET", ""),
		AccessTokenExpire:  getEnvAsInt("JWT_ACCESS_TOKEN_EXPIRE", fc.JWT.AccessTokenExpire),
		RefreshTokenExpire: getEnvAsInt("JWT_REFRESH_TOKEN_EXPIRE", fc.JWT.RefreshTokenExpire),
	}
}

// LoadChatConfig 从环境变量加载聊天配置
func LoadChatConfig() *ChatConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	source := common.ModelSource(getEnv("CHAT_SOURCE", fc.Chat.Source))

	return &ChatConfig{
		Source:    source,
		BaseURL:   getEnv("CHAT_BASE_URL", fc.Chat.BaseURL),
		ModelName: getEnv("CHAT_MODEL_NAME", fc.Chat.ModelName),
		APIKey:    getEnv("CHAT_API_KEY", ""), // 密钥：仅 env
		Provider:  getEnv("CHAT_PROVIDER", fc.Chat.Provider),
	}
}

// LoadSearchConfig 从环境变量加载搜索配置
func LoadSearchConfig() *SearchConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &SearchConfig{
		MetasoAPIKey: getEnv("METASO_API_KEY", ""), // 密钥：仅 env
		APIEndpoint:  getEnv("SEARCH_API_ENDPOINT", fc.Search.APIEndpoint),
	}
}

// LoadEmbeddingConfig 从环境变量加载 Embedding 配置
func LoadEmbeddingConfig() *EmbeddingConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &EmbeddingConfig{
		Provider: getEnv("EMBEDDING_PROVIDER", fc.Embedding.Provider),
		APIKey:   getEnv("EMBEDDING_API_KEY", ""), // 密钥：仅 env
		Model:    getEnv("EMBEDDING_MODEL", fc.Embedding.Model),
		BaseURL:  getEnv("EMBEDDING_BASE_URL", fc.Embedding.BaseURL),
	}
}

// LoadTenantConfig 从环境变量加载租户配置
func LoadTenantConfig() *TenantConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &TenantConfig{
		EnableMultiTenant:       getEnvAsBool("TENANT_ENABLED", fc.Tenant.EnableMultiTenant),
		EnableCrossTenantAccess: getEnvAsBool("TENANT_CROSS_ACCESS", fc.Tenant.EnableCrossTenantAccess),
		DefaultStorageQuota:     getEnvAsInt64("TENANT_DEFAULT_QUOTA", fc.Tenant.DefaultStorageQuota),
	}
}

// LoadServerConfig 从环境变量加载服务配置
func LoadServerConfig() *ServerConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &ServerConfig{
		Port: getEnv("SERVER_PORT", fc.Server.Port),
		Mode: getEnv("GIN_MODE", fc.Server.Mode),
		Host: getEnv("SERVER_HOST", fc.Server.Host),
	}
}

// LoadPythonGrpcConfig 加载 Python gRPC 服务配置
func LoadPythonGrpcConfig() *PythonGrpcConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &PythonGrpcConfig{
		Enabled: getEnvAsBool("PYTHON_GRPC_ENABLED", fc.PythonGrpc.Enabled),
		Target:  getEnv("PYTHON_GRPC_TARGET", fc.PythonGrpc.Target),
		Timeout: getEnvAsInt("PYTHON_GRPC_TIMEOUT", fc.PythonGrpc.Timeout),
	}
}

// LoadRedisConfig 加载 Redis 配置
func LoadRedisConfig() *RedisConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &RedisConfig{
		Addr:         getEnv("REDIS_ADDR", fc.Redis.Addr),
		Password:     getEnv("REDIS_PASSWORD", ""), // 密钥：仅 env
		DB:           getEnvAsInt("REDIS_DB", fc.Redis.DB),
		PoolSize:     getEnvAsInt("REDIS_POOL_SIZE", fc.Redis.PoolSize),
		MinIdleConns: getEnvAsInt("REDIS_MIN_IDLE_CONNS", fc.Redis.MinIdleConns),
		MaxRetries:   getEnvAsInt("REDIS_MAX_RETRIES", fc.Redis.MaxRetries),
	}
}

// LoadTaskQueueConfig 加载任务队列配置
func LoadTaskQueueConfig() *TaskQueueConfig {
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &TaskQueueConfig{
		DequeueTimeout: getEnvAsInt("TASK_DEQUEUE_TIMEOUT", fc.TaskQueue.DequeueTimeout),
	}
}

// LoadWorkerConfig 加载任务处理器配置
func LoadWorkerConfig() *WorkerConfig {
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &WorkerConfig{
		Concurrency:     getEnvAsInt("WORKER_CONCURRENCY", fc.Worker.Concurrency),
		ShutdownTimeout: getEnvAsInt("WORKER_SHUTDOWN_TIMEOUT", fc.Worker.ShutdownTimeout),
		Enabled:         getEnvAsBool("WORKER_ENABLED", fc.Worker.Enabled),
	}
}

// LoadCollaborationConfig 加载 Agent 协作配置
func LoadCollaborationConfig() *CollaborationConfig {
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return defaultCollaborationConfig()
}

// LoadSemanticCacheConfig 加载语义缓存配置
func LoadSemanticCacheConfig() *SemanticCacheConfig {
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return defaultSemanticCacheConfig()
}

// defaultSemanticCacheConfig 返回默认的语义缓存配置
func defaultSemanticCacheConfig() *SemanticCacheConfig {
	fc := loadFileConfig()
	cfg := &SemanticCacheConfig{
		Enabled:   fc.SemanticCache.Enabled, // 非密真源在 config.yaml，默认关闭
		Threshold: fc.SemanticCache.Threshold,
		TTL:       fc.SemanticCache.TTL,
		TopK:      fc.SemanticCache.TopK,
		Agents:    make(map[string]AgentCacheConfigOverride),
	}

	// 从环境变量覆盖
	mergeEnvToSemanticCacheConfig(cfg)

	// 设置默认 Agent 配置
	if len(cfg.Agents) == 0 {
		cfg.Agents = map[string]AgentCacheConfigOverride{
			"rag_agent": {
				Enabled:   true,
				Threshold: 0.90,
				TTL:       "24h",
				TopK:      3,
			},
			"qa_agent": {
				Enabled:   true,
				Threshold: 0.95,
				TTL:       "168h", // 7天
				TopK:      5,
			},
			"code_agent": {
				Enabled:   true,
				Threshold: 0.92,
				TTL:       "24h",
				TopK:      5,
			},
			"react_agent": {
				Enabled:   false, // ReAct 默认关闭
				Threshold: 0.85,
				TTL:       "5m",
				TopK:      5,
			},
			"deep_research": {
				Enabled:   false, // 深度研究默认关闭
				Threshold: 0.80,
				TTL:       "1h",
				TopK:      5,
			},
		}
	}

	return cfg
}

// mergeEnvToSemanticCacheConfig 合并环境变量到语义缓存配置
func mergeEnvToSemanticCacheConfig(cfg *SemanticCacheConfig) {
	if enabled := os.Getenv("SEMANTIC_CACHE_ENABLED"); enabled != "" {
		cfg.Enabled = getEnvAsBool("SEMANTIC_CACHE_ENABLED", false)
	}
	if threshold := os.Getenv("SEMANTIC_CACHE_THRESHOLD"); threshold != "" {
		var val float32
		if _, err := fmt.Sscanf(threshold, "%f", &val); err == nil {
			cfg.Threshold = val
		}
	}
	if ttl := os.Getenv("SEMANTIC_CACHE_TTL"); ttl != "" {
		cfg.TTL = ttl
	}
	if topK := os.Getenv("SEMANTIC_CACHE_TOP_K"); topK != "" {
		cfg.TopK = getEnvAsInt("SEMANTIC_CACHE_TOP_K", 5)
	}
}

// defaultCollaborationConfig 返回默认的协作配置
func defaultCollaborationConfig() *CollaborationConfig {
	fc := loadFileConfig()
	cfg := &CollaborationConfig{
		DefaultMode:           fc.Collaboration.DefaultMode,
		MaxDelegateDepth:      fc.Collaboration.MaxDelegateDepth,
		EnableCyclicDetection: fc.Collaboration.EnableCyclicDetection,
		Scenarios:             make(map[string]ScenarioConfig),
	}

	// 从环境变量覆盖
	mergeEnvToCollaborationConfig(cfg)

	// 设置默认场景
	if len(cfg.Scenarios) == 0 {
		cfg.Scenarios = map[string]ScenarioConfig{
			"data_analysis": {
				Description:  "数据分析场景",
				AgentTypes:   []string{"analyst", "query_agent"},
				ContextMode:  "summary",
				ContextLimit: 10,
			},
			"research": {
				Description: "研究分析场景",
				AgentTypes:  []string{"researcher", "writer"},
				ContextMode: "full",
			},
			"simple_task": {
				Description: "简单任务场景",
				AgentTypes:  []string{"executor"},
				ContextMode: "none",
			},
		}
	}

	return cfg
}

// mergeEnvToCollaborationConfig 合并环境变量到协作配置
func mergeEnvToCollaborationConfig(cfg *CollaborationConfig) {
	if mode := os.Getenv("COLLAB_DEFAULT_MODE"); mode != "" {
		cfg.DefaultMode = mode
	}
	if depth := os.Getenv("COLLAB_MAX_DELEGATE_DEPTH"); depth != "" {
		cfg.MaxDelegateDepth = getEnvAsInt("COLLAB_MAX_DELEGATE_DEPTH", 10)
	}
	if cyclic := os.Getenv("COLLAB_ENABLE_CYCLIC_DETECTION"); cyclic != "" {
		cfg.EnableCyclicDetection = getEnvAsBool("COLLAB_ENABLE_CYCLIC_DETECTION", true)
	}
}

// LoadEvaluationConfig 加载评测系统配置
func LoadEvaluationConfig() *EvaluationConfig {
	fc := loadFileConfig()
	return &EvaluationConfig{
		MaxConcurrent:       getEnvAsInt("EVALUATION_MAX_CONCURRENT", fc.Evaluation.MaxConcurrent),
		WorkerEnabled:       getEnvAsBool("EVALUATION_WORKER_ENABLED", fc.Evaluation.WorkerEnabled),
		PythonEndpoint:      getEnv("PYTHON_EVALUATION_ENDPOINT", fc.Evaluation.PythonEndpoint),
		DefaultTimeout:      getEnvAsInt("EVALUATION_DEFAULT_TIMEOUT", fc.Evaluation.DefaultTimeout),
		AgentTimeout:        getEnvAsInt("EVALUATION_AGENT_TIMEOUT", fc.Evaluation.AgentTimeout),
		MaxRetries:          getEnvAsInt("EVALUATION_MAX_RETRIES", fc.Evaluation.MaxRetries),
		ProgressCacheExpiry: getEnvAsInt("EVALUATION_PROGRESS_CACHE_EXPIRY", fc.Evaluation.ProgressCacheExpiry),
	}
}

// LoadSkillConfig 加载 Skill 系统配置
func LoadSkillConfig() *SkillConfig {
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	fc := loadFileConfig()
	return &SkillConfig{
		Enabled: getEnvAsBool("SKILL_ENABLED", fc.Skill.Enabled),
		// MCP/技能工具端点：必须指向 Python MCP 服务（dev.sh py-mcp 监听 :3100），
		// 而非 Go 自身的 Gin 端口(:8080，无 /mcp 路由)。默认值配错会导致 data_analysis
		// 工具（correlation/insight）POST /mcp 打到 8080 → 404，Agent 退回手写 SQL。
		Endpoint:   getEnv("SKILL_ENDPOINT", fc.Skill.Endpoint),
		Timeout:    getEnvAsInt("SKILL_TIMEOUT", fc.Skill.Timeout),
		CacheTTL:   getEnvAsInt("SKILL_CACHE_TTL", fc.Skill.CacheTTL),
		MaxRetries: getEnvAsInt("SKILL_MAX_RETRIES", fc.Skill.MaxRetries),
	}
}

// LoadUploadConfig 加载文件上传配置
func LoadUploadConfig() *UploadConfig {
	// 默认落在仓库根 var/uploads（服务从 services/cognida-go/ 启动，故 ../../var/uploads）；
	// 与 Python 侧 UPLOAD_BASE_DIR/ALLOWED_PATHS 指向同一共享目录。
	fc := loadFileConfig()
	uploadDir := getEnv("UPLOAD_DIR", fc.Upload.UploadDir)

	// 确保上传目录存在
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		// 如果创建失败，使用默认目录
		uploadDir = "../../var/uploads"
		_ = os.MkdirAll(uploadDir, 0755)
	}

	return &UploadConfig{
		UploadDir: uploadDir,
	}
}

// LoadConfig 加载完整配置
func LoadConfig() *Config {
	return &Config{
		Database:      LoadDatabaseConfig(),
		Milvus:        LoadMilvusConfig(),
		Neo4j:         LoadNeo4jConfig(),
		JWT:           LoadJWTConfig(),
		Tenant:        LoadTenantConfig(),
		Chat:          LoadChatConfig(),
		Search:        LoadSearchConfig(),
		Embedding:     LoadEmbeddingConfig(),
		Server:        LoadServerConfig(),
		PythonGrpc:    LoadPythonGrpcConfig(),
		Redis:         LoadRedisConfig(),
		TaskQueue:     LoadTaskQueueConfig(),
		Worker:        LoadWorkerConfig(),
		Collaboration: LoadCollaborationConfig(),
		SemanticCache: LoadSemanticCacheConfig(),
		Evaluation:    LoadEvaluationConfig(),
		Skill:         LoadSkillConfig(),
		Upload:        LoadUploadConfig(),
	}
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		var intValue int64
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1" || value == "yes"
}

// GetDSN 获取数据库连接字符串
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Database,
	)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// PromptTemplate 提示词模板
type PromptTemplate struct {
	Templates []struct {
		ID      string `yaml:"id"`
		Content string `yaml:"content"`
	} `yaml:"templates"`
}

// LoadPromptTemplate 加载提示词模板
func LoadPromptTemplate(templateName string) (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	// 按优先级尝试多个路径
	paths := []string{
		// 相对于工作目录的路径
		filepath.Join(workingDir, "internal", "infrastructure", "config", "prompt_templates", templateName+".yaml"),
		// 如果在仓库根运行（服务位于 services/cognida-go 下）
		filepath.Join(workingDir, "services", "cognida-go", "internal", "infrastructure", "config", "prompt_templates", templateName+".yaml"),
	}

	var lastErr error
	for _, templatePath := range paths {
		content, err := os.ReadFile(templatePath)
		if err == nil {
			var pt PromptTemplate
			if err := yaml.Unmarshal(content, &pt); err != nil {
				return "", fmt.Errorf("failed to parse template YAML: %w", err)
			}
			if len(pt.Templates) == 0 {
				return "", fmt.Errorf("no templates found in %s", templatePath)
			}
			return pt.Templates[0].Content, nil
		}
		lastErr = err
	}

	return "", fmt.Errorf("failed to read template file %s (tried %d paths): %w", templateName, len(paths), lastErr)
}
