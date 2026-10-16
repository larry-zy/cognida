package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domain "cognida/internal/model/collection"
	l "cognida/internal/model/lifecycle"
	collectionsvc "cognida/internal/service/collection"
	knowledge "cognida/internal/service/knowledge"
)

// CollectionHandler exposes the collection bounded context. It deliberately
// does not use the Data Agent or its web-tool registry.
type CollectionHandler struct {
	service *collectionsvc.Service
	repo    domain.Repository
	kbs     knowledge.KnowledgeBaseService
}

func NewCollectionHandler(service *collectionsvc.Service, repo domain.Repository, kbs knowledge.KnowledgeBaseService) *CollectionHandler {
	return &CollectionHandler{service: service, repo: repo, kbs: kbs}
}

type createCollectionTaskRequest struct {
	KnowledgeBaseID  string   `json:"knowledge_base_id" binding:"required"`
	Sources          []string `json:"sources" binding:"required,min=1"`
	MaxResponseBytes int64    `json:"max_response_bytes"`
}

// List returns the tenant's collection tasks. Task details remain available
// from Get so the list remains bounded and inexpensive for the console.
func (h *CollectionHandler) List(c *gin.Context) {
	tenantID, userID, ok := MustGetTenantAndUserID(c)
	if !ok {
		return
	}
	status := domain.TaskStatus(c.Query("status"))
	if status != "" && status != domain.TaskStatusDraft && status != domain.TaskStatusRunning && !status.IsTerminal() {
		BadRequest(c, "invalid collection task status")
		return
	}
	tasks, err := h.repo.ListTasks(c.Request.Context(), tenantID, status, 100)
	if err != nil {
		h.writeError(c, err)
		return
	}
	visible := make([]*domain.Task, 0, len(tasks))
	for _, task := range tasks {
		if task.UserID == userID {
			visible = append(visible, task)
		}
	}
	OK(c, gin.H{"tasks": visible})
}

const (
	maxCollectionSources = 50
	maxCollectionBytes   = 8 << 20
)

// Create creates a draft task. An Idempotency-Key header is mandatory because
// collection work can have externally visible ingestion effects.
func (h *CollectionHandler) Create(c *gin.Context) {
	tenantID, userID, ok := MustGetTenantAndUserID(c)
	if !ok {
		return
	}
	var req createCollectionTaskRequest
	if !BindJSON(c, &req) {
		return
	}
	if len(req.Sources) > maxCollectionSources {
		BadRequest(c, "at most 50 collection sources are allowed")
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		BadRequest(c, "Idempotency-Key is required")
		return
	}
	kb, err := h.kbs.FindByID(c.Request.Context(), req.KnowledgeBaseID, tenantID)
	if err != nil || kb == nil || kb.TenantID != tenantID || kb.DeletedAt != nil || !kb.IsActive() {
		NotFound(c, "knowledge base not found")
		return
	}
	maxBytes := req.MaxResponseBytes
	if maxBytes == 0 {
		maxBytes = 2 << 20
	}
	if maxBytes < 1 || maxBytes > maxCollectionBytes {
		BadRequest(c, "max_response_bytes must be between 1 and 8388608")
		return
	}
	now := time.Now().UTC()
	task := &domain.Task{ID: uuid.NewString(), TenantID: tenantID, UserID: userID, KnowledgeBaseID: req.KnowledgeBaseID, IdempotencyKey: key, Status: domain.TaskStatusDraft, Limits: domain.Limits{MaxSources: len(req.Sources), MaxResponseBytes: maxBytes}, CreatedAt: now, UpdatedAt: now}
	sources := make([]*domain.Source, 0, len(req.Sources))
	for _, raw := range req.Sources {
		sources = append(sources, &domain.Source{ID: uuid.NewString(), TaskID: task.ID, URL: raw, CreatedAt: now})
	}
	created, err := h.service.Create(c.Request.Context(), task, sources)
	if err != nil {
		h.writeError(c, err)
		return
	}
	Created(c, created)
}

func (h *CollectionHandler) Start(c *gin.Context) {
	tenantID, userID, ok := MustGetTenantAndUserID(c)
	if !ok {
		return
	}
	if !h.requireOwner(c, tenantID, userID) {
		return
	}
	if err := h.service.Start(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		h.writeError(c, err)
		return
	}
	OK(c, gin.H{"message": "collection task started"})
}

