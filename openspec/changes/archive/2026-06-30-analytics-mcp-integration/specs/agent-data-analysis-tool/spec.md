## ADDED Requirements

### Requirement: data_analysis Agent tool

The system SHALL provide a `data_analysis` Agent tool implementing the eino `tool.InvokableTool` interface that lets the Agent run analytics on a retrieved row set.

#### Scenario: Tool advertises its interface

- **WHEN** the Agent inspects available tools
- **THEN** `data_analysis` SHALL expose a name, description, and parameters including `analysis_type` (one of `describe`/`trend`/`anomaly`/`correlation`/`insight`), `data` (row set), and optional `options`

#### Scenario: Tool registered in the Agent tool set

- **WHEN** the Agent tool registry is initialized
- **THEN** `data_analysis` SHALL be registered and available to the Text2SQL / data-analysis Agent

### Requirement: MCP-backed execution

The `data_analysis` tool SHALL execute by calling the corresponding Analytics MCP tool through the existing `infrastructure/mcp.MCPClient`, reusing its retry and caching behavior.

#### Scenario: Routes analysis_type to MCP tool

- **WHEN** `data_analysis` runs with `analysis_type=trend`
- **THEN** it SHALL call MCP tool `data_trend` with the mapped arguments and return the result content

#### Scenario: Reuses configured MCP endpoint

- **WHEN** the tool constructs its MCP client
- **THEN** it SHALL use the MCP endpoint from `infrastructure/config` (shared with the skill MCP path)

#### Scenario: MCP failure is surfaced

- **WHEN** the MCP call fails after retries
- **THEN** the tool SHALL return a non-fatal error result that the Agent can reason about rather than crashing the run

### Requirement: Analysis feeds the conclusion chain

The conclusion generation hook SHALL treat `data_analysis` as a data tool so that conclusions and recommendations are grounded in computed analysis output.

#### Scenario: data_analysis recognized as a data tool

- **WHEN** the Agent calls `data_analysis` during a run
- **THEN** `ConclusionGenerator` SHALL detect it within its data-tool set and trigger conclusion generation

#### Scenario: Conclusion uses analysis output

- **WHEN** a conclusion is generated after `data_analysis` ran
- **THEN** the conclusion's key findings, insights, and recommendations SHALL be derived from the analysis tool output rather than from raw rows alone
