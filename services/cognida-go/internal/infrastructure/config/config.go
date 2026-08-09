package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
	"cognida/internal/model/common"
)

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
	Endpoint   string // MCP Server 端点，如: http://localhost:8080/mcp
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

	return &DatabaseConfig{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "3306"),
		User:     getEnv("DB_USER", "root"),
		Password: getEnv("DB_PASSWORD", ""),
		Database: getEnv("DB_NAME", "cognida"),
	}
}

// LoadMilvusConfig 从环境变量加载Milvus配置
func LoadMilvusConfig() *MilvusConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &MilvusConfig{
		Host:  getEnv("MILVUS_HOST", ""),
		Token: getEnv("MILVUS_TOKEN", ""),
	}
}

// LoadNeo4jConfig 从环境变量加载Neo4j配置
func LoadNeo4jConfig() *Neo4jConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &Neo4jConfig{
		URI:         getEnv("NEO4J_URI", "bolt://localhost:7687"),
		Username:    getEnv("NEO4J_USERNAME", "neo4j"),
		Password:    getEnv("NEO4J_PASSWORD", ""),
		MaxPoolSize: getEnvAsInt("NEO4J_MAX_POOL_SIZE", 50),
	}
}

// LoadJWTConfig 从环境变量加载JWT配置
func LoadJWTConfig() *JWTConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &JWTConfig{
		// 无默认密钥：缺失由 Validate() 在启动装配处 fail-closed
		Secret:             getEnv("JWT_SECRET", ""),
		AccessTokenExpire:  getEnvAsInt("JWT_ACCESS_TOKEN_EXPIRE", 86400),   // 24小时
		RefreshTokenExpire: getEnvAsInt("JWT_REFRESH_TOKEN_EXPIRE", 604800), // 7天
	}
}

// LoadChatConfig 从环境变量加载聊天配置
func LoadChatConfig() *ChatConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	source := common.ModelSource(getEnv("CHAT_SOURCE", string(common.ModelSourceRemote)))

	return &ChatConfig{
		Source:    source,
		BaseURL:   getEnv("CHAT_BASE_URL", "https://api.deepseek.com/v1"),
		ModelName: getEnv("CHAT_MODEL_NAME", "deepseek-chat"),
		APIKey:    getEnv("CHAT_API_KEY", ""),
		Provider:  getEnv("CHAT_PROVIDER", "deepseek"),
	}
}

// LoadSearchConfig 从环境变量加载搜索配置
func LoadSearchConfig() *SearchConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &SearchConfig{
		MetasoAPIKey: getEnv("METASO_API_KEY", ""),
		APIEndpoint:  getEnv("SEARCH_API_ENDPOINT", "https://metaso.cn/api/v1/search"),
	}
}

// LoadEmbeddingConfig 从环境变量加载 Embedding 配置
func LoadEmbeddingConfig() *EmbeddingConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &EmbeddingConfig{
		Provider: getEnv("EMBEDDING_PROVIDER", "dashscope"),
		APIKey:   getEnv("EMBEDDING_API_KEY", ""),
		Model:    getEnv("EMBEDDING_MODEL", "text-embedding-v3"),
		BaseURL:  getEnv("EMBEDDING_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1/embeddings"),
	}
}

// LoadTenantConfig 从环境变量加载租户配置
func LoadTenantConfig() *TenantConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &TenantConfig{
		EnableMultiTenant:       getEnvAsBool("TENANT_ENABLED", false),
		EnableCrossTenantAccess: getEnvAsBool("TENANT_CROSS_ACCESS", false),
		DefaultStorageQuota:     getEnvAsInt64("TENANT_DEFAULT_QUOTA", 10*1024*1024*1024), // 10GB
	}
}

// LoadServerConfig 从环境变量加载服务配置
func LoadServerConfig() *ServerConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &ServerConfig{
		Port: getEnv("SERVER_PORT", "8080"),
		Mode: getEnv("GIN_MODE", "debug"),
		Host: getEnv("SERVER_HOST", "0.0.0.0"),
	}
}

