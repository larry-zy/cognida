"""LLM-as-a-Judge 评分器。"""

from typing import Any

from ..client import LLMClient
from .base import Grader


class LLMJudgeGrader(Grader):
    """LLM 裁判评分器。"""

    name = "llm_judge"

    def __init__(self, llm_client: LLMClient, judge_prompt_template: str | None = None) -> None:
        """初始化 LLM 评分器。

        Args:
            llm_client: LLM 客户端
            judge_prompt_template: 评分提示词模板
        """
        self._llm = llm_client
        self._prompt_template = judge_prompt_template or self._default_prompt()

    def _default_prompt(self) -> str:
        """默认评分提示词。"""
        return """你是一个专业的评分员。请评估以下模型输出的质量。

参考答案:
{reference}

模型输出:
{model_output}

请按照以下标准评分:
1. 准确性 (40分): 答案是否正确
2. 完整性 (30分): 是否覆盖了所有要点
3. 清晰度 (20分): 表达是否清晰易懂
4. 相关性 (10分): 是否回答了问题

请以 JSON 格式返回评分结果:
{{
    "score": <总分 0-100>,
    "accuracy": <准确性分数>,
    "completeness": <完整性分数>,
    "clarity": <清晰度分数>,
    "relevance": <相关性分数>,
    "reason": <评分理由>
}}"""

    async def score(
        self,
        model_output: str,
        reference: str | None = None,
        **kwargs: Any,
    ) -> tuple[float, str, bool]:
        """使用 LLM 进行评分。"""
        if reference is None:
            return 0.0, "未提供参考答案", False

        prompt = self._prompt_template.format(
            reference=reference,
            model_output=model_output,
        )

        try:
            result, _ = await self._llm.generate_json(prompt)

            score = result.get("score", 0)
            reason = result.get("reason", "无评分理由")

            # 判断是否通过（60分及格）
            passed = score >= 60

            return float(score), str(reason), passed
        except Exception as e:
            return 0.0, f"LLM 评分失败: {e}", False

    def set_prompt_template(self, template: str) -> None:
        """设置评分提示词模板。"""
        self._prompt_template = template
