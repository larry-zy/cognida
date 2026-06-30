# Python Evaluation Service - Implementation Tasks

## 1. Project Setup

- [x] 1.1 Create `services/evaluation/` directory structure
- [x] 1.2 Create `services/evaluation/metrics/` subdirectory
- [x] 1.3 Create `services/evaluation/datasets/` subdirectory
- [x] 1.4 Create `services/evaluation/datasets/data/` for dataset files
- [x] 1.5 Add Python dependencies: jieba, sentence-transformers, rouge-chinese

## 2. gRPC Protocol Definition

- [x] 2.1 Create `proto/evaluation.proto` with EvaluationService
- [x] 2.2 Define EvaluationRequest message
- [x] 2.3 Define EvaluationResponse message with oneof (Progress/Result/Error)
- [x] 2.4 Define Progress message (stage, current, total, message)
- [x] 2.5 Define EvaluationResult message structure
- [x] 2.6 Define EvaluationConfig message
- [x] 2.7 Generate gRPC Python code from proto

## 3. Dataset Management

- [x] 3.1 Create default dataset JSON file in `datasets/data/default.json`
- [x] 3.2 Implement dataset loader in `datasets/manager.py`
- [x] 3.3 Implement dataset validation (structure check, required fields)
- [x] 3.4 Implement dataset listing functionality
- [x] 3.5 Add support for multiple datasets by ID
- [x] 3.6 Implement dataset hot-reload mechanism

## 4. Tokenization (Chinese/English)

- [x] 4.1 Create tokenization utilities in `metrics/tokenizer.py`
- [x] 4.2 Implement jieba-based Chinese word segmentation
- [x] 4.3 Implement English word tokenization
- [x] 4.4 Implement mixed Chinese-English text handling
- [x] 4.5 Add tests for tokenization correctness

## 5. Retrieval Metrics

- [x] 5.1 Create `metrics/retrieval.py`
- [x] 5.2 Implement Precision@k calculation
- [x] 5.3 Implement Recall@k calculation
- [x] 5.4 Implement NDCG@k calculation (DCG/IDCG)
- [x] 5.5 Implement MRR (Mean Reciprocal Rank) calculation
- [x] 5.6 Implement MAP (Mean Average Precision) calculation
- [x] 5.7 Add tests for retrieval metrics

## 6. Generation Metrics

- [x] 6.1 Create `metrics/generation.py`
- [x] 6.2 Implement ROUGE-1 calculation with proper tokenization
- [x] 6.3 Implement ROUGE-2 calculation
- [x] 6.4 Implement ROUGE-L calculation (LCS)
- [x] 6.5 Implement BLEU-1 calculation with brevity penalty
- [x] 6.6 Implement BLEU-2 calculation
- [x] 6.7 Implement BLEU-4 calculation
- [x] 6.8 Add tests for generation metrics

## 7. Grader Plugin System

- [x] 7.1 Create `graders/` directory structure
- [x] 7.2 Create `graders/base.py` with BaseGrader abstract class
- [x] 7.3 Create `graders/registry.py` with grader registration mechanism
- [x] 7.4 Implement auto-discovery of graders from builtin/ and custom/ directories
- [x] 7.5 Implement grader validation (signature check, dependency check)
- [x] 7.6 Implement grader metadata (name, description, parameters, return_type)
- [x] 7.7 Add tests for grader registration and discovery

## 8. Built-in Graders

- [x] 8.1 Create `graders/builtin/retrieval.py` with retrieval graders
- [x] 8.2 Implement PrecisionGrader, RecallGrader, NDCGGrader, MRRGrader, MAPGrader
- [x] 8.3 Create `graders/builtin/generation.py` with generation graders
- [x] 8.4 Implement RougeGrader (ROUGE-1/2/L), BleuGrader (BLEU-1/2/4)
- [x] 8.5 Create `graders/builtin/semantic.py` with semantic similarity grader
- [x] 8.6 Create `graders/builtin/llm.py` with LLM-as-Judge grader
- [x] 8.7 Support custom dimensions in LLM judge grader
- [x] 8.8 Add tests for all built-in graders

