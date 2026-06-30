// Package handler 提供安全护栏的 HTTP 处理器
package handler

import (
	"github.com/gin-gonic/gin"

	agenttools "link/internal/service/agent/tools"
	domainguardrail "link/internal/model/guardrail"
)

// ========================================
// GuardrailHandler 安全护栏处理器
// ========================================

// GuardrailHandler 安全护栏处理器
type GuardrailHandler struct {
	agenttoolsService *agenttools.GuardrailServiceImpl
}

// NewGuardrailHandler 创建安全护栏处理器
func NewGuardrailHandler(agenttoolsService *agenttools.GuardrailServiceImpl) *GuardrailHandler {
	return &GuardrailHandler{
		agenttoolsService: agenttoolsService,
	}
}

// ========================================
// 输入检查接口
// ========================================

// CheckInput 检查输入是否安全
// @Summary 检查输入
// @Description 检查用户输入内容是否安全
// @Tags agenttools
// @Accept json
// @Produce json
// @Router /api/v1/agenttools/input/check [post]
func (h *GuardrailHandler) CheckInput(c *gin.Context) {
	var req struct {
		Input    string  `json:"input" binding:"required"`
		Strict   bool    `json:"strict"`
		Language string  `json:"language"`
		Domain   string  `json:"domain"`
	}
	if !BindJSON(c, &req) {
		return
	}

	opts := &domainguardrail.FilterOptions{}

	result, err := h.agenttoolsService.CheckInput(c.Request.Context(), req.Input, opts)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// SanitizeInput 清理输入内容
// @Summary 清理输入
// @Description 清理用户输入中的敏感内容
// @Tags agenttools
// @Accept json
// @Produce json
// @Router /api/v1/agenttools/input/sanitize [post]
func (h *GuardrailHandler) SanitizeInput(c *gin.Context) {
	var req struct {
		Input  string `json:"input" binding:"required"`
		Strict bool   `json:"strict"`
	}
	if !BindJSON(c, &req) {
		return
	}

	opts := &domainguardrail.FilterOptions{}

	sanitized, err := h.agenttoolsService.SanitizeInput(c.Request.Context(), req.Input, opts)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, map[string]string{
		"original":  req.Input,
		"sanitized": sanitized,
	})
}

// ========================================
// 输出检查接口
// ========================================

// CheckOutput 检查输出是否安全
// @Summary 检查输出
// @Description 检查 AI 输出内容是否安全
// @Tags agenttools
// @Accept json
// @Produce json
// @Router /api/v1/agenttools/output/check [post]
func (h *GuardrailHandler) CheckOutput(c *gin.Context) {
	var req struct {
		Input           string `json:"input"`
		Output          string `json:"output" binding:"required"`
		EnablePII       bool   `json:"enable_pii"`
		EnableProfanity bool   `json:"enable_profanity"`
		EnableHateSpeech bool  `json:"enable_hate_speech"`
		EnableViolence  bool   `json:"enable_violence"`
	}
	if !BindJSON(c, &req) {
		return
	}

	opts := &domainguardrail.OutputFilterOptions{
		EnablePII:        req.EnablePII,
		EnableProfanity:  req.EnableProfanity,
		EnableHateSpeech: req.EnableHateSpeech,
		EnableViolence:   req.EnableViolence,
	}

	result, err := h.agenttoolsService.CheckOutput(c.Request.Context(), req.Output, req.Input, opts)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// SanitizeOutput 清理输出内容
// @Summary 清理输出
// @Description 清理 AI 输出中的敏感内容
// @Tags agenttools
// @Accept json
// @Produce json
// @Router /api/v1/agenttools/output/sanitize [post]
func (h *GuardrailHandler) SanitizeOutput(c *gin.Context) {
	var req struct {
		Input  string `json:"input"`
		Output string `json:"output" binding:"required"`
		MaskPII bool   `json:"mask_pii"`
	}
	if !BindJSON(c, &req) {
		return
	}

	opts := &domainguardrail.OutputFilterOptions{
		MaskPII: req.MaskPII,
	}

	sanitized, err := h.agenttoolsService.SanitizeOutput(c.Request.Context(), req.Output, req.Input, opts)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, map[string]string{
		"original":  req.Output,
		"sanitized": sanitized,
	})
}

// ========================================
// 越狱检测接口
// ========================================

