//go:build integration

package tools

import (
	"cognida/internal/service/agent/resultstore"
	"context"
	"encoding/json"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

func TestDecisionIntegrationRealRedis(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", Password: os.Getenv("REDIS_PASSWORD")})
	ctx := context.Background()
	require.NoError(t, client.Ping(ctx).Err())
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	store := resultstore.NewRedisStore(client)
	call, req := comparisonFixture(t, store)
	t.Cleanup(func() {
		require.NoError(t, client.Del(ctx, "agent:resultstore:"+req.LeftResultID, "agent:resultstore:"+req.RightResultID).Err())
	})
	out, e := NewVerifyResultsTool(store).InvokableRun(call, `{"left_result_id":"`+req.LeftResultID+`","right_result_id":"`+req.RightResultID+`","key_columns":["region"],"value_columns":["revenue"],"absolute_tolerance":0.2}`)
	require.NoError(t, e)
	require.Contains(t, out, "numeric_agreement_only")

	ids := []string{}
	t.Cleanup(func() {
		for _, id := range ids {
			require.NoError(t, client.Del(ctx, "agent:resultstore:"+id).Err())
		}
	})
	for _, value := range []json.Number{"9007199254740992", "9007199254740993"} {
		id, e := store.Put(call, &resultstore.Result{Owner: resultstore.OwnerKey(7, "decision-session"), Columns: []string{"n"}, Rows: []map[string]any{{"n": value}}}, time.Minute)
		require.NoError(t, e)
		ids = append(ids, id)
	}
	check, e := verifyResults(call, store, &VerifyResultsRequest{LeftResultID: ids[0], RightResultID: ids[1], ValueColumns: []string{"n"}})
	require.NoError(t, e)
	require.True(t, check.RequiresClarification)
	require.Equal(t, 1, check.DifferenceCount)
}
