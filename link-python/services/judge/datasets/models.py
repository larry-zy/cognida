"""数据集模型。"""

from datetime import datetime
from pathlib import Path
from typing import Any
from pydantic import BaseModel, Field


class DatasetMeta(BaseModel):
    """数据集元信息。"""
    dataset_id: str
    name: str
    description: str = ""
    fields: list[str] = Field(default_factory=list)  # 如: ["question", "answer"]
    metadata: dict[str, str] = Field(default_factory=dict)
    created_at: datetime = Field(default_factory=datetime.now)
    updated_at: datetime = Field(default_factory=datetime.now)


class DatasetSample(BaseModel):
    """数据集样本。"""
    index: int
    data: dict[str, Any]  # 样本数据，如 {"question": "...", "answer": "..."}


class Dataset(BaseModel):
    """数据集。"""
    meta: DatasetMeta
    samples: list[DatasetSample]
    path: Path

    @property
    def sample_count(self) -> int:
        return len(self.samples)
