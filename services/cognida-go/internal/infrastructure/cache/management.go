// Package cache provides cache management service implementation
package cache

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	domaincache "cognida/internal/model/cache"
	milvuscache "cognida/internal/repository/milvus"
	rediscache "cognida/internal/repository/redis"
)

// ========================================
// Cache Management Types
// ========================================

// 缓存管理 DTO 已下沉至 model/cache（领域端口）。此处保留别名，
// 使 infrastructure 内部引用与外部依赖均指向同一领域类型。
type (
	// ClearCacheRequest 清除缓存请求
	ClearCacheRequest = domaincache.ClearCacheRequest
	// ClearCacheResponse 清除缓存响应
	ClearCacheResponse = domaincache.ClearCacheResponse
	// CacheStatsResponse 缓存统计响应
	CacheStatsResponse = domaincache.CacheStatsResponse
	// WarmupCacheQuery 预热缓存查询项
	WarmupCacheQuery = domaincache.WarmupCacheQuery
	// WarmupCacheRequest 预热缓存请求
	WarmupCacheRequest = domaincache.WarmupCacheRequest
	// WarmupCacheResponse 预热缓存响应
	WarmupCacheResponse = domaincache.WarmupCacheResponse
	// CacheHealthResponse 缓存健康状态响应
	CacheHealthResponse = domaincache.CacheHealthResponse
)

// 编译期断言：*CacheManagementService 满足领域端口 CacheManager。
var _ domaincache.CacheManager = (*CacheManagementService)(nil)

// CacheManagementService 缓存管理服务
type CacheManagementService struct {
	cacheService *SemanticCacheService
	vectorRepo   domaincache.CacheVectorRepository
	contentRepo  domaincache.CacheContentRepository
}

// NewCacheManagementService 创建缓存管理服务
func NewCacheManagementService(
	cacheService *SemanticCacheService,
	vectorRepo domaincache.CacheVectorRepository,
	contentRepo domaincache.CacheContentRepository,
) *CacheManagementService {
	return &CacheManagementService{
		cacheService: cacheService,
		vectorRepo:   vectorRepo,
		contentRepo:  contentRepo,
	}
}

// ========================================
// Cache Management API
// ========================================

// ClearCache 清除缓存
func (s *CacheManagementService) ClearCache(ctx context.Context, req *ClearCacheRequest) (*ClearCacheResponse, error) {
	count, err := s.clearCacheInternal(ctx, req.TenantID, req.AgentType, req.Model)
	if err != nil {
		return nil, err
	}

	return &ClearCacheResponse{
		DeletedCount: count,
		Message:      fmt.Sprintf("Successfully cleared %d cache entries", count),
	}, nil
}

// GetCacheStats 获取缓存统计
func (s *CacheManagementService) GetCacheStats(ctx context.Context) (*CacheStatsResponse, error) {
	stats, err := s.cacheService.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get cache stats: %w", err)
	}

	return &CacheStatsResponse{
		Hits:           stats.Hits,
		Misses:         stats.Misses,
		HitRate:        stats.HitRate,
		SimilarityDist: stats.SimilarityDist,
	}, nil
}

// WarmupCache 预热缓存
func (s *CacheManagementService) WarmupCache(ctx context.Context, req *WarmupCacheRequest) (*WarmupCacheResponse, error) {
	total := len(req.Queries)
	success := 0
	failed := 0
	cacheIDs := make([]string, 0, total)

	for i, item := range req.Queries {
		cacheReq := &domaincache.CacheRequest{
			Query:          item.Query,
			PromptTemplate: item.PromptTemplate,
			TenantID:       req.TenantID,
			AgentType:      item.AgentType,
			Model:          item.Model,
			Metadata:       item.Metadata,
		}

		// 模拟生成响应（实际使用时需要调用 LLM）
		resp := fmt.Sprintf("Cached response for: %s", item.Query)

		if err := s.cacheService.Set(ctx, cacheReq, resp, 0); err != nil {
			failed++
			log.Printf("[Cache] Warmup failed for item %d: %v", i, err)
		} else {
			success++
			// 注意：实际 cache_id 是在异步写入时生成的，这里简化处理
			cacheIDs = append(cacheIDs, fmt.Sprintf("warmup_%d", i))
		}

		// 更新进度
		progress := float64(i+1) / float64(total) * 100
		log.Printf("[Cache] Warmup progress: %.1f%% (%d/%d)", progress, i+1, total)
	}

	return &WarmupCacheResponse{
		Total:    total,
		Success:  success,
		Failed:   failed,
		Progress: 100.0,
		Message:  fmt.Sprintf("Warmup completed: %d succeeded, %d failed", success, failed),
		CacheIDs: cacheIDs,
	}, nil
}

