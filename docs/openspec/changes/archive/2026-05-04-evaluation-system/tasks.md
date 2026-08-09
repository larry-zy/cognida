# Evaluation System Implementation Tasks

## 1. Go - Infrastructure Layer

- [x] 1.1 Create Redis evaluation queue module (`infrastructure/redis/evaluation/queue.go`)
  - Implement `Enqueue()` / `Dequeue()` operations
  - Implement `AcquireSlot()` / `ReleaseSlot()` for concurrency control
  - Add unit tests for queue operations

- [x] 1.2 Create Redis progress cache module (`infrastructure/redis/evaluation/progress.go`)
  - Implement `SetProgress()` / `GetProgress()` operations
  - Implement `UpdateStage()` for stage transitions
  - Add 1-hour expiration to progress entries

- [x] 1.3 Create dataset loader (`application/evaluation/dataset_loader.go`)
  - Implement `LoadFromFile()` for file system datasets
  - Implement `LoadFromDB()` for database datasets
  - Implement `Load()` with mixed mode logic
  - Add validation for dataset format

## 2. Go - Python Client

- [x] 2.1 Create Python HTTP client (`application/evaluation/python_client.go`)
  - Implement `ComputeMetrics()` method
  - Add request/response DTOs
  - Implement HTTP timeout and retry logic
  - Add error handling for service unavailable

## 3. Go - Executors

- [x] 3.1 Create executor interface and registry (`application/evaluation/executor/registry.go`)
  - Define `Executor` interface
  - Implement `ExecutorRegistry` with type-based routing

- [x] 3.2 Create Agent executor (`application/evaluation/executor/agent.go`)
  - Implement agent loading from repository
  - Implement `Execute()` with Agent.Chat() calls
  - Add 30s timeout per QA

- [x] 3.3 Create RAG executor (`application/evaluation/executor/rag.go`)
  - Implement retrieval with top_k configuration
  - Implement prompt building with context
  - Implement LLM generation
  - Add 10s retrieval + 30s generation timeouts

- [x] 3.4 Create QA executor (`application/evaluation/executor/qa.go`)
  - Implement direct LLM generation
  - Add 30s timeout per QA

## 4. Go - Worker

- [x] 4.1 Create Worker core (`application/evaluation/worker.go`)
  - Implement `Run()` main loop with BRPOP
  - Implement `executeTask()` orchestration logic
  - Add graceful shutdown support

- [x] 4.2 Implement retry logic
  - Add retry count tracking
  - Implement backoff for retryable errors
  - Handle max retries (3) exceeded case

- [x] 4.3 Implement progress updates
  - Update Redis progress cache during execution
  - Report stage transitions
  - Calculate percentage completion

## 5. Go - HTTP API

- [x] 5.1 Create task API (`interface/http/handler/evaluation_handler.go`)
  - Implement `CreateTask()` handler
  - Implement `GetTask()` handler
  - Implement `GetResults()` handler
  - Implement `ListResults()` handler
  - Implement `DeleteTask()` handler

- [x] 5.2 Create SSE stream handler
  - Implement `StreamTask()` handler
  - Add SSE headers
  - Implement 1-second polling interval
  - Handle client disconnect

- [x] 5.3 Create dataset API
  - Implement `ListDatasets()` handler
  - Implement `GetDatasetInfo()` handler

## 6. Go - Application Service

- [x] 6.1 Update evaluation service (`application/evaluation/service.go`)
  - Add `CreateEvaluation()` with task creation
  - Integrate with task repository
  - Add evaluation type validation

## 7. Python - HTTP API

- [x] 7.1 Add FastAPI route for metrics computation (`services/evaluation/api.py`)
  - Implement `POST /api/v1/evaluation/compute-metrics` endpoint
  - Add request validation with Pydantic
  - Add error handling

- [x] 7.2 Create compute service (`services/evaluation/compute_service.py`)
  - Implement batch metrics computation
  - Integrate with existing graders (rouge, bleu, llm_judge)
  - Return both aggregate and per-item results

## 8. Testing

- [x] 8.1 Add unit tests for Redis operations
- [x] 8.2 Add unit tests for executors
- [x] 8.3 Add unit tests for Python client
- [x] 8.4 Add integration test for complete flow
- [x] 8.5 Add end-to-end test with sample dataset

## 9. Configuration & Deployment

- [x] 9.1 Add environment variables for configuration
  - `EVALUATION_MAX_CONCURRENT=3`
  - `EVALUATION_WORKER_ENABLED=true`
  - `PYTHON_EVALUATION_ENDPOINT`

- [x] 9.2 Wire up Worker in service initialization
- [x] 9.3 Add Worker startup to main.go

## 10. Documentation

- [x] 10.1 Update API documentation
- [x] 10.2 Add evaluation system usage guide
- [x] 10.3 Update README with new capabilities
