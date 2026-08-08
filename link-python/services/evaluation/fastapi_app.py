"""评测服务 FastAPI 接口。"""

import logging
from pathlib import Path
from typing import Any, List, Optional

from dotenv import load_dotenv
from fastapi import FastAPI, HTTPException, status
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel

logger = logging.getLogger(__name__)

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
    compute_semantic_similarities,
    compute_llm_judge_metrics,
    compute_llm_judge_metrics_async,
    # Agent 指标
    compute_agent_metrics,
    # SQL 指标
    compute_sql_metrics,
    # RAG 指标
    compute_rag_metrics,
    compute_retrieval_metrics,
    faithfulness,
    context_relevance,
    noise_ratio,
)
from services.evaluation.graders.base import GraderScore, normalize_eval_type
from services.evaluation.graders.registry import get_global_registry


# 评分器注册表(懒加载:首次使用时发现并注册全部 grader)
_registry_ready = False


def _ensure_registry():
    """确保全局评分器注册表已初始化(幂等)。

    单个 grader 加载失败由 registry.discover_* 内部记录 warning 并跳过；
    initialize 整体失败必须记录日志且不固化 ready——否则注册表为空却
    永远显示"就绪"，下次调用也不会重试。
    """
    global _registry_ready
    registry = get_global_registry()
    if not _registry_ready:
        try:
            registry.initialize()
            _registry_ready = True
        except Exception as e:
            from core import get_logger
            get_logger(__name__).error(
                "Grader registry initialization failed",
                error=str(e),
            )
    return registry

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

# 请求 ID 透传（最外层）：从 Go 传入的 X-Request-ID 绑定到日志上下文并回写响应头
from core.request_context import RequestIDMiddleware  # noqa: E402

app.add_middleware(RequestIDMiddleware)


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
                try:
                    score = compute_llm_judge_metrics(
                        reference=ref,
                        hypothesis=out,
                        question=q or "",
                        dimensions=dimensions,
                    )
                except Exception:
                    continue  # 单条裁判失败不拖垮整批（与批量端点口径一致）
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
                try:
                    score = compute_llm_judge_metrics(
                        reference=ref.get("final_answer", ""),
                        hypothesis=out.get("final_answer", ""),
                        question=q or "",
                        dimensions=dimensions,
                    )
                except Exception:
                    continue  # 单条裁判失败不拖垮整批（与批量端点口径一致）
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
                try:
                    score = compute_llm_judge_metrics(
                        reference=ref.get("answer", ""),
                        hypothesis=out.get("answer", ""),
                        question=q,
                        dimensions=dimensions,
                    )
                except Exception:
                    continue  # 单条裁判失败不拖垮整批（与批量端点口径一致）
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
    retrieved_pids: List[str] = []       # 检索到的分块ID（用于 precision/recall/ndcg/mrr）
    relevant_pids: List[str] = []        # 标注的相关分块ID
    retrieved_contexts: List[str] = []   # 检索到的分块正文（用于 faithfulness/context_relevance）
    # Agent 评测字段：references(expected_*) + outputs(tools_used/trajectory/total_steps)
    expected_tools: List[str] = []       # 期望调用的工具名（tool_selection/tool_order）
    expected_steps: List[str] = []       # 期望步骤序列（trajectory_match/step_efficiency）
    tools_used: List[str] = []           # 实际调用的工具名（按调用顺序）
    trajectory: List[str] = []           # 实际步骤序列
    total_steps: int = 0                 # 步骤总数
    # Text2SQL 评测字段：金标准/生成 SQL 文本 + 双方只读执行结果集（Go 侧采集）
    gold_sql: str = ""                   # 金标准 SQL（sql_exact_match/sql_component_match）
    generated_sql: str = ""              # Agent 生成的 SQL
    # 结果集默认 None（而非 []）：区分「未执行/执行失败」(None → sql_execution_accuracy 剔除出
    # 分母) 与「执行成功但零行」([] → 参与比对，空==空为合法正确答案)。默认 [] 会把未发送结果集的
    # 未执行样本误判成空==空满分（#1）。Go 侧对应字段去掉 omitempty，nil→null→此处 None。
    result_set: Optional[List[Any]] = None       # 生成 SQL 的结果集（行数组，sql_execution_accuracy）
    gold_result_set: Optional[List[Any]] = None  # 金标准 SQL 的结果集（行数组）


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

    # 动态指标载体:注册表驱动的 name->value(与固定字段并存以兼容)
    scores: dict[str, float] = {}


