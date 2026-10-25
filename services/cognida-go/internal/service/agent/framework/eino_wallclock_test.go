package framework

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"

	ctxeng "cognida/internal/service/agent/context"
)

// These tests use a longer parent deadline only as a cleanup backstop. They
// assert which context expired, rather than relying on millisecond timing.
const wallClockTestBudget = 100 * time.Millisecond

type wallClockTestModel struct {
	calls    atomic.Int32
	generate func(context.Context, []*schema.Message, int32) (*schema.Message, error)
	stream   func(context.Context) (*schema.StreamReader[*schema.Message], error)
}

func (m *wallClockTestModel) Generate(ctx context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return m.generate(ctx, msgs, m.calls.Add(1))
}

func (m *wallClockTestModel) Stream(ctx context.Context, msgs []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if m.stream != nil {
		m.calls.Add(1)
		return m.stream(ctx)
	}
	msg, err := m.Generate(ctx, msgs)
	if err != nil {
		return nil, err
	}
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Send(msg, nil)
	writer.Close()
	return reader, nil
}

func (m *wallClockTestModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

type wallClockTestTool struct {
	name  string
	calls atomic.Int32
	run   func(context.Context) (string, error)
}

func (w *wallClockTestTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: w.name, Desc: "wall-clock regression test"}, nil
}

func (w *wallClockTestTool) InvokableRun(ctx context.Context, _ string, _ ...tool.Option) (string, error) {
	w.calls.Add(1)
	if w.run != nil {
		return w.run(ctx)
	}
	return `{"result_id":"saved-result"}`, nil
}

type wallClockTestSummarizer func(context.Context, []ctxeng.Turn, []string) (string, error)

func (s wallClockTestSummarizer) Summarize(ctx context.Context, turns []ctxeng.Turn, ids []string) (string, error) {
	return s(ctx, turns, ids)
}

func wallClockTestContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx, cancel
}

// Use the public entry points. The observer captures the actual response for
// Stream, while the collected chunks independently verify its delivery.
func runWallClockTestAgent(t *testing.T, ctx context.Context, a *agentImpl, mode string) (*Response, []*Chunk, error) {
	t.Helper()
	if mode == "chat" {
		resp, err := a.Chat(ctx, "finish this task")
		return resp, nil, err
	}
	a.outputGuardrailActive = mode == "guarded_stream"
	observed := make(chan *Response, 1)
	a.observerHooks = append(a.observerHooks, func(_ context.Context, resp *Response) error {
		observed <- resp
		return nil
	})
	ch, err := a.Stream(ctx, "finish this task")
	if err != nil {
		return nil, nil, err
	}
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	var chunks []*Chunk
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				select {
				case resp := <-observed:
					return resp, chunks, nil
				default:
					return nil, chunks, nil
				}
			}
			chunks = append(chunks, chunk)
		case <-timer.C:
			t.Fatal("agent did not close its stream after the parent cleanup deadline")
		}
	}
}

func requireWallClockPartial(t *testing.T, parent context.Context, resp *Response, chunks []*Chunk, mode string) {
	t.Helper()
	require.NoError(t, parent.Err(), "the ReAct budget must expire before its parent")
	require.NotNil(t, resp, "deadline exhaustion must produce a response")
	require.NotEmpty(t, strings.TrimSpace(resp.Content))
	require.Equal(t, TerminatedByDeadline, resp.Metadata["terminated_by"])
	require.Equal(t, true, resp.Metadata["partial"])
	if mode == "chat" {
		return
	}
	var content strings.Builder
	for _, chunk := range chunks {
		content.WriteString(chunk.Content)
		require.NotEqual(t, string(EventError), chunk.Metadata["event"], "budget exhaustion is a partial result, not a transport error")
	}
	require.Equal(t, resp.Content, content.String())
	require.NotEmpty(t, chunks)
	end := chunks[len(chunks)-1]
	require.True(t, end.Done)
	require.Equal(t, string(EventEnd), end.Metadata["event"])
	require.Equal(t, TerminatedByDeadline, end.Metadata["terminated_by"])
	require.Equal(t, true, end.Metadata["partial"])
}

