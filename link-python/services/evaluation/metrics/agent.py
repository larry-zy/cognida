"""Agent 评测指标。"""

import re
from typing import Any, List, Optional

from .generation import compute_generation_metrics
from .semantic import compute_semantic_metrics

# 答案准确性校准区间：语义相似度低于 LOW 记 0 分，高于 HIGH 记满分，
# 区间内线性给分。相比旧的「> 0.8 记 1 否则 0」二值判定，分级打分能
# 区分「基本答对但表述不同」与「完全答错」，避免评分非 0 即 1 的塌缩。
_ACC_SIM_LOW = 0.45
_ACC_SIM_HIGH = 0.85


def _graded_accuracy(sim: float) -> float:
    """把语义相似度映射为 0-1 的分级准确度（分段线性 + 截断）。"""
    if sim <= _ACC_SIM_LOW:
        return 0.0
    if sim >= _ACC_SIM_HIGH:
        return 1.0
    return (sim - _ACC_SIM_LOW) / (_ACC_SIM_HIGH - _ACC_SIM_LOW)


def answer_accuracy_items(
    references: List[str],
    outputs: List[str],
) -> List[float]:
    """逐样本的分级准确度（0-1），供逐条落库；聚合值即其均值。"""
    if len(references) != len(outputs):
        raise ValueError("references 和 outputs 长度必须相同")
    return [
        _graded_accuracy(compute_semantic_metrics([ref], [out]).get("similarity", 0.0))
        for ref, out in zip(references, outputs)
    ]


def answer_accuracy(
    references: List[str],
    outputs: List[str],
) -> float:
    """计算答案准确性（分级语义匹配，返回平均得分 0-1）。

    Args:
        references: 参考答案列表
        outputs: Agent 输出列表

    Returns:
        平均准确率 (0-1)
    """
    if not references:
        return 0.0
    per = answer_accuracy_items(references, outputs)
    return sum(per) / len(per)


# 工具名归一化别名表：把 Agent 实际上报的调用名映射到数据集里标注的规范名，
# 避免大小写/连字符/同义命名差异导致集合命中恒为空（旧实现直接 set 精确求交）。
_TOOL_ALIASES = {
    "sqlexecute": "sql_execute",
    "executesql": "sql_execute",
    "runsql": "sql_execute",
    "sqlquery": "sql_execute",
    "query": "sql_execute",
    "getschema": "get_schema",
    "schema": "get_schema",
    "describeschema": "get_schema",
    "dataanalysis": "data_analysis",
    "analyze": "data_analysis",
    "analysis": "data_analysis",
    "renderui": "render_ui",
    "render": "render_ui",
    "chart": "render_ui",
    "semanticquery": "semantic_query",
    "semanticmodels": "semantic_models",
    "groundterms": "ground_terms",
    "graphquery": "graph_query",
    "sqlmutate": "sql_mutate",
    "skillinvoke": "skill_invoke",
    "skilllist": "skill_list",
}


def _normalize_tool(name: str) -> str:
    """归一化单个工具名：小写、去空白、去分隔符后按别名表折叠。"""
    if not name:
        return ""
    key = re.sub(r"[\s_\-./]+", "", str(name).strip().lower())
    return _TOOL_ALIASES.get(key, re.sub(r"[\s\-./]+", "_", str(name).strip().lower()))


def _normalize_tools(names: List[str]) -> List[str]:
    """归一化工具名序列并丢弃空项（保持原有顺序，供 tool_order 使用）。"""
    return [n for n in (_normalize_tool(x) for x in (names or [])) if n]


def tool_selection_items(
    expected_tools: List[List[str]],
    used_tools: List[List[str]],
) -> List[Optional[dict[str, float]]]:
    """逐样本的工具选择 P/R/F1（供逐条落库）。

    无期望工具标注的样本返回 None，表示"不参与统计"——聚合时跳过，避免 0 分拉低均值
    （与 tool_order 口径一致）。工具名先归一化，消除大小写/连字符/同义命名差异导致的假不命中。
    """
    if len(expected_tools) != len(used_tools):
        raise ValueError("expected_tools 和 used_tools 长度必须相同")

    out: List[Optional[dict[str, float]]] = []
    for expected, used in zip(expected_tools, used_tools):
        expected_set = set(_normalize_tools(expected))
        if not expected_set:
            out.append(None)
            continue
        used_set = set(_normalize_tools(used))
        match = expected_set & used_set

        p = len(match) / len(used_set) if used_set else 0.0
        r = len(match) / len(expected_set)
        f = 2 * p * r / (p + r) if (p + r) > 0 else 0.0
        out.append({"precision": p, "recall": r, "f1": f})
    return out


