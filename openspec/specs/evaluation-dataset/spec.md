# evaluation-dataset Specification

## Purpose
TBD - created by archiving change evaluation-system. Update Purpose after archive.
## Requirements
### Requirement: Mixed dataset storage
The system SHALL support dataset storage in both file system and database.

#### Scenario: Load public dataset from file
- **WHEN** dataset_id matches file in `evaluation/datasets/` directory
- **THEN** system loads from file system
- **AND** system parses `meta.json` and `samples.jsonl`

#### Scenario: Load user dataset from database
- **WHEN** dataset_id does not exist in file system
- **THEN** system queries `evaluation_dataset_records` table
- **AND** system filters by tenant_id and dataset_id

#### Scenario: Dataset not found
- **WHEN** dataset_id exists in neither location
- **THEN** system returns error "Dataset not found"

### Requirement: Dataset format validation
The system SHALL validate dataset format before loading.

#### Scenario: Valid JSONL format
- **WHEN** file contains valid JSONL with question/answer pairs
- **THEN** system successfully loads dataset
- **AND** system returns QA pairs list

#### Scenario: Invalid format
- **WHEN** file contains invalid JSON or missing fields
- **THEN** system returns error "Invalid dataset format"
- **AND** system does not load dataset

### Requirement: Dataset metadata
The system SHALL provide dataset metadata information.

#### Scenario: Get dataset info
- **WHEN** user queries dataset information
- **THEN** system returns:
  - dataset_id
  - description
  - evaluation_type
  - qa_count
  - modified_time

### Requirement: List available datasets
The system SHALL list all available datasets for a tenant.

#### Scenario: List datasets
- **WHEN** user GETs `/api/v1/evaluation/datasets`
- **THEN** system returns list of dataset metadata
- **AND** list includes both file system and database datasets

