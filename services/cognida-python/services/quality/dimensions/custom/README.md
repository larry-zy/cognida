# 自定义结构化数据评估器

## 创建自定义评估器

在此目录下创建 Python 文件，定义继承自 `DimensionEvaluator` 的类：

```python
from ..base import DimensionEvaluator
from ...registry import register_evaluator
from ...models import DimensionScore, QualityIssue, SeverityLevel

@register_evaluator("my_custom_evaluator")
class MyCustomEvaluator(DimensionEvaluator):
    """自定义评估器描述。"""

    dimension_name = "my_custom_evaluator"
    description = "我的自定义评估器"

    def evaluate(self, data, rules=None, config=None):
        """实现评估逻辑。

        Args:
            data: pandas DataFrame
            rules: 字段规则列表
            config: 配置参数

        Returns:
            DimensionScore 对象
        """
        issues = []

        # 实现评估逻辑
        # ...

        score = self._calculate_score(len(issues), len(data))
        return DimensionScore(
            name=self.dimension_name,
            score=score,
            passed=self.is_passed(score, config.get("threshold", 70)),
            issues=issues,
        )
```

## 注册评估器

使用 `@register_evaluator` 装饰器自动注册评估器：

```python
@register_evaluator("evaluator_name")
class MyEvaluator(DimensionEvaluator):
    ...
```

## 使用自定义评估器

```python
from services.quality.registry import EvaluatorRegistry

# 获取评估器实例
evaluator = EvaluatorRegistry.get_instance("my_custom_evaluator")

# 执行评估
result = evaluator.evaluate(data)
```
