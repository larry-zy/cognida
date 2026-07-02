"""集成评测策略。

组合多个评分器的结果。
"""

from typing import Any, Dict, List, Optional

from ..graders import get_grader
from .base import BaseStrategy, StrategyConfig


class EnsembleStrategy(BaseStrategy):
    """集成评测策略。

    组合多个评分器的结果，使用指定的聚合方法。
    """

    def __init__(self, config: Optional[StrategyConfig] = None) -> None:
        super().__init__(
            name="ensemble",
            description="组合多个评分器的结果",
            config=config,
        )
        self._aggregation_method = config.params.get("aggregation", "average") if config else "average"

    async def execute(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        graders: Optional[List[str]] = None,
        aggregation: Optional[str] = None,
        weights: Optional[Dict[str, float]] = None,
        **kwargs
    ) -> Dict[str, Any]:
        """执行集成评测。

        Args:
            query: 问题
            response: 回答
            reference: 参考答案
            context: 上下文
            graders: 评分器名称列表
            aggregation: 聚合方法 ("average", "max", "min", "weighted")
            weights: 各评分器的权重

        Returns:
            聚合后的评分结果
        """
        if not graders:
            return {}

        aggregation = aggregation or self._aggregation_method

        # 收集所有评分器的结果
        grader_scores = {}
        zero_shot = ZeroShotStrategy()

        raw_results = await zero_shot.execute(
            query=query,
            response=response,
            reference=reference,
            context=context,
            graders=graders,
            **kwargs,
        )

        # 提取主分数
        for grader_name, result in raw_results.items():
            if "error" in result:
                continue
            # 取第一个指标作为主分数
            if isinstance(result, dict) and result:
                first_value = next(iter(result.values()))
                grader_scores[grader_name] = float(first_value)

        if not grader_scores:
            return {}

        # 聚合
        if aggregation == "average":
            final_score = sum(grader_scores.values()) / len(grader_scores)
        elif aggregation == "max":
            final_score = max(grader_scores.values())
        elif aggregation == "min":
            final_score = min(grader_scores.values())
        elif aggregation == "weighted" and weights:
            total_weight = 0
            weighted_sum = 0
            for name, score in grader_scores.items():
                weight = weights.get(name, 1.0)
                weighted_sum += score * weight
                total_weight += weight
            final_score = weighted_sum / total_weight if total_weight > 0 else 0
        else:
            final_score = sum(grader_scores.values()) / len(grader_scores)

        return {
            "ensemble_score": final_score,
            "individual_scores": grader_scores,
            "aggregation": aggregation,
        }


class MajorityVotingStrategy(BaseStrategy):
    """多数投票策略。

    对二值评分器使用多数投票。
    """

    def __init__(self, config: Optional[StrategyConfig] = None) -> None:
        super().__init__(
            name="majority_voting",
            description="多数投票策略（适用于二值评分）",
            config=config,
        )

    async def execute(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        graders: Optional[List[str]] = None,
        threshold: float = 0.5,
        **kwargs
    ) -> Dict[str, Any]:
        """执行多数投票。"""
        if not graders:
            return {}

        zero_shot = ZeroShotStrategy()
        raw_results = await zero_shot.execute(
            query=query,
            response=response,
            reference=reference,
            context=context,
            graders=graders,
            **kwargs,
        )

        # 统计投票
        positive_votes = 0
        negative_votes = 0
        total_votes = 0

        for result in raw_results.values():
            if "error" in result:
                continue
            for value in result.values():
                if value >= threshold:
                    positive_votes += 1
                else:
                    negative_votes += 1
                total_votes += 1

        if total_votes == 0:
            return {"majority_vote": 0.5, "positive": 0, "negative": 0}

        majority_ratio = positive_votes / total_votes

        return {
            "majority_vote": majority_ratio,
            "positive": positive_votes,
            "negative": negative_votes,
            "total": total_votes,
        }


# 导入 ZeroShotStrategy
from .zero_shot import ZeroShotStrategy  # noqa: E402
