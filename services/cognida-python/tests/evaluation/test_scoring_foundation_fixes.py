"""计分地基坏打分器修复回归测试。

覆盖本轮修复的五类 P0 缺陷（见 memory: agent-eval-scoring-defects）：
1. 中文分词全线失效：generation.py(ROUGE/BLEU) 与 rag.py(faithfulness/context_relevance)
   旧用 \\w+/空白切词，无空格中文整句塌成 1 token → 恒近 0。
2. 语义相似度评分器：签名/返回类型对不上 + _model_cache 未初始化 → 恒 0（此处只验非崩+落分）。
3. 检索评分器把 1D 相关性标签误包成 2D → precision/mrr 恒 100%、ndcg 崩成 0。
4. LLM 裁判解析抓正文第一个 1-5 字符 → 误读年份/序号。
5. 数值型 answer_accuracy 仅靠语义相似度 → 数值全对但表述不同被压分。
"""

import pytest

from services.evaluation.graders.builtin.llm import extract_score_1_5
from services.evaluation.graders.builtin.retrieval import (
    MRRGrader,
    NDCGGrader,
    PrecisionGrader,
)
from services.evaluation.metrics.agent import (
    _numeric_coverage,
    _salient_tokens,
    answer_accuracy_items,
)
from services.evaluation.metrics.generation import bleu_at_n, rouge_l, rouge_n
from services.evaluation.metrics.rag import context_relevance, faithfulness


# ============================================================
# 1. 中文分词：ROUGE/BLEU/faithfulness 在无空格中文上非零
# ============================================================

def test_chinese_rouge_nonzero_for_paraphrase():
    ref = "爱因斯坦提出了相对论并获得诺贝尔奖"
    hyp = "相对论是爱因斯坦提出的，他还获得了诺贝尔奖"
    assert rouge_n(ref, hyp, 1) > 0.3
    assert rouge_l(ref, hyp) > 0.2


def test_chinese_rouge_identical_is_one():
    s = "上个月的销售额是一百二十八万元"
    assert rouge_n(s, s, 1) == pytest.approx(1.0)
    assert rouge_l(s, s) == pytest.approx(1.0)
    assert bleu_at_n(s, s, 1) == pytest.approx(1.0)


def test_chinese_rouge_disjoint_is_low():
    # 完全不相关的两句：分词后词集合几乎不相交，应接近 0
    assert rouge_n("今天天气很好", "数据库连接超时", 1) < 0.2


def test_chinese_faithfulness_and_relevance_nonzero():
    answers = ["订单总数为六万单，客单价一百二十八元"]
    contexts = [["本月订单总数六万单", "平均客单价一百二十八元人民币"]]
    assert faithfulness(answers, contexts) > 0.5

    questions = ["本月客单价是多少"]
    assert context_relevance(questions, contexts) > 0.3


# ============================================================
# 3. 检索评分器：1D 标签不再被误包成 2D
# ============================================================

async def test_precision_grader_not_stuck_at_100():
    grader = PrecisionGrader()
    # 5 个结果里只有 1 个相关 → precision@5 = 0.2（旧 bug 恒 1.0/100）
    res = await grader.aevaluate(query="q", response="r", retrieved_relevant=[1, 0, 0, 0, 0], k=5)
    assert res.metrics["precision_raw"] == pytest.approx(0.2)
    assert res.metrics["precision"] == pytest.approx(20.0)


async def test_precision_grader_all_relevant_is_100():
    grader = PrecisionGrader()
    res = await grader.aevaluate(query="q", response="r", retrieved_relevant=[1, 1, 1], k=3)
    assert res.metrics["precision_raw"] == pytest.approx(1.0)


async def test_mrr_grader_reflects_rank():
    grader = MRRGrader()
    # 首个相关文档排在第 3 位 → MRR = 1/3（旧 bug 恒 1.0）
    res = await grader.aevaluate(query="q", response="r", retrieved_relevant=[0, 0, 1])
    assert res.metrics["mrr_raw"] == pytest.approx(1.0 / 3.0)


async def test_ndcg_grader_no_crash_and_orders_matter():
    grader = NDCGGrader()
    # 相关文档靠前 → NDCG=1.0；靠后 → <1（旧 bug：2D 入参触发 TypeError → GraderError）
    good = await grader.aevaluate(query="q", response="r", retrieved_relevant=[1, 0, 0])
    bad = await grader.aevaluate(query="q", response="r", retrieved_relevant=[0, 0, 1])
    assert good.metrics["ndcg_raw"] == pytest.approx(1.0)
    assert bad.metrics["ndcg_raw"] < good.metrics["ndcg_raw"]
    assert 0.0 < bad.metrics["ndcg_raw"] < 1.0


# ============================================================
# 4. LLM 裁判分数抽取：不误读年份/序号
# ============================================================

def test_extract_plain_number():
    assert extract_score_1_5("4") == 4.0
    assert extract_score_1_5("评分：3.5") == 3.5


def test_extract_ignores_year():
    assert extract_score_1_5("该产品于 2024 年发布，答案完全正确，评分 5") == 5.0


def test_extract_prefers_labeled_over_leading_count():
    # 正文先出现"5 项检查"，真正的分数是末尾的 4
    assert extract_score_1_5("根据 5 项检查标准，本题得分为 4 分") == 4.0


def test_extract_x_over_5_format():
    assert extract_score_1_5("综合评估：4/5，基本正确") == 4.0


def test_extract_out_of_range_returns_none():
    assert extract_score_1_5("参考了 2024 年的 100 份文档") is None
    assert extract_score_1_5("没有任何数字") is None


# ============================================================
# 5. 数值感知 answer_accuracy：数值/ID 全对不被语义相似度压分
# ============================================================

def test_salient_tokens_extracts_numbers_and_ids():
    toks = _salient_tokens("订单 E2E001 金额 1,280.5 元，共 60000 单")
    assert "e2e001" in toks
    assert "1280.5" in toks
    assert "60000" in toks


def test_salient_tokens_id_digits_not_double_counted():
    # E2E001 里的 001 不应作为独立数字 "1" 混入
    assert "1" not in _salient_tokens("单号 E2E001")


def test_numeric_coverage_full_hit():
    ref = "客单价 128.5 元，共 60000 单"
    out = "本月一共 60000 单，平均每单 128.5 元"
    assert _numeric_coverage(ref, out) == pytest.approx(1.0)


def test_numeric_coverage_none_when_no_numbers():
    assert _numeric_coverage("答案完全正确", "对的") is None


def test_answer_accuracy_floored_by_numeric_coverage():
    # 数值全对但语序/措辞不同：应因数值覆盖率兜底而拿到高分
    refs = ["2023 年第三季度营收为 384400 万元"]
    outs = ["三季度（2023）的营业收入是 384400 万元人民币"]
    acc = answer_accuracy_items(refs, outs)[0]
    assert acc == pytest.approx(1.0)
