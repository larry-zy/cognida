"""评测任务模型。"""

from datetime import datetime
from typing import Any
from pydantic import BaseModel, Field
from enum import Enum


class TaskStatus(str, Enum):
    """任务状态。"""
    PENDING = "pending"
    QUEUED = "queued"
    RUNNING = "running"
    SCORING = "scoring"
    COMPLETED = "completed"
    FAILED = "failed"


class JudgeTask(BaseModel):
    """评测任务。"""
    task_id: str
    dataset_id: str
    model_config: dict[str, Any]
    scoring_method: dict[str, Any]
    timeout: int = 1800  # 默认30分钟
    status: TaskStatus = TaskStatus.PENDING
    created_at: datetime = Field(default_factory=datetime.now)


class ModelConfig(BaseModel):
    """模型配置。"""
    provider: str = "openai"  # openai, anthropic, etc.
    model: str = "gpt-4"
    api_base: str | None = None
    api_key: str
    temperature: float = 0.7
    max_tokens: int = 2000
    timeout: int = 60


class SampleResult(BaseModel):
    """样本评测结果。"""
    sample_index: int
    model_output: str
    score: float
    passed: bool
    reason: str = ""
    latency_ms: int
    prompt_tokens: int = 0
    completion_tokens: int = 0


class EvaluationResult(BaseModel):
    """评测结果。"""
    task_id: str
    dataset_id: str
    model_config: dict[str, Any]
    total_samples: int
    scored_samples: int
    avg_score: float
    pass_rate: float
    total_tokens: int
    total_latency_ms: int
    samples: list[SampleResult]
    started_at: datetime
    completed_at: datetime
    error_message: str = ""


class ProgressUpdate(BaseModel):
    """进度更新。"""
    task_id: str
    status: TaskStatus
    progress: int  # 0-100
    stage: str
    current_sample: int
    total_samples: int
    message: str
    timestamp: datetime = Field(default_factory=datetime.now)
