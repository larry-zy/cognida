"""评测服务 FastAPI 接口。"""

import asyncio
import os
from pathlib import Path
from typing import Any, List

from dotenv import load_dotenv
from fastapi import FastAPI, HTTPException, status
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel

# 加载环境变量（支持不同工作目录）
possible_paths = [
    Path(__file__).parent.parent.parent / ".env",  # 从服务文件向上查找
    Path.cwd() / ".env",                             # 当前工作目录
    Path("D:/link/link-python") / ".env",            # 绝对路径
]
for env_path in possible_paths:
    if env_path.exists():
        load_dotenv(env_path)
        break

from services.evaluation.models import (
    # 请求模型
    EvaluateRequest,
    LLMEvaluateRequest,
    AgentEvaluateRequest,
    RAGEvaluateRequest,
    # 响应模型
    EvaluateResponse,
    LLMEvaluateResponse,
    AgentEvaluateResponse,
    RAGEvaluateResponse,
    HealthResponse,
)
from services.evaluation.metrics import (
    # 通用指标
    compute_generation_metrics,
    compute_semantic_metrics,
    compute_llm_judge_metrics,
    # Agent 指标
    compute_agent_metrics,
    # RAG 指标
    compute_rag_metrics,
    compute_retrieval_metrics,
    faithfulness,
    context_relevance,
    noise_ratio,
)

# 创建 FastAPI 应用
app = FastAPI(
    title="Evaluation Service",
    description="LLM/Agent/RAG 评测服务",
    version="1.0.0",
)

# CORS 中间件
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


# ============================================================
# 健康检查
# ============================================================

@app.get("/health", response_model=HealthResponse)
async def health_check() -> HealthResponse:
    """健康检查。"""
    return HealthResponse(status="healthy", version="1.0.0")


# ============================================================
# 通用评测接口
# ============================================================

@app.post("/api/evaluate", response_model=EvaluateResponse)
async def evaluate(request: EvaluateRequest) -> EvaluateResponse:
    """通用评测接口（根据 mode 分发）。"""
    try:
        if request.mode == "llm":
            # 合并 data, metrics, llm_judge_config
            llm_data = {**request.data, "metrics": request.metrics, "llm_judge_config": request.llm_judge_config}
            llm_req = LLMEvaluateRequest(**llm_data)
            llm_resp = await evaluate_llm(llm_req)
            return EvaluateResponse(
                mode="llm",
                result=llm_resp.model_dump(exclude_none=True),
            )
        elif request.mode == "agent":
            agent_data = {**request.data, "metrics": request.metrics, "llm_judge_config": request.llm_judge_config}
            agent_req = AgentEvaluateRequest(**agent_data)
            agent_resp = await evaluate_agent(agent_req)
            return EvaluateResponse(
                mode="agent",
                result=agent_resp.model_dump(exclude_none=True),
            )
        elif request.mode == "rag":
            rag_data = {**request.data, "metrics": request.metrics, "llm_judge_config": request.llm_judge_config}
            rag_req = RAGEvaluateRequest(**rag_data)
            rag_resp = await evaluate_rag(rag_req)
            return EvaluateResponse(
                mode="rag",
                result=rag_resp.model_dump(exclude_none=True),
            )
        else:
            raise HTTPException(
                status_code=status.HTTP_400_BAD_REQUEST,
                detail=f"Unsupported mode: {request.mode}",
            )
    except Exception as e:
        return EvaluateResponse(
            mode=request.mode,
            result={},
            error=str(e),
        )


# ============================================================
# LLM 评测接口
# ============================================================