// CheckJailbreak 检查越狱攻击
// @Summary 检查越狱攻击
// @Description 检测用户输入是否为越狱攻击
// @Tags agenttools
// @Accept json
// @Produce json
// @Router /api/v1/agenttools/jailbreak/check [post]
func (h *GuardrailHandler) CheckJailbreak(c *gin.Context) {
	var req struct {
		Input     string `json:"input" binding:"required"`
		Strict    bool   `json:"strict"`
		EnableLLM bool   `json:"enable_llm"`
	}
	if !BindJSON(c, &req) {
		return
	}

	opts := &domainguardrail.JailbreakOptions{}

	result, err := h.agenttoolsService.CheckJailbreak(c.Request.Context(), req.Input, opts)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// ========================================
// 综合检查接口
// ========================================

// FullInputCheck 完整的输入检查
// @Summary 完整输入检查
// @Description 执行完整的输入安全检查（包含越狱检测）
// @Tags agenttools
// @Accept json
// @Produce json
// @Router /api/v1/agenttools/full-check [post]
func (h *GuardrailHandler) FullInputCheck(c *gin.Context) {
	var req struct {
		Input              string   `json:"input" binding:"required"`
		EnableJailbreak    bool     `json:"enable_jailbreak"`
		EnableInputCheck   bool     `json:"enable_input_check"`
		EnablePatternCheck bool     `json:"enable_pattern_check"`
		CustomPatterns     []string `json:"custom_patterns"`
		Strict             bool     `json:"strict"`
	}
	if !BindJSON(c, &req) {
		return
	}

	opts := &agenttools.FullCheckOptions{
		EnableJailbreakCheck: req.EnableJailbreak,
		EnableInputCheck:     req.EnableInputCheck,
		EnablePatternCheck:   req.EnablePatternCheck,
		CustomPatterns:       req.CustomPatterns,
	}

	if req.Strict {
		opts.JailbreakOptions = &domainguardrail.JailbreakOptions{}
		opts.InputOptions = &domainguardrail.FilterOptions{}
	}

	result, err := h.agenttoolsService.FullInputCheck(c.Request.Context(), req.Input, opts)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// CheckInputAndOutput 检查输入和输出
// @Summary 检查输入和输出
// @Description 同时检查输入和输出的安全性
// @Tags agenttools
// @Accept json
// @Produce json
// @Router /api/v1/agenttools/check-both [post]
func (h *GuardrailHandler) CheckInputAndOutput(c *gin.Context) {
	var req struct {
		Input  string `json:"input" binding:"required"`
		Output string `json:"output" binding:"required"`
		Strict bool   `json:"strict"`
	}
	if !BindJSON(c, &req) {
		return
	}

	var inputOpts *domainguardrail.FilterOptions
	var outputOpts *domainguardrail.OutputFilterOptions

	if req.Strict {
		inputOpts = &domainguardrail.FilterOptions{}
		outputOpts = &domainguardrail.OutputFilterOptions{}
	}

	result, err := h.agenttoolsService.CheckInputAndOutput(c.Request.Context(), req.Input, req.Output, inputOpts, outputOpts)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, result)
}

// ========================================
// 快捷方法接口
// ========================================

// QuickCheck 快速检查输入
// @Summary 快速检查
// @Description 使用默认配置快速检查输入是否安全
// @Tags agenttools
// @Accept json
// @Produce json
// @Router /api/v1/agenttools/quick-check [post]
func (h *GuardrailHandler) QuickCheck(c *gin.Context) {
	var req struct {
		Input string `json:"input" binding:"required"`
	}
	if !BindJSON(c, &req) {
		return
	}

	isSafe, err := h.agenttoolsService.QuickCheck(c.Request.Context(), req.Input)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, map[string]interface{}{
		"is_safe": isSafe,
	})
}

// QuickSanitize 快速清理输入
// @Summary 快速清理
// @Description 使用默认配置快速清理输入
// @Tags agenttools
// @Accept json
// @Produce json
// @Router /api/v1/agenttools/quick-sanitize [post]
func (h *GuardrailHandler) QuickSanitize(c *gin.Context) {
	var req struct {
		Input string `json:"input" binding:"required"`
	}
	if !BindJSON(c, &req) {
		return
	}

	sanitized, err := h.agenttoolsService.QuickSanitize(c.Request.Context(), req.Input)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, map[string]string{
		"sanitized": sanitized,
	})
}

// IsJailbreak 快速检查是否是越狱攻击
// @Summary 快速越狱检查
// @Description 使用默认配置快速检查是否为越狱攻击
// @Tags agenttools
// @Accept json
// @Produce json
// @Router /api/v1/agenttools/is-jailbreak [post]
func (h *GuardrailHandler) IsJailbreak(c *gin.Context) {
	var req struct {
		Input string `json:"input" binding:"required"`
	}
	if !BindJSON(c, &req) {
		return
	}

	isJailbreak, result, err := h.agenttoolsService.IsJailbreak(c.Request.Context(), req.Input)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	OK(c, map[string]interface{}{
		"is_jailbreak": isJailbreak,
		"result":       result,
	})
}

// ========================================
// 获取配置接口
// ========================================

// GetDefaultConfig 获取默认配置
// @Summary 获取默认配置
// @Description 获取安全护栏的默认配置
// @Tags agenttools
// @Router /api/v1/agenttools/config/default [get]
func (h *GuardrailHandler) GetDefaultConfig(c *gin.Context) {
	config := agenttools.DefaultFullCheckOptions()
	OK(c, config)
}

// GetRecommendation 获取处理建议
// @Summary 获取处理建议
// @Description 根据检查结果获取处理建议
// @Tags agenttools
// @Accept json
// @Produce json
// @Router /api/v1/agenttools/recommendation [post]
func (h *GuardrailHandler) GetRecommendation(c *gin.Context) {
	var req agenttools.FullCheckResult
	if !BindJSON(c, &req) {
		return
	}

	recommendation := h.agenttoolsService.GetRecommendation(&req)
	OK(c, map[string]string{
		"recommendation": recommendation,
	})
}
