"""条件评测策略。

根据问题类型选择合适的评分器。
"""

import re
from typing import Any, Dict, List, Optional

from ..graders import get_grader
from .base import BaseStrategy, StrategyConfig


class ConditionalStrategy(BaseStrategy):
    """条件评测策略。

    根据问题类型选择使用哪些评分器。
    """

    def __init__(self, config: Optional[StrategyConfig] = None):
        super().__init__(
            name="conditional",
            description="根据问题类型选择评分器",
            config=config,
        )

        # 默认的问题类型到评分器映射
        self._type_to_graders: Dict[str, List[str]] = {
            "factual": ["rouge_1", "bleu_4", "llm_factual"],
            "opinion": ["semantic_similarity", "llm_judge"],
            "procedural": ["rouge_l", "bleu_2"],
            "math": ["exact_match", "numeric_match"],
            "code": ["exact_match"],
            "default": ["rouge_1", "rouge_l", "bleu_4"],
        }

        # 问题类型检测规则
        self._type_patterns: Dict[str, List[str]] = {
            "math": [r"\d+\s*[\+\-\*\/]\s*\d+", r"计算", r"求", r"等于多少"],
            "code": [r"代码", r"函数", r"编程", r"写一个", r"实现"],
            "factual": [r"什么是", r"什么是", r"解释", r"定义"],
            "procedural": [r"如何", r"怎么", r"怎样", r"步骤"],
            "opinion": [r"认为", r"看法", r"评价", r"推荐"],
        }

    async def execute(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        graders: Optional[List[str]] = None,
        question_type: Optional[str] = None,
        **kwargs
    ) -> Dict[str, Any]:
        """执行条件评测。

        Args:
            query: 问题
            response: 回答
            reference: 参考答案
            context: 上下文
            graders: 默认评分器列表（如果未检测到类型）
            question_type: 指定的问题类型

        Returns:
            评分结果
        """
        # 检测问题类型
        if question_type is None:
            question_type = self._detect_question_type(query)

        # 选择评分器
        selected_graders = self._type_to_graders.get(
            question_type,
            graders or self._type_to_graders["default"],
        )

        # 执行评测
        from .zero_shot import ZeroShotStrategy

        zero_shot = ZeroShotStrategy()
        results = await zero_shot.execute(
            query=query,
            response=response,
            reference=reference,
            context=context,
            graders=selected_graders,
            **kwargs,
        )

        return {
            "question_type": question_type,
            "selected_graders": selected_graders,
            "scores": results,
        }

    def _detect_question_type(self, query: str) -> str:
        """检测问题类型。"""
        for qtype, patterns in self._type_patterns.items():
            for pattern in patterns:
                if re.search(pattern, query):
                    return qtype
        return "default"


class RouterStrategy(BaseStrategy):
    """路由策略。

    根据问题内容路由到不同的评分策略。
    """

    def __init__(self, config: Optional[StrategyConfig] = None):
        super().__init__(
            name="router",
            description="根据问题内容路由到不同策略",
            config=config,
        )
        self._conditional = ConditionalStrategy(config)

    async def execute(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        graders: Optional[List[str]] = None,
        **kwargs
    ) -> Dict[str, Any]:
        """执行路由策略。"""
        # 先检测类型
        question_type = self._conditional._detect_question_type(query)

        # 根据类型选择不同的聚合方法
        if question_type == "math":
            from .ensemble import EnsembleStrategy
            ensemble = EnsembleStrategy()
            return await ensemble.execute(
                query=query,
                response=response,
                reference=reference,
                context=context,
                graders=["exact_match", "numeric_match"],
                aggregation="max",
                **kwargs,
            )
        elif question_type == "factual":
            from .ensemble import EnsembleStrategy
            ensemble = EnsembleStrategy()
            return await ensemble.execute(
                query=query,
                response=response,
                reference=reference,
                context=context,
                graders=["rouge_1", "bleu_4", "llm_factual"],
                aggregation="weighted",
                weights={"rouge_1": 0.3, "bleu_4": 0.3, "llm_factual": 0.4},
                **kwargs,
            )
        else:
            # 默认使用条件策略
            return await self._conditional.execute(
                query=query,
                response=response,
                reference=reference,
                context=context,
                graders=graders,
                question_type=question_type,
                **kwargs,
            )
