package handler

import (
	a "cognida/internal/model/agent"
	ev "cognida/internal/model/evaluation"
	d "cognida/internal/model/feedback"
	l "cognida/internal/model/lifecycle"
	tc "cognida/internal/model/taskcontrol"
	"cognida/internal/service/feedback"
	control "cognida/internal/service/taskcontrol"
	"errors"
	"github.com/gin-gonic/gin"
)

type FeedbackHandler struct{ s *feedback.Service }

func NewFeedbackHandler(s *feedback.Service) *FeedbackHandler { return &FeedbackHandler{s} }
func (h *FeedbackHandler) Register(api *gin.RouterGroup) {
	contextMiddleware := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		p := gp(c)
		ctx := a.WithUserID(a.WithTenantID(c.Request.Context(), p.TenantID), p.UserID)
		c.Request = c.Request.WithContext(a.WithRequestID(ctx, lifecycleRequestID(c)))
		c.Next()
	}
	g := api.Group("/feedback-cases")
	g.Use(contextMiddleware)
	g.GET("", h.list)
	g.POST("", h.create)
	g.GET("/:id", h.get)
	g.GET("/:id/publications", h.publications)
	g.GET("/:id/events", h.events)
	g.POST("/:id/corrections", h.correct)
	g.POST("/:id/rework", h.rework)
	g.POST("/:id/candidates", h.candidate)
	g.POST("/:id/withdraw", h.withdraw)
	g.POST("/:id/rollback", h.rollback)
	v := api.Group("/feedback-publications")
	v.Use(contextMiddleware)
	v.POST("/:id/:action", h.control)
}
func feedbackError(c *gin.Context, e error) {
	code := 500
	switch {
	case errors.Is(e, d.ErrInvalid), errors.Is(e, l.ErrInvalid), errors.Is(e, tc.ErrInvalid), errors.Is(e, ev.ErrVersionInvalid), errors.Is(e, ev.ErrInvalidConfig), errors.Is(e, ev.ErrDatasetTypeMismatch):
		code = 400
	case errors.Is(e, d.ErrForbidden), errors.Is(e, l.ErrForbidden), errors.Is(e, tc.ErrForbidden), errors.Is(e, l.ErrExpired), errors.Is(e, ev.ErrVersionForbidden):
		code = 403
	case errors.Is(e, d.ErrNotFound), errors.Is(e, l.ErrNotFound), errors.Is(e, tc.ErrNotFound), errors.Is(e, ev.ErrVersionNotFound), errors.Is(e, ev.ErrDatasetNotFound), errors.Is(e, ev.ErrTaskNotFound):
		code = 404
	case errors.Is(e, d.ErrConflict), errors.Is(e, d.ErrGate), errors.Is(e, d.ErrBudget), errors.Is(e, l.ErrConflict), errors.Is(e, tc.ErrConflict), errors.Is(e, ev.ErrVersionConflict):
		code = 409
	}
	var validation *control.ValidationError
	if errors.As(e, &validation) {
		code = 400
	}
	message := "反馈闭环操作失败"
	if code < 500 {
		message = e.Error()
	}
	c.JSON(code, gin.H{"code": code, "message": message})
}
func (h *FeedbackHandler) list(c *gin.Context) {
	rows, next, e := h.s.List(c.Request.Context(), gp(c), c.Query("cursor"))
	if e != nil {
		feedbackError(c, e)
		return
	}
	OK(c, gin.H{"items": rows, "next_cursor": next})
}
func (h *FeedbackHandler) create(c *gin.Context) {
	var in d.Create
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.s.Create(c.Request.Context(), gp(c), in, c.GetHeader("Idempotency-Key"))
	if e != nil {
		feedbackError(c, e)
		return
	}
	OK(c, v)
}
func (h *FeedbackHandler) get(c *gin.Context) {
	v, e := h.s.Get(c.Request.Context(), gp(c), c.Param("id"))
	if e != nil {
		feedbackError(c, e)
		return
	}
	OK(c, v)
}
func (h *FeedbackHandler) publications(c *gin.Context) {
	v, e := h.s.Publications(c.Request.Context(), gp(c), c.Param("id"))
	if e != nil {
		feedbackError(c, e)
		return
	}
	OK(c, gin.H{"items": v})
}
func (h *FeedbackHandler) events(c *gin.Context) {
	v, e := h.s.Events(c.Request.Context(), gp(c), c.Param("id"))
	if e != nil {
		feedbackError(c, e)
		return
	}
	OK(c, gin.H{"items": v})
}
func (h *FeedbackHandler) rework(c *gin.Context) {
	var in struct {
		ExpectedVersion int              `json:"expected_version"`
		Plan            control.PlanSpec `json:"plan"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.s.Rework(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion, in.Plan, c.GetHeader("Idempotency-Key"), lifecycleRequestID(c))
	if e != nil {
		feedbackError(c, e)
		return
	}
	OK(c, v)
}
func (h *FeedbackHandler) correct(c *gin.Context) {
	var in struct {
		ExpectedVersion int    `json:"expected_version"`
		Text            string `json:"text"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.s.Correct(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion, in.Text, c.GetHeader("Idempotency-Key"))
	if e != nil {
		feedbackError(c, e)
		return
	}
	OK(c, v)
}
func (h *FeedbackHandler) candidate(c *gin.Context) {
	var in struct {
		ExpectedVersion int              `json:"expected_version"`
		Candidate       d.CandidateInput `json:"candidate"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.s.CreateCandidate(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion, in.Candidate, c.GetHeader("Idempotency-Key"))
	if e != nil {
		feedbackError(c, e)
		return
	}
	OK(c, v)
}
func (h *FeedbackHandler) withdraw(c *gin.Context) {
	var in struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.s.Withdraw(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion)
	if e != nil {
		feedbackError(c, e)
		return
	}
	OK(c, v)
}
func (h *FeedbackHandler) rollback(c *gin.Context) {
	var in struct {
		ExpectedVersion int    `json:"expected_version"`
		PublicationID   string `json:"publication_id"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	v, e := h.s.Rollback(c.Request.Context(), gp(c), c.Param("id"), in.PublicationID, in.ExpectedVersion)
	if e != nil {
		feedbackError(c, e)
		return
	}
	OK(c, v)
}
func (h *FeedbackHandler) control(c *gin.Context) {
	var in struct {
		ExpectedVersion     int    `json:"expected_version"`
		ExpectedCaseVersion int    `json:"expected_case_version"`
		Comment             string `json:"comment"`
	}
	if !readTaskJSON(c, &in) {
		return
	}
	var result any
	var e error
	switch c.Param("action") {
	case "project":
		result, e = h.s.Project(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion)
	case "validate":
		result, e = h.s.ValidateCandidate(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion)
	case "compare":
		result, e = h.s.Compare(c.Request.Context(), gp(c), c.Param("id"))
	case "approve":
		result, e = h.s.Approve(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedVersion, in.Comment)
	case "activate":
		result, e = h.s.Activate(c.Request.Context(), gp(c), c.Param("id"), in.ExpectedCaseVersion, in.ExpectedVersion)
	default:
		e = d.ErrInvalid
	}
	if e != nil {
		feedbackError(c, e)
		return
	}
	OK(c, result)
}
