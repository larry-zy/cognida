package catalog

import (
	agent "cognida/internal/model/agent"
	audit "cognida/internal/model/audit"
	auditsvc "cognida/internal/service/audit"
	"context"
	"encoding/json"
)

type AuditWriter struct{ Writer *auditsvc.Writer }

func (a AuditWriter) Record(ctx context.Context, tenant, user int64, action, id string, details any) {
	if a.Writer == nil {
		return
	}
	b, _ := json.Marshal(details)
	rid, _ := agent.GetRequestID(ctx)
	a.Writer.Enqueue(audit.NewAuditLog(&tenant, &user, "catalog", action).WithResource("catalog", &id, nil).WithDetails(string(b)).WithRequest(&rid, nil))
}
