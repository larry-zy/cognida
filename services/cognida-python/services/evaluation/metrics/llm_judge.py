"""LLM-as-a-Judge 评测指标（基于 LangChain）。

分数量纲统一为 1-5（单一口径）：与 graders/builtin/llm.py、前端展示一致。
调用/解析失败直接抛出，由调用方（runner）显式记录——不再静默返回零分，
避免"LLM 从未被真正调用却产出了分数"的静默损坏。
"""

from typing import Any

from pydantic import BaseModel

from ..llm import LLMClient, get_llm_client

# 统一量纲：1-5 分
SCORE_MIN = 1.0
SCORE_MAX = 5.0


class LLMJudgeMetrics(BaseModel):
    """LLM 裁判评测指标（1-5 分口径）。"""

    total_score: float  # 总分
    dimension_scores: dict[str, float]  # 各维度分数
    reasoning: str = ""  # 评分理由

    model_config = {"arbitrary_types_allowed": True}

    @classmethod
    def compute(
        cls,
        reference: str,
        hypothesis: str,
        question: str = "",
        llm_client: LLMClient | None = None,
        dimensions: list[str] | None = None,
    ) -> "LLMJudgeMetrics":
        """使用 LLM 评分（同步）。

        Args:
            reference: 参考答案
            hypothesis: 生成答案
            question: 问题（可选，用于上下文）
            llm_client: LLM 客户端（可选）
            dimensions: 评分维度列表

        Returns:
            LLM 评测指标结果

        Raises:
            ValueError: LLM 返回缺维度/非数字时（不静默填零分）
        """
        dimensions = dimensions or ["accuracy", "completeness", "clarity", "relevance"]
        llm_client = llm_client or cls._default_client()

        result = llm_client.generate_json(
            prompt=cls._build_prompt(question, reference, hypothesis, dimensions),
            system_prompt=cls._build_system_prompt(dimensions),
        )
        return cls._from_llm_result(result, dimensions)

    @classmethod
    async def acompute(
        cls,
        reference: str,
        hypothesis: str,
        question: str = "",
        llm_client: LLMClient | None = None,
        dimensions: list[str] | None = None,
    ) -> "LLMJudgeMetrics":
        """使用 LLM 评分（异步，不阻塞事件循环）。"""
        dimensions = dimensions or ["accuracy", "completeness", "clarity", "relevance"]
        llm_client = llm_client or cls._default_client()

        result = await llm_client.agenerate_json(
            prompt=cls._build_prompt(question, reference, hypothesis, dimensions),
            system_prompt=cls._build_system_prompt(dimensions),
        )
        return cls._from_llm_result(result, dimensions)

    @classmethod
    def _default_client(cls) -> LLMClient:
        """创建默认 LLM 客户端（从环境变量读取配置）。"""
        import os
        provider = os.getenv("LLM_PROVIDER", "openai")
        model = os.getenv("LLM_MODEL", None)
        return get_llm_client(provider=provider, model=model)

    @classmethod
    def _from_llm_result(
        cls,
        result: dict[str, Any],
        dimensions: list[str],
    ) -> "LLMJudgeMetrics":
        """从 LLM 结构化返回解析评分（严格模式：缺维度/非数字即抛错）。"""
        if not dimensions:
            # 空维度会在下方均分计算处除零，显式抛错由上层统一包装
            raise ValueError("评分维度列表为空，无法计算总分")
        if not isinstance(result, dict) or not result:
            raise ValueError(f"LLM 裁判返回非 JSON 对象: {result!r}")

        dimension_scores: dict[str, float] = {}
        for dim in dimensions:
            if dim not in result:
                raise ValueError(f"LLM 裁判返回缺少维度 {dim!r}: {result!r}")
            try:
                value = float(result[dim])
            except (TypeError, ValueError) as e:
                raise ValueError(f"维度 {dim!r} 的分数非数字: {result[dim]!r}") from e
            dimension_scores[dim] = max(SCORE_MIN, min(SCORE_MAX, value))

        # 总分 = 维度均分（与维度同一 1-5 口径）
        total_score = sum(dimension_scores.values()) / len(dimension_scores)

        # 提取评分理由（提示词要求返回 reason 字段，兼容 reasoning 命名）
        reasoning = str(result.get("reason") or result.get("reasoning") or "")

        return cls(
            total_score=total_score,
            dimension_scores=dimension_scores,
            reasoning=reasoning,
        )

    @classmethod
    def _build_system_prompt(cls, dimensions: list[str]) -> str:
        """构建系统提示词。"""
        return """你是一个专业的评测员。请根据给定的标准对回答进行评分。

评分要求：
- 每项指标 1-5 分（1=很差，5=优秀）
- 只返回 JSON 格式结果
- 分数应为数字，不要包含文字描述
"""

    @classmethod
    def _build_prompt(
        cls,
        question: str,
        reference: str,
        hypothesis: str,
        dimensions: list[str],
    ) -> str:
        """构建评分提示词。"""
        # 维度描述
        dimension_descriptions = {
            # 通用维度
            "accuracy": "准确性 - 答案是否正确、事实是否准确",
            "completeness": "完整性 - 是否覆盖了所有要点、信息是否完整",
            "clarity": "清晰度 - 表达是否清晰易懂、逻辑是否连贯",
            "relevance": "相关性 - 是否回答了问题、是否切题",
            "helpfulness": "有用性 - 答案是否有帮助、是否解决了问题",
            "safety": "安全性 - 是否包含有害、不当或有偏见的内容",
            "creativity": "创造性 - 答案是否有创意、是否有独到见解",
            # Agent 专用维度
            "reasoning": "推理能力 - 推理过程是否合理、逻辑是否严密",
            "tool_use": "工具使用 - 是否正确使用了工具、工具调用是否合理",
            "efficiency": "效率 - 是否以最少的步骤完成了任务",
            "correctness": "正确性 - 最终答案是否正确",
            # RAG 专用维度
            "faithfulness": "忠实度 - 答案是否基于检索内容、有无幻觉",
            "context_usage": "上下文利用 - 是否有效利用了检索到的上下文",
        }

        # 构建评分标准（统一 1-5 口径）
        criteria = "\n".join(
            f"{i+1}. {dimension_descriptions.get(dim, dim)} (1-5分)"
            for i, dim in enumerate(dimensions)
        )

        prompt_parts = [f"""请评估以下回答的质量。

评分标准:
{criteria}

"""]

        if question:
            prompt_parts.append(f"""问题:
{question}

""")

        prompt_parts.append(f"""参考答案:
{reference}

待评测回答:
{hypothesis}

请严格按照以下 JSON 格式返回评分结果:
{{
{', '.join([f'"{dim}": <分数>' for dim in dimensions])},
  "reason": "<评分理由，简述评分依据>"
}}
""")

        return "".join(prompt_parts)


# 便捷函数
def compute_llm_judge_metrics(
    reference: str,
    hypothesis: str,
    question: str = "",
    dimensions: list[str] | None = None,
) -> dict:
    """计算 LLM 评测指标（同步）。"""
    metrics = LLMJudgeMetrics.compute(
        reference=reference,
        hypothesis=hypothesis,
        question=question,
        dimensions=dimensions,
    )
    return metrics.model_dump()


async def compute_llm_judge_metrics_async(
    reference: str,
    hypothesis: str,
    question: str = "",
    dimensions: list[str] | None = None,
) -> dict:
    """计算 LLM 评测指标（异步，单条样本）。

    返回 dict：调用方读 result["dimension_scores"] / result["total_score"]。
    失败时抛出，由调用方显式处理。
    """
    metrics = await LLMJudgeMetrics.acompute(
        reference=reference,
        hypothesis=hypothesis,
        question=question,
        dimensions=dimensions,
    )
    return metrics.model_dump()
