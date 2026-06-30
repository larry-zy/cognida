"""评测服务测试脚本。"""

import asyncio
import os
from pathlib import Path

from services.judge import get_dataset_manager
from services.judge.models import JudgeTask, TaskStatus


async def test_dataset_manager() -> None:
    """测试数据集管理器。"""
    print("=== 测试数据集管理器 ===")

    manager = get_dataset_manager()

    # 列出数据集
    datasets = manager.list_datasets()
    print(f"找到 {len(datasets)} 个数据集:")
    for ds in datasets:
        print(f"  - {ds.dataset_id}: {ds.name}")

    # 加载数据集
    if datasets:
        dataset = manager.get_dataset(datasets[0].dataset_id)
        if dataset:
            print(f"\n数据集详情: {dataset.meta.name}")
            print(f"  样本数: {dataset.sample_count}")
            print(f"  字段: {dataset.meta.fields}")
            print(f"  前3个样本:")
            for sample in dataset.samples[:3]:
                print(f"    {sample.data}")


async def test_graders() -> None:
    """测试评分器。"""
    print("\n=== 测试评分器 ===")

    from services.judge.graders import EXACT_MATCH, CONTAINS, REGEX, NUMERIC

    # 精确匹配
    score, reason, passed = await EXACT_MATCH.score(
        model_output="北京",
        reference="北京",
    )
    print(f"精确匹配: score={score}, passed={passed}, reason={reason}")

    # 包含匹配
    score, reason, passed = await CONTAINS.score(
        model_output="北京是中国的首都",
        reference="北京|首都",
    )
    print(f"包含匹配: score={score}, passed={passed}, reason={reason}")

    # 正则匹配
    score, reason, passed = await REGEX.score(
        model_output="答案是42",
        reference=r"答案是\s*\d+",
    )
    print(f"正则匹配: score={score}, passed={passed}, reason={reason}")

    # 数值比较
    score, reason, passed = await NUMERIC.score(
        model_output="计算结果是 3.1416",
        reference="3.14",
        tolerance=0.01,
    )
    print(f"数值比较: score={score}, passed={passed}, reason={reason}")


async def test_llm_client() -> None:
    """测试 LLM 客户端。"""
    print("\n=== 测试 LLM 客户端 ===")

    from services.judge.client import create_llm_client

    api_key = os.getenv("OPENAI_API_KEY")
    if not api_key:
        print("未设置 OPENAI_API_KEY，跳过 LLM 测试")
        return

    client = create_llm_client(
        provider="openai",
        api_key=api_key,
        model="gpt-4o-mini",
    )

    try:
        output, metadata = await client.generate(
            prompt="1 + 1 等于几？请只回答数字。",
            max_tokens=10,
        )
        print(f"LLM 输出: {output}")
        print(f"元数据: {metadata}")
    finally:
        await client.close()


async def test_executor() -> None:
    """测试执行器。"""
    print("\n=== 测试执行器 ===")

    from services.judge.executor import JudgeExecutor

    # 创建测试任务
    task = JudgeTask(
        task_id="test-001",
        dataset_id="sample_qa",
        model_config={
            "provider": "mock",  # 使用模拟客户端
            "model": "mock",
        },
        scoring_method={
            "type": "exact",
        },
        timeout=60,
    )

    executor = JudgeExecutor()

    # 使用模拟的 LLM 客户端
    from services.judge.client import LLMClient

    class MockLLMClient(LLMClient):
        name = "mock"

        async def generate(self, prompt: str, **kwargs):
            # 简单的模拟响应
            question = prompt.split("问题：")[-1].strip() if "问题：" in prompt else prompt
            if "北京" in question:
                return "北京", {}
            if "1 + 1" in question:
                return "2", {}
            return "模拟回答", {}

        async def generate_json(self, prompt: str, **kwargs):
            return {}, {}

        def get_token_usage(self, metadata):
            return {"prompt_tokens": 10, "completion_tokens": 5}

        async def close(self):
            pass

    # 替换 LLM 客户端创建
    import services.judge.executor.executor as executor_module
    original_create = executor_module.create_llm_client
    executor_module.create_llm_client = lambda **kw: MockLLMClient()

    try:
        result = await executor.execute_task(task)
        print(f"评测完成:")
        print(f"  总样本数: {result.total_samples}")
        print(f"  已评分: {result.scored_samples}")
        print(f"  平均分: {result.avg_score:.2f}")
        print(f"  通过率: {result.pass_rate:.2%}")
    finally:
        executor_module.create_llm_client = original_create


async def main() -> None:
    """主函数。"""
    await test_dataset_manager()
    await test_graders()
    await test_llm_client()
    await test_executor()

    print("\n=== 测试完成 ===")


if __name__ == "__main__":
    asyncio.run(main())
