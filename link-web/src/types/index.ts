/**
 * 全局类型定义
 */

// ============ 认证相关 ============
export interface LoginRequest {
  email: string
  password: string
}

export interface RegisterRequest {
  email: string
  password: string
  username: string
}

export interface AuthResponse {
  access_token: string
  refresh_token: string
  expires_at: number
  user: UserInfo
  tenant_id?: number
}

export interface UserInfo {
  id: number
  email: string
  username: string
  avatar?: string
  created_at: string
  updated_at: string
}

// ============ 租户相关 ============
export interface Tenant {
  id: number
  name: string
  description?: string
  api_key?: string
  storage_used?: number
  storage_limit?: number
  created_at: string
  updated_at: string
}

export interface CreateTenantRequest {
  name: string
  description?: string
}

export interface UpdateTenantRequest {
  name?: string
  description?: string
}

// ============ 聊天相关 ============
export interface ChatRequest {
  content?: string
  history?: ChatMessage[]
  options?: ChatOptions
  session_id?: string
  stream?: boolean
  messages?: ChatMessage[]
  rag_config?: RAGConfig
}

// 用于聊天请求的 Message 类型
export interface ChatMessage {
  role: 'user' | 'assistant' | 'system'
  content: string
  name?: string
  tool_calls?: ToolCall[]
  metadata?: Record<string, any>
}

export interface Message {
  id: string
  session_id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  tool_calls?: string
  agent_steps?: string
  token_count: number
  created_at: string
}

export interface ChatOptions {
  temperature?: number
  top_p?: number
  max_tokens?: number
  frequency_penalty?: number
  presence_penalty?: number
}

export interface ChatResponse {
  message_id?: string
  content: string
  role?: string
  token_count?: number
  tool_calls?: ToolCall[]
  session_id?: string
  request_id?: string
  agent_id?: string
  metadata?: Record<string, any>
}

export interface ToolCall {
  id: string
  type: string
  function: {
    name: string
    arguments: string
  }
}

export interface StreamChatEvent {
  event: 'content' | 'end' | 'error' | 'session' | 'rag_context'
  content: string
  message_id?: string
  token_count?: number
  tool_calls?: ToolCall[]
  error?: string
  session_id?: string
  rag_context?: RAGContext
}

// ============ RAG 相关 ============
export interface RAGConfig {
  enabled: boolean
  kb_id: string
  retrieval_modes: string[]  // 检索模式数组：'vector'(必选), 'bm25', 'graph'
  vector_top_k: number
  keyword_top_k: number
  graph_top_k: number
  similarity_threshold: number
  alpha: number
}

export interface RAGContext {
  query: string
  final_query: string
  contexts: string[]
  contexts_with_score: Array<{
    content: string
    score: number
    chunk_id: string
    source: string
  }>
  source_types: string[]
  retrieved_count: number
  stages: Record<string, {
    success: boolean
    input_count?: number
    output_count?: number
  }>
}

export const defaultRAGConfig: RAGConfig = {
  enabled: false,
  kb_id: '',
  retrieval_modes: ['vector'],  // 默认仅向量检索
  vector_top_k: 15,
  keyword_top_k: 15,
  graph_top_k: 10,
  similarity_threshold: 0,
  alpha: 0.6
}

// ============ 会话相关 ============
export type ChatMode = 'normal' | 'agent'

export interface Session {
  id: string
  title: string
  description?: string
  status: number
  message_count?: number
  agent_type?: string  // Agent 类型：rag, text2sql 等
  rag_config?: RAGConfig  // RAG 配置（从后端聚合获取）
  created_at: string
  updated_at: string
}

export interface CreateSessionRequest {
  agent_type?: string  // Agent类型: rag, text2sql, default
  title?: string
  description?: string
  rag_config?: RAGConfig  // 创建时可指定 RAG 配置
}

export interface UpdateSessionRequest {
  title?: string
  description?: string
  status?: number
  rag_config?: RAGConfig  // 更新时可修改 RAG 配置
}

export interface SessionDetail {
  session: Session
  messages: Message[]
}

export interface SessionListResponse {
  sessions: Session[]
  total: number
  page: number
  size: number
}

// ============ 消息相关 ============
export interface Message {
  id: string
  session_id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  tool_calls?: string
  agent_steps?: string  // Agent 思考步骤（JSON 字符串）
  token_count: number
  created_at: string
}

export interface CreateMessageRequest {
  session_id: string
  role: string
  content: string
  tool_calls?: string
  agent_steps?: string  // Agent 思考步骤（JSON 字符串）
  token_count?: number
}

// ============ 知识库相关 ============
export interface KnowledgeBase {
  id: string
  name: string
  description?: string
  avatar?: string
  is_public?: boolean
  status: number
  created_at: string
  updated_at: string
  // 统计字段
  document_count?: number
  chunk_count?: number
  storage_size?: number
  // 数据处理配置（从 kb_settings 表获取）
  setting?: {
    chunk_size?: number
    chunk_overlap?: number
    graph_enabled?: boolean
    bm25_enabled?: boolean
    image_processing_mode?: string
    extract_mode?: string
  }
  // 检索模式数组（从后端返回）
  retrieval_modes?: string[]
}