func TestWallClock_ModelGenerationIsCancelled(t *testing.T) {
	for _, mode := range []string{"chat", "stream", "guarded_stream"} {
		t.Run(mode, func(t *testing.T) {
			parent, _ := wallClockTestContext(t)
			observed := make(chan error, 1)
			m := &wallClockTestModel{generate: func(ctx context.Context, _ []*schema.Message, _ int32) (*schema.Message, error) {
				<-ctx.Done()
				select {
				case observed <- ctx.Err():
				default:
				}
				return nil, ctx.Err()
			}}
			unused := &wallClockTestTool{name: "query"}
			a := &agentImpl{name: "deadline", toolModel: m, tools: []tool.BaseTool{unused}, maxIter: 4, wallClock: wallClockTestBudget}
			resp, chunks, err := runWallClockTestAgent(t, parent, a, mode)
			require.NoError(t, err)
			requireWallClockPartial(t, parent, resp, chunks, mode)
			require.Equal(t, int32(1), m.calls.Load(), "an expired budget must not launch a wind-down model call")
			select {
			case err := <-observed:
				require.ErrorIs(t, err, context.DeadlineExceeded)
			case <-time.After(time.Second):
				t.Fatal("model did not observe cancellation")
			}
			require.Zero(t, unused.calls.Load())
		})
	}
}

func TestWallClock_StreamReceiveStillDeliversEnd(t *testing.T) {
	parent, cancel := wallClockTestContext(t)
	producerStopped := make(chan struct{})
	m := &wallClockTestModel{stream: func(ctx context.Context) (*schema.StreamReader[*schema.Message], error) {
		reader, writer := schema.Pipe[*schema.Message](1)
		go func() {
			defer close(producerStopped)
			defer writer.Close()
			<-ctx.Done()
			writer.Send(nil, ctx.Err())
		}()
		return reader, nil
	}}
	t.Cleanup(func() {
		cancel()
		select {
		case <-producerStopped:
		case <-time.After(time.Second):
			t.Error("stream producer did not observe cancellation")
		}
	})
	a := &agentImpl{name: "deadline", toolModel: m, tools: []tool.BaseTool{&wallClockTestTool{name: "query"}}, maxIter: 4, wallClock: wallClockTestBudget}
	resp, chunks, err := runWallClockTestAgent(t, parent, a, "stream")
	require.NoError(t, err)
	requireWallClockPartial(t, parent, resp, chunks, "stream")
	require.Equal(t, int32(1), m.calls.Load())
}

func TestWallClock_CopiedStreamReaderIsReleasedAfterReceive(t *testing.T) {
	parent, _ := wallClockTestContext(t)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseProvider := func() { releaseOnce.Do(func() { close(release) }) }
	receiving := make(chan struct{})
	producerStopped := make(chan struct{})
	m := &wallClockTestModel{stream: func(context.Context) (*schema.StreamReader[*schema.Message], error) {
		reader, writer := schema.Pipe[*schema.Message](0)
		blocked := schema.StreamReaderWithConvert(reader, func(msg *schema.Message) (*schema.Message, error) {
			close(receiving)
			<-release // A provider may ignore context while receiving a chunk.
			return msg, nil
		})
		copies := blocked.Copy(2)
		copies[1].Close()
		go func() {
			defer close(producerStopped)
			defer writer.Close()
			writer.Send(schema.AssistantMessage("late provider output", nil), nil)
			<-release
			// This send unblocks only when the receiving goroutine closes its
			// reader. The test never closes a copied reader concurrently with Recv.
			writer.Send(schema.AssistantMessage("must not be read", nil), nil)
		}()
		return copies[0], nil
	}}
	t.Cleanup(func() {
		releaseProvider()
		select {
		case <-producerStopped:
		case <-time.After(time.Second):
			t.Error("copied stream reader was not closed after its provider returned")
		}
	})
	a := &agentImpl{name: "deadline", toolModel: m, tools: []tool.BaseTool{&wallClockTestTool{name: "query"}}, maxIter: 4, wallClock: wallClockTestBudget}
	resp, chunks, err := runWallClockTestAgent(t, parent, a, "stream")
	require.NoError(t, err)
	requireWallClockPartial(t, parent, resp, chunks, "stream")
	select {
	case <-receiving:
	default:
		t.Fatal("the deadline did not interrupt an active copied-reader receive")
	}
	content := resp.Content
	releaseProvider()
	select {
	case <-producerStopped:
	case <-time.After(time.Second):
		t.Fatal("provider did not finish after release")
	}
	require.Equal(t, content, resp.Content)
	require.NotContains(t, resp.Content, "late provider output")
	require.Equal(t, int32(1), m.calls.Load())
}

