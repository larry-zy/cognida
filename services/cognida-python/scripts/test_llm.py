"""测试 LLM 连接。"""

import os
import sys

# 设置项目根目录
PROJECT_ROOT = "D:\\link\\cognida-python"
sys.path.insert(0, PROJECT_ROOT)

# 设置环境变量（直接设置）
os.environ["LLM_PROVIDER"] = "deepseek"
os.environ["LLM_MODEL"] = "deepseek-v4-flash"
os.environ["LLM_API_KEY"] = "sk-34865eb399c54e11862853e370205cca"
os.environ["LLM_BASE_URL"] = "https://api.deepseek.com"

from services.evaluation.llm import get_llm_client


def test_llm_connection():
    """测试 LLM 连接。"""
    print("=" * 50)
    print("Testing LLM Connection")
    print("=" * 50)

    # 显示配置
    provider = os.getenv("LLM_PROVIDER", "deepseek")
    model = os.getenv("LLM_MODEL", "deepseek-v4-flash")
    base_url = os.getenv("LLM_BASE_URL", "")
    api_key = os.getenv("LLM_API_KEY", "") or os.getenv("ANTHROPIC_AUTH_TOKEN", "")

    print(f"\nConfig:")
    print(f"  Provider: {provider}")
    print(f"  Model: {model}")
    print(f"  Base URL: {base_url}")
    print(f"  API Key: {api_key[:10]}...{api_key[-4:] if api_key else 'None'}")

    try:
        # 获取客户端
        client = get_llm_client(provider=provider, model=model)
        print(f"\n[OK] Client created")

        # 测试简单生成
        print(f"\nTest 1: Simple generation")
        print(f"  Prompt: Hi")
        response = client.generate("Hello, please introduce yourself in one sentence.")
        print(f"  Response: {response}")
        print(f"  [OK] Text generation works")

        # 测试 JSON 生成
        print(f"\nTest 2: JSON generation")
        result = client.generate_json(
            prompt='Return a JSON with fields: name="test", value=100',
            system_prompt='You are a JSON generator. Only return valid JSON format.'
        )
        print(f"  Response: {result}")
        print(f"  [OK] JSON generation works")

        # 测试 LLM-as-Judge
        print(f"\nTest 3: LLM-as-Judge scoring")
        from services.evaluation.metrics import compute_llm_judge_metrics

        result = compute_llm_judge_metrics(
            reference="Beijing",
            hypothesis="The capital of China is Beijing",
            question="What is the capital of China?",
            dimensions=["accuracy"]
        )
        print(f"  Reference: Beijing")
        print(f"  Hypothesis: The capital of China is Beijing")
        print(f"  Question: What is the capital of China?")
        print(f"  Scores: {result}")
        print(f"  [OK] Judge scoring works")

        print(f"\n" + "=" * 50)
        print(f"[SUCCESS] All tests passed! LLM connection is working")
        print("=" * 50)

    except Exception as e:
        print(f"\n[FAILED] Test error: {e}")
        import traceback
        traceback.print_exc()
        return False

    return True


if __name__ == "__main__":
    success = test_llm_connection()
    sys.exit(0 if success else 1)
