"""Text2SQL / SQL 生成评测指标。

三类指标：
- sql_exact_match：金标准 SQL 与生成 SQL 归一化后精确相等（0/1）。
- sql_component_match：Spider 风格结构 F1——按子句关键词（SELECT/FROM/WHERE/
  GROUP BY/HAVING/ORDER BY/LIMIT/JOIN）拆分为组件集合，取集合 F1，容忍等价改写。
- sql_execution_accuracy：Go 侧只读执行「金标准 + 生成」两条 SQL，本函数无序比对
  两个结果集是否等价（0/1）。缺任一结果集（未执行/执行失败）该样本不计入分母。

均为「逐样本算一次、聚合即其均值」；开启 return_items 时附带逐样本分值供逐条落库。
无法评估（缺金标准/缺结果集）的样本在该指标缺席（而非记 0，避免误导）。
"""

import re
from typing import Any, List, Optional

# SQL 子句关键词（大写），用于结构组件拆分。多词关键词放前面优先匹配。
_CLAUSE_KEYWORDS = [
    "GROUP BY",
    "ORDER BY",
    "LEFT JOIN",
    "RIGHT JOIN",
    "INNER JOIN",
    "OUTER JOIN",
    "SELECT",
    "FROM",
    "WHERE",
    "HAVING",
    "LIMIT",
    "JOIN",
    "UNION",
    "DISTINCT",
]

# 归一化时剥离的尾部分号与多余空白
_WS_RE = re.compile(r"\s+")
_TOKEN_RE = re.compile(r"[A-Za-z0-9_.*]+|[<>=!]+|[(),]")


def normalize_sql(sql: str) -> str:
    """SQL 归一化：小写、压缩空白、去反引号/双引号、去尾分号，便于精确比对。

    仅做保守归一（不解析 AST），把大小写、空白、标识符引号差异抹平，
    避免把语义等价但写法不同的 SQL 误判为不匹配（交给 component/execution 兜底）。
    """
    if not sql:
        return ""
    s = sql.strip().rstrip(";").strip()
    s = s.replace("`", "").replace('"', "")
    s = _WS_RE.sub(" ", s)
    return s.lower()


def sql_exact_match(reference: str, generated: str) -> float:
    """归一化后精确相等返回 1.0，否则 0.0。金标准为空视为无法评估交由上层处理。"""
    return 1.0 if normalize_sql(reference) == normalize_sql(generated) else 0.0


def _extract_components(sql: str) -> set:
    """把 SQL 按子句关键词切成组件集合（Spider 风格结构 F1 的基础）。

    每个出现的子句关键词记一个组件，另把每个子句内的 token（表名/列名/操作符）
    也纳入集合，从而同时反映「结构骨架」与「引用实体」的重合度。
    """
    if not sql:
        return set()
    upper = sql.upper()
    components: set = set()
    for kw in _CLAUSE_KEYWORDS:
        if re.search(r"\b" + re.escape(kw) + r"\b", upper):
            components.add("CLAUSE:" + kw)
    # token 级：抽取标识符/数字/操作符，去掉纯关键词噪声后并入集合
    kw_flat = {k.replace(" ", "") for k in _CLAUSE_KEYWORDS}
    for tok in _TOKEN_RE.findall(sql.lower()):
        t = tok.strip()
        if not t or t in ("(", ")", ","):
            continue
        if t.upper() in kw_flat:
            continue
        components.add("TOK:" + t)
    return components


def _f1(ref_set: set, gen_set: set) -> float:
    """两个组件集合的 F1。任一为空时：都空记 1.0，一方空记 0.0。"""
    if not ref_set and not gen_set:
        return 1.0
    if not ref_set or not gen_set:
        return 0.0
    inter = len(ref_set & gen_set)
    if inter == 0:
        return 0.0
    precision = inter / len(gen_set)
    recall = inter / len(ref_set)
    return 2 * precision * recall / (precision + recall)


def sql_component_match(reference: str, generated: str) -> float:
    """Spider 风格结构组件 F1（0-1），容忍等价改写与列/表顺序差异。"""
    return _f1(_extract_components(reference), _extract_components(generated))


