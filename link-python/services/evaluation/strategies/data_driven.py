"""数据驱动评测策略。

从样本数据中学习评分模式，然后应用到新数据。
"""

from typing import Any, Dict, List, Optional

from ..graders import get_grader
from .base import BaseStrategy, StrategyConfig


class DataDrivenStrategy(BaseStrategy):
    """数据驱动评测策略。

    从样本 QA 对中学习，然后应用学到的模式进行评分。
    """

    def __init__(self, config: Optional[StrategyConfig] = None):
        super().__init__(
            name="data_driven",
            description="从样本数据学习评分模式",
            config=config,
        )
        self._sample_results: Optional[Dict[str, List[float]]] = None

    async def execute(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        graders: Optional[List[str]] = None,
        samples: Optional[List[Dict[str, Any]]] = None,
        **kwargs
    ) -> Dict[str, Any]:
        """执行数据驱动评测。

        Args:
            query: 问题
            response: 回答
            reference: 参考答案
            context: 上下文
            graders: 评分器名称列表
            samples: 样本数据列表

        Returns:
            评分结果字典
        """
        if not graders:
            return {}

        # 如果提供了样本，先在样本上学习
        if samples and self._sample_results is None:
            await self._learn_from_samples(samples, graders)

        # 使用评分器进行评测
        results = {}
        zero_shot = ZeroShotStrategy()

        # 先获取当前分数
        current_results = await zero_shot.execute(
            query=query,
            response=response,
            reference=reference,
            context=context,
            graders=graders,
            **kwargs,
        )

        # 如果有学习到的模式，进行调整
        if self._sample_results:
            for grader_name, grader_result in current_results.items():
                if "error" in grader_result:
                    results[grader_name] = grader_result
                    continue

                # 获取样本统计信息
                sample_stats = self._sample_results.get(grader_name, {})
                if sample_stats:
                    # 根据样本分布调整分数
                    adjusted = self._adjust_score(
                        grader_result,
                        sample_stats,
                    )
                    results[grader_name] = adjusted
                else:
                    results[grader_name] = grader_result
        else:
            results = current_results

        return results

    async def _learn_from_samples(
        self,
        samples: List[Dict[str, Any]],
        graders: List[str],
    ) -> None:
        """从样本中学习。"""
        self._sample_results = {}

        for grader_name in graders:
            grader = get_grader(grader_name)
            if grader is None:
                continue

            scores = []
            for sample in samples:
                try:
                    result = await grader.aevaluate(
                        query=sample.get("query", ""),
                        response=sample.get("response", ""),
                        reference=sample.get("reference"),
                        context=sample.get("context"),
                    )
                    if hasattr(result, "score"):
                        scores.append(result.score)
                except Exception:
                    pass

            if scores:
                self._sample_results[grader_name] = {
                    "mean": sum(scores) / len(scores),
                    "min": min(scores),
                    "max": max(scores),
                    "std": (sum((s - sum(scores) / len(scores)) ** 2 for s in scores) / len(scores)) ** 0.5,
                }

    def _adjust_score(
        self,
        score_dict: Dict[str, float],
        stats: Dict[str, float],
    ) -> Dict[str, float]:
        """根据样本统计调整分数。"""
        # 简单的归一化：将分数映射到样本分布范围
        sample_mean = stats["mean"]
        sample_min = stats["min"]
        sample_max = stats["max"]

        adjusted = {}
        for key, value in score_dict.items():
            # 如果分数超出样本范围，进行裁剪
            if value > sample_max:
                adjusted[key] = sample_max
            elif value < sample_min:
                adjusted[key] = sample_min
            else:
                adjusted[key] = value

        return adjusted


# 导入 ZeroShotStrategy 以避免循环导入
from .zero_shot import ZeroShotStrategy  # noqa: E402