// GetHealth 获取缓存健康状态
func (s *CacheManagementService) GetHealth(ctx context.Context) (*CacheHealthResponse, error) {
	response := &CacheHealthResponse{
		Healthy:      true,
		RedisStatus:  "ok",
		MilvusStatus: "ok",
	}

	// 检查 Redis
	if redisRepo, ok := s.contentRepo.(*rediscache.CacheContentRepository); ok {
		if err := redisRepo.Ping(ctx); err != nil {
			response.Healthy = false
			response.RedisStatus = "error"
			response.Message += fmt.Sprintf(" Redis error: %v", err)
		}
	}

	// 检查 Milvus
	if milvusRepo, ok := s.vectorRepo.(*milvuscache.CacheVectorRepository); ok {
		stats, err := milvusRepo.GetCacheCollectionStats(ctx)
		if err != nil {
			response.Healthy = false
			response.MilvusStatus = "error"
			response.Message += fmt.Sprintf(" Milvus error: %v", err)
		} else {
			if count, ok := stats["row_count"].(int64); ok {
				response.VectorCount = count
			}
		}
	}

	// 获取缓存数量
	stats, _ := s.cacheService.GetStats(ctx)
	if stats != nil {
		response.CacheCount = stats.Hits + stats.Misses
	}

	return response, nil
}

// ResetStats 重置统计信息
func (s *CacheManagementService) ResetStats(ctx context.Context) error {
	if redisRepo, ok := s.contentRepo.(*rediscache.CacheContentRepository); ok {
		return redisRepo.ResetStats(ctx)
	}
	return fmt.Errorf("content repo type assertion failed")
}

// ========================================
// Helper Methods
// ========================================

// clearCacheInternal 内部清除缓存方法
func (s *CacheManagementService) clearCacheInternal(ctx context.Context, tenantID int64, agentType, model string) (int64, error) {
	var count int64
	var err error

	// 根据条件清除
	if agentType != "" && model != "" {
		// 按 Agent 和 Model 清除
		count, err = s.clearByAgentAndModel(ctx, tenantID, agentType, model)
	} else if agentType != "" {
		// 按 Agent 清除
		if repo, ok := s.contentRepo.(*rediscache.CacheContentRepository); ok {
			count, err = repo.ClearByAgent(ctx, tenantID, agentType)
		}
		if vecRepo, ok := s.vectorRepo.(*milvuscache.CacheVectorRepository); ok {
			_ = vecRepo.DeleteByAgent(ctx, tenantID, agentType)
		}
	} else {
		// 按租户清除全部
		if repo, ok := s.contentRepo.(*rediscache.CacheContentRepository); ok {
			count, err = repo.ClearByTenant(ctx, tenantID)
		}
		if vecRepo, ok := s.vectorRepo.(*milvuscache.CacheVectorRepository); ok {
			_ = vecRepo.DeleteByTenant(ctx, tenantID)
		}
	}

	if err != nil {
		return 0, fmt.Errorf("clear cache failed: %w", err)
	}

	return count, nil
}

// clearByAgentAndModel 按 Agent 和 Model 清除
func (s *CacheManagementService) clearByAgentAndModel(ctx context.Context, tenantID int64, agentType, model string) (int64, error) {
	if repo, ok := s.contentRepo.(*rediscache.CacheContentRepository); ok {
		cacheIDs, err := repo.GetAgentCacheIDs(ctx, tenantID, agentType)
		if err != nil {
			return 0, err
		}

		// 筛选匹配模型的缓存
		targetIDs := make([]string, 0)
		for _, cacheID := range cacheIDs {
			entry, err := s.contentRepo.GetContent(ctx, cacheID)
			if err != nil {
				continue
			}
			if entry.Model == model {
				targetIDs = append(targetIDs, cacheID)
			}
		}

		// 批量删除
		for _, cacheID := range targetIDs {
			_ = s.contentRepo.Delete(ctx, cacheID)
			_ = s.vectorRepo.Delete(ctx, cacheID)
		}

		return int64(len(targetIDs)), nil
	}

	return 0, fmt.Errorf("content repo type assertion failed")
}

