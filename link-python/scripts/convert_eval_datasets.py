#!/usr/bin/env python3
"""评测数据集转换器：HuggingFace 基准 + 自造场景 → 统一 JSONL（供前端上传 / Go seed 灌库）。

产物布局（默认写入 link-go/cmd/seed-eval-datasets/data/）：
    manifest.json            # 数据集清单：id/name/description/evaluation_type/records_file/count/supports_trajectory
    <dataset_id>.jsonl       # 每行一条样本记录，字段与 QAPair 对齐：
                             #   question / reference_answer / relevant_pids? / expected_tools? / expected_steps?

设计取舍：
- QA 集（cmrc2018 中文、squad 英文）只映射 question + reference_answer，命中生成/语义/裁判类指标。
- Agent 基准（xlam-function-calling-60k）从 answers 抽 expected_tools（有序），命中 tool_selection/tool_order；
  该集无「步骤自然语言标注」，故 expected_steps 留空、supports_trajectory=false，仅产工具类 + QA 指标并计数告警。
- 网络不可用/HF 拉取失败时逐个跳过并告警，自造场景集（电商/知识库）始终可离线产出，保证 seed 可跑通。

用法：
    .venv/bin/python scripts/convert_eval_datasets.py                 # 全量，默认限量导出
    .venv/bin/python scripts/convert_eval_datasets.py --limit 80      # 覆盖每集上限
    .venv/bin/python scripts/convert_eval_datasets.py --only ecommerce_agent,kb_qa   # 只产指定集
    .venv/bin/python scripts/convert_eval_datasets.py --out /path/to/dir
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Callable

# 允许以 `python scripts/convert_eval_datasets.py` 直接运行时找到 services 包
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from services.dataset.loader import load_hf_dataset  # noqa: E402

# 默认产物目录：Go seed 命令的数据目录（提交入库，供 cmd/seed-eval-datasets 读取）
DEFAULT_OUT = (
    Path(__file__).resolve().parent.parent.parent
    / "link-go"
    / "cmd"
    / "seed-eval-datasets"
    / "data"
)

# 每集默认导出上限（限量，避免灌库过大）；task 要求 50–200 条区间
DEFAULT_LIMIT = 120


# ---------------------------------------------------------------------------
# HuggingFace 基准映射器
# ---------------------------------------------------------------------------
def _map_squad(row: dict[str, Any]) -> dict[str, Any] | None:
    """rajpurkar/squad：question + answers.text[0]（无答案样本跳过）。"""
    answers = row.get("answers") or {}
    texts = answers.get("text") or []
    if not row.get("question") or not texts:
        return None
    return {"question": row["question"].strip(), "reference_answer": texts[0].strip()}


def _map_cmrc2018(row: dict[str, Any]) -> dict[str, Any] | None:
    """hfl/cmrc2018：结构与 squad 一致（answers.text[0]）。"""
    return _map_squad(row)


def _map_xlam(row: dict[str, Any]) -> dict[str, Any] | None:
    """Salesforce/xlam-function-calling-60k：query + answers→expected_tools（有序）。

    answers 为 JSON 字符串，形如 '[{"name": "get_weather", "arguments": {...}}, ...]'。
    抽取 name 序列作为 expected_tools；reference_answer 落原始 answers 文本，供答案类指标兜底。
    无工具标注的样本跳过（该集应恒有工具，防脏数据）。
    """
    query = (row.get("query") or "").strip()
    raw_answers = row.get("answers")
    if not query or not raw_answers:
        return None
    try:
        calls = json.loads(raw_answers) if isinstance(raw_answers, str) else raw_answers
    except (json.JSONDecodeError, TypeError):
        return None
    tools = [c.get("name", "") for c in calls if isinstance(c, dict) and c.get("name")]
    if not tools:
        return None
    return {
        "question": query,
        "reference_answer": raw_answers if isinstance(raw_answers, str) else json.dumps(raw_answers, ensure_ascii=False),
        "expected_tools": tools,
    }


# ---------------------------------------------------------------------------
# 数据集配置
# ---------------------------------------------------------------------------
HF_DATASETS: list[dict[str, Any]] = [
    {
        "dataset_id": "hf_cmrc2018_zh",
        "name": "CMRC2018 中文阅读理解（QA）",
        "description": "中文抽取式问答基准，映射 question/reference_answer，用于 QA 生成/语义/裁判指标。",
        "evaluation_type": "qa",
        "hf_path": "hfl/cmrc2018",
        "split": "train",
        "mapper": _map_cmrc2018,
        "supports_trajectory": False,
    },
    {
        "dataset_id": "hf_squad_en",
        "name": "SQuAD 英文阅读理解（QA）",
        "description": "英文抽取式问答基准，映射 question/reference_answer，用于 QA 生成/语义/裁判指标。",
        "evaluation_type": "qa",
        "hf_path": "rajpurkar/squad",
        "split": "train",
        "mapper": _map_squad,
        "supports_trajectory": False,
    },
    {
        # Salesforce 官方集为 gated（需 HF_TOKEN），此处用 schema 完全一致的公开镜像
        # （字段同为 query/answers/tools），保证离线 seed 可复现。
        "dataset_id": "hf_xlam_agent",
        "name": "xLAM 工具调用基准（Agent）",
        "description": "xLAM function-calling 公开镜像：query→expected_tools（有序），命中 tool_selection/tool_order；无步骤标注故 supports_trajectory=false。",
        "evaluation_type": "agent",
        "hf_path": "NobodyExistsOnTheInternet/xlam-function-calling-60k",
        "split": "train",
        "mapper": _map_xlam,
        "supports_trajectory": False,
    },
]


# ---------------------------------------------------------------------------
# 自造场景集（离线恒可产出）
# ---------------------------------------------------------------------------
def _scenario_ecommerce_agent() -> dict[str, Any]:
    """电商场景 Agent 集：贴合本仓 ecommerce_demo 数据源，带期望工具序列与步骤标注。"""
    records = [
        {
            "question": "查一下订单 SO2024001 现在是什么状态？",
            "reference_answer": "订单 SO2024001 当前状态为「已发货」，预计 3 日内送达。",
            "expected_tools": ["query_order"],
            "expected_steps": ["解析订单号 SO2024001", "调用 query_order 查询订单", "整理状态并回复"],
        },
        {
            "question": "我想买 3 件商品 P1001，先看看还有没有货，有的话帮我下单。",
            "reference_answer": "商品 P1001 库存充足，已为你创建包含 3 件的订单。",
            "expected_tools": ["check_inventory", "create_order"],
            "expected_steps": ["调用 check_inventory 核对 P1001 库存", "库存充足则调用 create_order 下单", "回复下单结果"],
        },
        {
            "question": "帮我统计上个月销量最高的三个商品。",
            "reference_answer": "上月销量前三为 P1001、P1005、P1003，销量分别为 320/280/210 件。",
            "expected_tools": ["query_sales", "aggregate_topn"],
            "expected_steps": ["调用 query_sales 拉取上月销售明细", "调用 aggregate_topn 取销量前三", "整理榜单回复"],
        },
        {
            "question": "订单 SO2024007 想退货，帮我发起退款。",
            "reference_answer": "已为订单 SO2024007 发起退款申请，款项将原路退回。",
            "expected_tools": ["query_order", "create_refund"],
            "expected_steps": ["调用 query_order 校验订单可退", "调用 create_refund 发起退款", "回复退款进度"],
        },
        {
            "question": "客户 C2048 最近有哪些未支付的订单？",
            "reference_answer": "客户 C2048 有 2 笔未支付订单：SO2024011、SO2024015。",
            "expected_tools": ["query_customer_orders"],
            "expected_steps": ["调用 query_customer_orders 按客户与支付状态过滤", "汇总未支付订单回复"],
        },
    ]
    return {
        "dataset_id": "scenario_ecommerce_agent",
        "name": "电商场景 Agent 测评集（自造）",
        "description": "贴合 ecommerce_demo 的多轮工具调用样本，带 expected_tools + expected_steps，命中全量 Agent 指标。",
        "evaluation_type": "agent",
        "supports_trajectory": True,
        "records": records,
    }


def _scenario_kb_qa() -> dict[str, Any]:
    """知识库场景 QA 集：贴合知识库问答，仅 question/reference_answer。"""
    records = [
        {
            "question": "本系统的数据同步默认走哪种存储？",
            "reference_answer": "元数据与配置默认存 MySQL，向量存 Milvus，知识图谱存 Neo4j，缓存/队列走 Redis。",
        },
        {
            "question": "给业务表新增字段后应如何同步表结构？",
            "reference_answer": "运行 cmd/migrate-db 从 GORM model 幂等同步全部业务表结构，不手动 ALTER。",
        },
        {
            "question": "知识图谱的提取开关是库级还是提问级？",
            "reference_answer": "图谱提取是库级配置（kb_settings），图谱检索是提问级（随请求下发）。",
        },
        {
            "question": "服务间通信在什么场景用 gRPC，什么场景用 MCP？",
            "reference_answer": "高性能、大数据量走 gRPC；AI 工具调用与实验功能走 MCP。",
        },
        {
            "question": "request_id 在系统里起什么作用？",
            "reference_answer": "request_id 全链路透传，串联结构化审计日志与原始日志，便于按同一请求排障。",
        },
    ]
    return {
        "dataset_id": "scenario_kb_qa",
        "name": "知识库场景 QA 测评集（自造）",
        "description": "贴合本系统知识库的问答样本，映射 question/reference_answer，用于 QA/RAG 指标。",
        "evaluation_type": "qa",
        "supports_trajectory": False,
        "records": records,
    }


SCENARIO_BUILDERS: list[Callable[[], dict[str, Any]]] = [
    _scenario_ecommerce_agent,
    _scenario_kb_qa,
]


# ---------------------------------------------------------------------------
# 导出
# ---------------------------------------------------------------------------
def _export_hf(cfg: dict[str, Any], limit: int) -> dict[str, Any] | None:
    """拉取并转换一个 HF 数据集；网络/加载失败返回 None（调用方计数告警）。"""
    mapper: Callable[[dict[str, Any]], dict[str, Any] | None] = cfg["mapper"]
    try:
        ds = load_hf_dataset(cfg["hf_path"], split=cfg.get("split"))
    except Exception as exc:  # noqa: BLE001 — 网络/权限/字段变更等均降级跳过
        print(f"  [WARN] 跳过 {cfg['dataset_id']}（HF 加载失败）：{exc}", file=sys.stderr)
        return None

    records: list[dict[str, Any]] = []
    skipped = 0
    for row in ds:
        if len(records) >= limit:
            break
        mapped = mapper(dict(row))
        if mapped is None:
            skipped += 1
            continue
        records.append(mapped)

    if not records:
        print(f"  [WARN] {cfg['dataset_id']} 映射后无有效样本，跳过", file=sys.stderr)
        return None

    tool_missing = sum(1 for r in records if cfg["evaluation_type"] == "agent" and not r.get("expected_tools"))
    if cfg["evaluation_type"] == "agent" and tool_missing:
        print(f"  [WARN] {cfg['dataset_id']} 有 {tool_missing} 条无工具标注，仅产 QA 指标", file=sys.stderr)

    return {
        "dataset_id": cfg["dataset_id"],
        "name": cfg["name"],
        "description": cfg["description"],
        "evaluation_type": cfg["evaluation_type"],
        "supports_trajectory": cfg.get("supports_trajectory", False),
        "records": records,
    }


def _write_dataset(out_dir: Path, ds: dict[str, Any]) -> dict[str, Any]:
    """写出单集 JSONL，返回 manifest 条目。"""
    records_file = f"{ds['dataset_id']}.jsonl"
    with (out_dir / records_file).open("w", encoding="utf-8") as f:
        for rec in ds["records"]:
            f.write(json.dumps(rec, ensure_ascii=False) + "\n")
    return {
        "dataset_id": ds["dataset_id"],
        "name": ds["name"],
        "description": ds["description"],
        "evaluation_type": ds["evaluation_type"],
        "supports_trajectory": ds["supports_trajectory"],
        "records_file": records_file,
        "count": len(ds["records"]),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="评测数据集转换器（HF + 自造场景 → JSONL）")
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT, help="产物目录（默认 Go seed data 目录）")
    parser.add_argument("--limit", type=int, default=DEFAULT_LIMIT, help="每个 HF 集导出上限（默认 120）")
    parser.add_argument("--only", type=str, default="", help="逗号分隔的 dataset_id 白名单，缺省全量")
    args = parser.parse_args()

    only = {s.strip() for s in args.only.split(",") if s.strip()}
    out_dir: Path = args.out
    out_dir.mkdir(parents=True, exist_ok=True)

    manifest: list[dict[str, Any]] = []

    # 自造场景集优先（离线恒可产出）
    print("== 自造场景集 ==")
    for build in SCENARIO_BUILDERS:
        ds = build()
        if only and ds["dataset_id"] not in only:
            continue
        entry = _write_dataset(out_dir, ds)
        manifest.append(entry)
        print(f"  ✓ {entry['dataset_id']}: {entry['count']} 条 ({entry['evaluation_type']})")

    # HF 基准集
    print("== HuggingFace 基准集 ==")
    for cfg in HF_DATASETS:
        if only and cfg["dataset_id"] not in only:
            continue
        ds = _export_hf(cfg, args.limit)
        if ds is None:
            continue
        entry = _write_dataset(out_dir, ds)
        manifest.append(entry)
        print(f"  ✓ {entry['dataset_id']}: {entry['count']} 条 ({entry['evaluation_type']})")

    (out_dir / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    total = sum(e["count"] for e in manifest)
    print(f"\n完成：{len(manifest)} 个数据集，共 {total} 条样本 → {out_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
