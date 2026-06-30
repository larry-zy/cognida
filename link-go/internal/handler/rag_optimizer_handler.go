// Package handler 提供 RAG 检索优化的 HTTP 处理器
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	appKnowledge "link/internal/service/knowledge"
)

// ========================================
// RAGOptimizerHandler RAG 检索优化处理器
// ========================================

// RAGOptimizerHandler RAG 检索优化处理器
type RAGOptimizerHandler struct {
	optimizerService *appKnowledge.Optimizer
}

// NewRAGOptimizerHandler 创建 RAG 检索优化处理器
func NewRAGOptimizerHandler(optimizerService *appKnowledge.Optimizer) *RAGOptimizerHandler {
	return &RAGOptimizerHandler{
		optimizerService: optimizerService,
	}
}

// ========================================
// HyDE 检索接口
// ========================================

// HyDERetrieve 执行 HyDE 检索
// @Summary HyDE 检索
// @Description 使用假设文档嵌入进行检索优化
// @Tags rag-optimization
// @Accept json
// @Produce json
// @Router /api/v1/rag/hyde [post]
func (h *RAGOptimizerHandler) HyDERetrieve(c *gin.Context) {
	var req appKnowledge.HyDERetrieveRequest
	if !BindJSON(c, &req) {
		return
	}

	// 获取租户ID
	tenantID := c.GetString("tenant_id")
	if tenantID != "" {
		req.TenantID = tenantID
	}

	result, err := h.optimizerService.HyDERetrieve(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// ========================================
// Query Rewrite 接口
// ========================================

// RewriteQuery 重写查询
// @Summary 查询重写
// @Description 重写用户查询以优化检索效果
// @Tags rag-optimization
// @Accept json
// @Produce json
// @Router /api/v1/rag/query/rewrite [post]
func (h *RAGOptimizerHandler) RewriteQuery(c *gin.Context) {
	var req appKnowledge.QueryRewriteRequest
	if !BindJSON(c, &req) {
		return
	}

	result, err := h.optimizerService.RewriteQuery(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// ========================================
// Multi-Hop 检索接口
// ========================================

// MultiHopRetrieve 执行多跳检索
// @Summary 多跳检索
// @Description 执行多跳检索以处理复杂查询
// @Tags rag-optimization
// @Accept json
// @Produce json
// @Router /api/v1/rag/multi-hop [post]
func (h *RAGOptimizerHandler) MultiHopRetrieve(c *gin.Context) {
	var req appKnowledge.MultiHopRetrieveRequest
	if !BindJSON(c, &req) {
		return
	}

	// 获取租户ID
	tenantID := c.GetString("tenant_id")
	if tenantID != "" {
		req.TenantID = tenantID
	}

	result, err := h.optimizerService.MultiHopRetrieve(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// ========================================
// 综合优化检索接口
// ========================================

// OptimizedRetrieve 执行优化的检索
// @Summary 优化检索
// @Description 使用多种优化策略执行检索
// @Tags rag-optimization
// @Accept json
// @Produce json
// @Router /api/v1/rag/optimized [post]
func (h *RAGOptimizerHandler) OptimizedRetrieve(c *gin.Context) {
	var req appKnowledge.OptimizedRetrieveRequest
	if !BindJSON(c, &req) {
		return
	}

	// 获取租户ID
	tenantID := c.GetString("tenant_id")
	if tenantID != "" {
		req.TenantID = tenantID
	}

	result, err := h.optimizerService.OptimizedRetrieve(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// ========================================
// 配置管理接口
// ========================================

// GetHyDEConfig 获取 HyDE 默认配置
// @Summary 获取 HyDE 配置
// @Description 获取 HyDE 的默认配置
// @Tags rag-optimization
// @Router /api/v1/rag/config/hyde [get]
func (h *RAGOptimizerHandler) GetHyDEConfig(c *gin.Context) {
	config := appKnowledge.DefaultHyDEOptions()
	OK(c, config)
}

// GetRewriteConfig 获取查询重写默认配置
// @Summary 获取查询重写配置
// @Description 获取查询重写的默认配置
// @Tags rag-optimization
// @Router /api/v1/rag/config/rewrite [get]
func (h *RAGOptimizerHandler) GetRewriteConfig(c *gin.Context) {
	config := appKnowledge.DefaultRewriteOptions()
	OK(c, config)
}

// GetMultiHopConfig 获取多跳检索默认配置
// @Summary 获取多跳检索配置
// @Description 获取多跳检索的默认配置
// @Tags rag-optimization
// @Router /api/v1/rag/config/multi-hop [get]
func (h *RAGOptimizerHandler) GetMultiHopConfig(c *gin.Context) {
	config := appKnowledge.DefaultMultiHopOptions()
	OK(c, config)
}

// GetOptimizerConfig 获取优化器配置
// @Summary 获取优化器配置
// @Description 根据参数获取优化器配置
// @Tags rag-optimization
// @Router /api/v1/rag/config/optimizer [get]
func (h *RAGOptimizerHandler) GetOptimizerConfig(c *gin.Context) {
	enableHyDE, _ := strconv.ParseBool(c.DefaultQuery("enable_hyde", "true"))
	enableRewrite, _ := strconv.ParseBool(c.DefaultQuery("enable_rewrite", "true"))
	enableMultiHop, _ := strconv.ParseBool(c.DefaultQuery("enable_multi_hop", "false"))

	config := appKnowledge.CreateOptimizerConfig(enableHyDE, enableRewrite, enableMultiHop)
	OK(c, config)
}
