"""数据集管理器。"""

import json
from pathlib import Path
from typing import Any

from core import get_logger
from .models import Dataset, DatasetMeta, DatasetSample


class DatasetManager:
    """数据集管理器。"""

    def __init__(self, data_dir: Path | str = Path("services/judge/data")) -> None:
        """初始化数据集管理器。

        Args:
            data_dir: 数据集存储目录
        """
        self._data_dir = Path(data_dir)
        self._logger = get_logger(__name__)
        self._cache: dict[str, Dataset] = {}

    def list_datasets(self) -> list[DatasetMeta]:
        """列出所有数据集。

        Returns:
            数据集元信息列表
        """
        datasets = []
        for dataset_path in self._data_dir.iterdir():
            if not dataset_path.is_dir():
                continue

            meta_path = dataset_path / "meta.json"
            if not meta_path.exists():
                continue

            try:
                with open(meta_path, encoding="utf-8") as f:
                    meta_data = json.load(f)
                meta = DatasetMeta(**meta_data)
                datasets.append(meta)
            except Exception as e:
                self._logger.warning(f"Failed to load dataset meta: {dataset_path}", error=str(e))

        return datasets

    def get_dataset(self, dataset_id: str) -> Dataset | None:
        """获取数据集。

        Args:
            dataset_id: 数据集 ID

        Returns:
            数据集，不存在则返回 None
        """
        # 检查缓存
        if dataset_id in self._cache:
            return self._cache[dataset_id]

        dataset_path = self._data_dir / dataset_id
        if not dataset_path.exists():
            return None

        # 加载元信息
        meta_path = dataset_path / "meta.json"
        if not meta_path.exists():
            self._logger.error(f"Dataset meta not found: {dataset_id}")
            return None

        with open(meta_path, encoding="utf-8") as f:
            meta_data = json.load(f)
        meta = DatasetMeta(**meta_data)

        # 加载样本
        samples_path = dataset_path / "samples.jsonl"
        samples = []
        if samples_path.exists():
            with open(samples_path, encoding="utf-8") as f:
                for index, line in enumerate(f):
                    if line.strip():
                        data = json.loads(line)
                        samples.append(DatasetSample(index=index, data=data))

        dataset = Dataset(meta=meta, samples=samples, path=dataset_path)
        self._cache[dataset_id] = dataset
        return dataset

    def create_dataset(
        self,
        dataset_id: str,
        name: str,
        description: str = "",
        fields: list[str] | None = None,
    ) -> Dataset:
        """创建新数据集。

        Args:
            dataset_id: 数据集 ID
            name: 数据集名称
            description: 描述
            fields: 数据字段列表

        Returns:
            创建的数据集
        """
        dataset_path = self._data_dir / dataset_id
        dataset_path.mkdir(parents=True, exist_ok=True)

        meta = DatasetMeta(
            dataset_id=dataset_id,
            name=name,
            description=description,
            fields=fields or ["question", "answer"],
        )

        meta_path = dataset_path / "meta.json"
        with open(meta_path, "w", encoding="utf-8") as f:
            json.dump(meta.model_dump(), f, ensure_ascii=False, indent=2)

        samples_path = dataset_path / "samples.jsonl"
        samples_path.touch()

        return Dataset(meta=meta, samples=[], path=dataset_path)

    def add_sample(self, dataset_id: str, data: dict[str, Any]) -> bool:
        """向数据集添加样本。

        Args:
            dataset_id: 数据集 ID
            data: 样本数据

        Returns:
            是否添加成功
        """
        dataset = self.get_dataset(dataset_id)
        if dataset is None:
            return False

        samples_path = dataset.path / "samples.jsonl"
        with open(samples_path, "a", encoding="utf-8") as f:
            f.write(json.dumps(data, ensure_ascii=False) + "\n")

        # 清除缓存
        if dataset_id in self._cache:
            del self._cache[dataset_id]

        return True

    def import_from_jsonl(self, dataset_id: str, jsonl_path: Path | str) -> int:
        """从 JSONL 文件导入样本。

        Args:
            dataset_id: 数据集 ID
            jsonl_path: JSONL 文件路径

        Returns:
            导入的样本数量
        """
        dataset = self.get_dataset(dataset_id)
        if dataset is None:
            raise ValueError(f"Dataset not found: {dataset_id}")

        count = 0
        jsonl_path = Path(jsonl_path)
        samples_path = dataset.path / "samples.jsonl"

        with open(jsonl_path, encoding="utf-8") as src, \
             open(samples_path, "a", encoding="utf-8") as dst:
            for line in src:
                if line.strip():
                    dst.write(line)
                    count += 1

        # 清除缓存
        if dataset_id in self._cache:
            del self._cache[dataset_id]

        return count


# 全局实例
_dataset_manager: DatasetManager | None = None


def get_dataset_manager() -> DatasetManager:
    """获取数据集管理器单例。"""
    global _dataset_manager
    if _dataset_manager is None:
        _dataset_manager = DatasetManager()
    return _dataset_manager