// CleanupOrphanVectors 清理孤儿向量
func (s *CacheManagementService) CleanupOrphanVectors(ctx context.Context, req domaincache.OrphanCleanupRequest) (*domaincache.OrphanCleanupResult, error) {
	result := &domaincache.OrphanCleanupResult{}
	if req.GracePeriod == 0 {
		req.GracePeriod = time.Minute
	}
	if req.MaxVectors == 0 {
		req.MaxVectors = 10000
	}
	if req.BatchSize == 0 {
		req.BatchSize = 256
	}
	if req.TenantID < 1 || req.GracePeriod < time.Minute || req.GracePeriod > 30*24*time.Hour || req.MaxVectors < 1 || req.MaxVectors > 100000 || req.BatchSize < 1 || req.BatchSize > 1000 {
		return result, domaincache.ErrCleanupInvalid
	}
	vectors, vok := s.vectorRepo.(domaincache.CacheCleanupVectorRepository)
	contents, cok := s.contentRepo.(domaincache.CacheCleanupContentRepository)
	if !vok || !cok {
		return result, domaincache.ErrCleanupUnsupported
	}
	cutoff := time.Now().Add(-req.GracePeriod).Unix()
	rows, err := vectors.ListCleanupCandidates(ctx, req.TenantID, cutoff, req.MaxVectors, req.BatchSize)
	if err != nil {
		return result, fmt.Errorf("enumerate cache vectors: %w", err)
	}
	if len(rows) > req.MaxVectors {
		return result, domaincache.ErrCleanupIncomplete
	}
	fences, err := contents.ListCleanupFences(ctx, req.TenantID, req.MaxVectors, req.BatchSize)
	if err != nil {
		return result, fmt.Errorf("enumerate cleanup receipts: %w", err)
	}
	pending := make(map[string]domaincache.CacheVectorIdentity, len(fences))
	fencedIDs := make(map[string]bool, len(fences))
	for _, f := range fences {
		if f.Identity.Validate() != nil || f.Identity.TenantID != req.TenantID || f.Token == "" {
			return result, domaincache.ErrCacheOwnership
		}
		switch f.State {
		case "unknown":
			return result, domaincache.ErrCleanupReconcile
		case "active":
			return result, errors.Join(domaincache.ErrCleanupBusy, domaincache.ErrCleanupReconcile)
		case "failed":
		default:
			return result, domaincache.ErrCleanupReconcile
		}
		if _, exists := pending[f.Identity.CacheID]; exists {
			return result, domaincache.ErrCleanupIncomplete
		}
		pending[f.Identity.CacheID] = f.Identity
		fencedIDs[f.Identity.CacheID] = true
	}
	// Include definite-failure receipts whose vectors disappeared separately.
	// An exact acknowledged delete of zero rows can safely release those fences.
	for _, row := range rows {
		if f, exists := pending[row.CacheID]; exists {
			if f != row {
				return result, domaincache.ErrCacheOwnership
			}
			delete(pending, row.CacheID)
		}
	}
	for _, row := range pending {
		rows = append(rows, row)
	}
	if len(rows) > req.MaxVectors {
		return result, domaincache.ErrCleanupIncomplete
	}
	seen := make(map[string]bool, len(rows))
	candidates := make([]domaincache.CacheVectorIdentity, 0, len(rows))
	// Complete all reads before the first fence/delete. A late page, malformed
	// primary hash, ownership mismatch or Redis failure cannot authorize a sweep.
	for _, row := range rows {
		if row.Validate() != nil || row.TenantID != req.TenantID || row.CreatedAt > cutoff || seen[row.CacheID] {
			return result, domaincache.ErrCacheOwnership
		}
		seen[row.CacheID] = true
		present, err := contents.CleanupContentPresent(ctx, row)
		if err != nil {
			return result, fmt.Errorf("inspect cache content: %w", err)
		}
		result.Scanned++
		if present {
			if fencedIDs[row.CacheID] {
				return result, domaincache.ErrCleanupReconcile
			}
			result.Preserved++
		} else {
			candidates = append(candidates, row)
		}
	}
	result.Candidates = len(candidates)
	for _, row := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		fence, err := contents.ClaimCleanup(ctx, row)
		if err != nil {
			return result, fmt.Errorf("fence absent cache: %w", err)
		}
		if fence == nil {
			result.Preserved++
			continue
		}
		count, deleteErr := vectors.DeleteCleanupCandidate(ctx, row)
		if deleteErr == nil && (count < 0 || count > 1) {
			deleteErr = domaincache.ErrCleanupReconcile
		}
		state := "deleted"
		if deleteErr != nil {
			state = "unknown"
			var failure *domaincache.CacheDeleteError
			if errors.As(deleteErr, &failure) && failure.NotApplied {
				state = "failed"
			}
		} else {
			result.Deleted += count
		}
		// Preserve a receipt/fence even if the operation deadline just elapsed.
		final, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		finishErr := contents.FinishCleanup(final, *fence, state)
		cancel()
		if deleteErr != nil || finishErr != nil {
			if state == "unknown" {
				deleteErr = errors.Join(deleteErr, domaincache.ErrCleanupReconcile)
			}
			return result, errors.Join(deleteErr, finishErr)
		}
	}
	return result, nil
}

// ScheduleCleanup 定时清理任务
func (s *CacheManagementService) ScheduleCleanup(ctx context.Context, interval time.Duration, req domaincache.OrphanCleanupRequest) {
	if interval <= 0 || req.TenantID < 1 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Cache] Cleanup task stopped")
			return
		case <-ticker.C:
			log.Printf("[Cache] Running scheduled cleanup...")
			result, err := s.CleanupOrphanVectors(ctx, req)
			if err != nil {
				log.Printf("[Cache] Cleanup failed: %v", err)
			} else {
				log.Printf("[Cache] Cleanup completed: %d orphan vectors removed", result.Deleted)
			}
		}
	}
}