export interface CreateKnowledgeBaseRequest {
  name: string
  description?: string
  avatar?: string
  is_public?: boolean
  // 数据处理配置（存储到 kb_settings 表）
  chunk_size?: number
  chunk_overlap?: number
  graph_enabled?: boolean
  bm25_enabled?: boolean
}

export interface UpdateKnowledgeBaseRequest {
  name?: string
  description?: string
  avatar?: string
  is_public?: boolean
  status?: number
  // 数据处理配置（存储到 kb_settings 表）
  chunk_size?: number
  chunk_overlap?: number
  graph_enabled?: boolean
  bm25_enabled?: boolean
}

export interface KnowledgeBaseStats {
  kb_id: string
  knowledge_count: number
  chunk_count: number
  total_size: number
}

// 知识条目相关
export interface Knowledge {
  id: string
  kb_id: string
  title: string
  type: string
  storage_size: number
  file_path: string
  parse_status: 'unprocessed' | 'pending' | 'processing' | 'completed' | 'failed'
  enable_status: 'enabled' | 'disabled'
  chunk_count: number
  created_at: string
  processed_at?: string
}

export interface UploadKnowledgeFileRequest {
  file: File
  title?: string
  file_type?: string
  chunk_size?: number
  chunk_overlap?: number
}

export interface KnowledgeStatus {
  knowledge_id: string
  parse_status: 'pending' | 'processing' | 'completed' | 'failed'
  enable_status: 'enabled' | 'disabled'
  chunk_count: number
  created_at: string
  processed_at?: string
}

// 文档分块相关
export interface Chunk {
  id: string
  kb_id: string
  knowledge_id: string
  content: string
  chunk_index: number
  token_count?: number
  embedding_id?: string
  created_at: string
}

export interface ChunkListResponse {
  items: Chunk[]
  total: number
  page: number
  size: number
}

// 知识检索相关
// 注意：字段与后端 SearchKnowledge 绑定保持一致（min_score，而非 score_threshold）
export interface SearchRequest {
  query: string
  kb_ids: string[]
  top_k?: number
  min_score?: number
}

export interface SearchResult {
  chunk_id: string
  knowledge_id: string
  // 后端当前为占位值：knowledge_title 恒为空、score 恒为 1.0，展示层需谨慎处理
  knowledge_title: string
  content: string
  score: number
  highlight?: string
}

export interface SearchResponse {
  // 后端返回 { total, items }
  total: number
  items: SearchResult[]
}

// ============ FAQ相关 ============
export interface FAQEntry {
  id: string
  kb_id: string
  question: string
  answer: string
  priority: number
  enabled: boolean
  created_at: string
  updated_at: string
}

// ============ Agent相关 ============
export interface Agent {
  id: string
  name: string
  description?: string
  avatar?: string
  system_prompt?: string
  model?: string
  temperature?: number
  enabled: boolean
  created_at: string
  updated_at: string
}

// Agent 流式事件类型
export interface AgentStreamEvent {
  event: 'step' | 'done' | 'error' | 'session' | 'ui'
  session_id?: string
  step?: number
  step_count?: number
  // 步骤类型 - 扩展以支持更多类型
  type?: 'thinking' | 'agent_call' | 'search' | 'utility' | 'action'
    | 'plan' | 'analysis' | 'review' | 'synthesis' | 'retrieval'
    | 'tool_call' | 'tool_result'
    | 'complete' | 'error' | 'thought' | 'reflection'  // 兼容旧类型
  stage?: string  // 阶段: "信息检索", "反思校验", "规划阶段", "分析阶段", etc.
  content?: string
  thought?: string
  tool_name?: string
  tool_desc?: string
  tool_params?: Record<string, any>
  tool_input?: string   // 工具入参（JSON 字符串）
  tool_id?: string
  tool_output?: string
  status?: 'calling' | 'success' | 'error'  // 工具执行状态
  error?: string
  is_agent?: boolean
  agent_name?: string
  agent_stage?: string
  related_tool?: string
  related_step?: number
  reason?: string
  complete?: boolean
  answer?: string
}

// Agent 思考步骤（用于显示）
export interface AgentStep {
  id: string
  step: number
  // 扩展类型以支持更多步骤类型
  type: 'thinking' | 'agent_call' | 'search' | 'utility' | 'action'
    | 'plan' | 'analysis' | 'review' | 'synthesis' | 'retrieval'
    | 'tool_call' | 'tool_result'
    | 'complete' | 'error' | 'thought' | 'reflection'
  stage?: string  // 阶段
  content?: string
  thought?: string
  tool_name?: string
  tool_desc?: string
  tool_params?: Record<string, any>
  tool_input?: string   // 工具入参（JSON 字符串）
  tool_output?: string
  tool_id?: string
  status?: 'calling' | 'success' | 'error'  // 工具执行状态
  error?: string
  is_agent?: boolean
  agent_name?: string
  agent_stage?: string
  related_tool?: string
  related_step?: number
  reason?: string
  timestamp: number
  isActive?: boolean
}

// 导出图谱相关类型
export * from './graph'
// 导出测评相关类型
export * from './evaluation'
