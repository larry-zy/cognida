# Evaluation Metrics Spec

## ADDED Requirements

### Requirement: Compute metrics via HTTP API
The system SHALL provide HTTP API for batch metrics computation.

#### Scenario: Successful metrics computation
- **WHEN** Go Worker POSTs to `/api/v1/evaluation/compute-metrics` with valid QA items
- **THEN** Python service returns aggregate metrics
- **AND** Python service returns per-item metrics
- **AND** response includes success: true

#### Scenario: Invalid request
- **WHEN** request contains invalid graders or missing fields
- **THEN** system returns 400 error
- **AND** system returns validation error details

### Requirement: ROUGE metrics
The system SHALL compute ROUGE metrics (ROUGE-1, ROUGE-2, ROUGE-L).

#### Scenario: ROUGE computation
- **WHEN** graders include "rouge"
- **THEN** system computes rouge_1, rouge_2, rouge_l scores (0-100)
- **AND** system returns both aggregate and per-item scores

### Requirement: BLEU metrics
The system SHALL compute BLEU metrics (BLEU-1, BLEU-2, BLEU-4).

#### Scenario: BLEU computation
- **WHEN** graders include "bleu"
- **THEN** system computes bleu_1, bleu_2, bleu_4 scores (0-100)
- **AND** system returns both aggregate and per-item scores

### Requirement: LLM judge metrics
The system SHALL compute LLM-based judge scores for multiple dimensions.

#### Scenario: LLM judge computation
- **WHEN** graders include "llm_judge"
- **AND** dimensions include "accuracy", "relevance", "reasoning"
- **THEN** system calls LLM to judge each dimension
- **AND** system returns scores (1-5) for each dimension
- **AND** system returns total average score

#### Scenario: Custom LLM model
- **WHEN** grader_config specifies custom model
- **THEN** system uses specified LLM model for judging

### Requirement: Semantic similarity metrics
The system SHALL compute semantic similarity between generated and reference answers.

#### Scenario: Semantic similarity computation
- **WHEN** graders include "semantic"
- **THEN** system computes embedding similarity (0-100)
- **AND** system returns average, min, max similarity

### Requirement: Context is optional
The system SHALL handle metrics computation with or without context.

#### Scenario: Metrics without context
- **WHEN** request items do not include context
- **THEN** system computes only non-retrieval metrics
- **AND** system skips precision/recall/NDCG

#### Scenario: Metrics with context
- **WHEN** request items include retrieved context
- **THEN** system computes all applicable metrics including retrieval metrics
