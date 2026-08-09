// Package hooks provides Agent lifecycle hook implementations.
//
// All hooks in this package implement the domain.HookService interface defined in
// link/internal/model/agent/service.go.
//
// # Hook Types
//
// ## Before Hooks
//
// Before hooks execute before the agent processes a message. They can modify
// the context and message, or return an error to halt execution.
//
// Example: IntentClarifier - analyzes query clarity and asks for clarification if needed
//
// ## After Hooks
//
// After hooks execute after the agent generates a response. They can modify
// the response or trigger additional processing.
//
// Examples:
//   - ConclusionGenerator - analyzes tool calls and generates structured conclusions
//   - AutoCompressHook - automatically compresses chat history when token limit is reached
//
// # Usage
//
// Hooks are configured through the Agent builder:
//
//	// 结论生成 Hook
//	gen := hooks.NewConclusionGenerator(llm).
//	    Enable().
//	    AddDataTools("sql_query", "data_query")
//	builder.WithConclusion(gen)
//
//	// 意图澄清 Hook
//	clarifier := hooks.NewIntentClarifier(llm).
//	    Enable().
//	    WithMaxRounds(2)
//	builder.WithClarification(clarifier)
//
//	// 自动压缩 Hook
//	compressHook := hooks.NewAutoCompressHook(compressionService).
//	    Enable().
//	    WithThreshold(0.8).
//	    WithMaxTokens(4000)
//	builder.WithAutoCompress(compressHook)
package hooks
