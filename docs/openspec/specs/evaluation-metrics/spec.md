# evaluation-metrics Specification

## Purpose
TBD - created by archiving change python-evaluation-service. Update Purpose after archive.
## Requirements
### Requirement: Calculate retrieval metrics
The system SHALL calculate retrieval quality metrics including Precision, Recall, NDCG, MRR, and MAP.

#### Scenario: Precision calculation
- **WHEN** evaluating retrieval quality
- **THEN** system calculates Precision@k as (relevant_retrieved / total_retrieved)
- **AND** returns precision value between 0.0 and 1.0

#### Scenario: NDCG calculation
- **WHEN** evaluating retrieval ranking quality
- **THEN** system calculates NDCG@k using DCG/IDCG formula
- **AND** returns NDCG value between 0.0 and 1.0

#### Scenario: MRR calculation
- **WHEN** evaluating reciprocal rank
- **THEN** system calculates MRR as average of (1/rank_of_first_relevant)
- **AND** returns MRR value between 0.0 and 1.0

### Requirement: Calculate generation metrics with proper tokenization
The system SHALL calculate ROUGE and BLEU metrics with correct Chinese tokenization.

#### Scenario: ROUGE calculation with jieba
- **WHEN** evaluating Chinese text generation
- **THEN** system uses jieba for Chinese word segmentation
- **AND** calculates ROUGE-1, ROUGE-2, ROUGE-L F1 scores
- **AND** returns scores between 0.0 and 1.0

#### Scenario: BLEU calculation
- **WHEN** evaluating translation quality
- **THEN** system calculates BLEU-1, BLEU-2, BLEU-4 with proper n-gram matching
- **AND** applies brevity penalty for short candidates
- **AND** returns scores between 0.0 and 1.0

#### Scenario: Mixed Chinese-English text
- **WHEN** text contains both Chinese and English
- **THEN** system tokenizes each language appropriately
- **AND** calculates metrics on properly tokenized text

### Requirement: LLM-as-Judge evaluation
The system SHALL support using LLM to evaluate answer quality.

#### Scenario: LLM judge scoring
- **WHEN** LLM judge is enabled
- **THEN** system sends question, generated answer, and reference to LLM
- **AND** receives structured score with reasoning
- **AND** returns total score and dimension scores

#### Scenario: LLM judge dimensions
- **WHEN** LLM judge evaluates answers
- **THEN** system evaluates on dimensions: accuracy, completeness, relevance
- **AND** returns per-dimension scores

### Requirement: Semantic similarity calculation
The system SHALL calculate semantic similarity between texts using embeddings.

#### Scenario: Semantic similarity scoring
- **WHEN** comparing generated answer with reference
- **THEN** system encodes both texts using sentence-transformers
- **AND** calculates cosine similarity
- **AND** returns similarity score between 0.0 and 1.0

#### Scenario: Batch semantic similarity
- **WHEN** evaluating multiple QA pairs
- **THEN** system calculates semantic similarity for each pair
- **AND** returns average semantic similarity

### Requirement: Aggregated metrics reporting
The system SHALL aggregate metrics across all evaluated QA pairs.

#### Scenario: Average metrics calculation
- **WHEN** evaluation completes
- **THEN** system calculates average of each metric across all QA pairs
- **AND** returns aggregated RetrievalMetrics and GenerationMetrics

#### Scenario: Per-QA result details
- **WHEN** caller requests detailed results
- **THEN** system returns individual results for each QA pair
- **AND** includes question, reference, generated, and scores

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

