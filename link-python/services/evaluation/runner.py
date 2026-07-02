"""评测运行器。

负责评测流程的编排和执行。
"""

import asyncio
import time
import uuid
from contextlib import asynccontextmanager
from typing import Any, AsyncIterator, Callable, Dict, List, Optional, Union

from pydantic import BaseModel, Field

from core import get_logger

from .datasets import Dataset, QAPair, get_dataset_manager
from .graders import BaseGrader, GraderError, GraderResult, GraderScore, GraderRank, get_grader
from .metrics import (
    AgentMetrics,
    GenerationMetrics,
    LLMJudgeMetrics,
    RetrievalMetrics,
    SemanticMetrics,
)


# ============================================================
# 进度报告模型
# ============================================================

class ProgressStage(str):
    """评测进度阶段。"""
    INIT = "init"
    LOADING_DATASET = "loading_dataset"
    RETRIEVAL = "retrieval"
    GENERATION = "generation"
    METRICS = "metrics"
    COMPLETE = "complete"
    ERROR = "error"


class Progress(BaseModel):
    """评测进度信息。"""
    stage: str = Field(..., description="当前阶段")
    current: int = Field(0, description="当前进度")
    total: int = Field(0, description="总数量")
    message: str = Field("", description="进度消息")
    percentage: float = Field(0.0, description="百分比")

    def to_dict(self) -> Dict[str, Any]:
        """转换为字典。"""
        return {
            "stage": self.stage,
            "current": self.current,
            "total": self.total,
            "message": self.message,
            "percentage": self.percentage,
        }


# ============================================================
# 评测配置
# ============================================================

class EvaluationConfig(BaseModel):
    """评测配置。"""
    top_k: int = Field(5, description="检索 top-k")
    retrieval_metrics: List[str] = Field(
        default_factory=lambda: ["precision", "recall", "ndcg", "mrr", "map"],
        description="检索指标列表"
    )
    generation_metrics: List[str] = Field(
        default_factory=lambda: ["rouge_1", "rouge_2", "rouge_l", "bleu_4"],
        description="生成指标列表"
    )
    enable_semantic: bool = Field(True, description="是否启用语义相似度")
    enable_llm_judge: bool = Field(False, description="是否启用 LLM 评判")
    llm_judge_dimensions: List[str] = Field(
        default_factory=lambda: ["accuracy", "completeness", "relevance"],
        description="LLM 评判维度"
    )
    graders: List[Dict[str, Any]] = Field(
        default_factory=list,
        description="自定义评分器配置"
    )
    strategy: str = Field("zero_shot", description="评测策略")
    include_qa_results: bool = Field(True, description="是否包含 QA 详细结果")
    max_concurrent: int = Field(10, description="最大并发数")


# ============================================================
# 评测结果模型
# ============================================================

class QAResult(BaseModel):
    """单个 QA 对的评测结果。"""
    question: str = Field(..., description="问题")
    reference_answer: str = Field(..., description="参考答案")
    generated_answer: str = Field(..., description="生成答案")
    retrieved_chunks: List[str] = Field(default_factory=list, description="检索到的文档块")
    retrieval: Optional[RetrievalMetrics] = Field(None, description="检索指标")
    generation: Optional[GenerationMetrics] = Field(None, description="生成指标")
    semantic_similarity: Optional[float] = Field(None, description="语义相似度")
    llm_judge_scores: Dict[str, float] = Field(default_factory=dict, description="LLM 评判分数")
    custom_scores: Dict[str, float] = Field(default_factory=dict, description="自定义评分器分数")
    success: bool = Field(True, description="是否成功")
    error: Optional[str] = Field(None, description="错误信息")


