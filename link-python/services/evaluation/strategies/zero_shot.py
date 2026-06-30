"""零样本评测策略。

直接执行评分器，不做任何额外处理。
"""

from typing import Any, Dict, List, Optional

from ..graders import get_grader
from .base import BaseStrategy, StrategyConfig


class ZeroShotStrategy(BaseStrategy):
    """零样本评测策略。

    直接调用指定的评分器，不使用任何样本数据学习。
    """

    def __init__(self, config: Optional[StrategyConfig] = None):
        super().__init__(
            name="zero_shot",
            description="直接执行评分器，不使用样本数据",
            config=config,
        )

    async def execute(
        self,
        query: str,
        response: str,
        reference: Optional[str] = None,
        context: Optional[List[str]] = None,
        graders: Optional[List[str]] = None,
        **kwargs
    ) -> Dict[str, Any]:
        """执行零样本评测。

        Args:
            query: 问题
            response: 回答
            reference: 参考答案
            context: 上下文
            graders: 评分器名称列表

        Returns:
            评分结果字典
        """
        if not graders:
            return {}

        results = {}

        for grader_name in graders:
            grader = get_grader(grader_name)
            if grader is None:
                results[grader_name] = {"error": f"评分器 {grader_name} 未找到"}
                continue

            try:
                result = await grader.aevaluate(
                    query=query,
                    response=response,
                    reference=reference,
                    context=context,
                    **kwargs,
                )

                if hasattr(result, "metrics"):
                    results[grader_name] = result.metrics
                elif hasattr(result, "error"):
                    results[grader_name] = {"error": result.error}
                else:
                    results[grader_name] = {}
            except Exception as e:
                results[grader_name] = {"error": str(e)}

        return results
