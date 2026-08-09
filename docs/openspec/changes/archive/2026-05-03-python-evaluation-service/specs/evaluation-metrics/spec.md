# Evaluation Metrics Capability Specification

## ADDED Requirements

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
