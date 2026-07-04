"""评测服务集成测试 - 模拟 Go 服务调用 Python 评测服务。

测试流程:
1. 加载测试数据
2. 调用通用评测入口 /api/evaluate
3. 验证三种评测模式: llm, agent, rag
"""

import asyncio
import json
import sys
from pathlib import Path

# 本文件为端到端手动脚本: 依赖运行中的评测服务 (localhost:8000),
# check_* 函数接收位置参数而非 pytest fixture, 仅经 __main__ 手动运行,
# 刻意不用 test_ 前缀命名, 避免 pytest（含 integration 跑）收集报缺 fixture。
# 设置项目根目录
PROJECT_ROOT = str(Path(__file__).resolve().parents[2])
sys.path.insert(0, PROJECT_ROOT)

import httpx


# ============================================================
# 配置
# ============================================================

BASE_URL = "http://localhost:8000"
TEST_DATA_FILE = Path(PROJECT_ROOT) / "tests" / "evaluation" / "test_data.json"


# ============================================================
# 测试函数
# ============================================================

async def check_llm_evaluation(client: httpx.AsyncClient, test_data: dict) -> bool:
    """测试 LLM 评测模式。"""
    print("\n" + "=" * 60)
    print("Test 1: LLM Evaluation Mode")
    print("=" * 60)

    payload = test_data["llm_evaluation"]
    print(f"\nRequest:")
    print(f"  Mode: {payload['mode']}")
    print(f"  Samples: {len(payload['data']['outputs'])}")
    print(f"  Metrics: {', '.join(payload['metrics'])}")

    try:
        response = await client.post(f"{BASE_URL}/api/evaluate", json=payload)
        response.raise_for_status()
        result = response.json()

        print(f"\nResponse:")
        print(f"  Status: {result.get('status')}")
        print(f"  Mode: {result.get('mode')}")

        if "metrics" in result:
            print(f"\n  Metrics:")
            for metric_name, metric_value in result["metrics"].items():
                if isinstance(metric_value, dict):
                    print(f"    {metric_name}:")
                    for k, v in metric_value.items():
                        print(f"      {k}: {v}")
                else:
                    print(f"    {metric_name}: {metric_value}")

        print("\n[OK] LLM evaluation test passed")
        return True

    except Exception as e:
        print(f"\n[FAILED] LLM evaluation test failed: {e}")
        if hasattr(e, "response"):
            print(f"Response: {e.response.text}")
        return False


async def check_agent_evaluation(client: httpx.AsyncClient, test_data: dict) -> bool:
    """测试 Agent 评测模式。"""
    print("\n" + "=" * 60)
    print("Test 2: Agent Evaluation Mode")
    print("=" * 60)

    payload = test_data["agent_evaluation"]
    print(f"\nRequest:")
    print(f"  Mode: {payload['mode']}")
    print(f"  Samples: {len(payload['data']['outputs'])}")
    print(f"  Metrics: {', '.join(payload['metrics'])}")

    try:
        response = await client.post(f"{BASE_URL}/api/evaluate", json=payload)
        response.raise_for_status()
        result = response.json()

        print(f"\nResponse:")
        print(f"  Status: {result.get('status')}")
        print(f"  Mode: {result.get('mode')}")

        if "metrics" in result:
            print(f"\n  Metrics:")
            for metric_name, metric_value in result["metrics"].items():
                if isinstance(metric_value, dict):
                    print(f"    {metric_name}:")
                    for k, v in metric_value.items():
                        print(f"      {k}: {v}")
                else:
                    print(f"    {metric_name}: {metric_value}")

        print("\n[OK] Agent evaluation test passed")
        return True

    except Exception as e:
        print(f"\n[FAILED] Agent evaluation test failed: {e}")
        if hasattr(e, "response"):
            print(f"Response: {e.response.text}")
        return False


async def check_rag_evaluation(client: httpx.AsyncClient, test_data: dict) -> bool:
    """测试 RAG 评测模式。"""
    print("\n" + "=" * 60)
    print("Test 3: RAG Evaluation Mode")
    print("=" * 60)

    payload = test_data["rag_evaluation"]
    print(f"\nRequest:")
    print(f"  Mode: {payload['mode']}")
    print(f"  Samples: {len(payload['data']['outputs'])}")
    print(f"  Corpus size: {len(payload['data']['corpus'])}")
    print(f"  Metrics: {', '.join(payload['metrics'])}")

    try:
        response = await client.post(f"{BASE_URL}/api/evaluate", json=payload)
        response.raise_for_status()
        result = response.json()

        print(f"\nResponse:")
        print(f"  Status: {result.get('status')}")
        print(f"  Mode: {result.get('mode')}")

        if "metrics" in result:
            print(f"\n  Metrics:")
            for metric_name, metric_value in result["metrics"].items():
                if isinstance(metric_value, dict):
                    print(f"    {metric_name}:")
                    for k, v in metric_value.items():
                        print(f"      {k}: {v}")
                else:
                    print(f"    {metric_name}: {metric_value}")

        print("\n[OK] RAG evaluation test passed")
        return True

    except Exception as e:
        print(f"\n[FAILED] RAG evaluation test failed: {e}")
        if hasattr(e, "response"):
            print(f"Response: {e.response.text}")
        return False


