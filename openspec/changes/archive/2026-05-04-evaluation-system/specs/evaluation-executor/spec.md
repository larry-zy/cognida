# Evaluation Executor Spec

## ADDED Requirements

### Requirement: Agent evaluation execution
The system SHALL support Agent-based evaluation for agent-type tasks.

#### Scenario: Successful Agent evaluation
- **WHEN** task has agent_id configured
- **THEN** system loads Agent by agent_id
- **AND** system calls Agent.Chat() for each QA pair
- **AND** system collects generated responses

#### Scenario: Agent not found
- **WHEN** configured agent_id does not exist
- **THEN** system marks task as FAILED
- **AND** system returns error "Agent not found"

### Requirement: RAG evaluation execution
The system SHALL support RAG evaluation for RAG-type tasks.

#### Scenario: Successful RAG evaluation
- **WHEN** task has kb_id and model_id configured
- **THEN** system calls Retriever.Retrieve() with query and top_k
- **AND** system builds prompt with retrieved context
- **AND** system calls LLMChat.Chat() to generate answer
- **AND** system stores retrieved chunks for metrics

#### Scenario: Retrieval failure
- **WHEN** retrieval operation fails
- **THEN** system marks that QA as failed
- **AND** system continues processing remaining QAs

### Requirement: QA evaluation execution
The system SHALL support direct QA evaluation for qa-type tasks.

#### Scenario: Successful QA evaluation
- **WHEN** task has model_id but no kb_id
- **THEN** system builds prompt with question only
- **AND** system calls LLMChat.Chat() to generate answer
- **AND** system stores generated response for metrics

### Requirement: Result collection
The system SHALL collect QA results for all evaluation types.

#### Scenario: Collect results
- **WHEN** executor completes processing
- **THEN** system returns list of QAResult with:
  - question
  - reference_answer
  - generated_answer
  - retrieved_chunks (for RAG)
  - success status
  - error message (if failed)
