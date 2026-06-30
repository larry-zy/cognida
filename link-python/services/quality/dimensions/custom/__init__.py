"""自定义结构化数据评估器目录。

用户可以在此目录下创建自定义评估器。
"""

# 示例：
# from ..base import DimensionEvaluator
# from ...registry import register_evaluator
#
# @register_evaluator("custom_business_rule")
# class CustomBusinessEvaluator(DimensionEvaluator):
#     """自定义业务规则评估器。"""
#
#     dimension_name = "custom_business_rule"
#     description = "自定义业务规则评估"
#
#     def evaluate(self, data, rules=None, config=None):
#         # 实现评估逻辑
#         pass
