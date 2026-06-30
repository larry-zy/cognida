# Agent Orchestration

## ADDED Requirements

### Requirement: Orchestration as Agent

All orchestration patterns SHALL implement the Agent interface.

#### Scenario: Orchestration is composable
- **GIVEN** any orchestration pattern (Sequential, Parallel, etc.)
- **WHEN** it implements Agent interface
- **THEN** it can be used anywhere an Agent is expected
- **AND** orchestrations can nest within other orchestrations

### Requirement: Sequential Orchestration

The system SHALL provide Sequential execution pattern.

#### Scenario: Sequential executes agents in order
- **GIVEN** a Sequential orchestration with agent1, agent2, agent3
- **WHEN** calling Chat(ctx, message)
- **THEN** agent1 receives the original message
- **AND** agent2 receives agent1's response
- **AND** agent3 receives agent2's response
- **AND** the final response is agent3's output

#### Scenario: Sequential with variadic agents
- **GIVEN** multiple Agent instances
- **WHEN** calling Sequential(agent1, agent2, agent3, ...)
- **THEN** it returns an Agent that executes them sequentially

#### Scenario: Sequential error handling
- **GIVEN** a Sequential orchestration
- **WHEN** any agent returns an error
- **THEN** the sequential execution stops
- **AND** it returns the error

### Requirement: Parallel Orchestration

The system SHALL provide Parallel execution pattern.

#### Scenario: Parallel executes agents concurrently
- **GIVEN** a Parallel orchestration with agent1, agent2, agent3
- **WHEN** calling Chat(ctx, message)
- **THEN** all agents receive the same message concurrently
- **AND** responses are collected
- **AND** it returns combined responses

#### Scenario: Parallel response aggregation
- **GIVEN** a Parallel orchestration
- **WHEN** all agents complete
- **THEN** the response contains all agent outputs
- **AND** responses are aggregated in completion order

#### Scenario: Parallel error handling
- **GIVEN** a Parallel orchestration
- **WHEN** any agent returns an error
- **THEN** it continues waiting for other agents
- **AND** the error is included in the final response

### Requirement: Supervisor Orchestration

The system SHALL provide Supervisor pattern for coordinated execution.

#### Scenario: Supervisor with coordinator and workers
- **GIVEN** a coordinator Agent and multiple worker Agents
- **WHEN** calling Supervisor(coordinator, worker1, worker2)
- **THEN** it returns a supervisor Agent
- **AND** coordinator decides which worker to use
- **AND** workers execute assigned tasks

#### Scenario: Supervisor dynamic routing
- **GIVEN** a Supervisor orchestration
- **WHEN** Chat is called
- **THEN** coordinator analyzes the message
- **AND** selects appropriate worker
- **AND** routes message to selected worker
- **AND** returns worker's response

### Requirement: Conditional Orchestration

The system SHALL provide Conditional branching pattern.

#### Scenario: Conditional with predicate function
- **GIVEN** a predicate function and two agents
- **WHEN** calling Conditional(predicate, trueAgent, falseAgent)
- **THEN** it returns an Agent
- **AND** predicate is evaluated on each Chat call
- **AND** trueAgent executes if predicate returns true
- **AND** falseAgent executes if predicate returns false

#### Scenario: Conditional with multiple branches
- **GIVEN** multiple predicate-agent pairs
- **WHEN** calling Conditional with multiple branches
- **THEN** it evaluates predicates in order
- **AND** executes the first matching agent
- **AND** executes default agent if none match

### Requirement: Loop Orchestration

The system SHALL provide Loop execution pattern.

#### Scenario: Loop with continuation condition
- **GIVEN** a body Agent and continuation function
- **WHEN** calling Loop(bodyAgent, continuationFunc)
- **THEN** it executes bodyAgent repeatedly
- **AND** continues while continuationFunc returns true
- **AND** stops when continuationFunc returns false

#### Scenario: Loop with max iterations
- **GIVEN** a Loop orchestration
- **WHEN** specifying WithMaxIterations(n)
- **THEN** it stops after n iterations
- **AND** it returns error if max iterations exceeded

#### Scenario: Loop accumulates results
- **GIVEN** a Loop orchestration
- **WHEN** executing multiple iterations
- **THEN** each iteration receives previous iteration's output
- **AND** final response contains all accumulated results

### Requirement: Func Orchestration

The system SHALL provide a way to create Agent from function.

#### Scenario: Func creates custom Agent
- **GIVEN** a function with signature `func(ctx context.Context, message string) (*Response, error)`
- **WHEN** calling Func(fn)
- **THEN** it returns an Agent
- **AND** Chat calls the function
- **AND** it returns the function's response

#### Scenario: Func for custom logic
- **GIVEN** complex business logic
- **WHEN** wrapping it in Func
- **THEN** it behaves like any other Agent
- **AND** can be used in orchestrations

### Requirement: Orchestration Composition

The system SHALL support nested orchestration.

#### Scenario: Nested orchestrations
- **GIVEN** multiple orchestration patterns
- **WHEN** composing Sequential(Parallel(a1, a2), Supervisor(c, w1, w2))
- **THEN** it creates a complex Agent
- **AND** all nested patterns execute correctly

#### Scenario: Deep nesting
- **GIVEN** deeply nested orchestrations (3+ levels)
- **WHEN** calling Chat
- **THEN** execution flows through all levels correctly
- **AND** context is propagated properly

### Requirement: Orchestration Streaming

All orchestrations SHALL support streaming.

#### Scenario: Sequential streaming
- **GIVEN** a Sequential orchestration
- **WHEN** calling Stream(ctx, message)
- **THEN** chunks from each agent are streamed
- **AND** chunks are delivered in order

#### Scenario: Parallel streaming
- **GIVEN** a Parallel orchestration
- **WHEN** calling Stream(ctx, message)
- **THEN** chunks from all agents are streamed
- **AND** chunks are interleaved as they arrive
