package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	svc "cognida/internal/service/taskcontrol"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskJSONRejectsAmbiguousAndPrivilegedFields(t *testing.T) {
	for _, body := range []string{
		`{"goal":"a","goal":"b"}`, `{"goal":"a","Goal":"b"}`, `{"tenant_id":9}`, `{"approved":true}`, `{"execution_enabled":true}`, `{"goal":"a"} {}`, `null`, `[]`,
		`{"budget":{"max_nodes":1,"max_nodes":2}}`, strings.Repeat("[", 30) + strings.Repeat("]", 30), strings.Repeat(" ", 256<<10) + `{}`,
	} {
		t.Run(body[:min(len(body), 40)], func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
			var in svc.DraftInput
			require.False(t, readTaskJSON(c, &in))
			require.Equal(t, 400, w.Code)
		})
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"goal":"保存计划","input_refs":[],"budget":{"max_nodes":1}}`))
	var in svc.DraftInput
	require.True(t, readTaskJSON(c, &in))
	require.Equal(t, "保存计划", in.Goal)
}

func TestTaskJSONRejectsUnicodeCaseFoldAliases(t *testing.T) {
	for _, body := range []string{
		`{"schema_version":2,"ſchema_version":1}`,
		`{"schema_version":2,"\u017fchema_version":1}`,
		`{"budget":{"max_cost_micros":2,"max_coſt_micros":1}}`,
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		var in svc.PlanSpec
		require.False(t, readTaskJSON(c, &in), body)
		require.Equal(t, 400, w.Code)
	}
}
