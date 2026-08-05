"""RAG 评测指标。"""

from typing import Any, Dict, List

from .retrieval import compute_retrieval_metrics
from .generation import compute_generation_metrics
from .semantic import compute_semantic_metrics
from .tokenizer import remove_punctuation, tokenize


def _word_set(text: str) -> set:
    """把文本切成去重词集合（中文 jieba、英文按词、大小写归一）。

    历史缺陷：faithfulness/context_relevance 用 re.findall(r"\\w+") 抽词，中文没有空格，
    一整段中文会被 \\w+ 当作 1 个"词"，词集合几乎不可能相交 → 忠实度/相关性恒近 0。
    改用 tokenizer.tokenize 做真正的中文分词。
    """
    return {t for t in (tok.strip().lower() for tok in tokenize(remove_punctuation(text), language="auto")) if t}


def faithfulness(
    answers: List[str],
    retrieved_contexts: List[List[str]],
) -> float:
    """计算忠实度（答案是否基于检索内容）。

    使用简单策略：答案中的关键词在检索内容中出现比例

    Args:
        answers: 生成的答案列表
        retrieved_contexts: 检索到的上下文列表（每个答案对应的检索内容）

    Returns:
        忠实度 (0-1)
    """
    if len(answers) != len(retrieved_contexts):
        raise ValueError("answers 和 retrieved_contexts 长度必须相同")

    if not answers:
        return 0.0

    total_faithfulness = 0.0

    for answer, contexts in zip(answers, retrieved_contexts):
        if not answer or not contexts:
            continue

        # 提取答案中的关键词（中文分词后的词集合）
        words = _word_set(answer)

        if not words:
            continue

        # 构建检索内容的词集合
        context_words = _word_set(" ".join(contexts))

        # 计算答案中有多少词出现在检索内容中
        overlap = len(words & context_words)
        faith = overlap / len(words) if words else 0.0

        total_faithfulness += faith

    return total_faithfulness / len(answers) if answers else 0.0


def context_relevance(
    questions: List[str],
    retrieved_contexts: List[List[str]],
) -> float:
    """计算上下文相关性（检索内容与问题的相关性）。

    使用简单策略：问题关键词在检索内容中出现比例

    Args:
        questions: 问题列表
        retrieved_contexts: 检索到的上下文列表

    Returns:
        上下文相关性 (0-1)
    """
    if len(questions) != len(retrieved_contexts):
        raise ValueError("questions 和 retrieved_contexts 长度必须相同")

    if not questions:
        return 0.0

    total_relevance = 0.0

    for question, contexts in zip(questions, retrieved_contexts):
        if not question or not contexts:
            continue

        # 提取问题中的关键词（中文分词后的词集合）
        question_words = _word_set(question)

        if not question_words:
            continue

        # 构建检索内容的词集合
        context_words = _word_set(" ".join(contexts))

        # 计算问题中有多少词在检索内容中
        overlap = len(question_words & context_words)
        relevance = overlap / len(question_words) if question_words else 0.0

        total_relevance += relevance

    return total_relevance / len(questions) if questions else 0.0


def noise_ratio(
    retrieved_contexts: List[List[str]],
    relevant_doc_ids: List[List[str]],
    retrieved_doc_ids: List[List[str]],
) -> float:
    """计算噪声比例（检索到的不相关文档比例）。

    Args:
        retrieved_contexts: 检索到的上下文列表（未使用，保持接口一致）
        relevant_doc_ids: 每个问题对应的相关文档 ID 列表
        retrieved_doc_ids: 每个问题实际检索到的文档 ID 列表

    Returns:
        噪声比例 (0-1)
    """
    if len(relevant_doc_ids) != len(retrieved_doc_ids):
        raise ValueError("relevant_doc_ids 和 retrieved_doc_ids 长度必须相同")

    if not retrieved_doc_ids:
        return 0.0

    total_retrieved = 0
    total_noise = 0

    for relevant, retrieved in zip(relevant_doc_ids, retrieved_doc_ids):
        relevant_set = set(relevant)
        total_retrieved += len(retrieved)

        # 计算不相关文档数
        noise = len([doc_id for doc_id in retrieved if doc_id not in relevant_set])
        total_noise += noise

    return total_noise / total_retrieved if total_retrieved > 0 else 0.0