def _canonicalize_row(row: Any) -> tuple:
    """把一行结果规整为可哈希、可比较的元组（数值/字符串统一成字符串，去尾随零）。"""
    if not isinstance(row, (list, tuple)):
        row = [row]
    canon = []
    for cell in row:
        if cell is None:
            canon.append("\x00NULL")
        elif isinstance(cell, bool):
            canon.append("1" if cell else "0")
        elif isinstance(cell, float):
            # 12.0 与 12 视为相等；去掉浮点尾随零
            canon.append(("%f" % cell).rstrip("0").rstrip("."))
        elif isinstance(cell, int):
            canon.append(str(cell))
        else:
            s = str(cell).strip()
            # 数字字符串归一（"12.00" -> "12"）
            try:
                f = float(s)
                s = ("%f" % f).rstrip("0").rstrip(".")
            except (ValueError, TypeError):
                pass
            canon.append(s)
    return tuple(canon)


def sql_execution_accuracy(
    ref_result_set: Optional[List[Any]],
    gen_result_set: Optional[List[Any]],
) -> Optional[float]:
    """无序比对两个结果集是否等价（作为多重集合，忽略行顺序）。

    返回 1.0/0.0；任一结果集为 None（未执行/执行失败）返回 None，表示该样本
    无法评估执行准确率，由上层剔除出分母。空结果集之间视为相等（1.0）——
    金标准与生成都返回空是常见的「无满足条件行」正确答案。
    """
    if ref_result_set is None or gen_result_set is None:
        return None
    from collections import Counter

    ref_bag = Counter(_canonicalize_row(r) for r in ref_result_set)
    gen_bag = Counter(_canonicalize_row(r) for r in gen_result_set)
    return 1.0 if ref_bag == gen_bag else 0.0


def compute_sql_metrics(
    references: List[dict[str, Any]],
    outputs: List[dict[str, Any]],
    metrics: List[str] | None = None,
    return_items: bool = False,
) -> dict[str, Any]:
    """计算 Text2SQL 评测指标（便捷函数），与 compute_agent_metrics 同构。

    Args:
        references: 每项含 gold_sql、gold_result_set。
        outputs: 每项含 generated_sql、result_set。
        metrics: 需计算的指标名列表（sql_exact_match/sql_component_match/sql_execution_accuracy）。
        return_items: 是否附带逐样本分值 result["_items"]（供 fastapi 回填逐条 scores）。

    Returns:
        聚合指标 map（各指标为参与样本的均值）；return_items 时含 "_items"。
        无法评估的样本在对应指标缺席（不记 0）。
    """
    if metrics is None:
        metrics = ["sql_exact_match", "sql_component_match", "sql_execution_accuracy"]

    result: dict[str, Any] = {}
    n = len(outputs)
    per_item: List[dict[str, float]] = [dict() for _ in range(n)]

    gold_sqls = [r.get("gold_sql", "") or "" for r in references]
    gen_sqls = [o.get("generated_sql", "") or "" for o in outputs]

    # 精确匹配（仅在金标准 SQL 非空时计入）
    if "sql_exact_match" in metrics:
        vals: List[float] = []
        for i in range(n):
            if not gold_sqls[i].strip():
                continue
            v = sql_exact_match(gold_sqls[i], gen_sqls[i])
            per_item[i]["sql_exact_match"] = v
            vals.append(v)
        result["sql_exact_match"] = sum(vals) / len(vals) if vals else 0.0

    # 结构组件 F1（仅在金标准 SQL 非空时计入）
    if "sql_component_match" in metrics:
        vals = []
        for i in range(n):
            if not gold_sqls[i].strip():
                continue
            v = sql_component_match(gold_sqls[i], gen_sqls[i])
            per_item[i]["sql_component_match"] = v
            vals.append(v)
        result["sql_component_match"] = sum(vals) / len(vals) if vals else 0.0

    # 执行准确率（仅在金标准 + 生成结果集都取到时计入）
    if "sql_execution_accuracy" in metrics:
        vals = []
        for i in range(n):
            ref_rs = references[i].get("gold_result_set")
            gen_rs = outputs[i].get("result_set")
            v = sql_execution_accuracy(ref_rs, gen_rs)
            if v is None:
                continue
            per_item[i]["sql_execution_accuracy"] = v
            vals.append(v)
        result["sql_execution_accuracy"] = sum(vals) / len(vals) if vals else 0.0

    if return_items:
        result["_items"] = per_item

    return result
