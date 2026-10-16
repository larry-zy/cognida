package framework

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

type executableRegistryTool struct {
	info *schema.ToolInfo
	err  error
}

func (t *executableRegistryTool) Info(context.Context) (*schema.ToolInfo, error) {
	return t.info, t.err
}
func (t *executableRegistryTool) InvokableRun(ctx context.Context, input string, _ ...tool.Option) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "executed:" + input, nil
}

type metadataRegistryTool struct{}

func (*metadataRegistryTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "metadata"}, nil
}

type enhancedRegistryTool struct{ called bool }

func (*enhancedRegistryTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "enhanced"}, nil
}
func (t *enhancedRegistryTool) InvokableRun(context.Context, *schema.ToolArgument, ...tool.Option) (*schema.ToolResult, error) {
	t.called = true
	return &schema.ToolResult{}, nil
}

func TestToolRegistryAcceptsEnhancedExecutable(t *testing.T) {
	r := NewToolRegistry()
	executable := &enhancedRegistryTool{}
	require.NoError(t, r.RegisterDirect("enhanced", executable))
	registered, ok := r.GetDirect("enhanced")
	require.True(t, ok)
	result, err := registered.(tool.EnhancedInvokableTool).InvokableRun(context.Background(), &schema.ToolArgument{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, executable.called)
}

func TestToolRegistryExecutableRegistration(t *testing.T) {
	r := NewToolRegistry()
	executable := &executableRegistryTool{info: &schema.ToolInfo{Name: "query", Desc: "retrieval"}}
	require.NoError(t, r.RegisterEinoTool("query", executable))
	actual, ok := r.GetEinoTool("QUERY")
	require.True(t, ok)
	output, err := actual.(tool.InvokableTool).InvokableRun(context.Background(), `{"limit":2}`)
	require.NoError(t, err)
	require.Equal(t, `executed:{"limit":2}`, output)
	info, ok := r.GetInfo("QUERY")
	require.True(t, ok)
	require.Equal(t, "query", info.Name)
	require.NoError(t, r.Disable("QUERY"))
	_, ok = r.GetEinoTool("query")
	require.False(t, ok)
	_, ok = r.GetDirect("query")
	require.False(t, ok)
	_, ok = r.GetInfo("query")
	require.False(t, ok)
	require.Empty(t, r.GetTools())
	require.Empty(t, r.ListInfo())
	require.NoError(t, r.Enable("query"))
	require.Len(t, r.GetTools(), 1)
	require.NoError(t, r.Unregister("QUERY"))
	require.Zero(t, r.Size())
}

func TestToolRegistryRejectsInvalidExecutablesWithoutMutation(t *testing.T) {
	var typedNil *executableRegistryTool
	cases := []struct {
		name string
		tool tool.BaseTool
	}{
		{"query", nil}, {"query", typedNil}, {"metadata", &metadataRegistryTool{}},
		{"query", &executableRegistryTool{}},
		{"query", &executableRegistryTool{err: errors.New("private provider error")}},
		{"query", &executableRegistryTool{info: &schema.ToolInfo{Name: "other"}}},
		{" ", &executableRegistryTool{info: &schema.ToolInfo{Name: " "}}},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			r := NewToolRegistry()
			require.Error(t, r.RegisterDirect(tc.name, tc.tool))
			require.Zero(t, r.Size())
			require.Empty(t, r.ListInfo())
		})
	}
	r := NewToolRegistry()
	valid := &executableRegistryTool{info: &schema.ToolInfo{Name: "query"}}
	require.NoError(t, r.RegisterDirect("query", valid))
	require.Error(t, r.RegisterDirect("query", typedNil))
	actual, ok := r.GetDirect("query")
	require.True(t, ok)
	require.Same(t, valid, actual)
}

func TestToolRegistryConcurrentRegistrationReads(t *testing.T) {
	r := NewToolRegistry()
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			name := fmt.Sprintf("tool_%d", id)
			for n := 0; n < 30; n++ {
				if err := r.RegisterDirect(name, &executableRegistryTool{info: &schema.ToolInfo{Name: name}}); err != nil {
					t.Error(err)
					return
				}
				r.GetInfo(name)
				r.ListInfo()
				r.GetTools()
				r.ListDomain()
				if err := r.Unregister(name); err != nil {
					t.Error(err)
					return
				}
			}
		}(i)
	}
	workers.Wait()
	require.Empty(t, r.List())
}