class ComputeMetricsResponse(BaseModel):
    """批量指标计算响应。"""
    items: List[ComputeItemResult]
    aggregate: dict[str, float]          # 聚合结果本身即动态 scores map(name->value)
    unsupported: List[str] = []          # 请求了但注册表中无对应 grader 的指标名


# 走专属批量计算的评分器名(其余已注册指标走通用 grader 执行路径)。
# bleu_2 与 bleu_1/bleu_4 一样由 compute_generation_metrics 产出并镜像进 scores，
# 需一并列入白名单，否则显式请求 bleu_2 会被误判为 unsupported（C3）。
_GENERATION_GRADERS = {"rouge", "rouge_1", "rouge_2", "rouge_l", "bleu", "bleu_1", "bleu_2", "bleu_4"}
_RETRIEVAL_GRADERS = {"precision", "recall", "ndcg", "mrr", "map"}
_SEMANTIC_GRADERS = {"semantic", "semantic_similarity", "semantic_relevance"}
_RAG_GRADERS = {"faithfulness", "context_relevance", "noise_ratio"}
# Agent 家族评分器：走 compute_agent_metrics 专属批量算子（未注册进通用 registry，
# 由本端点按 references/outputs 组装后计算，避免落入 unsupported）
_AGENT_GRADERS = {"answer_accuracy", "tool_selection", "tool_order", "trajectory_match", "step_efficiency"}
# SQL 家族评分器：走 compute_sql_metrics 专属批量算子（结果集无序比对需跨字段协同）。
_SQL_GRADERS = {"sql_exact_match", "sql_component_match", "sql_execution_accuracy"}
# 每项固定字段 -> 动态 scores map 的键映射(用于把固定字段镜像进 scores)
_FIXED_SCORE_KEYS = [
    "precision", "recall", "ndcg", "rr",
    "rouge_1", "rouge_2", "rouge_l",
    "bleu_1", "bleu_2", "bleu_4",
    "semantic_similarity", "llm_score",
]


