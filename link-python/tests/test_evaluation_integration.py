"""评测服务集成测试。

测试完整的评测流程。
"""

import asyncio
import pytest

from services.evaluation.datasets import get_dataset_manager
from services.evaluation.runners import EvaluationRunner, EvaluationConfig, Progress
from services.evaluation.graders import list_graders, get_grader


@pytest.mark.asyncio
async def test_dataset_loading():
    """测试数据集加载。"""
    manager = get_dataset_manager()
    manager.initialize()

    # 检查默认数据集
    dataset = manager.get_dataset("default")
    assert dataset is not None
    assert dataset.qa_count > 0


@pytest.mark.asyncio
async def test_grader_registry():
    """测试评分器注册表。"""
    graders = list_graders()
    assert len(graders) > 0

    # 检查至少有一些内置评分器
    grader_names = [g["name"] for g in graders]
    assert "rouge_1" in grader_names


@pytest.mark.asyncio
async def test_rouge_grader():
    """测试 ROUGE 评分器。"""
    grader = get_grader("rouge_1")
    assert grader is not None

    result = await grader.aevaluate(
        query="什么是机器学习？",
        response="机器学习是人工智能的一个分支。",
        reference="机器学习是人工智能的子领域。",
    )

    assert result is not None
    assert hasattr(result, "metrics")
    assert "rouge_1" in result.metrics


@pytest.mark.asyncio
async def test_exact_match_grader():
    """测试精确匹配评分器。"""
    grader = get_grader("exact_match")
    assert grader is not None

    # 完全匹配
    result = await grader.aevaluate(
        query="1+1等于几？",
        response="等于2",
        reference="2",
    )

    assert result is not None
    assert hasattr(result, "score")


@pytest.mark.asyncio
async def test_evaluation_runner():
    """测试评测运行器。"""
    config = EvaluationConfig(
        top_k=5,
        enable_semantic=False,
        enable_llm_judge=False,
        include_qa_results=True,
    )

    runner = EvaluationRunner(config)

    # 准备好接收进度
    progress_updates = []

    async def track_progress(progress: Progress):
        progress_updates.append(progress)

    # 运行评测
    result = await runner.run(
        dataset_id="default",
        knowledge_base_id="test_kb",
        model_id="test_model",
        progress_callback=track_progress,
    )

    assert result is not None
    assert result.evaluation_id is not None
    assert result.total_count > 0
    assert len(progress_updates) > 0


@pytest.mark.asyncio
async def test_zero_shot_strategy():
    """测试零样本策略。"""
    from services.evaluation.strategies import get_strategy

    strategy = get_strategy("zero_shot")

    result = await strategy.execute(
        query="什么是Python？",
        response="Python是一种编程语言。",
        reference="Python是高级编程语言。",
        graders=["rouge_1", "bleu_4"],
    )

    assert result is not None
    assert "rouge_1" in result


@pytest.mark.asyncio
async def test_ensemble_strategy():
    """测试集成策略。"""
    from services.evaluation.strategies import get_strategy

    strategy = get_strategy("ensemble")

    result = await strategy.execute(
        query="什么是Python？",
        response="Python是一种编程语言。",
        reference="Python是高级编程语言。",
        graders=["rouge_1", "bleu_4"],
        aggregation="average",
    )

    assert result is not None
    assert "ensemble_score" in result


@pytest.mark.asyncio
async def test_conditional_strategy():
    """测试条件策略。"""
    from services.evaluation.strategies import get_strategy

    strategy = get_strategy("conditional")

    # 数学问题
    result = await strategy.execute(
        query="1 + 1 等于几？",
        response="等于2",
        reference="2",
    )

    assert result is not None
    assert "question_type" in result
    assert result["question_type"] == "math"


if __name__ == "__main__":
    # 运行测试
    asyncio.run(test_dataset_loading())
    asyncio.run(test_grader_registry())
    asyncio.run(test_rouge_grader())
    asyncio.run(test_exact_match_grader())
    asyncio.run(test_evaluation_runner())
    asyncio.run(test_zero_shot_strategy())
    asyncio.run(test_ensemble_strategy())
    asyncio.run(test_conditional_strategy())
    print("所有测试通过！")