def tool_selection(
    expected_tools: List[List[str]],
    used_tools: List[List[str]],
) -> dict[str, float]:
    """计算工具选择评分。

    Args:
        expected_tools: 期望使用的工具列表（每个样本的期望工具）
        used_tools: 实际使用的工具列表（每个样本的实际工具）

    Returns:
        包含 precision, recall, f1 的字典
    """
    if not expected_tools:
        return {"precision": 0.0, "recall": 0.0, "f1": 0.0}

    # 逐样本计算 P/R/F1 再宏平均（与 tool_order/trajectory_match/answer_accuracy 的逐样本
    # 聚合口径一致）。旧实现把所有样本的工具并成全局集合再算一次 P/R——单条过度调用
    # （如"库里有哪些订单表"这类探查题会调 10+ 个工具）会把全局 used 集合撑大，导致
    # 全局 precision 被个别稀有工具永久拉低、与逐条实际表现脱节（无论 golden 多准都压不上去）。
    scored = [x for x in tool_selection_items(expected_tools, used_tools) if x is not None]
    if not scored:
        return {"precision": 0.0, "recall": 0.0, "f1": 0.0}
    return {
        "precision": sum(x["precision"] for x in scored) / len(scored),
        "recall": sum(x["recall"] for x in scored) / len(scored),
        "f1": sum(x["f1"] for x in scored) / len(scored),
    }


def _step_text(step: Any) -> str:
    """从单个轨迹步骤提取可比文本，兼容字符串与 {content/name/tool} 字典。"""
    if isinstance(step, str):
        return step
    if isinstance(step, dict):
        return str(step.get("content") or step.get("name") or step.get("tool") or "")
    return str(step or "")


def _is_ordered_subsequence(expected: List[str], actual: List[str]) -> bool:
    """判断 expected 是否为 actual 的有序子序列（保持相对顺序，允许中间插入其他调用）。"""
    it = iter(actual)
    return all(tool in it for tool in expected)


def tool_order_items(
    expected_tools: List[List[str]],
    used_tools: List[List[str]],
) -> List[Optional[float]]:
    """逐样本的有序命中（1.0/0.0；无期望标注返回 None，不参与统计）。"""
    if len(expected_tools) != len(used_tools):
        raise ValueError("expected_tools 和 used_tools 长度必须相同")

    out: List[Optional[float]] = []
    for expected, used in zip(expected_tools, used_tools):
        exp_norm = _normalize_tools(expected)
        if not exp_norm:
            out.append(None)
            continue
        out.append(1.0 if _is_ordered_subsequence(exp_norm, _normalize_tools(used)) else 0.0)
    return out


def tool_order(
    expected_tools: List[List[str]],
    used_tools: List[List[str]],
) -> dict[str, float]:
    """计算工具调用的有序包含度（ordered inclusion）。

    与 tool_selection 只看集合命中不同，tool_order 要求期望工具按序作为实际
    调用序列的子序列出现——选对工具 ≠ 调用顺序正确（先查库存再下单，反之无效）。

    Args:
        expected_tools: 期望工具序列列表（每样本按期望顺序）
        used_tools: 实际工具调用序列列表（每样本按调用顺序）

    Returns:
        包含 ordered_match（有序子序列命中率）的字典
    """
    if not expected_tools:
        return {"ordered_match": 0.0}

    # 无期望工具标注的样本不参与统计，避免拉高/拉低分母
    scored = [x for x in tool_order_items(expected_tools, used_tools) if x is not None]
    ordered_match = sum(scored) / len(scored) if scored else 0.0
    return {"ordered_match": ordered_match}


def trajectory_match_items(
    expected_trajectories: List[List[str]],
    actual_trajectories: List[List[str]],
) -> List[dict[str, float]]:
    """逐样本的轨迹匹配 {exact_match, similarity}（供逐条落库）。

    exact_match 为 1.0/0.0；两侧任一为空则 similarity=0（与聚合口径一致：空样本按 0 计入分母）。
    """
    if len(expected_trajectories) != len(actual_trajectories):
        raise ValueError("expected_trajectories 和 actual_trajectories 长度必须相同")

    out: List[dict[str, float]] = []
    for expected, actual in zip(expected_trajectories, actual_trajectories):
        exact = 1.0 if expected == actual else 0.0
        sim = 0.0
        if expected and actual:
            sim = compute_semantic_metrics(
                [" ".join(expected)], [" ".join(actual)]
            ).get("similarity", 0.0)
        out.append({"exact_match": exact, "similarity": sim})
    return out


