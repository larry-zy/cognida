# evaluation-datasets Specification

## Purpose
TBD - created by archiving change python-evaluation-service. Update Purpose after archive.
## Requirements
### Requirement: Local dataset storage
The system SHALL store evaluation datasets locally in JSON format.

#### Scenario: Default dataset availability
- **WHEN** system starts
- **THEN** default evaluation dataset is loaded and available
- **AND** dataset ID "default" can be used for evaluation

#### Scenario: Dataset file format
- **WHEN** reading dataset file
- **THEN** system supports JSON format with qa_pairs array
- **AND** each QA pair contains: question, answer, relevant_chunks

### Requirement: Dataset validation
The system SHALL validate dataset structure before use.

#### Scenario: Valid dataset
- **WHEN** dataset has correct structure
- **THEN** validation passes and dataset is loaded

#### Scenario: Invalid dataset format
- **WHEN** dataset file has malformed JSON
- **THEN** system returns validation error with specific issue

#### Scenario: Missing required fields
- **WHEN** QA pair missing required field
- **THEN** system returns error specifying missing field

### Requirement: Multiple dataset support
The system SHALL support multiple evaluation datasets.

#### Scenario: Dataset by ID
- **WHEN** request specifies dataset_id
- **THEN** system loads corresponding dataset file
- **AND** uses it for evaluation

#### Scenario: Non-existent dataset
- **WHEN** request specifies unknown dataset_id
- **THEN** system returns error indicating dataset not found

### Requirement: Dataset metadata
The system SHALL provide metadata about datasets.

#### Scenario: Dataset info query
- **WHEN** querying dataset information
- **THEN** system returns: dataset_id, description, qa_pair_count

### Requirement: Dataset hot reload
The system SHALL support reloading datasets without restart.

#### Scenario: Dataset file updated
- **WHEN** dataset file is modified
- **THEN** system detects change and reloads dataset
- **AND** next evaluation uses updated data

#### Scenario: Dataset file deleted
- **WHEN** dataset file is deleted
- **THEN** system removes dataset from available list

