"""评测指标测试脚本。"""

import asyncio

# ===== 检索评测指标 =====

def test_retrieval_metrics():
    """测试检索评测指标。"""
    from services.evaluation.metrics import (
        precision_at_k,
        recall_at_k,
        ndcg_at_k,
        mrr,
        map_at_k,
        compute_retrieval_metrics,
    )

    print("=== 检索评测指标 ===\n")

    # 示例：检索结果，True 表示相关文档
    retrieved = [True, False, True, False, False, True, False]

    # Precision@K
    for k in [1, 3, 5, 10]:
        p = precision_at_k(retrieved, k)
        print(f"Precision@{k}: {p:.4f}")

    print()

    # Recall@K
    total_relevant = sum(retrieved)
    for k in [1, 3, 5, 10]:
        r = recall_at_k(retrieved, total_relevant, k)
        print(f"Recall@{k}: {r:.4f}")

    print()

    # NDCG@K
    for k in [3, 5, 10]:
        n = ndcg_at_k(retrieved, k)
        print(f"NDCG@{k}: {n:.4f}")

    print()

    # MRR
    m = mrr(retrieved)
    print(f"MRR: {m:.4f}")

    print()

    # MAP
    ap = map_at_k([retrieved], 10)
    print(f"MAP: {ap:.4f}")

    # 使用便捷函数
    print("\n--- 使用便捷函数 ---")
    metrics = compute_retrieval_metrics(retrieved, total_relevant, k=5)
    for key, value in metrics.items():
        print(f"{key}: {value:.4f}")


# ===== 生成评测指标 =====

def test_generation_metrics():
    """测试生成评测指标。"""
    from services.evaluation.metrics import (
        rouge_n,
        rouge_l,
        bleu_at_n,
        compute_generation_metrics,
    )

    print("\n=== 生成评测指标 ===\n")

    reference = "The quick brown fox jumps over the lazy dog."
    hypothesis = "A fast brown fox jumped over a sleeping dog."

    print(f"参考答案: {reference}")
    print(f"生成答案: {hypothesis}\n")

    # ROUGE-N
    print("ROUGE 指标:")
    print(f"  ROUGE-1: {rouge_n(reference, hypothesis, 1):.4f}")
    print(f"  ROUGE-2: {rouge_n(reference, hypothesis, 2):.4f}")
    print(f"  ROUGE-L: {rouge_l(reference, hypothesis):.4f}")

    print()

    # BLEU-N
    print("BLEU 指标:")
    print(f"  BLEU-1: {bleu_at_n(reference, hypothesis, 1):.4f}")
    print(f"  BLEU-2: {bleu_at_n(reference, hypothesis, 2):.4f}")
    print(f"  BLEU-4: {bleu_at_n(reference, hypothesis, 4):.4f}")

    print("\n--- 使用便捷函数 ---")
    metrics = compute_generation_metrics(reference, hypothesis)
    for key, value in metrics.items():
        print(f"{key}: {value:.4f}")


# ===== 语义相似度评测指标 =====

async def test_semantic_metrics():
    """测试语义相似度评测指标。"""
    from services.evaluation.metrics import (
        compute_semantic_metrics,
    )

    print("\n=== 语义相似度评测指标 ===\n")

    references = [
        "中国的首都是北京。",
        "Python 是一种编程语言。",
        "机器学习是人工智能的一个分支。",
    ]

    hypotheses = [
        "北京是中国的首都。",
        "Python 是一门编程语言。",
        "ML 是 AI 的一部分。",
    ]

    print("参考答案 vs 生成答案:")
    for ref, hyp in zip(references, hypotheses):
        print(f"  R: {ref}")
        print(f"  H: {hyp}\n")

    # 使用便捷函数（会尝试加载 sentence-transformers，失败则使用 TF-IDF）
    metrics = compute_semantic_metrics(references, hypotheses)

    print("语义相似度指标:")
    print(f"  平均相似度: {metrics['similarity']:.4f}")
    print(f"  最小相似度: {metrics['min_similarity']:.4f}")
    print(f"  最大相似度: {metrics['max_similarity']:.4f}")