class EvaluationResult(BaseModel):
    """评测结果。"""
    evaluation_id: str = Field(..., description="评测 ID")
    timestamp: int = Field(..., description="时间戳")
    retrieval: Optional[RetrievalMetrics] = Field(None, description="聚合检索指标")
    generation: Optional[GenerationMetrics] = Field(None, description="聚合生成指标")
    semantic: Optional[SemanticMetrics] = Field(None, description="语义相似度指标")
    llm_judge: Optional[LLMJudgeMetrics] = Field(None, description="LLM 评判指标")
    custom_scores: Dict[str, float] = Field(default_factory=dict, description="自定义评分器分数")
    qa_results: List[QAResult] = Field(default_factory=list, description="QA 详细结果")
    total_count: int = Field(0, description="总数量")
    success_count: int = Field(0, description="成功数量")
    failed_count: int = Field(0, description="失败数量")


# ============================================================
# 评测运行器
# ============================================================

class EvaluationRunner:
    """评测运行器。

    负责：
    - 评测流程编排
    - 进度报告
    - 错误处理
    - 并发控制
    """

    def __init__(self, config: EvaluationConfig) -> None:
        """初始化运行器。

        Args:
            config: 评测配置
        """
        self.config = config
        self._logger = get_logger(__name__)
        self._evaluation_id = str(uuid.uuid4())
        self._dataset_manager = get_dataset_manager()

    @property
    def evaluation_id(self) -> str:
        """获取评测 ID。"""
        return self._evaluation_id

    async def run(
        self,
        dataset_id: str,
        knowledge_base_id: str,
        model_id: str,
        progress_callback: Optional[Callable[[Progress], None]] = None,
    ) -> EvaluationResult:
        """运行评测。

        Args:
            dataset_id: 数据集 ID
            knowledge_base_id: 知识库 ID
            model_id: 模型 ID
            progress_callback: 进度回调函数

        Returns:
            评测结果
        """
        start_time = time.time()
        self._logger.info(
            f"Starting evaluation",
            evaluation_id=self._evaluation_id,
            dataset_id=dataset_id,
            knowledge_base_id=knowledge_base_id,
            model_id=model_id,
        )

        try:
            return await self._run_evaluation(
                dataset_id,
                knowledge_base_id,
                model_id,
                progress_callback,
            )
        except Exception as e:
            self._logger.error(f"Evaluation failed", error=str(e))
            raise
        finally:
            duration = time.time() - start_time
            self._logger.info(
                f"Evaluation completed",
                evaluation_id=self._evaluation_id,
                duration=f"{duration:.2f}s",
            )

    async def _run_evaluation(
        self,
        dataset_id: str,
        knowledge_base_id: str,
        model_id: str,
        progress_callback: Optional[Callable[[Progress], None]],
    ) -> EvaluationResult:
        """执行评测。"""
        result = EvaluationResult(
            evaluation_id=self._evaluation_id,
            timestamp=int(time.time()),
        )

        # 阶段 1: 加载数据集
        await self._report_progress(
            ProgressStage.LOADING_DATASET,
            0, 1,
            "加载数据集",
            progress_callback,
        )

        dataset = self._dataset_manager.get_dataset(dataset_id)
        if dataset is None:
            dataset = self._dataset_manager.load_dataset(dataset_id)

        result.total_count = dataset.qa_count

        # 阶段 2: 执行评测
        qa_results = []

        for idx, qa_pair in enumerate(dataset.qa_pairs):
            await self._report_progress(
                ProgressStage.METRICS,
                idx + 1,
                dataset.qa_count,
                f"评测 {idx + 1}/{dataset.qa_count}: {qa_pair.question[:30]}...",
                progress_callback,
            )

            try:
                qa_result = await self._evaluate_qa_pair(
                    qa_pair,
                    knowledge_base_id,
                    model_id,
                )
                qa_results.append(qa_result)

                if qa_result.success:
                    result.success_count += 1
                else:
                    result.failed_count += 1

            except Exception as e:
                self._logger.error(
                    f"QA evaluation failed",
                    question=qa_pair.question,
                    error=str(e),
                )
                qa_results.append(QAResult(
                    question=qa_pair.question,
                    reference_answer=qa_pair.answer,
                    generated_answer="",
                    success=False,
                    error=str(e),
                ))
                result.failed_count += 1

        # 添加 QA 详细结果
        if self.config.include_qa_results:
            result.qa_results = qa_results

        # 阶段 3: 聚合指标
        await self._report_progress(
            ProgressStage.COMPLETE,
            1, 1,
            "计算聚合指标",
            progress_callback,
        )

        await self._aggregate_metrics(result, qa_results)

        return result

    async def _evaluate_qa_pair(
        self,
        qa_pair: QAPair,
        knowledge_base_id: str,
        model_id: str,
    ) -> QAResult:
        """评测单个 QA 对。

        Args:
            qa_pair: QA 对
            knowledge_base_id: 知识库 ID
            model_id: 模型 ID

        Returns:
            QA 评测结果
        """
        # 这里是占位符实现
        # 实际实现需要：
        # 1. 从知识库检索相关文档（通过 Go 服务）
        # 2. 使用模型生成答案
        # 3. 计算各种指标

        # 模拟检索和生成
        retrieved_chunks = [c.content for c in qa_pair.relevant_chunks]
        generated_answer = await self._generate_answer(
            qa_pair.question,
            retrieved_chunks,
            model_id,
        )

        # 计算检索指标
        retrieval_metrics = await self._compute_retrieval_metrics(qa_pair, retrieved_chunks)

        # 计算生成指标
        generation_metrics = await self._compute_generation_metrics(
            qa_pair.answer,
            generated_answer,
        )

        # 计算语义相似度
        semantic_similarity = None
        if self.config.enable_semantic:
            semantic_similarity = await self._compute_semantic_similarity(
                qa_pair.answer,
                generated_answer,
            )

        # LLM 评判
        llm_judge_scores = {}
        if self.config.enable_llm_judge:
            llm_judge_scores = await self._compute_llm_judge(
                qa_pair.question,
                generated_answer,
                qa_pair.answer,
                retrieved_chunks,
            )

        # 自定义评分器
        custom_scores = {}
        for grader_config in self.config.graders:
            grader_name = grader_config.get("name")
            if grader_name:
                score = await self._run_custom_grader(
                    grader_name,
                    grader_config,
                    qa_pair.question,
                    generated_answer,
                    qa_pair.answer,
                    retrieved_chunks,
                )
                if score is not None:
                    custom_scores[grader_name] = score

        return QAResult(
            question=qa_pair.question,
            reference_answer=qa_pair.answer,
            generated_answer=generated_answer,
            retrieved_chunks=retrieved_chunks,
            retrieval=retrieval_metrics,
            generation=generation_metrics,
            semantic_similarity=semantic_similarity,
            llm_judge_scores=llm_judge_scores,
            custom_scores=custom_scores,
            success=True,
        )

    async def _generate_answer(
        self,
        question: str,
        context: List[str],
        model_id: str,
    ) -> str:
        """使用 LLM 生成答案。

        Args:
            question: 问题
            context: 上下文文档
            model_id: 模型 ID

        Returns:
            生成的答案
        """
        from .llm.client import get_llm_client

        # 构建系统提示词
        system_prompt = """你是一个专业的问答助手。请根据提供的上下文文档准确回答用户的问题。

回答要求：
1. 仅根据提供的上下文文档回答问题
2. 如果上下文文档中没有相关信息，明确说明"根据提供的文档无法回答"
3. 保持回答简洁准确，避免冗余
4. 如果问题无法从上下文中推断出答案，不要编造信息"""

        # 构建带上下文的提示词
        if context:
            context_text = "\n\n".join([f"【文档 {i+1}】\n{doc}" for i, doc in enumerate(context)])
            prompt = f"""上下文文档：
{context_text}

问题：{question}

请根据上述上下文文档回答问题。"""
        else:
            prompt = f"问题：{question}\n\n请直接回答这个问题。"

        try:
            # 解析 model_id 获取 provider
            # 格式: "provider:model" 或直接 "model"
            provider = "openai"
            if ":" in model_id:
                provider, model_id = model_id.split(":", 1)

            client = get_llm_client(provider=provider, model=model_id)
            answer = await client.agenerate(prompt, system_prompt)
            return answer
        except Exception as e:
            self._logger.error(f"LLM generation failed", question=question, error=str(e))
            # LLM 调用失败时返回占位符
            return f"[LLM 生成失败: {str(e)}]"

    async def _compute_retrieval_metrics(
        self,
        qa_pair: QAPair,
        retrieved_chunks: List[str],
    ) -> Optional[RetrievalMetrics]:
        """计算检索指标。

        Args:
            qa_pair: QA 对
            retrieved_chunks: 检索到的文档块

        Returns:
            检索指标
        """
        from .metrics import compute_retrieval_metrics

        # 构建相关性标签
        relevant_ids = {c.chunk_id for c in qa_pair.relevant_chunks}
        retrieved_ids = [f"chunk_{i}" for i in range(len(retrieved_chunks))]

        # 简化的相关性判断
        retrieved_relevant = [
            1 if i < len(qa_pair.relevant_chunks) else 0
            for i in range(len(retrieved_chunks))
        ]

        if not retrieved_relevant:
            return None

        return compute_retrieval_metrics(
            retrieved_relevant,
            k=min(self.config.top_k, len(retrieved_relevant)),
        )

    async def _compute_generation_metrics(
        self,
        reference: str,
        generated: str,
    ) -> Optional[GenerationMetrics]:
        """计算生成指标。

        Args:
            reference: 参考答案
            generated: 生成答案

        Returns:
            生成指标
        """
        from .metrics import compute_generation_metrics

        return compute_generation_metrics(
            reference=reference,
            hypothesis=generated,
        )

    async def _compute_semantic_similarity(
        self,
        reference: str,
        generated: str,
    ) -> Optional[float]:
        """计算语义相似度。

        Args:
            reference: 参考答案
            generated: 生成答案

        Returns:
            相似度分数
        """
        from .metrics import compute_semantic_metrics_async

        try:
            result = await compute_semantic_metrics_async(
                references=[reference],
                hypotheses=[generated],
            )
            return result.get("similarity") if result else None
        except Exception as e:
            self._logger.warning(f"Semantic similarity computation failed", error=str(e))
            return None

    async def _compute_llm_judge(
        self,
        question: str,
        generated: str,
        reference: str,
        context: List[str],
    ) -> Dict[str, float]:
        """计算 LLM 评判分数。

        Args:
            question: 问题
            generated: 生成答案
            reference: 参考答案
            context: 上下文

        Returns:
            各维度分数
        """
        from .metrics import compute_llm_judge_metrics_async

        try:
            result = await compute_llm_judge_metrics_async(
                questions=[question],
                outputs=[generated],
                references=[reference],
                contexts=[context] if context else None,
                dimensions=self.config.llm_judge_dimensions,
            )
            return result.dimension_scores if result else {}
        except Exception as e:
            self._logger.warning(f"LLM judge computation failed", error=str(e))
            return {}

    async def _run_custom_grader(
        self,
        grader_name: str,
        grader_config: Dict[str, Any],
        query: str,
        response: str,
        reference: str,
        context: List[str],
    ) -> Optional[float]:
        """运行自定义评分器。

        Args:
            grader_name: 评分器名称
            grader_config: 评分器配置
            query: 问题
            response: 回答
            reference: 参考答案
            context: 上下文

        Returns:
            评分结果
        """
        grader = get_grader(grader_name)
        if grader is None:
            self._logger.warning(f"Grader not found", name=grader_name)
            return None

        try:
            result = await grader.aevaluate(
                query=query,
                response=response,
                reference=reference,
                context=context,
                **grader_config.get("params", {}),
            )

            if isinstance(result, GraderError):
                self._logger.warning(f"Grader evaluation failed", name=grader_name, error=result.error)
                return None

            if isinstance(result, GraderScore):
                return result.score

            return None
        except Exception as e:
            self._logger.warning(f"Grader execution failed", name=grader_name, error=str(e))
            return None

    async def _aggregate_metrics(
        self,
        result: EvaluationResult,
        qa_results: List[QAResult],
    ) -> None:
        """聚合指标。

        Args:
            result: 评测结果
            qa_results: QA 结果列表
        """
        # 聚合检索指标
        retrieval_metrics = [r.retrieval for r in qa_results if r.retrieval]
        if retrieval_metrics:
            result.retrieval = self._average_retrieval_metrics(retrieval_metrics)

        # 聚合生成指标
        generation_metrics = [r.generation for r in qa_results if r.generation]
        if generation_metrics:
            result.generation = self._average_generation_metrics(generation_metrics)

        # 聚合语义相似度
        semantic_similarities = [r.semantic_similarity for r in qa_results if r.semantic_similarity]
        if semantic_similarities:
            avg_similarity = sum(semantic_similarities) / len(semantic_similarities)
            result.semantic = SemanticMetrics(
                similarity=avg_similarity,
                min_similarity=min(semantic_similarities),
                max_similarity=max(semantic_similarities),
            )

        # 聚合 LLM 评判分数
        llm_scores = {}
        for r in qa_results:
            for dim, score in r.llm_judge_scores.items():
                if dim not in llm_scores:
                    llm_scores[dim] = []
                llm_scores[dim].append(score)

        if llm_scores:
            dimension_scores = {k: sum(v) / len(v) for k, v in llm_scores.items()}
            total_score = sum(dimension_scores.values()) / len(dimension_scores) if dimension_scores else 0
            result.llm_judge = LLMJudgeMetrics(
                total_score=total_score,
                dimension_scores=dimension_scores,
            )

        # 聚合自定义评分器分数
        custom_scores = {}
        for r in qa_results:
            for name, score in r.custom_scores.items():
                if name not in custom_scores:
                    custom_scores[name] = []
                custom_scores[name].append(score)

        result.custom_scores = {
            k: sum(v) / len(v) for k, v in custom_scores.items()
        }

    def _average_retrieval_metrics(
        self,
        metrics: List[RetrievalMetrics],
    ) -> RetrievalMetrics:
        """平均检索指标。"""
        count = len(metrics)
        return RetrievalMetrics(
            precision=sum(m.precision for m in metrics if m.precision) / count,
            recall=sum(m.recall for m in metrics if m.recall) / count,
            ndcg=sum(m.ndcg for m in metrics if m.ndcg) / count,
            mrr=sum(m.mrr for m in metrics if m.mrr) / count,
            map_score=sum(m.map_score for m in metrics if m.map_score) / count,
        )

    def _average_generation_metrics(
        self,
        metrics: List[GenerationMetrics],
    ) -> GenerationMetrics:
        """平均生成指标。"""
        count = len(metrics)
        return GenerationMetrics(
            rouge_1=sum(m.rouge_1 for m in metrics if m.rouge_1) / count,
            rouge_2=sum(m.rouge_2 for m in metrics if m.rouge_2) / count,
            rouge_l=sum(m.rouge_l for m in metrics if m.rouge_l) / count,
            bleu_1=sum(m.bleu_1 for m in metrics if m.bleu_1) / count,
            bleu_2=sum(m.bleu_2 for m in metrics if m.bleu_2) / count,
            bleu_4=sum(m.bleu_4 for m in metrics if m.bleu_4) / count,
        )

    async def _report_progress(
        self,
        stage: str,
        current: int,
        total: int,
        message: str,
        callback: Optional[Callable[[Progress], None]],
    ) -> None:
        """报告进度。"""
        if callback is None:
            return

        percentage = (current / total * 100) if total > 0 else 0

        progress = Progress(
            stage=stage,
            current=current,
            total=total,
            message=message,
            percentage=percentage,
        )

        try:
            if asyncio.iscoroutinefunction(callback):
                await callback(progress)
            else:
                callback(progress)
        except Exception as e:
            self._logger.warning(f"Progress callback failed", error=str(e))


def create_runner(config: EvaluationConfig) -> EvaluationRunner:
    """创建评测运行器。

    Args:
        config: 评测配置

    Returns:
        评测运行器
    """
    return EvaluationRunner(config)
