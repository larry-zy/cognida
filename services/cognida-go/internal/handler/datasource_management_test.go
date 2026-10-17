package handler

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

type sourceManagementFunc func(context.Context, int64, int64) error

func (f sourceManagementFunc) AuthorizeTenantManagement(ctx context.Context, user, tenant int64) error {
	return f(ctx, user, tenant)
}

func TestDataSourceManagementGatePrecedesInputAndSecretAccess(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete", "test"} {
		for _, role := range []string{"member", "missing", "wrong_identity_type", "unconfigured"} {
			t.Run(operation+"/"+role, func(t *testing.T) {
				var checks int
				h := NewDataSourceHandler(nil, sourceManagementFunc(func(_ context.Context, user, tenant int64) error {
					checks++
					require.Equal(t, int64(7), user)
					require.Equal(t, int64(9), tenant)
					return errors.New("member cannot manage")
				}))
				if role == "unconfigured" {
					h = NewDataSourceHandler(nil)
				}
				r := gin.New()
				r.Use(func(c *gin.Context) {
					if role != "missing" {
						c.Set("tenant_id", int64(9))
						c.Set("user_id", int64(7))
						if role == "wrong_identity_type" {
							c.Set("user_id", 7)
						}
					}
					c.Next()
				})
				handlers := map[string]gin.HandlerFunc{"create": h.Create, "update": h.Update, "delete": h.Delete, "test": h.TestConnection}
				r.POST("/", handlers[operation])
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(`not json`)))
				require.Equal(t, 403, w.Code)
				if role == "member" {
					require.Equal(t, 1, checks)
				} else {
					require.Zero(t, checks)
				}
			})
		}
	}
}