@app.post("/api/evaluate/llm", response_model=LLMEvaluateResponse)
async def evaluate_llm(request: LLMEvaluateRequest) -> LLMEvaluateResponse:
    """LLM 评测。"""
    try:
        result: dict[str, Any] = {}
        metrics_to_compute = request.metrics

        # 生成指标 (ROUGE, BLEU)
        if any(m in metrics_to_compute for m in ["rouge", "bleu"]):
            rouge_scores = {}
            bleu_scores = {}

            for ref, out in zip(request.references, request.outputs):
                gen = compute_generation_metrics(ref, out)
                for k, v in gen.items():
                    if k.startswith("rouge"):
                        rouge_scores[k] = rouge_scores.get(k, 0) + v
                    elif k.startswith("bleu"):
                        bleu_scores[k] = bleu_scores.get(k, 0) + v

            # 平均
            n = len(request.references)
            result["rouge"] = {k: v / n for k, v in rouge_scores.items()} if rouge_scores else None
            result["bleu"] = {k: v / n for k, v in bleu_scores.items()} if bleu_scores else None

        # 语义指标
        if "semantic" in metrics_to_compute:
            sem = compute_semantic_metrics(request.references, request.outputs)
            result["semantic"] = sem

        # LLM-as-Judge
        if "llm_judge" in metrics_to_compute and request.llm_judge_config:
            questions = request.llm_judge_config.questions
            dimensions = request.llm_judge_config.dimensions

            judge_scores = {"total_score": [], "dimension_scores": {}}
            for dim in dimensions:
                judge_scores["dimension_scores"][dim] = []

            for ref, out, q in zip(
                request.references,
                request.outputs,
                questions if len(questions) == len(request.outputs) else [None] * len(request.outputs),
            ):
                score = compute_llm_judge_metrics(
                    reference=ref,
                    hypothesis=out,
                    question=q or "",
                    dimensions=dimensions,
                )
                judge_scores["total_score"].append(score.get("total_score", 0))
                for dim, dim_score in score.get("dimension_scores", {}).items():
                    if dim in judge_scores["dimension_scores"]:
                        judge_scores["dimension_scores"][dim].append(dim_score)

            # 平均
            avg_dimension_scores = {
                dim: sum(scores) / len(scores) if scores else 0
                for dim, scores in judge_scores["dimension_scores"].items()
            }
            result["llm_judge"] = {
                "total_score": sum(judge_scores["total_score"]) / len(judge_scores["total_score"]) if judge_scores["total_score"] else 0,
                **avg_dimension_scores,
            }

        return LLMEvaluateResponse(**result)

    except Exception as e:
        return LLMEvaluateResponse(error=str(e))


# ============================================================
# Agent 评测接口
# ============================================================

@app.post("/api/evaluate/agent", response_model=AgentEvaluateResponse)
async def evaluate_agent(request: AgentEvaluateRequest) -> AgentEvaluateResponse:
    """Agent 评测。"""
    try:
        # 转换为字典格式
        references = [r.model_dump() for r in request.references]
        outputs = [o.model_dump() for o in request.outputs]

        # 计算 Agent 指标
        result = compute_agent_metrics(
            references=references,
            outputs=outputs,
            metrics=request.metrics,
        )

        # LLM-as-Judge
        llm_judge = None
        if "llm_judge" in request.metrics and request.llm_judge_config:
            dimensions = request.llm_judge_config.dimensions
            questions = request.llm_judge_config.questions

            judge_scores = {"total_score": [], "dimension_scores": {}}
            for dim in dimensions:
                judge_scores["dimension_scores"][dim] = []

            for ref, out, q in zip(
                references,
                outputs,
                questions if len(questions) == len(outputs) else [None] * len(outputs),
            ):
                score = compute_llm_judge_metrics(
                    reference=ref.get("final_answer", ""),
                    hypothesis=out.get("final_answer", ""),
                    question=q or "",
                    dimensions=dimensions,
                )
                judge_scores["total_score"].append(score.get("total_score", 0))
                for dim, dim_score in score.get("dimension_scores", {}).items():
                    if dim in judge_scores["dimension_scores"]:
                        judge_scores["dimension_scores"][dim].append(dim_score)

            avg_dimension_scores = {
                dim: sum(scores) / len(scores) if scores else 0
                for dim, scores in judge_scores["dimension_scores"].items()
            }
            llm_judge = {
                "total_score": sum(judge_scores["total_score"]) / len(judge_scores["total_score"]) if judge_scores["total_score"] else 0,
                **avg_dimension_scores,
            }

        return AgentEvaluateResponse(
            answer_accuracy=result.get("answer_accuracy"),
            tool_selection=result.get("tool_selection"),
            trajectory_match=result.get("trajectory_match"),
            step_efficiency=result.get("step_efficiency"),
            llm_judge=llm_judge,
        )

    except Exception as e:
        return AgentEvaluateResponse(error=str(e))


