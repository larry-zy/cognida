"""评测数据集管理器。

支持数据集加载、验证、热重载等功能。
"""

import json
import os
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional

from pydantic import BaseModel, Field, field_validator, model_validator

from core import get_logger


# ============================================================
# 数据模型
# ============================================================

class RelevantChunk(BaseModel):
    """相关文档块。"""
    chunk_id: str = Field(..., description="文档块 ID")
    content: str = Field(..., description="文档块内容")
    score: Optional[float] = Field(None, description="相关性分数")


class QAPair(BaseModel):
    """问答对。"""
    question: str = Field(..., description="问题")
    answer: str = Field(..., description="参考答案")
    relevant_chunks: List[RelevantChunk] = Field(
        default_factory=list,
        description="相关文档块列表"
    )
    metadata: Dict[str, Any] = Field(
        default_factory=dict,
        description="额外元数据"
    )

    @field_validator("question", "answer")
    @classmethod
    def not_empty(cls, v: str) -> str:
        """验证字段非空。"""
        if not v or not v.strip():
            raise ValueError("question and answer must not be empty")
        return v.strip()


class DatasetMetadata(BaseModel):
    """数据集元数据。"""
    dataset_id: str = Field(..., description="数据集 ID")
    description: str = Field("", description="数据集描述")
    version: str = Field("1.0.0", description="版本号")
    domains: List[str] = Field(default_factory=list, description="领域列表")
    languages: List[str] = Field(default_factory=list, description="语言列表")
    metadata: Dict[str, Any] = Field(default_factory=dict, description="额外元数据")


class Dataset(BaseModel):
    """评测数据集。"""
    metadata: DatasetMetadata = Field(..., description="数据集元数据")
    qa_pairs: List[QAPair] = Field(..., description="问答对列表")

    @model_validator(mode="after")
    def validate_qa_pairs(self) -> "Dataset":
        """验证问答对列表。"""
        if not self.qa_pairs:
            raise ValueError("qa_pairs must not be empty")
        return self

    @property
    def dataset_id(self) -> str:
        """获取数据集 ID。"""
        return self.metadata.dataset_id

    @property
    def qa_count(self) -> int:
        """获取问答对数量。"""
        return len(self.qa_pairs)


# ============================================================
# 数据集管理器
# ============================================================