func TestWallClock_ContextSummarizerCannotExtendBudget(t *testing.T) {
	parent, _ := wallClockTestContext(t)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseSummarizer := func() { releaseOnce.Do(func() { close(release) }) }
	entered := make(chan struct{})
	returned := make(chan struct{})
	var summaries atomic.Int32
	summarizer := wallClockTestSummarizer(func(ctx context.Context, _ []ctxeng.Turn, _ []string) (string, error) {
		summaries.Add(1)
		close(entered)
		defer close(returned)
		<-release // Deliberately ignore ctx, as an uncooperative adapter can.
		return "late summary must not become the answer", nil
	})
	t.Cleanup(func() {
		releaseSummarizer()
		select {
		case <-entered:
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Error("summarizer did not finish after release")
			}
		default:
		}
	})
	query := &wallClockTestTool{name: "query"}
	m := &wallClockTestModel{generate: func(_ context.Context, _ []*schema.Message, call int32) (*schema.Message, error) {
		if call == 1 {
			return toolCallMsg("query", "query"), nil
		}
		return schema.AssistantMessage("unexpected model call after compression", nil), nil
	}}
	a := &agentImpl{
		name: "deadline", toolModel: m, tools: []tool.BaseTool{query}, maxIter: 4,
		wallClock: wallClockTestBudget, compactTrigger: 1, compactTarget: 1, summarizer: summarizer,
	}
	resp, chunks, err := runWallClockTestAgent(t, parent, a, "stream")
	require.NoError(t, err)
	requireWallClockPartial(t, parent, resp, chunks, "stream")
	require.Equal(t, int32(1), summaries.Load())
	require.Equal(t, int32(1), m.calls.Load(), "deadline during compression must not start another model call")
	require.Len(t, resp.ToolCalls, 1)
	require.Equal(t, `{"result_id":"saved-result"}`, resp.ToolCalls[0].Output)
	content := resp.Content
	metadata := make(map[string]interface{}, len(resp.Metadata))
	for key, value := range resp.Metadata {
		metadata[key] = value
	}
	releaseSummarizer()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("summarizer did not finish after release")
	}
	require.Equal(t, content, resp.Content, "a late summary must not change the delivered response")
	require.Equal(t, metadata, resp.Metadata)
	require.NotContains(t, resp.Content, "late summary")
	require.Equal(t, int32(1), m.calls.Load(), "a late summary must not resume the expired loop")
}

func TestWallClock_ModelPanicReturnsError(t *testing.T) {
	parent, _ := wallClockTestContext(t)
	m := &wallClockTestModel{generate: func(context.Context, []*schema.Message, int32) (*schema.Message, error) {
		panic("provider adapter panic")
	}}
	unused := &wallClockTestTool{name: "query"}
	a := &agentImpl{name: "deadline", toolModel: m, tools: []tool.BaseTool{unused}, maxIter: 4, wallClock: time.Second}
	resp, err := a.Chat(parent, "finish this task")
	require.ErrorContains(t, err, "provider adapter panic")
	require.Nil(t, resp, "a recovered model panic must not be reported as partial success")
	require.NoError(t, parent.Err())
	require.Equal(t, int32(1), m.calls.Load())
	require.Zero(t, unused.calls.Load())
}

