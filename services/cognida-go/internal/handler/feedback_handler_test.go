package handler

import (
	ev "cognida/internal/model/evaluation"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFeedbackErrorPreservesDatasetAccessFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		err    error
		status int
	}{
		{ev.ErrVersionForbidden, 403}, {ev.ErrVersionNotFound, 404},
		{ev.ErrVersionConflict, 409}, {ev.ErrDatasetTypeMismatch, 400},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			r := gin.New()
			r.GET("/feedback", func(c *gin.Context) { feedbackError(c, fmt.Errorf("fixed comparison: %w", tc.err)) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/feedback", nil))
			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), tc.err.Error())
		})
	}
}