def trajectory_match(
    expected_trajectories: List[List[str]],
    actual_trajectories: List[List[str]],
) -> dict[str, float]:
    """计算轨迹匹配度。

    Args:
        expected_trajectories: 期望轨迹列表（每个样本的期望步骤）
        actual_trajectories: 实际轨迹列表（每个样本的实际步骤）

    Returns:
        包含 exact_match 和 similarity 的字典
    """
    if not expected_trajectories:
        return {"exact_match": 0.0, "similarity": 0.0}

    items = trajectory_match_items(expected_trajectories, actual_trajectories)
    n = len(items)
    return {
        "exact_match": sum(x["exact_match"] for x in items) / n,
        "similarity": sum(x["similarity"] for x in items) / n,
    }


def _step_ratio(actual: int, optimal: int) -> float:
    """单样本步骤效率比（对称比 min(a/o, o/a)，越接近 1 越好；任一为 0 记 0）。"""
    if optimal > 0 and actual > 0:
        return min(optimal / actual, actual / optimal)
    return 0.0


def step_efficiency_items(
    actual_steps: List[int],
    optimal_steps: List[int],
) -> List[float]:
    """逐样本步骤效率比（供逐条落库）。"""
    if len(actual_steps) != len(optimal_steps):
        raise ValueError("actual_steps 和 optimal_steps 长度必须相同")
    return [_step_ratio(a, o) for a, o in zip(actual_steps, optimal_steps)]


def step_efficiency(
    actual_steps: List[int],
    optimal_steps: List[int],
) -> dict[str, float]:
    """计算步骤效率。

    Args:
        actual_steps: 实际步骤数列表
        optimal_steps: 最优步骤数列表

    Returns:
        包含 optimal_ratio, avg_steps, optimal_steps 的字典
    """
    if len(actual_steps) != len(optimal_steps):
        raise ValueError("actual_steps 和 optimal_steps 长度必须相同")

    if not actual_steps:
        return {"optimal_ratio": 0.0, "avg_steps": 0.0, "optimal_steps": 0.0}

    # optimal_ratio 取「逐样本比再平均」而非旧的「先平均步数再取比」（ratio-of-averages）：
    # 后者会让一条绕路样本被另一条步数偏少的样本抵消，掩盖个体绕路；且与逐条落库的
    # per-item 值不自洽（聚合 ≠ 逐条均值）。改后聚合恒等于 step_efficiency_items 的均值。
    per = step_efficiency_items(actual_steps, optimal_steps)
    return {
        "optimal_ratio": sum(per) / len(per),
        "avg_steps": sum(actual_steps) / len(actual_steps),
        "optimal_steps": sum(optimal_steps) / len(optimal_steps),
    }


# Agent 评测聚合结果
class AgentMetrics:
    """Agent 评测指标结果。"""

    def __init__(
        self,
        answer_accuracy: float = 0.0,
        tool_precision: float = 0.0,
        tool_recall: float = 0.0,
        tool_f1: float = 0.0,
        traj_exact_match: float = 0.0,
        traj_similarity: float = 0.0,
        step_optimal_ratio: float = 0.0,
        step_avg: float = 0.0,
        step_optimal: float = 0.0,
    ) -> None:
        self.answer_accuracy = answer_accuracy
        self.tool_precision = tool_precision
        self.tool_recall = tool_recall
        self.tool_f1 = tool_f1
        self.traj_exact_match = traj_exact_match
        self.traj_similarity = traj_similarity
        self.step_optimal_ratio = step_optimal_ratio
        self.step_avg = step_avg
        self.step_optimal = step_optimal

    def to_dict(self) -> dict:
        return {
            "answer_accuracy": self.answer_accuracy,
            "tool_selection": {
                "precision": self.tool_precision,
                "recall": self.tool_recall,
                "f1": self.tool_f1,
            },
            "trajectory_match": {
                "exact_match": self.traj_exact_match,
                "similarity": self.traj_similarity,
            },
            "step_efficiency": {
                "optimal_ratio": self.step_optimal_ratio,
                "avg_steps": self.step_avg,
                "optimal_steps": self.step_optimal,
            },
        }


