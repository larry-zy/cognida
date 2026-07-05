"""评测服务使用示例。"""

import asyncio
import json

from services.evaluation.graders import get_grader, list_graders
from services.evaluation.strategies import get_strategy


async def demo_single_grader():
    """演示单个评分器使用。"""
    print("\n=== 单个评分器示例 ===")

    # 获取 ROUGE-1 评分器
    grader = get_grader("rouge_1")

    # 评分
    result = await grader.aevaluate(
        query="什么是机器学习？",
        response="机器学习是人工智能的一个分支，它使计算机能够从数据中学习。",
        reference="机器学习是人工智能的子领域，通过数据让计算机改进性能。",
    )

    print(f"评分器: {result.name}")
    print(f"指标类型: {result.metric_type}")
    print(f"理由: {result.reason}")
    print(f"分数: {result.metrics}")
    print(f"主分数: {result.score:.4f}")


async def demo_list_graders():
    """演示列出所有评分器。"""
    print("\n=== 可用评分器列表 ===")

    graders = list_graders()

    for grader_info in graders:
        print(f"- {grader_info['name']}: {grader_info.get('description', '')}")


async def demo_zero_shot_strategy():
    """演示零样本策略。"""
    print("\n=== 零样本策略示例 ===")

    strategy = get_strategy("zero_shot")

    result = await strategy.execute(
        query="Python 是什么？",
        response="Python 是一种高级编程语言，以其简洁的语法和强大的功能而闻名。",
        reference="Python 是由 Guido van Rossum 创建的高级编程语言。",
        graders=["rouge_1", "bleu_4"],
    )

    print(f"评分结果:")
    for grader_name, scores in result.items():
        print(f"  {grader_name}: {scores}")


async def demo_ensemble_strategy():
    """演示集成策略。"""
    print("\n=== 集成策略示例 ===")

    strategy = get_strategy("ensemble")

    result = await strategy.execute(
        query="1 + 1 等于几？",
        response="等于 2",
        reference="2",
        graders=["exact_match", "rouge_1"],
        aggregation="max",
    )

    print(f"集成分数: {result['ensemble_score']:.4f}")
    print(f"个人分数: {result['individual_scores']}")


async def demo_conditional_strategy():
    """演示条件策略。"""
    print("\n=== 条件策略示例 ===")

    strategy = get_strategy("conditional")

    # 数学问题
    result = await strategy.execute(
        query="计算 1 + 1 等于多少？",
        response="等于 2",
        reference="2",
    )

    print(f"检测到的问题类型: {result['question_type']}")
    print(f"选择的评分器: {result['selected_graders']}")
    print(f"评分结果: {result['scores']}")


async def main():
    """主函数。"""
    print("=" * 60)
    print("Python 评测服务使用示例（无状态 compute：评分器 / 策略）")
    print("=" * 60)

    await demo_list_graders()
    await demo_single_grader()
    await demo_zero_shot_strategy()
    await demo_ensemble_strategy()
    await demo_conditional_strategy()

    print("\n" + "=" * 60)
    print("所有示例运行完成！")
    print("=" * 60)


if __name__ == "__main__":
    asyncio.run(main())
