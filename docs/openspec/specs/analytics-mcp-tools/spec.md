# analytics-mcp-tools Specification

## Purpose
TBD - created by archiving change analytics-mcp-integration. Update Purpose after archive.
## Requirements
### Requirement: Tool registration bootstrap

The system SHALL register all default analytics tools into the global `ToolRegistry` at MCP server startup so that `tools/list` returns a non-empty, discoverable tool set.

#### Scenario: Default tools registered on server init

- **WHEN** the Python MCP server is initialized
- **THEN** a `register_default_tools(registry)` bootstrap SHALL be invoked
- **AND** the global registry SHALL contain the analytics tools (`data_describe`, `data_trend`, `data_anomaly`, `data_correlation`, `data_insight`)

#### Scenario: tools/list exposes registered tools

- **WHEN** an MCP `tools/list` request is received after startup
- **THEN** the response SHALL include each registered analytics tool with its `name`, `description`, and `inputSchema`

#### Scenario: Idempotent registration

- **WHEN** `register_default_tools` is invoked more than once
- **THEN** registration SHALL NOT raise an error and the registry SHALL contain exactly one instance per tool name

### Requirement: Analytics tools exposed over MCP

The system SHALL expose the `services/analytics` engines as MCP tools callable via `tools/call`, each wrapping an existing analyzer without changing its computation behavior.

#### Scenario: Descriptive statistics tool

- **WHEN** `tools/call` is invoked with name `data_describe` and a row set
- **THEN** the system SHALL return descriptive statistics (count, mean, median, std, min, max, Q25, Q75, IQR) for the requested numeric columns

#### Scenario: Trend analysis tool

- **WHEN** `tools/call` is invoked with name `data_trend`, a time column and a value column
- **THEN** the system SHALL return trend direction, slope, and forecast/growth metrics

#### Scenario: Anomaly detection tool

- **WHEN** `tools/call` is invoked with name `data_anomaly` and a value column
- **THEN** the system SHALL return detected anomaly points with their method and severity

#### Scenario: Correlation tool

- **WHEN** `tools/call` is invoked with name `data_correlation` and two or more numeric columns
- **THEN** the system SHALL return the correlation result between the columns

#### Scenario: Insight generation tool

- **WHEN** `tools/call` is invoked with name `data_insight` and a row set
- **THEN** the system SHALL return aggregated insights and recommendations derived from trend, anomaly, and correlation finders

### Requirement: Row-set input contract

Analytics MCP tools SHALL accept tabular input as JSON records and convert it to a `pandas.DataFrame` internally, returning JSON-serializable results.

#### Scenario: JSON records converted to DataFrame

- **WHEN** a tool receives `{"columns": [...], "rows": [[...], ...]}` (or an equivalent records array)
- **THEN** the input SHALL be converted to a DataFrame and sanitized (NaN/type handling) before analysis

#### Scenario: Empty or invalid data

- **WHEN** a tool receives an empty row set or rows that cannot form a valid frame
- **THEN** the tool SHALL return a structured error message rather than raising an unhandled exception

#### Scenario: Result is JSON-serializable

- **WHEN** a tool completes successfully
- **THEN** its result SHALL be serializable to JSON for the MCP `tools/call` response

### Requirement: HTTP transport availability

The Python MCP server SHALL be runnable in HTTP mode so the Go service can call analytics tools over JSON-RPC.

#### Scenario: HTTP MCP serves tool calls

- **WHEN** the MCP server runs in `http` mode and receives a JSON-RPC `tools/call` POST
- **THEN** it SHALL execute the named tool and return a JSON-RPC result envelope

