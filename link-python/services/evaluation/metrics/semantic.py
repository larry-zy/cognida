"""语义相似度评测指标。"""

import asyncio
from typing import Any, List, Sequence

import numpy as np
from pydantic import BaseModel


class SemanticMetrics(BaseModel):
    """语义相似度评测指标。"""

    similarity: float  # 平均相似度
    min_similarity: float  # 最小相似度
    max_similarity: float  # 最大相似度

    model_config = {"arbitrary_types_allowed": True}

    @classmethod
    def compute(
        cls,
        references: List[str],
        hypotheses: List[str],
        model: "SentenceTransformer | None" = None,
    ) -> "SemanticMetrics":
        """计算语义相似度指标。

        Args:
            references: 参考文本列表
            hypotheses: 生成文本列表
            model: SentenceTransformer 模型（可选，如果为 None 则使用简单的 TF-IDF 余弦相似度）

        Returns:
            语义相似度指标结果
        """
        if len(references) != len(hypotheses):
            raise ValueError("references and hypotheses must have the same length")

        if not references:
            return cls(0.0, 0.0, 0.0)

        # 尝试使用 sentence-transformers
        try:
            from sentence_transformers import SentenceTransformer

            if model is None:
                model = SentenceTransformer("paraphrase-multilingual-MiniLM-L12-v2")

            # 编码文本
            ref_embeddings = model.encode(references, convert_to_numpy=True)
            hyp_embeddings = model.encode(hypotheses, convert_to_numpy=True)

            # 计算余弦相似度
            similarities = [
                float(np.dot(ref, hyp) / (np.linalg.norm(ref) * np.linalg.norm(hyp) + 1e-9))
                for ref, hyp in zip(ref_embeddings, hyp_embeddings)
            ]

            return cls(
                similarity=float(np.mean(similarities)),
                min_similarity=float(np.min(similarities)),
                max_similarity=float(np.max(similarities)),
            )

        except ImportError:
            # 降级到简单的 TF-IDF 余弦相似度
            from sklearn.feature_extraction.text import TfidfVectorizer
            from sklearn.metrics.pairwise import cosine_similarity

            vectorizer = TfidfVectorizer()
            all_texts = references + hypotheses
            tfidf_matrix = vectorizer.fit_transform(all_texts)

            ref_tfidf = tfidf_matrix[:len(references)]
            hyp_tfidf = tfidf_matrix[len(references):]

            similarities = cosine_similarity(ref_tfidf, hyp_tfidf).diagonal()

            return cls(
                similarity=float(np.mean(similarities)),
                min_similarity=float(np.min(similarities)),
                max_similarity=float(np.max(similarities)),
            )


class AsyncSemanticMetrics:
    """异步语义相似度评测指标。"""

    def __init__(self, model_name: str = "paraphrase-multilingual-MiniLM-L12-v2") -> None:
        self._model_name = model_name
        self._model = None

    async def _get_model(self) -> Any:
        """延迟加载模型。"""
        if self._model is None:
            from sentence_transformers import SentenceTransformer

            def load_model() -> Any:
                return SentenceTransformer(self._model_name)

            # 在线程池中加载模型
            loop = asyncio.get_event_loop()
            self._model = await loop.run_in_executor(None, load_model)
        return self._model

    async def compute(
        self,
        references: List[str],
        hypotheses: List[str],
    ) -> SemanticMetrics:
        """异步计算语义相似度。"""
        if len(references) != len(hypotheses):
            raise ValueError("references and hypotheses must have the same length")

        if not references:
            return SemanticMetrics(0.0, 0.0, 0.0)

        model = await self._get_model()

        # 在线程池中执行编码
        loop = asyncio.get_event_loop()

        def encode_texts(texts: list[str]) -> np.ndarray:
            return model.encode(texts, convert_to_numpy=True)

        ref_embeddings, hyp_embeddings = await asyncio.gather(
            loop.run_in_executor(None, encode_texts, references),
            loop.run_in_executor(None, encode_texts, hypotheses),
        )

        # 计算余弦相似度
        similarities = []
        for ref, hyp in zip(ref_embeddings, hyp_embeddings):
            sim = float(np.dot(ref, hyp) / (np.linalg.norm(ref) * np.linalg.norm(hyp) + 1e-9))
            similarities.append(sim)

        return SemanticMetrics(
            similarity=float(np.mean(similarities)),
            min_similarity=float(np.min(similarities)),
            max_similarity=float(np.max(similarities)),
        )


# 便捷函数
def compute_semantic_metrics(
    references: List[str],
    hypotheses: List[str],
) -> dict:
    """计算语义相似度指标（便捷函数）。"""
    metrics = SemanticMetrics.compute(references, hypotheses)
    return metrics.model_dump()


async def compute_semantic_metrics_async(
    references: List[str],
    hypotheses: List[str],
) -> dict:
    """异步计算语义相似度指标（便捷函数）。"""
    calculator = AsyncSemanticMetrics()
    metrics = await calculator.compute(references, hypotheses)
    return metrics.model_dump()