# ===== LLM-as-a-Judge 评测指标 =====

async def test_llm_judge_metrics():
    """测试 LLM 裁判评测指标。"""
    from services.evaluation.metrics import (
        compute_llm_judge_metrics,
    )

    print("\n=== LLM-as-a-Judge 评测指标 ===\n")

    question = "什么是 Python？"
    reference = "Python 是一种高级、解释型的通用编程语言，由 Guido van Rossum 于 1991 年创建。"
    hypothesis = "Python 是一种编程语言，语法简单易学，广泛用于 web 开发、数据分析、人工智能等领域。"

    print(f"问题: {question}")
    print(f"参考答案: {reference}")
    print(f"生成答案: {hypothesis}\n")

    print("注意: LLM 评分需要配置 API 密钥，这里只演示接口调用。")
    print("实际使用时需要设置 OPENAI_API_KEY 环境变量。")

    # 如果没有 API 密钥，跳过实际调用
    import os
    if not os.getenv("OPENAI_API_KEY"):
        print("\n未设置 OPENAI_API_KEY，跳过 LLM 评测。")
        return

    try:
        metrics = compute_llm_judge_metrics(
            reference=reference,
            hypothesis=hypothesis,
            question=question,
            dimensions=["accuracy", "completeness", "clarity"],
        )

        print("LLM 评测指标:")
        print(f"  总分: {metrics['total_score']:.2f}")
        print("  各维度分数:")
        for dim, score in metrics['dimension_scores'].items():
            print(f"    {dim}: {score:.2f}")
    except Exception as e:
        print(f"\nLLM 评测失败: {e}")


# ===== 综合评测示例 =====

async def test_comprehensive_evaluation():
    """综合评测示例。"""
    print("\n=== 综合评测示例 (RAG 系统) ===\n")

    # 示例 RAG 系统评测数据
    samples = [
        {
            "question": "什么是 Python？",
            "retrieved_docs": [True, False, True, False],  # 检索结果相关性
            "reference": "Python 是一种高级编程语言。",
            "hypothesis": "Python 是一门编程语言。",
        },
        {
            "question": "北京是哪个国家的首都？",
            "retrieved_docs": [True, True, False, False],
            "reference": "北京是中国的首都。",
            "hypothesis": "北京是中国的首都。",
        },
    ]

    from services.evaluation.metrics import (
        compute_retrieval_metrics,
        compute_generation_metrics,
    )

    # 检索评测
    print("检索评测:")
    all_retrieved = []
    for s in samples:
        all_retrieved.extend(s["retrieved_docs"])
    total_relevant = sum([sum(s["retrieved_docs"]) for s in samples])

    retrieval_metrics = compute_retrieval_metrics(all_retrieved, total_relevant, k=5)
    for key, value in retrieval_metrics.items():
        print(f"  {key}: {value:.4f}")

    print()

    # 生成评测
    print("生成评测:")
    gen_metrics_list = []
    for s in samples:
        m = compute_generation_metrics(s["reference"], s["hypothesis"])
        gen_metrics_list.append(m)

    # 聚合
    avg_metrics = {
        "avg_rouge_1": sum(m["rouge_1"] for m in gen_metrics_list) / len(gen_metrics_list),
        "avg_rouge_2": sum(m["rouge_2"] for m in gen_metrics_list) / len(gen_metrics_list),
        "avg_rouge_l": sum(m["rouge_l"] for m in gen_metrics_list) / len(gen_metrics_list),
        "avg_bleu_1": sum(m["bleu_1"] for m in gen_metrics_list) / len(gen_metrics_list),
    }
    for key, value in avg_metrics.items():
        print(f"  {key}: {value:.4f}")


async def main():
    """主函数。"""
    # 检索评测（同步）
    test_retrieval_metrics()

    # 生成评测（同步）
    test_generation_metrics()

    # 语义相似度评测（异步）
    await test_semantic_metrics()

    # LLM 评测（异步）
    await test_llm_judge_metrics()

    # 综合评测
    await test_comprehensive_evaluation()

    print("\n=== 测试完成 ===")


if __name__ == "__main__":
    asyncio.run(main())
