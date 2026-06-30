# 自定义非结构化数据评估器

## 创建自定义评估器

```python
from ..base import UnstructuredEvaluator
from ...registry import register_evaluator
from ...models import UnstructuredDimensionScore, TextQualityIssue, SeverityLevel

@register_evaluator("sentiment_analysis")
class SentimentAnalysisEvaluator(UnstructuredEvaluator):
    """情感分析评估器。"""

    dimension_name = "sentiment_analysis"
    description = "分析文本情感倾向"

    def evaluate(self, text, config=None):
        """实现评估逻辑。"""
        issues = []
        # ...
        return UnstructuredDimensionScore(
            name=self.dimension_name,
            score=score,
            passed=self.is_passed(score, config.get("threshold", 70)),
            issues=issues,
            details={},
        )
```