def compute_rag_metrics(
    questions: List[str],
    references: List[Dict[str, Any]],
    outputs: List[Dict[str, Any]],
    corpus: List[Dict[str, Any]] | None = None,
    metrics: List[str] | None = None,
) -> Dict[str, Any]:
    """计算 RAG 评测指标（便捷函数）。

    Args:
        questions: 问题列表
        references: 参考列表，每项包含 answer, relevant_docs
        outputs: 输出列表，每项包含 answer, retrieved_docs
        corpus: 文档语料（用于计算检索质量）
        metrics: 需要计算的指标列表

    Returns:
        RAG 评测指标结果
    """
    if metrics is None:
        metrics = ["retrieval_precision", "faithfulness"]

    result = {}

    # 提取数据
    answers = [o.get("answer", "") for o in outputs]
    ref_answers = [r.get("answer", "") for r in references]
    retrieved_doc_ids = [o.get("retrieved_docs", []) for o in outputs]
    relevant_doc_ids = [r.get("relevant_docs", []) for r in references]

    # 答案质量
    if any(m in metrics for m in ["answer_quality", "rouge", "bleu", "semantic"]):
        result["answer_quality"] = {}

        if "rouge" in metrics or "answer_quality" in metrics:
            gen = compute_generation_metrics(ref_answers[0], answers[0]) if ref_answers and answers else {}
            result["answer_quality"]["rouge_l"] = gen.get("rouge_l", 0)

        if "bleu" in metrics or "answer_quality" in metrics:
            gen = compute_generation_metrics(ref_answers[0], answers[0]) if ref_answers and answers else {}
            result["answer_quality"]["bleu_4"] = gen.get("bleu_4", 0)

        if "semantic" in metrics or "answer_quality" in metrics:
            sem = compute_semantic_metrics(ref_answers, answers)
            result["answer_quality"]["semantic_similarity"] = sem.get("similarity", 0)

    # 检索指标（compute_retrieval_metrics 是“逐条”接口：入参为单条的布尔相关性序列，
    # 而非文档ID二维列表。此处按问题逐条计算再求均值。）
    if any(m.startswith("retrieval_") for m in metrics):
        sums = {"precision": 0.0, "recall": 0.0, "ndcg": 0.0, "mrr": 0.0, "map": 0.0}
        n = 0
        for relevant, retrieved in zip(relevant_doc_ids, retrieved_doc_ids):
            relevant_set = set(relevant)
            retrieved_bool = [doc_id in relevant_set for doc_id in retrieved]
            ret = compute_retrieval_metrics(
                retrieved_bool,
                total_relevant=len(relevant_set),
                k=len(retrieved_bool),
            )
            sums["precision"] += ret.get("precision", 0.0)
            sums["recall"] += ret.get("recall", 0.0)
            sums["ndcg"] += ret.get("ndcg", 0.0)
            sums["mrr"] += ret.get("mrr", 0.0)
            sums["map"] += ret.get("map_score", 0.0)
            n += 1
        result["retrieval"] = {k: (v / n if n > 0 else 0.0) for k, v in sums.items()}

    # RAG 特有指标
    rag_specific = {}

    # faithfulness/context_relevance 需要每条问题一份上下文（List[List[str]]），
    # 与 answers/questions 逐条对齐，而非把全部文档拍平成一份。
    retrieved_contexts = [
        list(docs) if isinstance(docs, list) else [str(docs)]
        for docs in retrieved_doc_ids
    ]

    if "faithfulness" in metrics:
        rag_specific["faithfulness"] = faithfulness(answers, retrieved_contexts)

    if "context_relevance" in metrics:
        rag_specific["context_relevance"] = context_relevance(questions, retrieved_contexts)

    if "noise_ratio" in metrics:
        rag_specific["noise_ratio"] = noise_ratio(
            retrieved_contexts=retrieved_contexts,
            relevant_doc_ids=relevant_doc_ids,
            retrieved_doc_ids=retrieved_doc_ids,
        )

    if rag_specific:
        result["rag_specific"] = rag_specific

    return result
