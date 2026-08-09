"""HuggingFace 数据集加载示例。"""

from services.dataset import load_hf_dataset, load_hf_dataset_to_disk


def example_load_to_memory() -> None:
    """示例：加载数据集到内存。"""
    # 加载 IMDB 数据集的训练集
    dataset = load_hf_dataset("stanfordnlp/imdb", split="train")

    print(f"数据集大小: {len(dataset)}")
    print(f"数据列: {dataset.column_names}")
    print(f"第一条数据: {dataset[0]}")


def example_load_to_disk() -> None:
    """示例：加载数据集并保存到磁盘。"""
    # 加载 AG News 数据集并保存为 parquet 格式
    output_path = load_hf_dataset_to_disk(
        path="ag_news",
        output_dir="./data/ag_news",
        format="parquet",
    )

    print(f"数据集已保存到: {output_path}")


if __name__ == "__main__":
    # 加载到内存
    example_load_to_memory()

    # 加载并保存到磁盘
    example_load_to_disk()
