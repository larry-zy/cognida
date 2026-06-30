package orchestration

import (
	"context"
	"fmt"

	infraagent "link/internal/service/agent/framework"
)

// Func creates an agent from a simple function.
// The function signature matches Agent.Chat.
func Func(fn func(ctx context.Context, message string) (*infraagent.Response, error)) infraagent.Agent {
	return &funcAgent{
		chatFn: fn,
	}
}

// FuncStream creates an agent from functions that implement both Chat and Stream.
func FuncStream(
	chatFn func(ctx context.Context, message string) (*infraagent.Response, error),
	streamFn func(ctx context.Context, message string) (<-chan *infraagent.Chunk, error),
) infraagent.Agent {
	return &funcAgent{
		chatFn:   chatFn,
		streamFn: streamFn,
	}
}

type funcAgent struct {
	chatFn   func(ctx context.Context, message string) (*infraagent.Response, error)
	streamFn func(ctx context.Context, message string) (<-chan *infraagent.Chunk, error)
}

func (f *funcAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	if f.chatFn == nil {
		return &infraagent.Response{Content: ""}, nil
	}
	return f.chatFn(ctx, message)
}

func (f *funcAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	if f.streamFn != nil {
		return f.streamFn(ctx, message)
	}

	// Fallback: implement streaming using Chat
	ch := make(chan *infraagent.Chunk, 1)
	go func() {
		defer close(ch)

		resp, err := f.Chat(ctx, message)
		if err != nil {
			ch <- &infraagent.Chunk{
				Content: "",
				Done:    true,
				Metadata: map[string]interface{}{
					"error": err.Error(),
				},
			}
			return
		}

		ch <- &infraagent.Chunk{
			Content: resp.Content,
			Done:    true,
			Metadata: resp.Metadata,
		}
	}()

	return ch, nil
}

func (f *funcAgent) Name() string {
	return "FuncAgent"
}

// Transform creates an agent that transforms the output of another infraagent.
func Transform(base infraagent.Agent, transformer func(*infraagent.Response) *infraagent.Response) infraagent.Agent {
	return Func(func(ctx context.Context, message string) (*infraagent.Response, error) {
		resp, err := base.Chat(ctx, message)
		if err != nil {
			return nil, err
		}
		return transformer(resp), nil
	})
}

// Map creates an agent that applies a function to the input message before passing to the base infraagent.
func Map(base infraagent.Agent, mapper func(string) string) infraagent.Agent {
	return Func(func(ctx context.Context, message string) (*infraagent.Response, error) {
		mapped := mapper(message)
		return base.Chat(ctx, mapped)
	})
}

// Filter creates an agent that conditionally processes messages.
func Filter(base infraagent.Agent, predicate func(string) bool, fallback infraagent.Agent) infraagent.Agent {
	return Func(func(ctx context.Context, message string) (*infraagent.Response, error) {
		if predicate(message) {
			return base.Chat(ctx, message)
		}
		if fallback != nil {
			return fallback.Chat(ctx, message)
		}
		return &infraagent.Response{Content: "Message filtered out"}, nil
	})
}

// Memoize creates an agent that caches responses based on input messages.
func Memoize(base infraagent.Agent) infraagent.Agent {
	return &memoizeAgent{
		base:  base,
		cache: make(map[string]*infraagent.Response),
	}
}

type memoizeAgent struct {
	base  infraagent.Agent
	cache map[string]*infraagent.Response
}

func (m *memoizeAgent) Chat(ctx context.Context, message string) (*infraagent.Response, error) {
	if resp, ok := m.cache[message]; ok {
		return resp, nil
	}

	resp, err := m.base.Chat(ctx, message)
	if err != nil {
		return nil, err
	}

	m.cache[message] = resp
	return resp, nil
}

func (m *memoizeAgent) Stream(ctx context.Context, message string) (<-chan *infraagent.Chunk, error) {
	if _, ok := m.cache[message]; ok {
		resp := m.cache[message]
		ch := make(chan *infraagent.Chunk, 1)
		go func() {
			defer close(ch)
			ch <- &infraagent.Chunk{
				Content:  resp.Content,
				Done:     true,
				Metadata: resp.Metadata,
			}
		}()
		return ch, nil
	}

	return m.base.Stream(ctx, message)
}

func (m *memoizeAgent) Name() string {
	return fmt.Sprintf("Memoize(%s)", m.base.Name())
}

// Concat creates an agent that returns concatenated responses from multiple agents.
func Concat(agents ...infraagent.Agent) infraagent.Agent {
	return Func(func(ctx context.Context, message string) (*infraagent.Response, error) {
		var content string
		var allToolCalls []*infraagent.ToolCall

		for _, a := range agents {
			resp, err := a.Chat(ctx, message)
			if err != nil {
				return nil, err
			}
			content += resp.Content + "\n\n"
			allToolCalls = append(allToolCalls, resp.ToolCalls...)
		}

		return &infraagent.Response{
			Content:   content,
			ToolCalls: allToolCalls,
			Metadata: map[string]interface{}{
				"agent_count": len(agents),
			},
		}, nil
	})
}

// First creates an agent that returns the first successful response from multiple agents.
func First(agents ...infraagent.Agent) infraagent.Agent {
	return Func(func(ctx context.Context, message string) (*infraagent.Response, error) {
		for _, a := range agents {
			resp, err := a.Chat(ctx, message)
			if err == nil {
				return resp, nil
			}
		}
		return nil, fmt.Errorf("First: all agents failed")
	})
}

// Race creates an agent that races multiple agents and returns the first response.
// Note: This doesn't actually cancel the other agents - it just returns the first response.
func Race(agents ...infraagent.Agent) infraagent.Agent {
	return Func(func(ctx context.Context, message string) (*infraagent.Response, error) {
		type result struct {
			resp *infraagent.Response
			err  error
		}

		ch := make(chan result, len(agents))
		for _, a := range agents {
			go func(agent infraagent.Agent) {
				resp, err := agent.Chat(ctx, message)
				ch <- result{resp, err}
			}(a)
		}

		// Return first result
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		return r.resp, nil
	})
}

// All creates an agent that collects all successful responses.
func All(agents ...infraagent.Agent) infraagent.Agent {
	return Func(func(ctx context.Context, message string) (*infraagent.Response, error) {
		var responses []*infraagent.Response
		var errors []error

		for _, a := range agents {
			resp, err := a.Chat(ctx, message)
			if err != nil {
				errors = append(errors, err)
			} else {
				responses = append(responses, resp)
			}
		}

		if len(responses) == 0 {
			return nil, fmt.Errorf("All: all agents failed: %v", errors)
		}

		content := ""
		for i, resp := range responses {
			content += fmt.Sprintf("[Agent %d]\n%s\n\n", i, resp.Content)
		}

		return &infraagent.Response{
			Content: content,
			Metadata: map[string]interface{}{
				"successful": len(responses),
				"failed":     len(errors),
			},
		}, nil
	})
}