func TestWallClock_StopsToolBatchAndPreservesTrace(t *testing.T) {
	for _, tc := range []struct {
		mode        string
		toolTimeout time.Duration
	}{
		{mode: "chat", toolTimeout: 2 * time.Second},
		{mode: "stream", toolTimeout: 2 * time.Second},
		{mode: "chat", toolTimeout: 0},
	} {
		t.Run(tc.mode+"/tool_timeout_"+tc.toolTimeout.String(), func(t *testing.T) {
			mode := tc.mode
			parent, _ := wallClockTestContext(t)
			query := &wallClockTestTool{name: "query"}
			slow := &wallClockTestTool{name: "slow", run: func(ctx context.Context) (string, error) {
				<-ctx.Done()
				return "", ctx.Err()
			}}
			never := &wallClockTestTool{name: "never"}
			m := &wallClockTestModel{generate: func(_ context.Context, _ []*schema.Message, call int32) (*schema.Message, error) {
				if call == 1 {
					return toolCallMsg("saved", "query"), nil
				}
				if call == 2 {
					msg := toolCallMsg("slow-call", "slow")
					msg.ToolCalls = append(msg.ToolCalls, toolCallMsg("must-not-run", "never").ToolCalls...)
					return msg, nil
				}
				return schema.AssistantMessage("unexpected wind-down", nil), nil
			}}
			a := &agentImpl{name: "deadline", toolModel: m, tools: []tool.BaseTool{query, slow, never}, maxIter: 5, wallClock: wallClockTestBudget, toolTimeout: tc.toolTimeout}
			resp, chunks, err := runWallClockTestAgent(t, parent, a, mode)
			require.NoError(t, err)
			requireWallClockPartial(t, parent, resp, chunks, mode)
			require.Equal(t, int32(1), query.calls.Load())
			require.Equal(t, int32(1), slow.calls.Load())
			require.Zero(t, never.calls.Load(), "later tools in the same assistant message must not run after the deadline")
			require.Equal(t, int32(2), m.calls.Load())
			require.Len(t, resp.ToolCalls, 2)
			require.Equal(t, "query", resp.ToolCalls[0].Name)
			require.Equal(t, `{"result_id":"saved-result"}`, resp.ToolCalls[0].Output)
			require.Equal(t, "slow", resp.ToolCalls[1].Name)
			require.ErrorIs(t, resp.ToolCalls[1].Error, context.DeadlineExceeded)
			if mode == "stream" {
				var results []*ToolCallInStream
				for _, chunk := range chunks {
					if chunk.Metadata["event"] == string(EventToolResult) {
						ev, ok := chunk.Metadata["tool_call"].(*ToolCallInStream)
						require.True(t, ok)
						results = append(results, ev)
					}
				}
				require.Len(t, results, 2, "the expired execution context must not suppress the final tool result")
				require.Equal(t, "success", results[0].Status)
				require.Equal(t, "error", results[1].Status)
			}
		})
	}
}

func TestWallClock_WindDownSharesRemainingBudget(t *testing.T) {
	for _, reason := range []string{"max_iter", "token_budget"} {
		for _, mode := range []string{"chat", "stream"} {
			t.Run(reason+"/"+mode, func(t *testing.T) {
				parent, _ := wallClockTestContext(t)
				deadlines := make(chan time.Time, 2)
				m := &wallClockTestModel{generate: func(ctx context.Context, _ []*schema.Message, call int32) (*schema.Message, error) {
					deadline, _ := ctx.Deadline()
					select {
					case deadlines <- deadline:
					default:
					}
					if call == 1 {
						msg := toolCallMsg("query", "query")
						msg.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{TotalTokens: 100}}
						return msg, nil
					}
					<-ctx.Done()
					return nil, ctx.Err()
				}}
				a := &agentImpl{name: "deadline", toolModel: m, tools: []tool.BaseTool{&wallClockTestTool{name: "query"}}, maxIter: 1, wallClock: wallClockTestBudget}
				if reason == "token_budget" {
					a.maxIter, a.tokenBudget = 5, 100
				}
				resp, chunks, err := runWallClockTestAgent(t, parent, a, mode)
				require.NoError(t, err)
				requireWallClockPartial(t, parent, resp, chunks, mode)
				require.Equal(t, int32(2), m.calls.Load(), "one tool round and one bounded wind-down")
				require.Equal(t, <-deadlines, <-deadlines, "wind-down must not reset the total deadline")
				require.Len(t, resp.ToolCalls, 1)
			})
		}
	}
}