@app.post("/api/v1/evaluation/compute-metrics", response_model=ComputeMetricsResponse)
async def compute_metrics(request: ComputeMetricsRequest) -> ComputeMetricsResponse:
    """批量计算评测指标（供 Go Worker 调用）。

    注册表驱动:遍历 request.graders 从注册表解析,已注册但无专属批量算子的指标
    走通用 grader 执行路径;未注册的指标名收集到 unsupported 而非静默忽略。
    结果以动态 scores map 承载,同时保留既有固定字段以向后兼容。
    """
    try:
        registry = _ensure_registry()

        # —— 注册表解析:区分 supported / agent / unsupported ——
        requested = list(dict.fromkeys(request.graders))  # 去重保序
        supported_names: set[str] = set()
        agent_requested: set[str] = set()
        sql_requested: set[str] = set()
        unsupported: List[str] = []
        for name in requested:
            if name in _AGENT_GRADERS:
                agent_requested.add(name)  # Agent 家族走专属 compute_agent_metrics 路径
            elif name in _SQL_GRADERS:
                sql_requested.add(name)  # SQL 家族走专属 compute_sql_metrics 路径
            elif registry.exists(name):
                supported_names.add(name)
            else:
                unsupported.append(name)

        items_result: List[ComputeItemResult] = []
        aggregate: dict[str, float] = {}

        # 专属批量算子已处理的指标名(其余 supported 交由通用路径)
        handled: set[str] = set()

        # 准备聚合指标累加器
        rouge_1_sum = 0.0
        rouge_2_sum = 0.0
        rouge_l_sum = 0.0
        bleu_1_sum = 0.0
        bleu_2_sum = 0.0
        bleu_4_sum = 0.0
        # 生成指标（ROUGE/BLEU）专用分母：仅统计「有非空参考答案」的样本。
        # 无参考答案的样本（如 SQL/纯检索题）ROUGE/BLEU 恒 0，若并入总样本数分母会稀释均值，
        # 与 sql.py/检索指标「无标注即剔除」的口径不一致。
        generation_count = 0

        # 检索指标累加器（仅统计含相关文档标注的样本）
        precision_sum = 0.0
        recall_sum = 0.0
        ndcg_sum = 0.0
        mrr_sum = 0.0
        map_sum = 0.0
        retrieval_count = 0

        # 语义 / LLM 裁判累加器
        semantic_sum = 0.0
        semantic_count = 0
        llm_sum = 0.0
        llm_count = 0
        llm_failed = 0  # 裁判调用异常的样本数（网络/解析失败），用于可观测与失败率告警
        # 语义相似度所需数据（整批一次性计算，复用同一模型）
        semantic_indices: List[int] = []
        semantic_refs: List[str] = []
        semantic_hyps: List[str] = []

        # RAG 专属指标（忠实度/上下文相关性/噪声比）需按批计算，先收集
        rag_answers: List[str] = []
        rag_questions: List[str] = []
        rag_contexts: List[List[str]] = []
        rag_relevant_ids: List[List[str]] = []
        rag_retrieved_ids: List[List[str]] = []

        # 请求了哪些专属家族(基于注册表解析后的名字集合,而非写死 if 链)
        gen_requested = supported_names & _GENERATION_GRADERS
        retrieval_requested = supported_names & _RETRIEVAL_GRADERS
        semantic_requested = supported_names & _SEMANTIC_GRADERS
        rag_requested = supported_names & _RAG_GRADERS
        llm_judge_requested = "llm_judge" in supported_names

        for i, item in enumerate(request.items):
            item_result = ComputeItemResult(index=i)

            # 生成指标 (ROUGE, BLEU)——仅对有非空参考答案的样本计算并计入分母
            if gen_requested and (item.reference_answer or "").strip():
                handled |= gen_requested
                generation_count += 1
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
            elif gen_requested:
                # 被请求但该样本无参考答案：标记为已处理（不落到通用路径），但不计入生成分母。
                handled |= gen_requested

            # 检索指标（只要有相关文档标注即计入；检索结果为空是合法的零召回样本，
            # 应记 recall=0 并计入分母，而非静默剔除——否则「什么都没检索到」的差样本反而
            # 不拉低平均召回，虚高检索质量。空检索列表下 precision/recall/ndcg/mrr 均安全返回 0）。
            if retrieval_requested and item.relevant_pids:
                handled |= retrieval_requested
                # 将 检索ID 序列按是否命中相关ID 转成布尔相关性列表
                relevant_set = set(item.relevant_pids)
                retrieved_bool = [pid in relevant_set for pid in item.retrieved_pids]
                ret = compute_retrieval_metrics(
                    retrieved_bool,
                    total_relevant=len(relevant_set),
                    k=len(retrieved_bool),
                )
                item_result.precision = ret.get("precision")
                item_result.recall = ret.get("recall")
                item_result.ndcg = ret.get("ndcg")
                item_result.rr = ret.get("mrr")  # 单条为 Reciprocal Rank，键名为 mrr

                precision_sum += ret.get("precision", 0.0)
                recall_sum += ret.get("recall", 0.0)
                ndcg_sum += ret.get("ndcg", 0.0)
                mrr_sum += ret.get("mrr", 0.0)
                map_sum += ret.get("map_score", 0.0)
                retrieval_count += 1

            # 语义相似度（前端评测器为 semantic_similarity / semantic_relevance）
            # 收集后整批一次性计算，避免逐条重复加载模型
            if semantic_requested:
                handled |= semantic_requested
                semantic_indices.append(i)
                semantic_refs.append(item.reference_answer)
                semantic_hyps.append(item.generated_answer)

            # LLM Judge
            if llm_judge_requested and request.llm_judge:
                handled.add("llm_judge")
                try:
                    # 用异步变体：裁判调用是网络 I/O，同步调用会独占事件循环、串行阻塞 :18888。
                    score = await compute_llm_judge_metrics_async(
                        reference=item.reference_answer,
                        hypothesis=item.generated_answer,
                        question=item.question,
                        dimensions=request.llm_judge.get("dimensions", ["relevance", "accuracy"]),
                    )
                    item_result.llm_score = score.get("total_score")
                    item_result.llm_reasoning = score.get("reasoning")
                    if item_result.llm_score is not None:
                        llm_sum += item_result.llm_score
                        llm_count += 1
                except Exception as e:
                    # 裁判调用失败属「无法评估」而非「答错」：不记 0（避免冤枉）、不并入分母（避免虚高），
                    # 但必须显式记录并计数——否则一批样本静默失败后，llm_score 只按少数成功样本求均值
                    # （分母缩水、分数虚高），甚至全批失败时该指标无声消失，无从排查。
                    llm_failed += 1
                    logger.warning(
                        "LLM Judge 第 %d 条样本评分失败（question=%r）: %s",
                        i, (item.question or "")[:80], e, exc_info=True,
                    )

            # 收集 RAG 专属指标所需数据（按批计算）
            if item.retrieved_contexts:
                rag_answers.append(item.generated_answer)
                rag_questions.append(item.question)
                rag_contexts.append(item.retrieved_contexts)
                rag_relevant_ids.append(item.relevant_pids)
                rag_retrieved_ids.append(item.retrieved_pids)

            items_result.append(item_result)

        # 语义相似度（整批一次性计算，复用进程级共享模型）
        if semantic_indices:
            try:
                sims = compute_semantic_similarities(semantic_refs, semantic_hyps)
                for idx, sim in zip(semantic_indices, sims):
                    if sim is not None:
                        items_result[idx].semantic_similarity = sim
                        semantic_sum += sim
                        semantic_count += 1
            except Exception as e:
                # 整批语义相似度失败不影响其他指标，但必须落外层日志——否则整批语义分静默缺席，
                # 只有内层降级 warning 时无从判断是「未请求」还是「批量算子异常」。
                logger.error("批量语义相似度计算失败（%d 条样本）: %s", len(semantic_indices), e, exc_info=True)

        # 计算聚合指标（按请求的评分器输出，合法的 0 分也应保留）。
        # 生成指标以 generation_count（有参考答案的样本数）为分母，不用总样本数 count，
        # 避免无参考样本的 0 分稀释均值。
        if generation_count > 0:
            if "rouge" in supported_names or supported_names & {"rouge_1", "rouge_2", "rouge_l"}:
                aggregate["rouge_1"] = rouge_1_sum / generation_count
                aggregate["rouge_2"] = rouge_2_sum / generation_count
                aggregate["rouge_l"] = rouge_l_sum / generation_count
            if "bleu" in supported_names or supported_names & {"bleu_1", "bleu_2", "bleu_4"}:
                aggregate["bleu_1"] = bleu_1_sum / generation_count
                aggregate["bleu_2"] = bleu_2_sum / generation_count
                aggregate["bleu_4"] = bleu_4_sum / generation_count

        # 检索指标聚合（仅对含标注的样本求均值）
        if retrieval_count > 0:
            aggregate["precision"] = precision_sum / retrieval_count
            aggregate["recall"] = recall_sum / retrieval_count
            aggregate["ndcg"] = ndcg_sum / retrieval_count
            aggregate["mrr"] = mrr_sum / retrieval_count
            aggregate["map"] = map_sum / retrieval_count

        # 语义 / LLM 聚合
        if semantic_count > 0:
            aggregate["semantic_similarity"] = semantic_sum / semantic_count
        if llm_count > 0:
            aggregate["llm_score"] = llm_sum / llm_count
        # 裁判失败可观测：有失败即告警（含失败率）；全批失败时 llm_score 缺席但日志留痕。
        if llm_failed > 0:
            total_judged = llm_count + llm_failed
            logger.warning(
                "LLM Judge 失败 %d/%d 条（成功 %d 条，llm_score 仅按成功样本求均值）",
                llm_failed, total_judged, llm_count,
            )

        # RAG 专属指标聚合（忠实度 / 上下文相关性 / 噪声比）
        if rag_contexts:
            if "faithfulness" in supported_names:
                handled.add("faithfulness")
                aggregate["faithfulness"] = faithfulness(rag_answers, rag_contexts)
            if "context_relevance" in supported_names:
                handled.add("context_relevance")
                aggregate["context_relevance"] = context_relevance(rag_questions, rag_contexts)
            if "noise_ratio" in supported_names and any(rag_relevant_ids):
                handled.add("noise_ratio")
                # 仅统计「有相关文档标注」的样本，与 precision/recall 同口径：无标注样本的相关集合为空，
                # 会把整个检索结果都算成噪声（noise=100%），把无标注误当「全是噪声」而虚高噪声比。
                nr_contexts, nr_relevant, nr_retrieved = [], [], []
                for ctx, rel, ret in zip(rag_contexts, rag_relevant_ids, rag_retrieved_ids):
                    if rel:
                        nr_contexts.append(ctx)
                        nr_relevant.append(rel)
                        nr_retrieved.append(ret)
                if nr_relevant:
                    aggregate["noise_ratio"] = noise_ratio(
                        retrieved_contexts=nr_contexts,
                        relevant_doc_ids=nr_relevant,
                        retrieved_doc_ids=nr_retrieved,
                    )

        # 无论 RAG 数据是否齐全，rag 家族一旦被请求即视为由专属路径负责，不落到通用路径
        handled |= rag_requested

        # —— 把固定字段镜像进每项动态 scores map ——
        for it in items_result:
            for key in _FIXED_SCORE_KEYS:
                value = getattr(it, key)
                if value is not None:
                    it.scores[key] = value

        # —— 通用 grader 执行路径:已注册但无专属批量算子的指标(如 llm_factual/
        #    llm_safety/exact_match 等及开发者新增的 grader),一处新增即可计算 ——
        generic_names = [n for n in supported_names if n not in handled]
        for name in generic_names:
            grader = registry.get(name)
            if grader is None:
                continue
            g_sum = 0.0
            g_count = 0
            for i, item in enumerate(request.items):
                try:
                    res = await grader.aevaluate(
                        query=item.question,
                        response=item.generated_answer,
                        reference=item.reference_answer,
                        context=item.retrieved_contexts or None,
                        relevant_pids=item.relevant_pids,
                        retrieved_pids=item.retrieved_pids,
                    )
                except Exception:
                    continue  # 单个 grader 失败不影响其他指标
                if isinstance(res, GraderScore) and res.metrics:
                    value = res.score  # 取主指标(第一个)作为该 grader 的分值
                    items_result[i].scores[name] = value
                    g_sum += value
                    g_count += 1
            if g_count > 0:  # 以是否运行为准,合法 0 分保留
                aggregate[name] = g_sum / g_count

        # —— Agent 家族专属路径：按 references/outputs 组装后命中 compute_agent_metrics ——
        # references 取 expected_tools/expected_steps 作参考，outputs 取实际 tools_used/trajectory/total_steps。
        if agent_requested and request.items:
            references = [
                {
                    "final_answer": item.reference_answer,
                    "tools_used": item.expected_tools,
                    "expected_steps": item.expected_steps,
                }
                for item in request.items
            ]
            outputs = [
                {
                    "final_answer": item.generated_answer,
                    "tools_used": item.tools_used,
                    "trajectory": [{"content": s} for s in item.trajectory],
                    "total_steps": item.total_steps or len(item.trajectory),
                }
                for item in request.items
            ]
            agent_result = compute_agent_metrics(
                references=references,
                outputs=outputs,
                metrics=sorted(agent_requested),
                return_items=True,
            )
            # 逐条落库：compute_agent_metrics 已「逐样本算一次、聚合即其均值」，
            # _items[i] 是第 i 条样本的扁平化分值 map（键名与聚合平铺后一致）。
            # 直接并入 items_result[i].scores，前端评测详情即可逐条 debug 工具/轨迹/准确率，
            # 不必再从任务级聚合反推是哪条样本的工具或轨迹拖低了分数。
            for i, item_scores in enumerate(agent_result.get("_items", [])):
                if i < len(items_result) and item_scores:
                    items_result[i].scores.update(item_scores)
            # 将嵌套结果平铺进聚合 scores map（前端/Go 侧按 name->value 消费）
            if "answer_accuracy" in agent_result:
                aggregate["answer_accuracy"] = agent_result["answer_accuracy"]
            if isinstance(agent_result.get("tool_selection"), dict):
                ts = agent_result["tool_selection"]
                aggregate["tool_precision"] = ts.get("precision", 0.0)
                aggregate["tool_recall"] = ts.get("recall", 0.0)
                aggregate["tool_f1"] = ts.get("f1", 0.0)
            if isinstance(agent_result.get("tool_order"), dict):
                aggregate["tool_order"] = agent_result["tool_order"].get("ordered_match", 0.0)
            if isinstance(agent_result.get("trajectory_match"), dict):
                tm = agent_result["trajectory_match"]
                aggregate["traj_exact_match"] = tm.get("exact_match", 0.0)
                aggregate["traj_similarity"] = tm.get("similarity", 0.0)
            if isinstance(agent_result.get("step_efficiency"), dict):
                se = agent_result["step_efficiency"]
                aggregate["step_optimal_ratio"] = se.get("optimal_ratio", 0.0)

        # —— SQL 家族专属路径：按 references/outputs 组装后命中 compute_sql_metrics ——
        # references 取 gold_sql/gold_result_set，outputs 取 generated_sql/result_set；
        # 结构指标比对 SQL 文本，执行准确率无序比对两个结果集。
        if sql_requested and request.items:
            references = [
                {
                    "gold_sql": item.gold_sql,
                    "gold_result_set": item.gold_result_set,
                }
                for item in request.items
            ]
            outputs = [
                {
                    "generated_sql": item.generated_sql,
                    "result_set": item.result_set,
                }
                for item in request.items
            ]
            sql_result = compute_sql_metrics(
                references=references,
                outputs=outputs,
                metrics=sorted(sql_requested),
                return_items=True,
            )
            # 逐条落库：_items[i] 即第 i 条样本的分值 map（键名与聚合一致），
            # 无法评估的样本对应键缺席（如未执行则无 sql_execution_accuracy）。
            for i, item_scores in enumerate(sql_result.get("_items", [])):
                if i < len(items_result) and item_scores:
                    items_result[i].scores.update(item_scores)
            # 聚合平铺（compute_sql_metrics 聚合本身即 name->value 标量）
            for key in ("sql_exact_match", "sql_component_match", "sql_execution_accuracy"):
                if key in sql_result:
                    aggregate[key] = sql_result[key]

        return ComputeMetricsResponse(
            items=items_result,
            aggregate=aggregate,
            unsupported=unsupported,
        )

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Metrics computation failed: {str(e)}",
        )


# ============================================================
# 可用指标目录(注册表为唯一事实来源,供 Go 拉取)
# ============================================================

class GraderCatalogResponse(BaseModel):
    """可用指标目录响应。"""
    eval_type: str
    graders: List[dict[str, Any]]   # 每项含 name/label/group/eval_types/requires_* 等元数据


@app.get("/api/v1/evaluation/graders", response_model=GraderCatalogResponse)
async def list_available_graders(eval_type: str) -> GraderCatalogResponse:
    """按评测类型返回可用指标目录。

    注册表为唯一事实来源:每项都对应一个可执行 grader。qa 归一化为 llm;
    未知评测类型返回 400 而非无过滤全量。
    """
    registry = _ensure_registry()
    try:
        normalized = normalize_eval_type(eval_type)
    except ValueError:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Unknown eval_type: {eval_type}",
        )
    return GraderCatalogResponse(
        eval_type=normalized.value,
        graders=registry.list_graders_for(normalized),
    )


# ============================================================
# 启动入口
# ============================================================

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=18888)
