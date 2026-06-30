# Agent Tools

## ADDED Requirements

### Requirement: Tool Interface

The system SHALL provide a unified Tool interface compatible with eino.

#### Scenario: Tool has Info method
- **GIVEN** a Tool implementation
- **WHEN** calling Info(ctx)
- **THEN** it returns ToolInfo with name, description, and parameters schema

#### Scenario: Tool has Invoke method
- **GIVEN** a Tool implementation
- **WHEN** calling Invoke(ctx, input)
- **THEN** it executes the tool logic and returns result
- **AND** it returns an error if execution fails

### Requirement: Tool Function Wrapper

The system SHALL provide a way to convert simple functions to Tools.

#### Scenario: Create tool from function
- **GIVEN** a function with signature `func(ctx context.Context, input map[string]interface{}) (string, error)`
- **WHEN** calling ToolFunc(name, description, fn)
- **THEN** it returns a Tool instance
- **AND** the Tool wraps the function

### Requirement: Built-in WebSearch Tool

The system SHALL provide a WebSearch tool for internet search.

#### Scenario: WebSearch with API key
- **GIVEN** an API key for search service
- **WHEN** calling WebSearch(apiKey)
- **THEN** it returns a configured Tool
- **AND** the Tool accepts "query" parameter
- **AND** it returns search results

#### Scenario: WebSearch with custom endpoint
- **GIVEN** a custom search API endpoint
- **WHEN** calling WebSearch(apiKey, WithEndpoint(url))
- **THEN** it uses the custom endpoint

### Requirement: Built-in RAGQuery Tool

The system SHALL provide a RAGQuery tool for vector database retrieval.

#### Scenario: RAGQuery with Milvus
- **GIVEN** a Milvus client
- **WHEN** calling RAGQuery(milvusClient)
- **THEN** it returns a configured Tool
- **AND** the Tool accepts "query" and "top_k" parameters
- **AND** it returns relevant documents

#### Scenario: RAGQuery with collection
- **GIVEN** a specific collection name
- **WHEN** calling RAGQuery(client, WithCollection(name))
- **THEN** it queries the specified collection

### Requirement: Built-in WebScraper Tool

The system SHALL provide a WebScraper tool for web content extraction.

#### Scenario: WebScraper extracts content
- **GIVEN** a WebScraper Tool
- **WHEN** invoking with "url" parameter
- **THEN** it fetches the webpage
- **AND** it extracts main content
- **AND** it returns cleaned text

### Requirement: Built-in DataStorage Tool

The system SHALL provide a DataStorage tool for persisting data.

#### Scenario: DataStorage saves data
- **GIVEN** a DataStorage Tool with database connection
- **WHEN** invoking with "collection" and "data" parameters
- **THEN** it saves data to the specified collection
- **AND** it returns the saved document ID

#### Scenario: DataStorage retrieves data
- **GIVEN** a DataStorage Tool
- **WHEN** invoking with "collection" and "id" parameters
- **THEN** it returns the stored data

### Requirement: Tool Options

The system SHALL support functional options for Tool configuration.

#### Scenario: Tool with timeout
- **GIVEN** a Tool creation function
- **WHEN** calling WithTimeout(duration)
- **THEN** the Tool respects the timeout during invocation

#### Scenario: Tool with retry
- **GIVEN** a Tool creation function
- **WHEN** calling WithRetry(maxRetries)
- **THEN** the Tool retries on transient failures

### Requirement: Tool Composition

The system SHALL support composing multiple Tools.

#### Scenario: Tool list
- **GIVEN** multiple Tool instances
- **WHEN** calling Tools(tool1, tool2, tool3)
- **THEN** it returns a slice of Tools
- **AND** the slice can be passed to Agent Builder

### Requirement: Custom Tool Creation

The system SHALL support creating custom Tools via struct implementation.

#### Scenario: Custom struct tool
- **GIVEN** a struct implementing Tool interface
- **WHEN** the struct has Info and Invoke methods
- **THEN** it can be used with Agent Builder
- **AND** it behaves like built-in tools
