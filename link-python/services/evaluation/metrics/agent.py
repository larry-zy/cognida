"""Agent 评测指标。"""

from typing import Any, List

from .generation import compute_generation_metrics
from .semantic import compute_semantic_metrics


def answer_accuracy(
    references: List[str],
    outputs: List[str],
) -> float:
    """计算答案准确性。

    Args:
        references: 参考答案列表
        outputs: Agent 输出列表

    Returns:
        准确率 (0-1)
    """
    if len(references) != len(outputs):
        raise ValueError("references 和 outputs 长度必须相同")

    if not references:
        return 0.0

    # 使用语义相似度计算准确性
    correct = 0
    for ref, out in zip(references, outputs):
        # 简单策略：语义相似度 > 0.8 视为正确
        sim = compute_semantic_metrics([ref], [out])
        if sim.get("similarity", 0) > 0.8:
            correct += 1

    return correct / len(references)


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
    if len(expected_tools) != len(used_tools):
        raise ValueError("expected_tools 和 used_tools 长度必须相同")

    if not expected_tools:
        return {"precision": 0.0, "recall": 0.0, "f1": 0.0}

    # 统计所有样本
    total_expected = set()
    total_used = set()
    total_match = set()

    for expected, used in zip(expected_tools, used_tools):
        expected_set = set(expected)
        used_set = set(used)

        total_expected.update(expected_set)
        total_used.update(used_set)
        total_match.update(expected_set & used_set)

    # 计算指标
    precision = len(total_match) / len(total_used) if total_used else 0.0
    recall = len(total_match) / len(total_expected) if total_expected else 0.0
    f1 = (
        2 * precision * recall / (precision + recall)
        if (precision + recall) > 0
        else 0.0
    )

    return {"precision": precision, "recall": recall, "f1": f1}


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
    if len(expected_trajectories) != len(actual_trajectories):
        raise ValueError("expected_trajectories 和 actual_trajectories 长度必须相同")

    if not expected_trajectories:
        return {"exact_match": 0.0, "similarity": 0.0}

    exact_matches = 0
    total_similarity = 0.0

    for expected, actual in zip(expected_trajectories, actual_trajectories):
        # 精确匹配
        if expected == actual:
            exact_matches += 1

        # 语义相似度
        if expected and actual:
            # 将轨迹转换为文本比较
            expected_text = " ".join(expected)
            actual_text = " ".join(actual)
            sim = compute_semantic_metrics([expected_text], [actual_text])
            total_similarity += sim.get("similarity", 0)

    exact_match = exact_matches / len(expected_trajectories)
    similarity = total_similarity / len(expected_trajectories)

    return {"exact_match": exact_match, "similarity": similarity}


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

    # 计算平均步骤数
    avg_actual = sum(actual_steps) / len(actual_steps)
    avg_optimal = sum(optimal_steps) / len(optimal_steps)

    # 计算最优比率（实际/最优，越接近1越好）
    if avg_optimal > 0:
        optimal_ratio = min(avg_optimal / avg_actual, avg_actual / avg_optimal)
    else:
        optimal_ratio = 0.0

    return {
        "optimal_ratio": optimal_ratio,
        "avg_steps": avg_actual,
        "optimal_steps": avg_optimal,
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
) -> dict[str, Any]:
    """计算 Agent 评测指标（便捷函数）。

    Args:
        references: 参考列表，每项包含 final_answer, tools_used, expected_steps
        outputs: 输出列表，每项包含 final_answer, trajectory, tools_used, total_steps
        metrics: 需要计算的指标列表

    Returns:
        Agent 评测指标结果
    """
    if metrics is None:
        metrics = ["answer_accuracy", "tool_selection"]

    result = {}

    # 答案准确性
    if "answer_accuracy" in metrics:
        ref_answers = [r.get("final_answer", "") for r in references]
        out_answers = [o.get("final_answer", "") for o in outputs]
        result["answer_accuracy"] = answer_accuracy(ref_answers, out_answers)

    # 工具选择
    if "tool_selection" in metrics:
        expected_tools = [r.get("tools_used", []) for r in references]
        used_tools = [o.get("tools_used", []) for o in outputs]
        result["tool_selection"] = tool_selection(expected_tools, used_tools)

    # 轨迹匹配
    if "trajectory_match" in metrics:
        expected_steps = [r.get("expected_steps", []) for r in references]
        # 从 trajectory 提取步骤描述
        actual_steps = [
            [step.get("content", "") for step in o.get("trajectory", [])]
            for o in outputs
        ]
        result["trajectory_match"] = trajectory_match(expected_steps, actual_steps)

    # 步骤效率
    if "step_efficiency" in metrics:
        actual_steps = [o.get("total_steps", len(o.get("trajectory", []))) for o in outputs]
        optimal_steps = [
            len(r.get("expected_steps", [])) if r.get("expected_steps") else 1
            for r in references
        ]
        result["step_efficiency"] = step_efficiency(actual_steps, optimal_steps)

    return result