async def check_separate_endpoints(client: httpx.AsyncClient, test_data: dict) -> bool:
    """测试单独评测端点。"""
    print("\n" + "=" * 60)
    print("Test 4: Separate Endpoints")
    print("=" * 60)

    all_passed = True

    # 测试 LLM 单独端点
    print("\n  4.1 Testing /api/evaluate/llm...")
    try:
        response = await client.post(
            f"{BASE_URL}/api/evaluate/llm",
            json=test_data["llm_api_test"]
        )
        response.raise_for_status()
        result = response.json()
        print(f"      Status: {result.get('status')}")
        print(f"      [OK] LLM endpoint works")
    except Exception as e:
        print(f"      [FAILED] {e}")
        all_passed = False

    # 测试 Agent 单独端点
    print("\n  4.2 Testing /api/evaluate/agent...")
    try:
        response = await client.post(
            f"{BASE_URL}/api/evaluate/agent",
            json=test_data["agent_api_test"]
        )
        response.raise_for_status()
        result = response.json()
        print(f"      Status: {result.get('status')}")
        print(f"      [OK] Agent endpoint works")
    except Exception as e:
        print(f"      [FAILED] {e}")
        all_passed = False

    # 测试 RAG 单独端点
    print("\n  4.3 Testing /api/evaluate/rag...")
    try:
        response = await client.post(
            f"{BASE_URL}/api/evaluate/rag",
            json=test_data["rag_api_test"]
        )
        response.raise_for_status()
        result = response.json()
        print(f"      Status: {result.get('status')}")
        print(f"      [OK] RAG endpoint works")
    except Exception as e:
        print(f"      [FAILED] {e}")
        all_passed = False

    if all_passed:
        print("\n[OK] All separate endpoints test passed")
    else:
        print("\n[FAILED] Some separate endpoints failed")
    return all_passed


async def check_edge_cases(client: httpx.AsyncClient, test_data: dict) -> bool:
    """测试边界情况。"""
    print("\n" + "=" * 60)
    print("Test 5: Edge Cases")
    print("=" * 60)

    edge_cases = test_data["edge_cases"]
    all_passed = True

    for name, payload in edge_cases.items():
        print(f"\n  Testing edge case: {name}...")
        try:
            response = await client.post(f"{BASE_URL}/api/evaluate", json=payload)
            response.raise_for_status()
            result = response.json()
            print(f"    Status: {result.get('status')}")
            print(f"    [OK] {name} handled")
        except Exception as e:
            print(f"    [FAILED] {name}: {e}")
            all_passed = False

    if all_passed:
        print("\n[OK] All edge cases handled correctly")
    else:
        print("\n[WARNING] Some edge cases failed (may be expected)")

    return all_passed


# ============================================================
# 主测试流程
# ============================================================

async def main():
    """主测试流程。"""
    print("=" * 60)
    print("Evaluation Service Integration Test")
    print("=" * 60)
    print(f"\nBase URL: {BASE_URL}")
    print(f"Test Data: {TEST_DATA_FILE}")

    # 加载测试数据
    print("\nLoading test data...")
    try:
        with open(TEST_DATA_FILE, "r", encoding="utf-8") as f:
            test_data = json.load(f)
        print("[OK] Test data loaded")
    except Exception as e:
        print(f"[FAILED] Failed to load test data: {e}")
        return False

    # 检查服务是否运行
    print("\nChecking service health...")
    async with httpx.AsyncClient(timeout=10.0) as client:
        try:
            response = await client.get(f"{BASE_URL}/health")
            if response.status_code == 200:
                print("[OK] Service is running")
            else:
                print(f"[WARNING] Health check returned: {response.status_code}")
        except Exception as e:
            print(f"[FAILED] Service is not running: {e}")
            print("\nPlease start the service first:")
            print("  python -m uvicorn services.evaluation.fastapi_app:app --reload")
            return False

        # 运行测试
        results = []

        results.append(await check_llm_evaluation(client, test_data))
        results.append(await check_agent_evaluation(client, test_data))
        results.append(await check_rag_evaluation(client, test_data))
        results.append(await check_separate_endpoints(client, test_data))
        results.append(await check_edge_cases(client, test_data))

    # 总结
    print("\n" + "=" * 60)
    print("Test Summary")
    print("=" * 60)
    passed = sum(results)
    total = len(results)
    print(f"\nPassed: {passed}/{total}")

    if passed == total:
        print("\n[SUCCESS] All tests passed!")
        return True
    else:
        print(f"\n[FAILED] {total - passed} test(s) failed")
        return False


if __name__ == "__main__":
    success = asyncio.run(main())
    sys.exit(0 if success else 1)