## 9. Custom Graders Support

- [x] 9.1 Create `graders/custom/` directory for user-defined graders
- [x] 9.2 Implement function-based grader support
- [x] 9.3 Implement class-based grader support with config
- [x] 9.4 Implement hot-reload for custom graders
- [x] 9.5 Add example custom grader
- [x] 9.6 Add tests for custom grader loading and hot-reload

## 10. Evaluation Strategies

- [x] 10.1 Create `strategies/` directory
- [x] 10.2 Create `strategies/base.py` with BaseStrategy abstract class
- [x] 10.3 Implement ZeroShotStrategy (direct grader execution)
- [x] 10.4 Implement DataDrivenStrategy (learn from sample data)
- [x] 10.5 Implement EnsembleStrategy (combine multiple graders)
- [x] 10.6 Implement ConditionalStrategy (select grader by question type)
- [x] 10.7 Add tests for all strategies

## 11. LLM-as-Judge Integration (Legacy Refactor)

- [x] 11.1 Move existing LLM judge code to `graders/builtin/llm.py`
- [x] 11.2 Refactor as grader class implementing BaseGrader
- [x] 11.3 Support LLM judge as component in composite graders
- [x] 11.4 Add configuration for custom evaluation dimensions
- [x] 11.5 Add tests for LLM judge grader

## 12. Semantic Similarity

- [x] 12.1 Create `graders/builtin/semantic.py` (moved from metrics/)
- [x] 12.2 Implement sentence-transformers embedding
- [x] 12.3 Implement cosine similarity calculation
- [x] 12.4 Implement batch semantic similarity for multiple QA pairs
- [x] 12.5 Add model loading and caching
- [x] 12.6 Add tests for semantic similarity

## 13. Evaluation Runner

- [x] 13.1 Create `runner.py` - evaluation orchestration
- [x] 13.2 Implement stage-based progress tracking
- [x] 13.3 Implement retrieval stage execution using grader system
- [x] 13.4 Implement generation stage execution using grader system
- [x] 13.5 Implement metrics calculation stage using strategy
- [x] 13.6 Implement error handling and recovery
- [x] 13.7 Implement Go knowledge base integration (placeholder for now)

## 14. gRPC Service Implementation

- [x] 14.1 Create `service.py` - gRPC EvaluationService servicer
- [x] 14.2 Implement ExecuteEvaluation RPC method
- [x] 14.3 Implement streaming response logic
- [x] 14.4 Implement request validation
- [x] 14.5 Implement error response formatting
- [x] 14.6 Add concurrent evaluation support

## 15. Integration and Configuration

- [x] 15.1 Update `grpc/servicer.py` to register EvaluationService
- [x] 15.2 Add environment variable configuration for Go KB service
- [x] 15.3 Add LLM configuration (model, API key)
- [x] 15.4 Add evaluation configuration (default graders, strategies, thresholds)
- [x] 15.5 Add custom grader directory configuration

## 16. Testing

- [x] 16.1 Create sample dataset for testing
- [x] 16.2 Write integration test for complete evaluation flow
- [x] 16.3 Write test for streaming progress updates
- [x] 16.4 Write test for error scenarios
- [x] 16.5 Write test for concurrent evaluations
- [x] 16.6 Write test for custom grader loading and execution
- [x] 16.7 Write test for evaluation strategies
- [x] 16.8 Performance test for large datasets

## 17. Documentation

- [x] 17.1 Update README.md with evaluation service documentation
- [x] 17.2 Document gRPC interface usage
- [x] 17.3 Document dataset format
- [x] 17.4 Document configuration options
- [x] 17.5 Document how to create custom graders
- [x] 17.6 Document available evaluation strategies
- [x] 17.7 Add example evaluation request/response
- [x] 17.8 Add example custom grader code

## 18. Deployment

- [x] 18.1 Update Docker image with new dependencies
- [x] 18.2 Configure environment variables for production
- [x] 18.3 Add health check for evaluation service
- [x] 18.4 Setup monitoring and logging