# ============================================================
# RAG 评测接口
# ============================================================

@app.post("/api/evaluate/rag", response_model=RAGEvaluateResponse)
async def evaluate_rag(request: RAGEvaluateRequest) -> RAGEvaluateResponse:
    """RAG 评测。"""
    try:
        # 转换为字典格式
        questions = request.questions
        references = [r.model_dump() for r in request.references]
        outputs = [o.model_dump() for o in request.outputs]
        corpus = [c.model_dump() for c in request.corpus] if request.corpus else None

        # 计算 RAG 指标
        result = compute_rag_metrics(
            questions=questions,
            references=references,
            outputs=outputs,
            corpus=corpus,
            metrics=request.metrics,
        )

        # 提取各类指标
        answer_quality = result.get("answer_quality")
        retrieval = result.get("retrieval")
        rag_specific = result.get("rag_specific")

        # LLM-as-Judge
        llm_judge = None
        if "llm_judge" in request.metrics and request.llm_judge_config:
            dimensions = request.llm_judge_config.dimensions

            judge_scores = {"total_score": [], "dimension_scores": {}}
            for dim in dimensions:
                judge_scores["dimension_scores"][dim] = []

            for q, ref, out in zip(questions, references, outputs):
                score = compute_llm_judge_metrics(
                    reference=ref.get("answer", ""),
                    hypothesis=out.get("answer", ""),
                    question=q,
                    dimensions=dimensions,
                )
                judge_scores["total_score"].append(score.get("total_score", 0))
                for dim, dim_score in score.get("dimension_scores", {}).items():
                    if dim in judge_scores["dimension_scores"]:
                        judge_scores["dimension_scores"][dim].append(dim_score)

            avg_dimension_scores = {
                dim: sum(scores) / len(scores) if scores else 0
                for dim, scores in judge_scores["dimension_scores"].items()
            }
            llm_judge = {
                "total_score": sum(judge_scores["total_score"]) / len(judge_scores["total_score"]) if judge_scores["total_score"] else 0,
                **avg_dimension_scores,
            }

        return RAGEvaluateResponse(
            answer_quality=answer_quality,
            retrieval=retrieval,
            rag_specific=rag_specific,
            llm_judge=llm_judge,
        )

    except Exception as e:
        return RAGEvaluateResponse(error=str(e))


# ============================================================
# 批量指标计算接口 (Go Worker 调用)
# ============================================================

class ComputeItem(BaseModel):
    """单项计算请求。"""
    question: str
    reference_answer: str
    generated_answer: str
    retrieved_pids: List[str] = []
    relevant_pids: List[str] = []


class ComputeMetricsRequest(BaseModel):
    """批量指标计算请求。"""
    items: List[ComputeItem]
    graders: List[str] = ["rouge", "bleu"]
    llm_judge: dict[str, Any] = {}
    reference: dict[str, Any] = {}


class ComputeItemResult(BaseModel):
    """单项计算结果。"""
    index: int

    # 检索指标
    precision: float | None = None
    recall: float | None = None
    ndcg: float | None = None
    rr: float | None = None

    # 生成指标
    rouge_1: float | None = None
    rouge_2: float | None = None
    rouge_l: float | None = None
    bleu_1: float | None = None
    bleu_2: float | None = None
    bleu_4: float | None = None

    # LLM Judge
    llm_score: float | None = None
    llm_reasoning: str | None = None

    # 语义相似度
    semantic_similarity: float | None = None


class ComputeMetricsResponse(BaseModel):
    """批量指标计算响应。"""
    items: List[ComputeItemResult]
    aggregate: dict[str, float]