func TestWallClock_ParentCancellationIsNotPartialSuccess(t *testing.T) {
	for _, mode := range []string{"chat", "stream"} {
		for _, stage := range []string{"initial", "wind_down"} {
			for _, cause := range []string{"cancel", "deadline"} {
				t.Run(mode+"/"+stage+"/"+cause, func(t *testing.T) {
					parent, cancel := wallClockTestContext(t)
					wantErr := context.Canceled
					if cause == "deadline" {
						var deadlineCancel context.CancelFunc
						parent, deadlineCancel = context.WithTimeout(parent, wallClockTestBudget)
						t.Cleanup(deadlineCancel)
						wantErr = context.DeadlineExceeded
					}
					m := &wallClockTestModel{generate: func(ctx context.Context, _ []*schema.Message, call int32) (*schema.Message, error) {
						if stage == "wind_down" && call == 1 {
							return toolCallMsg("query", "query"), nil
						}
						if cause == "cancel" {
							cancel()
						}
						<-ctx.Done()
						return nil, ctx.Err()
					}}
					a := &agentImpl{name: "deadline", toolModel: m, tools: []tool.BaseTool{&wallClockTestTool{name: "query"}}, maxIter: 1, wallClock: time.Second}
					resp, chunks, err := runWallClockTestAgent(t, parent, a, mode)
					if mode == "chat" {
						require.ErrorIs(t, err, wantErr)
					}
					require.Nil(t, resp)
					for _, chunk := range chunks {
						require.NotEqual(t, string(EventEnd), chunk.Metadata["event"])
					}
					wantCalls := int32(1)
					if stage == "wind_down" {
						wantCalls = 2
					}
					require.Equal(t, wantCalls, m.calls.Load())
				})
			}
		}
	}
}

func TestWallClock_NormalDisabledAndNoToolsRemainUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name      string
		wallClock time.Duration
		hasTools  bool
	}{
		{name: "normal", wallClock: time.Second, hasTools: true},
		{name: "disabled", hasTools: true},
		{name: "no_tools", wallClock: time.Nanosecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent, _ := wallClockTestContext(t)
			parentDeadline, _ := parent.Deadline()
			seenDeadline := make(chan time.Time, 1)
			m := &wallClockTestModel{generate: func(ctx context.Context, _ []*schema.Message, _ int32) (*schema.Message, error) {
				deadline, _ := ctx.Deadline()
				seenDeadline <- deadline
				return schema.AssistantMessage("complete answer", nil), nil
			}}
			a := &agentImpl{name: "deadline", toolModel: m, maxIter: 4, wallClock: tc.wallClock}
			if tc.hasTools {
				a.tools = []tool.BaseTool{&wallClockTestTool{name: "query"}}
			}
			resp, err := a.Chat(parent, "hello")
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.Equal(t, "complete answer", resp.Content)
			require.NotContains(t, resp.Metadata, "terminated_by")
			require.NotContains(t, resp.Metadata, "partial")
			if tc.name == "normal" {
				require.True(t, (<-seenDeadline).Before(parentDeadline))
			} else {
				require.Equal(t, parentDeadline, <-seenDeadline, "disabled and no-tool paths must inherit the parent unchanged")
			}
		})
	}
}
