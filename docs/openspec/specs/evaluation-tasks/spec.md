# evaluation-tasks Specification

## Purpose
TBD - created by archiving change evaluation-system. Update Purpose after archive.
## Requirements
### Requirement: Create evaluation task
The system SHALL allow users to create evaluation tasks through HTTP API.

#### Scenario: Successful task creation
- **WHEN** user POSTs to `/api/v1/evaluation/tasks` with valid dataset_id and evaluation_type
- **THEN** system creates a task record in MySQL
- **AND** system pushes task_id to Redis queue `eval:queue`
- **AND** system returns task_id and PENDING status

#### Scenario: Invalid dataset
- **WHEN** user POSTs with non-existent dataset_id
- **THEN** system returns 404 error
- **AND** no task is created

### Requirement: Query task status
The system SHALL allow users to query task status by task_id.

#### Scenario: Task found
- **WHEN** user GETs `/api/v1/evaluation/tasks/{task_id}`
- **THEN** system returns task status (PENDING/PROCESSING/COMPLETED/FAILED)
- **AND** system returns current progress percentage

#### Scenario: Task not found
- **WHEN** user GETs non-existent task_id
- **THEN** system returns 404 error

### Requirement: Get evaluation results
The system SHALL provide evaluation results after task completion.

#### Scenario: Results available
- **WHEN** user GETs `/api/v1/evaluation/tasks/{task_id}/results` for completed task
- **THEN** system returns aggregate metrics (ROUGE, BLEU, LLM judge scores)
- **AND** system returns per-QA results if requested

### Requirement: List evaluation results
The system SHALL allow users to list their evaluation results with pagination.

#### Scenario: List results
- **WHEN** user GETs `/api/v1/evaluation/results?page=1&page_size=10`
- **THEN** system returns paginated list of evaluation results
- **AND** results are filtered by user's tenant_id

### Requirement: Delete evaluation
The system SHALL allow users to delete their evaluation tasks and results.

#### Scenario: Successful deletion
- **WHEN** user DELETEs `/api/v1/evaluation/tasks/{task_id}`
- **THEN** system deletes task from MySQL
- **AND** system deletes associated QA results
- **AND** system returns 204 No Content

