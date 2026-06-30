# Agent Core

## ADDED Requirements

### Requirement: Agent Interface

The system SHALL provide a simple, unified Agent interface.

#### Scenario: Agent has Chat method
- **GIVEN** an Agent implementation
- **WHEN** calling Chat(ctx, message)
- **THEN** it returns a Response with content and optional tool calls
- **AND** it returns an error if the call fails

#### Scenario: Agent has Stream method
- **GIVEN** an Agent implementation
- **WHEN** calling Stream(ctx, message)
- **THEN** it returns a channel that streams Chunk objects
- **AND** the channel closes when streaming is complete

#### Scenario: Agent has Name method
- **GIVEN** an Agent implementation
- **WHEN** calling Name()
- **THEN** it returns the agent's name

### Requirement: Agent Builder

The system SHALL provide a fluent builder for creating Agents.

#### Scenario: Chain configuration
- **GIVEN** a Builder instance
- **WHEN** chaining method calls (.Name().Prompt().Tools())
- **THEN** each method returns the Builder for chaining
- **AND** Build() creates the final Agent

#### Scenario: Build with model
- **GIVEN** a model.ChatModel instance
- **WHEN** calling agent.New(model)
- **THEN** it returns a configured Builder

#### Scenario: Build with tools
- **GIVEN** a Builder with tools configured
- **WHEN** calling Build(ctx)
- **THEN** the Agent has access to the configured tools

### Requirement: Response Structure

The Response SHALL contain content, tool calls, and metadata.

#### Scenario: Response with content
- **GIVEN** a Response from an Agent
- **WHEN** accessing the Content field
- **THEN** it contains the agent's text response

#### Scenario: Response with tool calls
- **GIVEN** a Response from an Agent that used tools
- **WHEN** accessing the ToolCalls field
- **THEN** it contains a list of tools called with their inputs and outputs

### Requirement: Streaming Support

The system SHALL support streaming responses.

#### Scenario: Stream content chunks
- **GIVEN** a stream channel
- **WHEN** receiving chunks
- **THEN** each Chunk contains partial content
- **AND** the final Chunk has Done=true

### Requirement: Hooks and Middleware

The Builder SHALL support hooks and middleware.

#### Scenario: Before hook
- **GIVEN** a Builder with Before(fn) configured
- **WHEN** Chat is called
- **THEN** the Before function is called with the original message
- **AND** the Agent uses the potentially modified message

#### Scenario: After hook
- **GIVEN** a Builder with After(fn) configured
- **WHEN** Chat completes
- **THEN** the After function is called with the Response
- **AND** the Response can be modified

#### Scenario: Middleware chain
- **GIVEN** a Builder with multiple Middleware configured
- **WHEN** Chat is called
- **THEN** middleware are called in order (Before)
- **AND** middleware are called in reverse order (After)

### Requirement: Tool Registry Integration

The Agent framework SHALL integrate with existing Tool Registry.

#### Scenario: Tools from registry by name
- **GIVEN** the existing Tool Registry with registered tools
- **WHEN** calling ToolsFromRegistry("rag_query", "web_search")
- **THEN** it retrieves tools from the registry
- **AND** the Agent has access to the specified tools

#### Scenario: List available tools
- **GIVEN** the existing Tool Registry
- **WHEN** querying available tools
- **THEN** it returns all registered tool names
- **AND** includes tool descriptions

### Requirement: Automatic Tool Selection

The Builder SHALL support automatic tool selection by LLM.

#### Scenario: Auto-select all tools
- **GIVEN** a Builder with ToolsAutoSelect() configured
- **WHEN** Build(ctx) is called
- **THEN** it registers all tools from Tool Registry
- **AND** the LLM can choose any tool automatically

#### Scenario: LLM selects appropriate tool
- **GIVEN** an Agent with ToolsAutoSelect()
- **WHEN** user asks "latest AI news"
- **THEN** the LLM automatically selects web_search tool
- **AND** returns search results

#### Scenario: LLM selects RAG for document query
- **GIVEN** an Agent with ToolsAutoSelect()
- **WHEN** user asks "company product architecture"
- **THEN** the LLM automatically selects rag_query tool
- **AND** returns document results

#### Scenario: Mixed auto and manual tools
- **GIVEN** a Builder with ToolsAutoSelect() and additional Tools(customTool)
- **WHEN** Build(ctx) is called
- **THEN** it includes both registry tools and custom tools
- **AND** LLM can select from all available tools

### Requirement: Memory Integration

The Builder SHALL integrate with existing Session/Memory.

#### Scenario: With session ID
- **GIVEN** a Builder with WithSession(sessionID) configured
- **WHEN** Chat is called
- **THEN** it loads message history from Session
- **AND** it includes history in the context
- **AND** it saves the response to Session

#### Scenario: With custom memory
- **GIVEN** a Builder with Memory(memoryStore) configured
- **WHEN** Chat is called multiple times
- **THEN** it maintains conversation context
- **AND** responses reference previous messages

### Requirement: RAG Integration

The Builder SHALL integrate with existing RAG Service.

#### Scenario: With RAG service
- **GIVEN** a Builder with WithRAG(ragService) configured
- **WHEN** Chat is called
- **THEN** it can use RAG retrieval capabilities
- **AND** it can use graph query capabilities

#### Scenario: RAG as tool
- **GIVEN** RAG tool is registered in Tool Registry
- **WHEN** Agent with ToolsAutoSelect() processes document query
- **THEN** it automatically uses RAG tool
- **AND** returns relevant document snippets