func (h *CollectionHandler) Cancel(c *gin.Context) {
	tenantID, userID, ok := MustGetTenantAndUserID(c)
	if !ok {
		return
	}
	if !h.requireOwner(c, tenantID, userID) {
		return
	}
	if err := h.service.Cancel(c.Request.Context(), tenantID, c.Param("id")); err != nil {
		h.writeError(c, err)
		return
	}
	OK(c, gin.H{"message": "collection task cancelled"})
}

// Get returns the task, its immutable seeds, and the latest run's inspectable
// item outcomes. Finished run history will be added when the repository grows
// a paged ListRuns port.
func (h *CollectionHandler) Get(c *gin.Context) {
	tenantID, userID, ok := MustGetTenantAndUserID(c)
	if !ok {
		return
	}
	task, err := h.repo.FindTask(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	if task.UserID != userID {
		Forbidden(c, "无权查看此采集任务")
		return
	}
	sources, err := h.repo.ListSources(c.Request.Context(), tenantID, task.ID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response := gin.H{"task": task, "sources": sources}
	if run, err := h.repo.FindLatestRun(c.Request.Context(), tenantID, task.ID); err == nil {
		items, itemErr := h.repo.ListItems(c.Request.Context(), tenantID, run.ID)
		if itemErr != nil {
			h.writeError(c, itemErr)
			return
		}
		response["run"], response["items"] = run, items
		summary := domain.SummarizeItems(len(sources), items)
		if run.Status == domain.RunStatusRunning || run.Status == domain.RunStatusCancelled || run.Status == domain.RunStatusFailed {
			summary.Outcome = string(run.Status)
		}
		response["summary"] = summary
	} else if !errors.Is(err, domain.ErrRunNotFound) {
		h.writeError(c, err)
		return
	}
	OK(c, response)
}

// GetRun reads an exact historical run. It never starts collection or expands
// the latest-run cursor, and ownership is checked before returning sources.
func (h *CollectionHandler) GetRun(c *gin.Context) {
	tenant, user, ok := MustGetTenantAndUserID(c)
	if !ok {
		return
	}
	reader, valid := h.repo.(interface {
		FindRun(context.Context, int64, string) (*domain.Run, error)
	})
	if !valid {
		InternalError(c, "采集运行读取不可用")
		return
	}
	run, err := reader.FindRun(c.Request.Context(), tenant, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	task, err := h.repo.FindTask(c.Request.Context(), tenant, run.TaskID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if task.UserID != user {
		Forbidden(c, "无权查看此采集任务")
		return
	}
	sources, err := h.repo.ListSources(c.Request.Context(), tenant, task.ID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	items, err := h.repo.ListItems(c.Request.Context(), tenant, run.ID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	summary := domain.SummarizeItems(len(sources), items)
	if run.Status != domain.RunStatusCompleted {
		summary.Outcome = string(run.Status)
	}
	OK(c, gin.H{"task": task, "sources": sources, "run": run, "items": items, "summary": summary})
}

func (h *CollectionHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, l.ErrForbidden):
		Forbidden(c, "无权访问采集任务")
	case errors.Is(err, l.ErrConflict):
		Conflict(c, "idempotency key conflicts", nil)
	case errors.Is(err, domain.ErrTaskNotFound), errors.Is(err, domain.ErrRunNotFound):
		NotFound(c, "collection task not found")
	case errors.Is(err, domain.ErrInvalidTask), errors.Is(err, domain.ErrInvalidSource):
		BadRequest(c, err.Error())
	case errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrActiveRunExists):
		Conflict(c, err.Error(), nil)
	default:
		InternalError(c, "collection task operation failed")
	}
}

func (h *CollectionHandler) requireOwner(c *gin.Context, tenant, user int64) bool {
	task, e := h.repo.FindTask(c.Request.Context(), tenant, c.Param("id"))
	if e != nil {
		h.writeError(c, e)
		return false
	}
	if task.UserID != user {
		Forbidden(c, "无权操作此采集任务")
		return false
	}
	if task.Mode == domain.SnapshotMode {
		Conflict(c, "请通过统一任务控制受控采集", nil)
		return false
	}
	return true
}
