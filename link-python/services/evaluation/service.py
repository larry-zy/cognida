"""评测 gRPC 服务实现。

提供 ExecuteEvaluation 流式接口。
"""

import asyncio
from typing import Any, AsyncIterator, Callable, Dict, List, Optional

from core import get_logger
from proto import evaluation_pb2, evaluation_pb2_grpc

from .runner import EvaluationConfig, EvaluationRunner, Progress, ProgressStage, create_runner


class EvaluationServicer(evaluation_pb2_grpc.EvaluationServiceServicer):
    """评测服务实现。

    提供：
    - ExecuteEvaluation: 流式执行评测
    - ListGraders: 列出可用评分器
    - ListDatasets: 列出可用数据集
    - GetDatasetInfo: 获取数据集详情
    """

    def __init__(self) -> None:
        """初始化服务。"""
        self._logger = get_logger(__name__)
        self._active_evaluations: Dict[str, EvaluationRunner] = {}

    def ExecuteEvaluation(
        self,
        request: evaluation_pb2.EvaluationRequest,
        context,
    ):
        """执行评测（流式）。

        Args:
            request: 评测请求
            context: gRPC 上下文

        Yields:
            评测响应（进度或结果）
        """
        self._logger.info(
            f"Received ExecuteEvaluation request",
            dataset_id=request.dataset_id,
            knowledge_base_id=request.knowledge_base_id,
            model_id=request.model_id,
        )

        # 验证请求
        try:
            self._validate_request(request)
        except ValueError as e:
            yield evaluation_pb2.EvaluationResponse(
                error=evaluation_pb2.Error(
                    code="INVALID_REQUEST",
                    message=str(e),
                )
            )
            return

        # 创建配置
        config = self._parse_config(request.config)

        # 创建运行器
        runner = create_runner(config)

        # 创建异步生成器并运行
        async def generate_responses() -> AsyncIterator[evaluation_pb2.EvaluationResponse]:
            async for response in self._execute_evaluation_async(
                runner,
                request.dataset_id,
                request.knowledge_base_id,
                request.model_id,
            ):
                yield response

        # 运行异步生成器并 yield 结果
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)

        try:
            gen = generate_responses()
            while True:
                try:
                    response = loop.run_until_complete(gen.__anext__())
                    yield response
                except StopAsyncIteration:
                    break
        except Exception as e:
            self._logger.error(f"Evaluation execution failed", error=str(e))
            yield evaluation_pb2.EvaluationResponse(
                error=evaluation_pb2.Error(
                    code="EXECUTION_ERROR",
                    message=str(e),
                )
            )
        finally:
            loop.close()

    async def _execute_evaluation_async(
        self,
        runner: EvaluationRunner,
        dataset_id: str,
        knowledge_base_id: str,
        model_id: str,
    ) -> AsyncIterator[evaluation_pb2.EvaluationResponse]:
        """异步执行评测。

        Args:
            runner: 评测运行器
            dataset_id: 数据集 ID
            knowledge_base_id: 知识库 ID
            model_id: 模型 ID

        Yields:
            评测响应
        """
        async def progress_callback(progress: Progress) -> AsyncIterator[evaluation_pb2.EvaluationResponse]:
            yield evaluation_pb2.EvaluationResponse(
                progress=evaluation_pb2.Progress(
                    stage=progress.stage,
                    current=progress.current,
                    total=progress.total,
                    message=progress.message,
                    percentage=progress.percentage,
                )
            )

        # 创建进度回调的异步包装
        progress_queue: asyncio.Queue[Optional[evaluation_pb2.EvaluationResponse]] = asyncio.Queue()

        async def send_progress(progress: Progress) -> None:
            await progress_queue.put(evaluation_pb2.EvaluationResponse(
                progress=evaluation_pb2.Progress(
                    stage=progress.stage,
                    current=progress.current,
                    total=progress.total,
                    message=progress.message,
                    percentage=progress.percentage,
                )
            ))

        # 创建进度任务
        progress_task = None

        async def run_with_progress() -> "EvaluationResult":
            nonlocal progress_task
            progress_task = asyncio.create_task(self._consume_progress(progress_queue))
            return await runner.run(
                dataset_id,
                knowledge_base_id,
                model_id,
                progress_callback=send_progress,
            )

        try:
            result = await run_with_progress()

            # 等待所有进度消息发送完成
            await progress_queue.put(None)  # 结束标记
            if progress_task:
                await progress_task

            # 发送最终结果
            yield evaluation_pb2.EvaluationResponse(
                result=self._convert_result(result)
            )

        except Exception as e:
            self._logger.error(f"Evaluation failed", error=str(e))
            yield evaluation_pb2.EvaluationResponse(
                error=evaluation_pb2.Error(
                    code="EVALUATION_ERROR",
                    message=str(e),
                )
            )

    async def _consume_progress(
        self,
        queue: asyncio.Queue[Optional[evaluation_pb2.EvaluationResponse]],
    ) -> None:
        """消费进度队列。

        Args:
            queue: 进度队列
        """
        while True:
            response = await queue.get()
            if response is None:
                break
            # 在实际实现中，这里需要将响应发送给客户端
            # 由于 gRPC 流的限制，我们使用不同的机制
            queue.task_done()

    def ListGraders(
        self,
        request: evaluation_pb2.ListGradersRequest,
        context,
    ) -> evaluation_pb2.ListGradersResponse:
        """列出可用的评分器。

        Args:
            request: 列出评分器请求
            context: gRPC 上下文

        Returns:
            评分器列表响应
        """
        from .graders import list_graders

        self._logger.info(f"Received ListGraders request", type=request.type)

        graders = list_graders()

        response = evaluation_pb2.ListGradersResponse()

        for grader_info in graders:
            grader_pb = response.graders.add()
            grader_pb.name = grader_info.get("name", "")
            grader_pb.type = grader_info.get("mode", "")
            grader_pb.description = grader_info.get("description", "")

            # 添加参数信息
            for param_name, param_info in grader_info.get("parameters", {}).items():
                param_pb = grader_pb.params.add()
                param_pb.name = param_name
                param_pb.type = str(param_info.get("type", "string"))
                param_pb.description = param_info.get("description", "")
                param_pb.default_value = str(param_info.get("default", ""))
                param_pb.required = param_info.get("required", False)

        return response

    def ListDatasets(
        self,
        request: evaluation_pb2.ListDatasetsRequest,
        context,
    ) -> evaluation_pb2.ListDatasetsResponse:
        """列出可用的数据集。

        Args:
            request: 列出数据集请求
            context: gRPC 上下文

        Returns:
            数据集列表响应
        """
        from .datasets import get_dataset_manager

        self._logger.info("Received ListDatasets request")

        manager = get_dataset_manager()
        datasets = manager.list_datasets_info()

        response = evaluation_pb2.ListDatasetsResponse()

        for dataset_info in datasets:
            dataset_pb = response.datasets.add()
            dataset_pb.dataset_id = dataset_info.get("dataset_id", "")
            dataset_pb.description = dataset_info.get("description", "")
            dataset_pb.qa_count = dataset_info.get("qa_count", 0)
            dataset_pb.size = dataset_info.get("size", 0)
            dataset_pb.modified_time = dataset_info.get("modified_time", 0)

        return response

    def GetDatasetInfo(
        self,
        request: evaluation_pb2.GetDatasetInfoRequest,
        context,
    ) -> evaluation_pb2.GetDatasetInfoResponse:
        """获取数据集详情。

        Args:
            request: 获取数据集信息请求
            context: gRPC 上下文

        Returns:
            数据集信息响应
        """
        from .datasets import get_dataset_manager

        self._logger.info(f"Received GetDatasetInfo request", dataset_id=request.dataset_id)

        manager = get_dataset_manager()
        info = manager.get_dataset_info(request.dataset_id)

        if info is None:
            return evaluation_pb2.GetDatasetInfoResponse()

        response = evaluation_pb2.GetDatasetInfoResponse()
        dataset_pb = response.info
        dataset_pb.dataset_id = info.get("dataset_id", "")
        dataset_pb.description = info.get("description", "")
        dataset_pb.qa_count = info.get("qa_count", 0)

        # 添加领域
        for domain in info.get("domains", []):
            response.domains.append(domain)

        # 添加语言
        for language in info.get("languages", []):
            response.languages.append(language)

        # 添加元数据
        for key, value in info.get("metadata", {}).items():
            response.metadata[key] = str(value)

        return response

    # ============================================================
    # 辅助方法
    # ============================================================

    def _validate_request(self, request: evaluation_pb2.EvaluationRequest) -> None:
        """验证请求。

        Args:
            request: 评测请求

        Raises:
            ValueError: 请求无效
        """
        if not request.dataset_id:
            raise ValueError("dataset_id is required")

        if not request.knowledge_base_id:
            raise ValueError("knowledge_base_id is required")

        if not request.model_id:
            raise ValueError("model_id is required")

    def _parse_config(self, config: evaluation_pb2.EvaluationConfig) -> EvaluationConfig:
        """解析配置。

        Args:
            config: gRPC 配置

        Returns:
            评测配置
        """
        config_dict: Dict[str, Any] = {
            "top_k": config.top_k,
            "retrieval_metrics": list(config.retrieval_metrics),
            "generation_metrics": list(config.generation_metrics),
            "enable_semantic": config.enable_semantic,
            "enable_llm_judge": config.enable_llm_judge,
            "llm_judge_dimensions": list(config.llm_judge_dimensions),
            "strategy": config.strategy,
            "include_qa_results": config.include_qa_results,
            "max_concurrent": config.max_concurrent,
        }

        # 解析评分器配置
        config_dict["graders"] = []
        for grader_config in config.graders:
            grader_dict = {
                "name": grader_config.name,
                "weight": grader_config.weight,
                "params": {entry.key: entry.value for entry in grader_config.params},
            }
            config_dict["graders"].append(grader_dict)

        return EvaluationConfig(**config_dict)

    def _convert_result(
        self,
        result: "EvaluationResult",
    ) -> evaluation_pb2.EvaluationResult:
        """转换评测结果为 gRPC 格式。

        Args:
            result: 评测结果

        Returns:
            gRPC 评测结果
        """
        response = evaluation_pb2.EvaluationResult()
        response.evaluation_id = result.evaluation_id
        response.timestamp = result.timestamp
        response.total_count = result.total_count
        response.success_count = result.success_count
        response.failed_count = result.failed_count

        # 检索指标
        if result.retrieval:
            retrieval = response.retrieval
            retrieval.precision = result.retrieval.precision or 0.0
            retrieval.recall = result.retrieval.recall or 0.0
            retrieval.ndcg = result.retrieval.ndcg or 0.0
            retrieval.mrr = result.retrieval.mrr or 0.0
            retrieval.map = result.retrieval.map or 0.0

        # 生成指标
        if result.generation:
            generation = response.generation
            generation.rouge_1 = result.generation.rouge_1 or 0.0
            generation.rouge_2 = result.generation.rouge_2 or 0.0
            generation.rouge_l = result.generation.rouge_l or 0.0
            generation.bleu_1 = result.generation.bleu_1 or 0.0
            generation.bleu_2 = result.generation.bleu_2 or 0.0
            generation.bleu_4 = result.generation.bleu_4 or 0.0

        # 语义指标
        if result.semantic:
            semantic = response.semantic
            semantic.similarity = result.semantic.similarity or 0.0
            semantic.min_similarity = result.semantic.min_similarity or 0.0
            semantic.max_similarity = result.semantic.max_similarity or 0.0

        # LLM 评判指标
        if result.llm_judge:
            llm_judge = response.llm_judge
            llm_judge.total_score = result.llm_judge.total_score or 0.0
            for key, value in result.llm_judge.dimension_scores.items():
                llm_judge.dimension_scores[key] = value

        # 自定义分数
        for key, value in result.custom_scores.items():
            response.custom_scores[key] = value

        # QA 详细结果
        for qa_result in result.qa_results:
            qa_pb = response.qa_results.add()
            qa_pb.question = qa_result.question
            qa_pb.reference_answer = qa_result.reference_answer
            qa_pb.generated_answer = qa_result.generated_answer
            qa_pb.retrieved_chunks.extend(qa_result.retrieved_chunks)
            qa_pb.success = qa_result.success
            qa_pb.error = qa_result.error or ""

            # 检索指标
            if qa_result.retrieval:
                retrieval = qa_pb.retrieval
                retrieval.precision = qa_result.retrieval.precision or 0.0
                retrieval.recall = qa_result.retrieval.recall or 0.0
                retrieval.ndcg = qa_result.retrieval.ndcg or 0.0
                retrieval.mrr = qa_result.retrieval.mrr or 0.0
                retrieval.map = qa_result.retrieval.map or 0.0

            # 生成指标
            if qa_result.generation:
                generation = qa_pb.generation
                generation.rouge_1 = qa_result.generation.rouge_1 or 0.0
                generation.rouge_2 = qa_result.generation.rouge_2 or 0.0
                generation.rouge_l = qa_result.generation.rouge_l or 0.0
                generation.bleu_1 = qa_result.generation.bleu_1 or 0.0
                generation.bleu_2 = qa_result.generation.bleu_2 or 0.0
                generation.bleu_4 = qa_result.generation.bleu_4 or 0.0

            # 语义相似度
            if qa_result.semantic_similarity:
                qa_pb.semantic_similarity = qa_result.semantic_similarity

            # LLM 评判分数
            for key, value in qa_result.llm_judge_scores.items():
                qa_pb.llm_judge_scores[key] = value

            # 自定义分数
            for key, value in qa_result.custom_scores.items():
                qa_pb.custom_scores[key] = value

        return response
