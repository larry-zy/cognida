package chat

import (
	agentctx "cognida/internal/model/agent"
	"cognida/internal/model/conversation"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type messageAuthRepository struct {
	conversation.MessageRepository
	deletes int
}

func (r *messageAuthRepository) DeleteBySessionID(context.Context, string) error {
	r.deletes++
	return nil
}
func TestMessageServiceBulkDeleteRequiresExplicitOwner(t *testing.T) {
	sessions := newMockSessionRepository()
	sessions.sessions["s"] = &conversation.Session{ID: "s", TenantID: 11, UserID: 12}
	messages := &messageAuthRepository{}
	s := NewMessageService(messages, sessions)
	require.ErrorIs(t, s.DeleteMessagesBySessionID(context.Background(), "s"), ErrMessageForbidden)
	for _, scope := range [][2]int64{{11, 13}, {12, 12}} {
		ctx := agentctx.WithUserID(agentctx.WithTenantID(context.Background(), scope[0]), scope[1])
		require.ErrorIs(t, s.DeleteMessagesBySessionID(ctx, "s"), ErrMessageForbidden)
	}
	require.Zero(t, messages.deletes)
	ctx := agentctx.WithUserID(agentctx.WithTenantID(context.Background(), 11), 12)
	require.NoError(t, s.DeleteMessagesBySessionID(ctx, "s"))
	require.Equal(t, 1, messages.deletes)
	require.ErrorIs(t, NewMessageService(messages, nil).DeleteMessagesBySessionID(ctx, "s"), ErrMessageForbidden)
}
