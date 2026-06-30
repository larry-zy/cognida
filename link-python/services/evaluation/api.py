"""评测服务 API。"""

import asyncio
from pathlib import Path
from typing import Any, List

from core import get_logger
from .datasets import DatasetManager
from .metrics import (
    compute_retrieval_metrics,
    compute_generation_metrics,
    compute_semantic_metrics,
)


class EvaluationService:
    """评测服务。"""

    def __init__(self, dataset_root: Path | str = "/d/link/datasets") -> None:
        """初始化评测服务。

        Args:
            dataset_root: 数据集根目录
        """
        self._dataset_root = Path(dataset_root)
        self._dataset_manager = DatasetManager(self._dataset_root)
        self._logger = get_logger(__name__)

    async def evaluate(
        self,
        dataset_id: str,
        results: List[dict[str, Any]],
        metrics: List[str] | None = None,
    ) -> dict[str, Any]:
        """执行评测。

        Args:
            dataset_id: 数据集 ID
            results: 模型输出列表，每项包含 sample_index 和 model_output
            metrics: 需要计算的指标列表

        Returns:
            评测结果
        """
        # 加载数据集
        dataset = self._dataset_manager.get_dataset(dataset_id)
        if dataset is None:
            raise ValueError(f"Dataset not found: {dataset_id}")

        # 构建样本映射
        sample_map = {s.index: s for s in dataset.samples}

        # 默认指标
        if metrics is None:
            metrics = ["rouge", "bleu"]

        # 收集评测数据
        references = []
        hypotheses = []
        sample_metrics = []

        for result in results:
            sample_index = result["sample_index"]
            model_output = result["model_output"]

            sample = sample_map.get(sample_index)
            if sample is None:
                self._logger.warning(f"Sample not found: {sample_index}")
                continue

            reference = sample.data.get("answer", "")
            question = sample.data.get("question", "")

            references.append(reference)
            hypotheses.append(model_output)

            # 计算单个样本的指标
            sample_metric = {
                "sample_index": sample_index,
                "question": question,
                "reference": reference,
                "model_output": model_output,
            }

            # 生成指标
            if "rouge" in metrics or "bleu" in metrics:
                gen_metrics = compute_generation_metrics(reference, model_output)
                sample_metric["generation"] = gen_metrics

            sample_metrics.append(sample_metric)

        # 计算聚合指标
        aggregated = {}

        if "rouge" in metrics and references and hypotheses:
            rouge_scores = [m.get("generation", {}).get("rouge_1", 0) for m in sample_metrics]
            aggregated["avg_rouge_1"] = sum(rouge_scores) / len(rouge_scores) if rouge_scores else 0

        if "bleu" in metrics and references and hypotheses:
            bleu_scores = [m.get("generation", {}).get("bleu_1", 0) for m in sample_metrics]
            aggregated["avg_bleu_1"] = sum(bleu_scores) / len(bleu_scores) if bleu_scores else 0

        # 语义相似度（异步）
        if "semantic" in metrics and references and hypotheses:
            try:
                sem_metrics = await compute_semantic_metrics(references, hypotheses)
                aggregated["semantic"] = sem_metrics
            except Exception as e:
                self._logger.warning(f"Semantic metrics failed: {e}")

        return {
            "dataset_id": dataset_id,
            "total_samples": len(dataset.samples),
            "evaluated_samples": len(sample_metrics),
            "metrics": aggregated,
            "samples": sample_metrics,
        }

    def list_datasets(self) -> List[dict[str, Any]]:
        """列出所有数据集。"""
        datasets = self._dataset_manager.list_datasets()
        return [
            {
                "dataset_id": d.dataset_id,
                "name": d.name,
                "description": d.description,
                "sample_count": d.sample_count,
                "fields": d.fields,
            }
            for d in datasets
        ]

    def get_dataset(self, dataset_id: str) -> dict[str, Any] | None:
        """获取数据集详情。"""
        dataset = self._dataset_manager.get_dataset(dataset_id)
        if dataset is None:
            return None

        return {
            "dataset_id": dataset.meta.dataset_id,
            "name": dataset.meta.name,
            "description": dataset.meta.description,
            "sample_count": dataset.sample_count,
            "fields": dataset.meta.fields,
            "metadata": dataset.meta.metadata,
        }


# 全局实例
_service: EvaluationService | None = None


def get_evaluation_service() -> EvaluationService:
    """获取评测服务单例。"""
    global _service
    if _service is None:
        _service = EvaluationService()
    return _service