// LoadPythonGrpcConfig 加载 Python gRPC 服务配置
func LoadPythonGrpcConfig() *PythonGrpcConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &PythonGrpcConfig{
		Enabled: getEnvAsBool("PYTHON_GRPC_ENABLED", true),
		Target:  getEnv("PYTHON_GRPC_TARGET", "localhost:50051"),
		Timeout: getEnvAsInt("PYTHON_GRPC_TIMEOUT", 30),
	}
}

// LoadRedisConfig 加载 Redis 配置
func LoadRedisConfig() *RedisConfig {
	// 尝试加载 .env 文件
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &RedisConfig{
		Addr:         getEnv("REDIS_ADDR", "localhost:6379"),
		Password:     getEnv("REDIS_PASSWORD", ""),
		DB:           getEnvAsInt("REDIS_DB", 0),
		PoolSize:     getEnvAsInt("REDIS_POOL_SIZE", 10),
		MinIdleConns: getEnvAsInt("REDIS_MIN_IDLE_CONNS", 2),
		MaxRetries:   getEnvAsInt("REDIS_MAX_RETRIES", 3),
	}
}

// LoadTaskQueueConfig 加载任务队列配置
func LoadTaskQueueConfig() *TaskQueueConfig {
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &TaskQueueConfig{
		DequeueTimeout: getEnvAsInt("TASK_DEQUEUE_TIMEOUT", 30),
	}
}

// LoadWorkerConfig 加载任务处理器配置
func LoadWorkerConfig() *WorkerConfig {
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &WorkerConfig{
		Concurrency:     getEnvAsInt("WORKER_CONCURRENCY", 4),
		ShutdownTimeout: getEnvAsInt("WORKER_SHUTDOWN_TIMEOUT", 30),
		Enabled:         getEnvAsBool("WORKER_ENABLED", false),
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
	cfg := &SemanticCacheConfig{
		Enabled:   false, // 默认关闭
		Threshold: 0.85,
		TTL:       "24h",
		TopK:      5,
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
	cfg := &CollaborationConfig{
		DefaultMode:           "summary",
		MaxDelegateDepth:      10,
		EnableCyclicDetection: true,
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
	return &EvaluationConfig{
		MaxConcurrent:       getEnvAsInt("EVALUATION_MAX_CONCURRENT", 3),
		WorkerEnabled:       getEnvAsBool("EVALUATION_WORKER_ENABLED", true),
		PythonEndpoint:      getEnv("PYTHON_EVALUATION_ENDPOINT", "http://localhost:18888"),
		DefaultTimeout:      getEnvAsInt("EVALUATION_DEFAULT_TIMEOUT", 30),
		AgentTimeout:        getEnvAsInt("EVALUATION_AGENT_TIMEOUT", 180),
		MaxRetries:          getEnvAsInt("EVALUATION_MAX_RETRIES", 3),
		ProgressCacheExpiry: getEnvAsInt("EVALUATION_PROGRESS_CACHE_EXPIRY", 3600),
	}
}

// LoadSkillConfig 加载 Skill 系统配置
func LoadSkillConfig() *SkillConfig {
	projectRoot, _ := os.Getwd()
	envPath := filepath.Join(projectRoot, ".env")
	_ = godotenv.Load(envPath)

	return &SkillConfig{
		Enabled:    getEnvAsBool("SKILL_ENABLED", false),
		Endpoint:   getEnv("SKILL_ENDPOINT", "http://localhost:8080/mcp"),
		Timeout:    getEnvAsInt("SKILL_TIMEOUT", 30),
		CacheTTL:   getEnvAsInt("SKILL_CACHE_TTL", 60),
		MaxRetries: getEnvAsInt("SKILL_MAX_RETRIES", 3),
	}
}

// LoadUploadConfig 加载文件上传配置
func LoadUploadConfig() *UploadConfig {
	// 默认落在仓库根 var/uploads（服务从 services/cognida-go/ 启动，故 ../../var/uploads）；
	// 与 Python 侧 UPLOAD_BASE_DIR/ALLOWED_PATHS 指向同一共享目录。
	uploadDir := getEnv("UPLOAD_DIR", "../../var/uploads")

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