class DatasetManager:
    """评测数据集管理器。

    负责：
    - 数据集加载和验证
    - 数据集缓存
    - 热重载
    """

    def __init__(self, data_dir: Optional[str | Path] = None) -> None:
        """初始化数据集管理器。

        Args:
            data_dir: 数据集目录路径，默认为 datasets/data/
        """
        self._logger = get_logger(__name__)

        if data_dir is None:
            # 默认路径：相对于当前文件
            current_dir = Path(__file__).parent
            data_dir = current_dir / "data"

        self.data_dir = Path(data_dir)
        self._datasets: Dict[str, Dataset] = {}
        self._file_timestamps: Dict[str, float] = {}

        # 确保数据目录存在
        self.data_dir.mkdir(parents=True, exist_ok=True)

        self._logger.info(f"DatasetManager initialized", data_dir=str(self.data_dir))

    def list_datasets(self) -> List[str]:
        """列出所有可用的数据集 ID。

        Returns:
            数据集 ID 列表
        """
        self._reload_if_changed()
        return list(self._datasets.keys())

    def get_dataset(self, dataset_id: str) -> Optional[Dataset]:
        """获取数据集。

        Args:
            dataset_id: 数据集 ID

        Returns:
            数据集对象，如果不存在返回 None
        """
        self._reload_if_changed()
        return self._datasets.get(dataset_id)

    def get_dataset_info(self, dataset_id: str) -> Optional[Dict[str, Any]]:
        """获取数据集信息。

        Args:
            dataset_id: 数据集 ID

        Returns:
            数据集信息字典，如果不存在返回 None
        """
        dataset = self.get_dataset(dataset_id)
        if dataset is None:
            return None

        return {
            "dataset_id": dataset.dataset_id,
            "description": dataset.metadata.description,
            "version": dataset.metadata.version,
            "domains": dataset.metadata.domains,
            "languages": dataset.metadata.languages,
            "qa_count": dataset.qa_count,
            "size": len(dataset.model_dump_json()),
        }

    def list_datasets_info(self) -> List[Dict[str, Any]]:
        """列出所有数据集的信息。

        Returns:
            数据集信息列表
        """
        result = []
        for dataset_id in self.list_datasets():
            info = self.get_dataset_info(dataset_id)
            if info:
                # 添加修改时间
                file_path = self._get_file_path(dataset_id)
                if file_path and file_path.exists():
                    mtime = os.path.getmtime(file_path)
                    info["modified_time"] = int(mtime)
                result.append(info)
        return result

    def load_dataset(self, dataset_id: str) -> Dataset:
        """加载数据集。

        Args:
            dataset_id: 数据集 ID

        Returns:
            数据集对象

        Raises:
            FileNotFoundError: 数据集文件不存在
            ValueError: 数据集格式错误
        """
        file_path = self._get_file_path(dataset_id)

        if not file_path or not file_path.exists():
            raise FileNotFoundError(f"Dataset not found: {dataset_id}")

        self._logger.info(f"Loading dataset", dataset_id=dataset_id, path=str(file_path))

        try:
            with open(file_path, "r", encoding="utf-8") as f:
                data = json.load(f)
        except json.JSONDecodeError as e:
            raise ValueError(f"Invalid JSON in dataset file: {e}")

        # 验证并解析数据集
        try:
            dataset = Dataset(**data)
        except Exception as e:
            raise ValueError(f"Dataset validation failed: {e}")

        # 更新缓存
        self._datasets[dataset_id] = dataset
        self._file_timestamps[dataset_id] = os.path.getmtime(file_path)

        self._logger.info(
            f"Dataset loaded successfully",
            dataset_id=dataset_id,
            qa_count=dataset.qa_count
        )

        return dataset

    def reload_dataset(self, dataset_id: str) -> Optional[Dataset]:
        """重新加载数据集。

        Args:
            dataset_id: 数据集 ID

        Returns:
            重新加载的数据集对象，如果不存在返回 None
        """
        if dataset_id not in self._datasets:
            return self.load_dataset(dataset_id)

        # 清除缓存
        self._datasets.pop(dataset_id, None)
        self._file_timestamps.pop(dataset_id, None)

        # 重新加载
        return self.load_dataset(dataset_id)

    def reload_all(self) -> None:
        """重新加载所有数据集。"""
        self._logger.info("Reloading all datasets")
        self._datasets.clear()
        self._file_timestamps.clear()
        self._scan_and_load()

    def _get_file_path(self, dataset_id: str) -> Optional[Path]:
        """获取数据集文件路径。

        Args:
            dataset_id: 数据集 ID

        Returns:
            文件路径，如果不存在返回 None
        """
        # 尝试 .json 文件
        json_path = self.data_dir / f"{dataset_id}.json"
        if json_path.exists():
            return json_path

        return None

    def _has_file_changed(self, dataset_id: str) -> bool:
        """检查数据集文件是否已更改。

        Args:
            dataset_id: 数据集 ID

        Returns:
            如果文件已更改返回 True
        """
        file_path = self._get_file_path(dataset_id)
        if not file_path:
            return False

        current_mtime = os.path.getmtime(file_path)
        cached_mtime = self._file_timestamps.get(dataset_id, 0)

        return current_mtime > cached_mtime

    def _reload_if_changed(self) -> None:
        """如果文件已更改，自动重新加载。"""
        for dataset_id in list(self._datasets.keys()):
            if self._has_file_changed(dataset_id):
                self._logger.info(f"Dataset file changed, reloading", dataset_id=dataset_id)
                try:
                    self.reload_dataset(dataset_id)
                except Exception as e:
                    self._logger.error(f"Failed to reload dataset", dataset_id=dataset_id, error=str(e))

    def _scan_and_load(self) -> None:
        """扫描数据目录并加载所有数据集。"""
        for json_file in self.data_dir.glob("*.json"):
            dataset_id = json_file.stem
            try:
                self.load_dataset(dataset_id)
            except Exception as e:
                self._logger.warning(
                    f"Failed to load dataset file",
                    file=str(json_file),
                    error=str(e)
                )

    def initialize(self) -> None:
        """初始化管理器，扫描并加载所有数据集。"""
        self._logger.info("Initializing dataset manager")
        self._scan_and_load()
        self._logger.info(
            f"Dataset manager initialized",
            count=len(self._datasets),
            datasets=list(self._datasets.keys())
        )


# ============================================================
# 默认数据集内容
# ============================================================

DEFAULT_DATASET = {
    "metadata": {
        "dataset_id": "default",
        "description": "默认评测数据集",
        "version": "1.0.0",
        "domains": ["general"],
        "languages": ["zh"],
    },
    "qa_pairs": [
        {
            "question": "什么是机器学习？",
            "answer": "机器学习是人工智能的一个分支，它使计算机能够在没有被明确编程的情况下学习。",
            "relevant_chunks": [
                {
                    "chunk_id": "chunk_1",
                    "content": "机器学习是人工智能的一个分支...",
                    "score": 0.95
                }
            ]
        },
        {
            "question": "Python 是什么？",
            "answer": "Python 是一种高级编程语言，以其简洁的语法和强大的功能而闻名。",
            "relevant_chunks": [
                {
                    "chunk_id": "chunk_2",
                    "content": "Python 是由 Guido van Rossum 创建的高级编程语言...",
                    "score": 0.92
                }
            ]
        }
    ]
}


def create_default_dataset(data_dir: Optional[Path] = None) -> Path:
    """创建默认数据集文件。

    Args:
        data_dir: 数据集目录

    Returns:
        创建的文件路径
    """
    if data_dir is None:
        data_dir = Path(__file__).parent / "data"

    data_dir.mkdir(parents=True, exist_ok=True)
    file_path = data_dir / "default.json"

    if not file_path.exists():
        with open(file_path, "w", encoding="utf-8") as f:
            json.dump(DEFAULT_DATASET, f, ensure_ascii=False, indent=2)

    return file_path


# ============================================================
# 全局实例
# ============================================================

_global_manager: Optional[DatasetManager] = None


def get_dataset_manager() -> DatasetManager:
    """获取全局数据集管理器实例。"""
    global _global_manager
    if _global_manager is None:
        _global_manager = DatasetManager()
        _global_manager.initialize()
    return _global_manager
