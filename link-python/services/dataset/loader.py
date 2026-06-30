"""HuggingFace 数据集加载器。"""

from pathlib import Path
from typing import Any

from datasets import Dataset, DatasetDict, load_dataset


def load_hf_dataset(
    path: str,
    split: str | None = None,
    **kwargs: Any,
) -> Dataset | DatasetDict:
    """加载 HuggingFace 数据集到内存。

    Args:
        path: 数据集路径，如 "username/dataset-name"
        split: 数据集分割，如 "train"、"test"，默认加载全部
        **kwargs: 传递给 load_dataset 的其他参数

    Returns:
        Dataset 或 DatasetDict 对象

    Examples:
        >>> dataset = load_hf_dataset("imdb", split="train")
        >>> datasets = load_hf_dataset("ag_news")
    """
    return load_dataset(path, split=split, **kwargs)


def load_hf_dataset_to_disk(
    path: str,
    output_dir: str | Path,
    split: str | None = None,
    format: str = "parquet",  # noqa: A002
    **kwargs: Any,
) -> Path:
    """加载 HuggingFace 数据集并保存到磁盘。

    Args:
        path: 数据集路径，如 "username/dataset-name"
        output_dir: 输出目录
        split: 数据集分割，如 "train"、"test"，默认加载全部
        format: 保存格式，支持 "parquet"、"csv"、"json"、"arrow"
        **kwargs: 传递给 load_dataset 的其他参数

    Returns:
        保存的文件/目录路径

    Examples:
        >>> path = load_hf_dataset_to_disk("imdb", "./data/imdb", split="train")
        >>> path = load_hf_dataset_to_disk("ag_news", "./data/ag_news")
    """
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    dataset = load_dataset(path, split=split, **kwargs)

    if isinstance(dataset, Dataset):
        _save_single_dataset(dataset, output_path, format)
    elif isinstance(dataset, DatasetDict):
        for split_name, ds in dataset.items():
            split_path = output_path / split_name
            split_path.mkdir(exist_ok=True)
            _save_single_dataset(ds, split_path, format)

    return output_path


def _save_single_dataset(dataset: Dataset, output_path: Path, format: str) -> None:  # noqa: A002
    """保存单个数据集到磁盘。"""
    match format:
        case "parquet":
            dataset.to_parquet(output_path / "data.parquet")
        case "csv":
            dataset.to_csv(output_path / "data.csv")
        case "json":
            dataset.to_json(output_path / "data.json")
        case "arrow":
            dataset.save_to_disk(output_path)
        case _:
            raise ValueError(f"不支持的格式: {format}")
