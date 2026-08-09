# Eino Agent Integration - Implementation Tasks

## 1. Foundation Layer

- [x] 1.1 Create `internal/agent/` directory structure
- [x] 1.2 Create `internal/agent/agent/` for core Agent interface
- [x] 1.3 Create `internal/agent/orchestration/` for orchestration patterns
- [x] 1.4 Create `internal/agent/integration/` for integrating existing capabilities

## 2. Core Agent Interface

- [x] 2.1 Define Agent interface (Chat, Stream, Name methods)
- [x] 2.2 Define Response struct (Content, ToolCalls, Metadata)
- [x] 2.3 Define Chunk struct for streaming (Content, Done, Metadata)
- [x] 2.4 Add godoc comments to all interfaces
- [ ] 2.5 Add interface unit tests

## 3. Agent Builder

- [x] 3.1 Implement Builder struct with fluent methods
- [x] 3.2 Implement New(model) function
- [x] 3.3 Implement Name(string) method
- [x] 3.4 Implement Description(string) method
- [x] 3.5 Implement Prompt(string) method
- [x] 3.6 Implement Tools(...Tool) method - manual tool specification
- [x] 3.7 Implement ToolsFromRegistry(...string) method - get tools by name
- [x] 3.8 Implement ToolsAutoSelect() method - LLM auto-selects tools
- [x] 3.9 Implement Before(hook) method
- [x] 3.10 Implement After(hook) method
- [x] 3.11 Implement Middleware(...Middleware) method
- [x] 3.12 Implement Build(ctx) method
- [x] 3.13 Implement agentImpl struct wrapping eino ADK
- [ ] 3.14 Add builder unit tests

## 4. Tool Registry Integration (复用现有)

- [x] 4.1 Create `internal/agent/integration/tools.go`
- [x] 4.2 Implement GetDefaultRegistry() wrapper
- [x] 4.3 Implement ToolsFromRegistry builder method
- [x] 4.4 Implement ToolsAutoSelect builder method
- [x] 4.5 Add tool info retrieval for LLM function calling
- [ ] 4.6 Add integration tests with existing tools

## 5. RAG Integration (复用现有)

- [x] 5.1 Create `internal/agent/integration/rag.go`
- [x] 5.2 Implement WithRAG(builder option)
- [x] 5.3 Wrap RAGService as Agent tool
- [x] 5.4 Support RAG in auto tool selection
- [ ] 5.5 Add RAG integration tests

## 6. Memory Integration (复用现有)

- [x] 6.1 Create `internal/agent/integration/memory.go`
- [x] 6.2 Implement WithSession(builder option)
- [x] 6.3 Implement WithMemory(builder option)
- [x] 6.4 Load history from Session on Chat
- [x] 6.5 Save responses to Session
- [ ] 6.6 Add memory integration tests

## 7. Middleware Support

- [x] 7.1 Define Middleware interface (Before, After methods)
- [x] 7.2 Implement middleware chain execution
- [x] 7.3 Implement loggingMiddleware example
- [x] 7.4 Implement metricsMiddleware example
- [ ] 7.5 Add middleware tests

## 8. Orchestration - Sequential

- [x] 8.1 Implement Sequential(agent1, agent2, ...) function
- [x] 8.2 Implement sequentialAgent struct
- [x] 8.3 Implement Chat method with pass-through logic
- [x] 8.4 Implement Stream method
- [x] 8.5 Add error handling for agent failures
- [ ] 8.6 Add Sequential tests

## 9. Orchestration - Parallel

- [x] 9.1 Implement Parallel(agent1, agent2, ...) function
- [x] 9.2 Implement parallelAgent struct
- [x] 9.3 Implement Chat method with concurrent execution
- [x] 9.4 Implement Stream method with multiplexing
- [x] 9.5 Add response aggregation logic
- [ ] 9.6 Add Parallel tests

## 10. Orchestration - Supervisor

- [x] 10.1 Implement Supervisor(coordinator, workers...) function
- [x] 10.2 Implement supervisorAgent struct
- [x] 10.3 Implement Chat method with routing logic
- [x] 10.4 Integrate with eino supervisor pattern
- [ ] 10.5 Add Supervisor tests

## 11. Orchestration - Conditional

- [x] 11.1 Implement Conditional(predicate, trueAgent, falseAgent) function
- [x] 11.2 Implement conditionalAgent struct
- [x] 11.3 Implement predicate evaluation
- [x] 11.4 Support multiple branches with switch-like behavior
- [ ] 11.5 Add Conditional tests

## 12. Orchestration - Loop

- [x] 12.1 Implement Loop(bodyAgent, continuationFunc) function
- [x] 12.2 Implement loopAgent struct
- [x] 12.3 Implement iteration logic
- [x] 12.4 Add max iterations safeguard
- [x] 12.5 Implement result accumulation
- [ ] 12.6 Add Loop tests

## 13. Orchestration - Func

- [x] 13.1 Implement Func(fn) function
- [x] 13.2 Implement funcAgent struct
- [x] 13.3 Implement Chat and Stream methods
- [ ] 13.4 Add Func tests

## 14. Examples

- [ ] 14.1 Create `examples/basic/` directory
- [ ] 14.2 Write auto tool selection example
- [ ] 14.3 Write manual tool selection example
- [ ] 14.4 Write with session/memory example
- [ ] 14.5 Write sequential orchestration example
- [ ] 14.6 Write parallel orchestration example
- [ ] 14.7 Write supervisor orchestration example
- [ ] 14.8 Write custom logic with Func example
- [ ] 14.9 Write middleware example
- [ ] 14.10 Write data collection pipeline example

## 15. Integration

- [ ] 15.1 Wire up Agent framework in dependency injection
- [ ] 15.2 Update cmd/wire/wire.go if needed
- [ ] 15.3 Add optional HTTP handlers for Agent framework
- [ ] 15.4 Add integration tests
- [ ] 15.5 Verify backward compatibility

## 16. Documentation

- [ ] 16.1 Write getting started guide
- [ ] 16.2 Document Agent API
- [ ] 16.3 Document Tool Integration (auto vs manual)
- [ ] 16.4 Document RAG Integration
- [ ] 16.5 Document Memory Integration
- [ ] 16.6 Document Orchestration patterns
- [ ] 16.7 Write migration guide from raw eino
- [ ] 16.8 Add Go package documentation
- [ ] 16.9 Update main README with Agent framework section

## 17. Code Quality

- [ ] 17.1 Run `go vet` on all new code
- [ ] 17.2 Run `golangci-lint` and fix issues
- [ ] 17.3 Add godoc comments to all exported symbols
- [ ] 17.4 Ensure code follows project conventions
- [ ] 17.5 Add benchmark tests for key paths
- [ ] 17.6 Clean up TODO comments

## 18. Testing

- [ ] 18.1 Add table-driven tests for Agent Builder
- [ ] 18.2 Add property-based tests for orchestration
- [ ] 18.3 Add end-to-end tests with mock LLM
- [ ] 18.4 Add stress tests for concurrent execution
- [ ] 18.5 Add integration tests with existing Tool Registry
- [ ] 18.6 Add integration tests with existing RAG Service
- [ ] 18.7 Add integration tests with existing Session
- [ ] 18.8 Verify test coverage > 80%

## 19. Optional Enhancements

- [ ] 19.1 Add tracing/integration with OpenTelemetry
- [ ] 19.2 Add Prometheus metrics
- [ ] 19.3 Add circuit breaker for external tool calls
- [ ] 19.4 Add rate limiting middleware
- [ ] 19.5 Add cache decorator
- [ ] 19.6 Add observability dashboard
