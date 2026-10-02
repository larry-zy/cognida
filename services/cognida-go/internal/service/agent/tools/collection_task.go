package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/google/uuid"

	agentctx "cognida/internal/model/agent"
	domain "cognida/internal/model/collection"
	domainknowledge "cognida/internal/model/knowledge"
	collectionsvc "cognida/internal/service/collection"
)

type collectionCreateRequest struct {
	KnowledgeBaseID string   `json:"knowledge_base_id"`
	Sources         []string `json:"sources"`
	IdempotencyKey  string   `json:"idempotency_key"`
}
type collectionTaskRequest struct {
	TaskID string `json:"task_id"`
}
type collectionTaskResponse struct {
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// NewCollectionTaskTools exposes only task commands. It deliberately does not
// expose an arbitrary URL fetch primitive to an LLM.
func NewCollectionTaskTools(service *collectionsvc.Service, repo domain.Repository, kbRepo domainknowledge.KnowledgeBaseRepository) []tool.BaseTool {
	return []tool.BaseTool{
		NewTypedBaseTool("collection_create_task", "创建受治理的网页采集任务。仅接受明确指定的 HTTPS URL；不会直接抓取网页。", func(ctx context.Context, req *collectionCreateRequest) (*collectionTaskResponse, error) {
			tenantID, userID := agentctx.MustGetTenantID(ctx), agentctx.MustGetUserID(ctx)
			if tenantID <= 0 || userID <= 0 || strings.TrimSpace(req.KnowledgeBaseID) == "" || len(req.Sources) == 0 || len(req.Sources) > 50 {
				return nil, fmt.Errorf("tenant、user、knowledge_base_id 与 1-50 个 sources 均为必填")
			}
			if kbRepo == nil {
				return nil, fmt.Errorf("知识库校验服务未接线")
			}
			if _, err := kbRepo.FindByIDForTenant(ctx, req.KnowledgeBaseID, tenantID); err != nil {
				return nil, fmt.Errorf("知识库不可访问: %w", err)
			}
			now := time.Now().UTC()
			key := strings.TrimSpace(req.IdempotencyKey)
			if key == "" {
				key = uuid.NewString()
			}
			task := &domain.Task{ID: uuid.NewString(), TenantID: tenantID, UserID: userID, KnowledgeBaseID: req.KnowledgeBaseID, IdempotencyKey: key, Status: domain.TaskStatusDraft, Limits: domain.Limits{MaxSources: len(req.Sources), MaxResponseBytes: 2 << 20}, CreatedAt: now, UpdatedAt: now}
			sources := make([]*domain.Source, 0, len(req.Sources))
			for _, raw := range req.Sources {
				sources = append(sources, &domain.Source{ID: uuid.NewString(), TaskID: task.ID, URL: strings.TrimSpace(raw), CreatedAt: now})
			}
			created, err := service.Create(ctx, task, sources)
			if err != nil {
				return nil, err
			}
			return &collectionTaskResponse{TaskID: created.ID, Status: string(created.Status)}, nil
		}),
		NewTypedBaseTool("collection_start_task", "启动已创建的数据收集任务。", func(ctx context.Context, req *collectionTaskRequest) (*collectionTaskResponse, error) {
			if err := service.Start(ctx, agentctx.MustGetTenantID(ctx), req.TaskID); err != nil {
				return nil, err
			}
			return &collectionTaskResponse{TaskID: req.TaskID, Status: string(domain.TaskStatusRunning)}, nil
		}),
		NewTypedBaseTool("collection_inspect_task", "查看数据收集任务的持久状态。", func(ctx context.Context, req *collectionTaskRequest) (*collectionTaskResponse, error) {
			task, err := repo.FindTask(ctx, agentctx.MustGetTenantID(ctx), req.TaskID)
			if err != nil {
				return nil, err
			}
			return &collectionTaskResponse{TaskID: task.ID, Status: string(task.Status), Message: task.ErrorMessage}, nil
		}),
		NewTypedBaseTool("collection_cancel_task", "取消未结束的数据收集任务。", func(ctx context.Context, req *collectionTaskRequest) (*collectionTaskResponse, error) {
			if err := service.Cancel(ctx, agentctx.MustGetTenantID(ctx), req.TaskID); err != nil {
				return nil, err
			}
			return &collectionTaskResponse{TaskID: req.TaskID, Status: string(domain.TaskStatusCancelled)}, nil
		}),
	}
}