def compute_agent_metrics(
    references: List[dict[str, Any]],
    outputs: List[dict[str, Any]],
    metrics: List[str] | None = None,
    return_items: bool = False,
) -> dict[str, Any]:
    """计算 Agent 评测指标（便捷函数）。

    每个指标都「逐样本算一次、聚合即其均值」，聚合与逐条自洽。开启 return_items 时
    额外返回 result["_items"]：长度 == 样本数的列表，每项是该样本的扁平化分值 map
    （键名与聚合扁平化后一致，如 answer_accuracy / tool_f1 / tool_order / traj_similarity /
    step_optimal_ratio），供 fastapi 回填 items_result[i].scores 做逐条 debug。无期望标注
    因而不参与统计的指标，会在该样本的 map 中缺席（而非记 0，避免误导）。

    Args:
        references: 参考列表，每项包含 final_answer, tools_used, expected_steps
        outputs: 输出列表，每项包含 final_answer, trajectory, tools_used, total_steps
        metrics: 需要计算的指标列表
        return_items: 是否附带逐样本分值（供逐条落库）

    Returns:
        Agent 评测指标结果（return_items 时含 "_items"）
    """
    if metrics is None:
        metrics = ["answer_accuracy", "tool_selection"]

    result: dict[str, Any] = {}
    n = len(outputs)
    per_item: List[dict[str, float]] = [dict() for _ in range(n)]

    # 答案准确性
    if "answer_accuracy" in metrics:
        ref_answers = [r.get("final_answer", "") for r in references]
        out_answers = [o.get("final_answer", "") for o in outputs]
        acc_items = answer_accuracy_items(ref_answers, out_answers)
        result["answer_accuracy"] = sum(acc_items) / len(acc_items) if acc_items else 0.0
        for i, v in enumerate(acc_items):
            per_item[i]["answer_accuracy"] = v

    # 工具选择（集合命中：precision/recall/f1）
    if "tool_selection" in metrics:
        expected_tools = [r.get("tools_used", []) for r in references]
        used_tools = [o.get("tools_used", []) for o in outputs]
        ts_items = tool_selection_items(expected_tools, used_tools)
        scored = [x for x in ts_items if x is not None]
        if scored:
            result["tool_selection"] = {
                k: sum(x[k] for x in scored) / len(scored) for k in ("precision", "recall", "f1")
            }
        else:
            result["tool_selection"] = {"precision": 0.0, "recall": 0.0, "f1": 0.0}
        for i, x in enumerate(ts_items):
            if x is not None:
                per_item[i]["tool_precision"] = x["precision"]
                per_item[i]["tool_recall"] = x["recall"]
                per_item[i]["tool_f1"] = x["f1"]

    # 工具顺序（有序子序列命中：ordered_match）
    if "tool_order" in metrics:
        expected_tools = [r.get("tools_used", []) for r in references]
        used_tools = [o.get("tools_used", []) for o in outputs]
        to_items = tool_order_items(expected_tools, used_tools)
        scored_o = [x for x in to_items if x is not None]
        result["tool_order"] = {
            "ordered_match": sum(scored_o) / len(scored_o) if scored_o else 0.0
        }
        for i, x in enumerate(to_items):
            if x is not None:
                per_item[i]["tool_order"] = x

    # 轨迹匹配
    if "trajectory_match" in metrics:
        expected_steps = [r.get("expected_steps", []) for r in references]
        # 从 trajectory 提取步骤描述。Go 端可能上报字符串序列（工具名）或
        # 结构化 step 字典（含 content 字段），两种形态都要能取到文本。
        actual_steps = [
            [_step_text(step) for step in o.get("trajectory", [])]
            for o in outputs
        ]
        tm_items = trajectory_match_items(expected_steps, actual_steps)
        if tm_items:
            result["trajectory_match"] = {
                "exact_match": sum(x["exact_match"] for x in tm_items) / len(tm_items),
                "similarity": sum(x["similarity"] for x in tm_items) / len(tm_items),
            }
        else:
            result["trajectory_match"] = {"exact_match": 0.0, "similarity": 0.0}
        for i, x in enumerate(tm_items):
            per_item[i]["traj_exact_match"] = x["exact_match"]
            per_item[i]["traj_similarity"] = x["similarity"]

    # 步骤效率
    if "step_efficiency" in metrics:
        actual_cnt = [o.get("total_steps", len(o.get("trajectory", []))) for o in outputs]
        optimal_cnt = [
            len(r.get("expected_steps", [])) if r.get("expected_steps") else 1
            for r in references
        ]
        se_items = step_efficiency_items(actual_cnt, optimal_cnt)
        result["step_efficiency"] = {
            "optimal_ratio": sum(se_items) / len(se_items) if se_items else 0.0,
            "avg_steps": sum(actual_cnt) / len(actual_cnt) if actual_cnt else 0.0,
            "optimal_steps": sum(optimal_cnt) / len(optimal_cnt) if optimal_cnt else 0.0,
        }
        for i, v in enumerate(se_items):
            per_item[i]["step_optimal_ratio"] = v

    if return_items:
        result["_items"] = per_item

    return result