@app.post("/api/v1/evaluation/compute-metrics", response_model=ComputeMetricsResponse)
async def compute_metrics(request: ComputeMetricsRequest) -> ComputeMetricsResponse:
    """批量计算评测指标（供 Go Worker 调用）。"""
    try:
        items_result: List[ComputeItemResult] = []
        aggregate: dict[str, float] = {}

        # 准备聚合指标累加器
        rouge_1_sum = 0.0
        rouge_2_sum = 0.0
        rouge_l_sum = 0.0
        bleu_1_sum = 0.0
        bleu_2_sum = 0.0
        bleu_4_sum = 0.0
        count = 0

        for i, item in enumerate(request.items):
            item_result = ComputeItemResult(index=i)

            # 生成指标 (ROUGE, BLEU)
            if any(g in request.graders for g in ["rouge", "bleu"]):
                gen = compute_generation_metrics(
                    item.reference_answer,
                    item.generated_answer,
                )

                # ROUGE
                if "rouge_1" in gen:
                    item_result.rouge_1 = gen["rouge_1"]
                    rouge_1_sum += gen["rouge_1"]
                if "rouge_2" in gen:
                    item_result.rouge_2 = gen["rouge_2"]
                    rouge_2_sum += gen["rouge_2"]
                if "rouge_l" in gen:
                    item_result.rouge_l = gen["rouge_l"]
                    rouge_l_sum += gen["rouge_l"]

                # BLEU
                if "bleu_1" in gen:
                    item_result.bleu_1 = gen["bleu_1"]
                    bleu_1_sum += gen["bleu_1"]
                if "bleu_2" in gen:
                    item_result.bleu_2 = gen["bleu_2"]
                    bleu_2_sum += gen["bleu_2"]
                if "bleu_4" in gen:
                    item_result.bleu_4 = gen["bleu_4"]
                    bleu_4_sum += gen["bleu_4"]

            # 检索指标（如果有相关文档信息）
            if item.relevant_pids and item.retrieved_pids:
                if any(g in request.graders for g in ["precision", "recall", "ndcg"]):
                    ret = compute_retrieval_metrics(
                        relevant_pids=item.relevant_pids,
                        retrieved_pids=item.retrieved_pids,
                    )
                    item_result.precision = ret.get("precision")
                    item_result.recall = ret.get("recall")
                    item_result.ndcg = ret.get("ndcg")
                    item_result.rr = ret.get("rr")

            # 语义相似度
            if "semantic" in request.graders:
                try:
                    sem = await compute_semantic_metrics(
                        [item.reference_answer],
                        [item.generated_answer],
                    )
                    if sem and "similarity" in sem:
                        item_result.semantic_similarity = sem["similarity"][0] if isinstance(sem["similarity"], list) else sem["similarity"]
                except Exception as e:
                    pass  # 语义指标失败不影响其他指标

            # LLM Judge
            if "llm_judge" in request.graders and request.llm_judge:
                try:
                    score = compute_llm_judge_metrics(
                        reference=item.reference_answer,
                        hypothesis=item.generated_answer,
                        question=item.question,
                        dimensions=request.llm_judge.get("dimensions", ["relevance", "accuracy"]),
                    )
                    item_result.llm_score = score.get("total_score")
                    item_result.llm_reasoning = score.get("reasoning")
                except Exception as e:
                    pass  # LLM Judge 失败不影响其他指标

            items_result.append(item_result)
            count += 1

        # 计算聚合指标
        if count > 0:
            if rouge_1_sum > 0:
                aggregate["rouge_1"] = rouge_1_sum / count
            if rouge_2_sum > 0:
                aggregate["rouge_2"] = rouge_2_sum / count
            if rouge_l_sum > 0:
                aggregate["rouge_l"] = rouge_l_sum / count
            if bleu_1_sum > 0:
                aggregate["bleu_1"] = bleu_1_sum / count
            if bleu_2_sum > 0:
                aggregate["bleu_2"] = bleu_2_sum / count
            if bleu_4_sum > 0:
                aggregate["bleu_4"] = bleu_4_sum / count

        return ComputeMetricsResponse(
            items=items_result,
            aggregate=aggregate,
        )

    except Exception as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Metrics computation failed: {str(e)}",
        )


# ============================================================
# 启动入口
# ============================================================

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=18888)
