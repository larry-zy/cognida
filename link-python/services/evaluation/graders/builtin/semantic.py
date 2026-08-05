"""语义相似度评分器。

使用 sentence-transformers 计算文本之间的语义相似度。
"""

import asyncio
from typing import List, Optional

from ...graders.base import (
    BaseGrader,
    EvalType,
    GraderMode,
    GraderScore,
    MetricType,
)
from ...graders.registry import register_grader
from ...metrics.semantic import compute_semantic_similarities


@register_grader("semantic_similarity")
class SemanticSimilarityGrader(BaseGrader):
    """语义相似度评分器。

    使用 sentence-transformers 计算两段文本的语义相似度。
    """

    def __init__(self) -> None:
        super().__init__(
            name="semantic_similarity",
            mode=GraderMode.POINTWISE,
            description="语义相似度评分器 (0-100 分)",
            label="语义相似度",
            group="semantic",
            eval_types=[EvalType.LLM, EvalType.RAG],
            requires_reference=True,
        )
        self._model_cache = None

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        model_name: str = "paraphrase-multilingual-MiniLM-L12-v2",
        **kwargs
    ) -> GraderScore:
        """评估语义相似度。

        Args:
            response: 生成的文本
            reference: 参考文本
            model_name: 使用的模型名称
        """
        if not reference or not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少参考答案或生成答案",
                metrics={"similarity": 0.0},
            )

        try:
            # 复用进程级共享 SentenceTransformer（含 TF-IDF 降级），避免逐条评测重复加载模型；
            # 该同步函数会阻塞事件循环上的编码，放到线程池执行。
            #
            # 历史缺陷：此处误传 outputs=/model_name= 给只接受 (references, hypotheses) 的
            # compute_semantic_metrics_async，触发 TypeError 被下方 except 吞掉 → similarity 恒 0；
            # 且该便捷函数返回 dict，旧代码却按 result.similarity 取属性，即便签名对上也会再崩。
            loop = asyncio.get_event_loop()
            sims = await loop.run_in_executor(
                None, compute_semantic_similarities, [reference], [response]
            )
            similarity = max(0.0, min(1.0, sims[0] if sims else 0.0))
            score = similarity * 100

            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason=f"语义相似度: {similarity:.4f}",
                metrics={
                    "similarity": score,
                    "similarity_raw": similarity,
                },
            )
        except Exception as e:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason=f"语义相似度计算失败: {str(e)}",
                metrics={"similarity": 0.0},
            )


@register_grader("semantic_relevance")
class SemanticRelevanceGrader(BaseGrader):
    """语义相关性评分器。

    评估生成答案与问题的语义相关性。
    """

    def __init__(self) -> None:
        super().__init__(
            name="semantic_relevance",
            mode=GraderMode.POINTWISE,
            description="语义相关性评分器 (0-100 分)",
            label="语义相关性",
            group="semantic",
            eval_types=[EvalType.LLM, EvalType.RAG],
            requires_reference=False,
        )
        # 进程内模型缓存。历史缺陷：此 __init__ 未初始化 _model_cache，_aevaluate 里
        # `if self._model_cache is None` 直接抛 AttributeError 被 except 吞 → relevance 恒 0。
        self._model_cache = None

    async def _aevaluate(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        model_name: str = "paraphrase-multilingual-MiniLM-L12-v2",
        **kwargs
    ) -> GraderScore:
        """评估语义相关性。

        Args:
            query: 问题
            response: 生成的答案
            model_name: 使用的模型名称
        """
        if not query or not response:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason="缺少问题或答案",
                metrics={"relevance": 0.0},
            )

        try:
            from sentence_transformers import SentenceTransformer

            if self._model_cache is None:
                self._model_cache = SentenceTransformer(model_name)

            embeddings = self._model_cache.encode([query, response])
            similarity = (embeddings[0] @ embeddings[1].T).item()
            similarity = max(0, min(1, similarity))  # 限制在 [0, 1]
            score = similarity * 100

            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason=f"答案与问题的语义相关性: {similarity:.4f}",
                metrics={
                    "relevance": score,
                    "relevance_raw": similarity,
                },
            )
        except Exception as e:
            return GraderScore(
                name=self.name,
                metric_type=MetricType.PERCENTAGE,
                reason=f"语义相关性计算失败: {str(e)}",
                metrics={"relevance": 0.0},
            )
