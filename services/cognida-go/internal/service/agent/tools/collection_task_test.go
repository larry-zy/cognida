package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCollectionTaskToolsExposeOnlyTaskLifecycleCommands(t *testing.T) {
	tools := NewCollectionTaskTools(nil, nil, nil)
	require.Len(t, tools, 4)

	names := make([]string, 0, len(tools))
	for _, candidate := range tools {
		info, err := candidate.Info(context.Background())
		require.NoError(t, err)
		names = append(names, info.Name)
	}

	require.ElementsMatch(t, []string{
		"collection_create_task",
		"collection_start_task",
		"collection_inspect_task",
		"collection_cancel_task",
	}, names)
	require.NotContains(t, names, "fetch_url")
}
