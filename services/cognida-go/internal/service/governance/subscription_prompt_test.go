package governance

import (
	"context"
	"strings"
	"testing"
	"time"

	d "cognida/internal/model/governance"
	l "cognida/internal/model/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionPromptLimitCountsUnicodeCharacters(t *testing.T) {
	ref := testAsset("source", time.Now())
	assets := &unitAssets{refs: map[string]l.DataAssetRef{ref.AssetRefID: ref}, input: l.Input{Kind: "text", Text: "evidence"}}
	repo := &unitRepo{}
	s := NewService(repo, assets, nil, nil, func(context.Context, l.Principal, string) error { return nil })
	p := l.Principal{TenantID: 1, UserID: 1}
	in := d.Subscription{Name: "report", Kind: "report", Channel: "in_app", Schedule: d.Schedule{Cron: "0 9 * * *", Timezone: "UTC"}, Input: d.AnalysisInput{AssetRefID: ref.AssetRefID, Prompt: strings.Repeat("析", 4000)}}
	_, err := s.SaveSubscription(context.Background(), p, "", "unicode-boundary", in, 0)
	require.NoError(t, err)
	require.Equal(t, 1, repo.creates)
	in.Input.Prompt += "析"
	_, err = s.SaveSubscription(context.Background(), p, "", "unicode-too-long", in, 0)
	require.ErrorIs(t, err, d.ErrInvalid)
	require.Equal(t, 1, repo.creates)
}
