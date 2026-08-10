#!/usr/bin/env python3
"""评测数据集转换器：HuggingFace 基准 + 自造场景 → 统一 JSONL（供前端上传 / Go seed 灌库）。

产物布局（默认写入 cognida-go/cmd/seed/eval-datasets/data/）：
    manifest.json            # 数据集清单：id/name/description/evaluation_type/records_file/count/supports_trajectory
    <dataset_id>.jsonl       # 每行一条样本记录，字段与 QAPair 对齐：
                             #   question / reference_answer / relevant_pids? / expected_tools? / expected_steps?

设计取舍：
- QA 集（cmrc2018 中文、squad 英文）只映射 question + reference_answer，命中生成/语义/裁判类指标。
- Agent 评测只用自造电商场景集 scenario_ecommerce_agent：其 expected_tools 锚定被测 DataAgent 的真实
  工具（sql_execute/get_schema/render_ui/data_analysis）。外部 function-calling 基准（如 xLAM）的工具
  宇宙与被测 DataAgent 完全不相交，用它做 agent 评测会让工具/轨迹指标恒趋 0，故不纳入 seed。
- 网络不可用/HF 拉取失败时逐个跳过并告警，自造场景集（电商/知识库）始终可离线产出，保证 seed 可跑通。

reseed 注意（金标准一致性）：
- scenario_ecommerce_agent 的 golden 在本脚本生成期直连 ecommerce_demo 现算并写入 JSONL，随后被
  go:embed 打进 seed 二进制。若 ecommerce_demo 被 `cmd/seed/ecommerce`（DROP+CREATE 随机重建），
  冻结的 golden 会与新库失配。重建电商库后必须重跑：
      .venv/bin/python scripts/convert_eval_datasets.py --only scenario_ecommerce_agent
  重新生成 golden 再 `go run ./cmd/seed/eval-datasets` 灌库。seed 工具带 --strict 会对若干整库计数题
  现场对拍，失配则非零退出，把「静默失配」变「显眼报错」。

用法：
    .venv/bin/python scripts/convert_eval_datasets.py                 # 全量，默认限量导出
    .venv/bin/python scripts/convert_eval_datasets.py --limit 200     # 覆盖每集上限
    .venv/bin/python scripts/convert_eval_datasets.py --only scenario_ecommerce_agent,scenario_kb_qa
    .venv/bin/python scripts/convert_eval_datasets.py --out /path/to/dir
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Any, Callable

# 允许以 `python scripts/convert_eval_datasets.py` 直接运行时找到 services 包
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from services.dataset.loader import load_hf_dataset  # noqa: E402

# 默认产物目录：Go seed 命令的数据目录（提交入库，供 cmd/seed/eval-datasets 读取）
DEFAULT_OUT = (
    Path(__file__).resolve().parent.parent.parent
    / "cognida-go"
    / "cmd"
    / "seed"
    / "eval-datasets"
    / "data"
)

# 每集默认导出上限（限量，避免灌库过大）；task 要求 50–200 条区间。
# 锁定为 80 与已提交产物（manifest 中 HF 集 count=80）一致，避免重跑漂移。
DEFAULT_LIMIT = 80


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


# 注：外部 function-calling 基准（如 xLAM）曾在此映射 query→expected_tools，但其工具宇宙
# （live_giveaways_by_type 等外部 API）与被测 DataAgent 的固定工具集（sql_execute/get_schema/
# render_ui/data_analysis）完全不相交，用于 agent 评测会让工具/轨迹指标恒趋 0，已剔除。
# Agent 评测统一走自造电商场景集 scenario_ecommerce_agent。


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
]


# ---------------------------------------------------------------------------
# 自造场景集（离线恒可产出）
# ---------------------------------------------------------------------------
# ---------------------------------------------------------------------------
# 电商 Agent 集：DB-backed（从真实 ecommerce_demo 现算 golden）
# ---------------------------------------------------------------------------
# 旧实现的问题：题面/期望工具/答案全是编造的（SO2024001、query_order/check_inventory…），
# 既非真实数据也非 data-agent 真实工具名 → tool/answer 指标恒 0。改为构建期直接查
# ecommerce_demo 现算 golden，并把 expected_tools 锚定到 data-agent 的路径不变量工具，
# 使评测在「被测 Agent 也路由到同一数据源」后能拿到有效分数。
#
# 工具锚点取舍：无论走词法 NL2SQL（get_schema→sql_execute）还是语义层
# （semantic_models→semantic_query→sql_execute），实际执行 SQL 的工具恒为 sql_execute，
# 故数据类任务 expected_tools 只锚定 sql_execute（最小必需集，避免误伤 recall）；
# 画图任务追加 render_ui，纯结构探查任务用 get_schema。

_STATUS_ZH = {
    "completed": "已完成",
    "paid": "已支付",
    "shipped": "已发货",
    "cancelled": "已取消",
    "refunded": "已退款",
    "pending": "待处理",
}


class _EcommerceDBError(RuntimeError):
    """ecommerce_demo 不可用（未启库/未灌数/凭据不符），电商集将被跳过。"""


def _ecommerce_db_config() -> dict[str, str]:
    """电商库连接参数：优先 ECOMMERCE_DB_*，回落到 DB_*，再回落到本地 dev 默认。"""
    getenv = os.getenv
    return {
        "host": getenv("ECOMMERCE_DB_HOST") or getenv("DB_HOST") or "127.0.0.1",
        "port": getenv("ECOMMERCE_DB_PORT") or getenv("DB_PORT") or "3306",
        "user": getenv("ECOMMERCE_DB_USER") or getenv("DB_USER") or "root",
        "password": getenv("ECOMMERCE_DB_PASSWORD") or getenv("DB_PASSWORD") or "1234",
        "db": getenv("ECOMMERCE_DB_NAME") or "ecommerce_demo",
    }


def _mysql_rows(sql: str) -> list[list[str]]:
    """用 mysql CLI 跑一条查询，返回去表头的行（每行按 \\t 切列）。失败抛 _EcommerceDBError。"""
    cfg = _ecommerce_db_config()
    cmd = [
        "mysql", "-u" + cfg["user"], "-h" + cfg["host"], "-P" + str(cfg["port"]),
        "-N", "-B", cfg["db"], "-e", sql,
    ]
    env = dict(os.environ, MYSQL_PWD=cfg["password"])  # 用 MYSQL_PWD 避免密码进 argv
    try:
        proc = subprocess.run(cmd, env=env, capture_output=True, text=True, timeout=30)
    except (FileNotFoundError, subprocess.TimeoutExpired) as exc:
        raise _EcommerceDBError(f"mysql 调用失败：{exc}") from exc
    if proc.returncode != 0:
        raise _EcommerceDBError(f"查询失败(rc={proc.returncode})：{proc.stderr.strip()} | SQL: {sql}")
    return [line.split("\t") for line in proc.stdout.splitlines() if line != ""]


def _one(sql: str) -> list[str]:
    """取首行（无结果视为数据异常，抛错让整集跳过而非产出空 golden）。"""
    rows = _mysql_rows(sql)
    if not rows:
        raise _EcommerceDBError(f"查询无结果：{sql}")
    return rows[0]


def _money(s: str) -> str:
    """金额千分位格式化（保留两位），非数字原样返回。"""
    try:
        return f"{float(s):,.2f}"
    except (TypeError, ValueError):
        return s


def _int(s: str) -> str:
    """整数千分位格式化，非数字原样返回。"""
    try:
        return f"{int(float(s)):,}"
    except (TypeError, ValueError):
        return s


def _rec(question: str, answer: str, tools: list[str], steps: list[str], difficulty: str) -> dict[str, Any]:
    return {
        "question": question,
        "reference_answer": answer,
        "expected_tools": tools,
        "expected_steps": steps,
        "difficulty": difficulty,
    }


def _build_ecommerce_records() -> list[dict[str, Any]]:
    """从 ecommerce_demo 现算，产出三档难度的 agent 任务（golden 随库重建自动更新）。"""
    records: list[dict[str, Any]] = []

    # ===== 易：单表查找 / 简单过滤（expected_tools=["sql_execute"]）=====
    order_no, status, pay_amount, _cid = _one(
        "SELECT order_no,status,pay_amount,customer_id FROM orders "
        "WHERE status='completed' ORDER BY id LIMIT 1"
    )
    records.append(_rec(
        f"订单 {order_no} 现在是什么状态？",
        f"订单 {order_no} 当前状态为 {status}（{_STATUS_ZH.get(status, status)}），"
        f"实付金额 {_money(pay_amount)} 元。",
        ["sql_execute"],
        [f"理解问题：查询订单 {order_no} 的状态",
         "调用 sql_execute 在 orders 表按 order_no 过滤查询 status",
         "整理查询结果并回复用户"],
        "easy",
    ))

    p_name, p_brand, p_price, p_stock, p_status = _one(
        "SELECT name,brand,price,stock,status FROM products ORDER BY id LIMIT 1"
    )
    records.append(_rec(
        f"商品「{p_name}」（{p_brand}）现在的售价和库存分别是多少？",
        f"商品「{p_name}」（{p_brand}）当前售价 {_money(p_price)} 元，库存 {_int(p_stock)} 件，"
        f"状态 {p_status}。",
        ["sql_execute"],
        ["理解问题：查询该商品的售价与库存",
         "调用 sql_execute 在 products 表按商品名过滤查询 price、stock",
         "整理并回复商品价格与库存"],
        "easy",
    ))

    c_id, c_name, c_city, c_cnt = _one(
        "SELECT c.id,c.name,c.city,COUNT(o.id) FROM customers c "
        "JOIN orders o ON o.customer_id=c.id GROUP BY c.id,c.name,c.city "
        "ORDER BY c.id LIMIT 1"
    )
    records.append(_rec(
        f"客户{c_name}（ID {c_id}，{c_city}）一共下过多少笔订单？",
        f"客户{c_name}（ID {c_id}）累计下过 {_int(c_cnt)} 笔订单。",
        ["sql_execute"],
        ["理解问题：统计指定客户的订单数",
         "调用 sql_execute 关联 orders 按 customer_id 计数",
         "回复订单总数"],
        "easy",
    ))

    total_orders = _one("SELECT COUNT(*) FROM orders")[0]
    records.append(_rec(
        "系统里目前一共有多少笔订单？",
        f"目前系统内共有 {_int(total_orders)} 笔订单。",
        ["sql_execute"],
        ["理解问题：统计订单总量",
         "调用 sql_execute 对 orders 表 COUNT(*)",
         "回复订单总数"],
        "easy",
    ))

    cancelled = _one("SELECT COUNT(*) FROM orders WHERE status='cancelled'")[0]
    records.append(_rec(
        "有多少笔订单处于已取消（cancelled）状态？",
        f"共有 {_int(cancelled)} 笔订单处于已取消（cancelled）状态。",
        ["sql_execute"],
        ["理解问题：统计已取消订单数",
         "调用 sql_execute 在 orders 表按 status='cancelled' 过滤计数",
         "回复已取消订单数"],
        "easy",
    ))

    rv_name, rv_n, rv_avg = _one(
        "SELECT p.name,COUNT(r.id),ROUND(AVG(r.rating),2) FROM product_reviews r "
        "JOIN products p ON p.id=r.product_id GROUP BY p.id,p.name "
        "ORDER BY COUNT(r.id) DESC LIMIT 1"
    )
    records.append(_rec(
        f"商品「{rv_name}」目前有多少条评价，平均评分是多少？",
        f"商品「{rv_name}」共有 {_int(rv_n)} 条评价，平均评分 {rv_avg} 分（满分 5 分）。",
        ["sql_execute"],
        ["理解问题：查询该商品的评价数与平均评分",
         "调用 sql_execute 关联 product_reviews 按 product_id 计数并 AVG(rating)",
         "回复评价数与平均分"],
        "easy",
    ))

    # ===== 中：聚合 / 分组 / 连接（expected_tools=["sql_execute"]）=====
    gmv_cnt, gmv_sum = _one(
        "SELECT COUNT(*),ROUND(SUM(pay_amount),2) FROM orders "
        "WHERE status='completed' AND created_at>='2026-07-01' AND created_at<'2026-08-01'"
    )
    records.append(_rec(
        "统计上个月（2026 年 7 月）已完成订单的总成交额（GMV）。",
        f"2026 年 7 月已完成订单共 {_int(gmv_cnt)} 笔，总成交额（GMV）为 {_money(gmv_sum)} 元。",
        ["sql_execute"],
        ["理解问题：按月统计已完成订单 GMV",
         "调用 sql_execute 在 orders 表按 status 与 created_at 月份过滤，SUM(pay_amount)",
         "回复订单笔数与 GMV"],
        "medium",
    ))

    top_rows = _mysql_rows(
        "SELECT p.name,SUM(oi.quantity) q FROM order_items oi "
        "JOIN orders o ON o.id=oi.order_id JOIN products p ON p.id=oi.product_id "
        "WHERE o.status='completed' AND o.created_at>='2026-07-01' AND o.created_at<'2026-08-01' "
        "GROUP BY p.id,p.name ORDER BY q DESC LIMIT 3"
    )
    top_txt = "；".join(f"{name}（{_int(q)} 件）" for name, q in top_rows)
    records.append(_rec(
        "上个月（2026 年 7 月）已完成订单里，销量最高的三个商品分别是哪些，各卖了多少件？",
        f"2026 年 7 月销量前三的商品为：{top_txt}。",
        ["sql_execute"],
        ["理解问题：求上月销量 Top3 商品",
         "调用 sql_execute 关联 order_items/orders/products 按商品聚合 SUM(quantity)，取前三",
         "整理榜单回复"],
        "medium",
    ))

    ret_rows = _mysql_rows(
        "SELECT reason,COUNT(*) c FROM order_returns GROUP BY reason ORDER BY c DESC"
    )
    ret_txt = "；".join(f"{reason} {_int(c)} 单" for reason, c in ret_rows)
    ret_top_reason, ret_top_c = ret_rows[0]
    records.append(_rec(
        "各退货原因的退货单数分别是多少？最常见的退货原因是什么？",
        f"各退货原因分布为：{ret_txt}。其中最常见的是「{ret_top_reason}」，共 {_int(ret_top_c)} 单。",
        ["sql_execute"],
        ["理解问题：统计退货原因分布",
         "调用 sql_execute 在 order_returns 表按 reason 分组计数并降序",
         "回复分布与最常见原因"],
        "medium",
    ))

    ch_rows = _mysql_rows(
        "SELECT channel,ROUND(AVG(pay_amount),2) FROM orders WHERE status='completed' "
        "GROUP BY channel ORDER BY 2 DESC"
    )
    ch_txt = "；".join(f"{ch} {_money(v)} 元" for ch, v in ch_rows)
    records.append(_rec(
        "各下单渠道（channel）已完成订单的平均客单价分别是多少？",
        f"已完成订单各渠道平均客单价：{ch_txt}。",
        ["sql_execute"],
        ["理解问题：按渠道求已完成订单的平均客单价",
         "调用 sql_execute 在 orders 表按 status='completed' 过滤后按 channel 分组求 AVG(pay_amount)",
         "回复各渠道客单价"],
        "medium",
    ))

    city, city_c = _one(
        "SELECT city,COUNT(*) FROM customers GROUP BY city ORDER BY COUNT(*) DESC LIMIT 1"
    )
    records.append(_rec(
        "客户数量最多的城市是哪个，有多少名客户？",
        f"客户数量最多的城市是{city}，共有 {_int(city_c)} 名客户。",
        ["sql_execute"],
        ["理解问题：求客户数最多的城市",
         "调用 sql_execute 在 customers 表按 city 分组计数取 Top1",
         "回复城市与客户数"],
        "medium",
    ))

    cat_name, cat_c = _one(
        "SELECT cat.name,COUNT(*) FROM products p JOIN categories cat ON cat.id=p.category_id "
        "WHERE p.status='on_sale' GROUP BY cat.id,cat.name ORDER BY COUNT(*) DESC LIMIT 1"
    )
    records.append(_rec(
        "哪个商品类目在售（on_sale）商品数量最多，有多少款？",
        f"在售商品数量最多的类目是「{cat_name}」，共 {_int(cat_c)} 款在售商品。",
        ["sql_execute"],
        ["理解问题：求在售商品最多的类目",
         "调用 sql_execute 关联 products/categories 按类目统计 on_sale 商品数取 Top1",
         "回复类目与款数"],
        "medium",
    ))

    # ===== 难：多步分析 / 趋势 / 可视化 / 结构探查 =====
    mom_rows = _mysql_rows(
        "SELECT DATE_FORMAT(created_at,'%Y-%m') m,ROUND(SUM(pay_amount),2) FROM orders "
        "WHERE status='completed' AND created_at>='2026-06-01' AND created_at<'2026-08-01' "
        "GROUP BY m ORDER BY m"
    )
    mom = {m: v for m, v in mom_rows}
    jun, jul = float(mom["2026-06"]), float(mom["2026-07"])
    delta = (jul - jun) / jun * 100 if jun else 0.0
    records.append(_rec(
        "对比 2026 年 6 月和 7 月已完成订单的 GMV，7 月环比 6 月变化了多少？",
        f"2026 年 6 月已完成 GMV 为 {_money(mom['2026-06'])} 元，7 月为 {_money(mom['2026-07'])} 元，"
        f"7 月环比{'下降' if delta < 0 else '上升'} {abs(delta):.1f}%。",
        ["sql_execute", "data_analysis"],
        ["理解问题：计算 GMV 环比变化",
         "调用 sql_execute 按月汇总 6、7 月已完成 GMV",
         "调用 data_analysis 计算环比变化率",
         "回复两月 GMV 与环比结论"],
        "hard",
    ))

    sla_rows = _mysql_rows(
        "SELECT priority,ROUND(AVG(first_response_minutes),1) FROM support_tickets "
        "GROUP BY priority ORDER BY 2"
    )
    sla_txt = "；".join(f"{p} {v} 分钟" for p, v in sla_rows)
    sla_fastest = sla_rows[0][0]
    records.append(_rec(
        "按优先级统计客服工单的平均首次响应时长（分钟），并指出哪个优先级响应最快。",
        f"各优先级平均首次响应时长：{sla_txt}。其中「{sla_fastest}」优先级响应最快。",
        ["sql_execute"],
        ["理解问题：按优先级求平均首次响应时长",
         "调用 sql_execute 在 support_tickets 表按 priority 分组求 AVG(first_response_minutes)",
         "回复各优先级时长并指出最快"],
        "hard",
    ))

    rev_rows = _mysql_rows(
        "SELECT cat.name,ROUND(SUM(oi.subtotal),2) rev FROM order_items oi "
        "JOIN products p ON p.id=oi.product_id JOIN categories cat ON cat.id=p.category_id "
        "JOIN orders o ON o.id=oi.order_id WHERE o.status='completed' "
        "GROUP BY cat.id,cat.name ORDER BY rev DESC LIMIT 3"
    )
    rev_txt = "；".join(f"{name}（{_money(rev)} 元）" for name, rev in rev_rows)
    records.append(_rec(
        "按已完成订单的销售额统计，营收最高的三个商品类目是哪些？",
        f"营收最高的三个类目为：{rev_txt}。",
        ["sql_execute"],
        ["理解问题：求营收 Top3 类目",
         "调用 sql_execute 关联 order_items/products/categories 按类目 SUM(subtotal) 取前三",
         "回复类目营收榜"],
        "hard",
    ))

    chart_rows = _mysql_rows(
        "SELECT DATE_FORMAT(created_at,'%Y-%m') m,COUNT(*) FROM orders "
        "WHERE created_at>='2026-02-01' AND created_at<'2026-08-01' GROUP BY m ORDER BY m"
    )
    chart_txt = "，".join(f"{m}: {_int(c)}" for m, c in chart_rows)
    records.append(_rec(
        "把最近半年（2026-02 至 2026-07）每月的订单量用柱状图展示出来。",
        f"最近半年各月订单量为：{chart_txt}。已按月生成柱状图。",
        ["sql_execute", "render_ui"],
        ["理解问题：需要每月订单量并可视化",
         "调用 sql_execute 按月分组统计订单量",
         "调用 render_ui 生成柱状图",
         "回复图表与数据"],
        "hard",
    ))

    order_tabs = _mysql_rows(
        "SELECT table_name FROM information_schema.tables WHERE table_schema='ecommerce_demo' "
        "AND table_name LIKE '%order%' ORDER BY table_name"
    )
    tab_list = "、".join(t[0] for t in order_tabs)
    records.append(_rec(
        "ecommerce_demo 数据库里都有哪些跟订单相关的表？它们大致存什么？",
        f"订单相关的表主要有：{tab_list}。其中 orders 为订单主表（状态/金额/渠道），"
        f"order_items 为订单明细行（商品与数量），order_returns 记录退货；"
        f"此外 shipments 记录发货物流。",
        ["get_schema"],
        ["理解问题：了解订单相关的表结构",
         "调用 get_schema 获取库表结构",
         "筛选与订单相关的表并说明用途"],
        "hard",
    ))

    return records


def _scenario_ecommerce_agent() -> dict[str, Any] | None:
    """电商场景 Agent 集：构建期直连 ecommerce_demo 现算 golden。

    DB 不可用时返回 None（由 main 跳过并告警），不阻断其余数据集产出。
    """
    try:
        records = _build_ecommerce_records()
    except _EcommerceDBError as exc:
        print(f"  [WARN] 跳过 scenario_ecommerce_agent（ecommerce_demo 不可用）：{exc}", file=sys.stderr)
        return None
    return {
        "dataset_id": "scenario_ecommerce_agent",
        "name": "电商场景 Agent 测评集（DB 现算）",
        "description": (
            "从 ecommerce_demo 现算 golden 的三档难度 agent 任务，expected_tools 锚定路径不变量 "
            "sql_execute（数据类任务无论走词法 get_schema→sql_execute 还是语义 semantic_query→sql_execute "
            "都恒调 sql_execute）；分析题追加 data_analysis、画图题追加 render_ui、纯结构探查用 get_schema，"
            "命中全量 Agent 指标。"
        ),
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
    parser.add_argument("--limit", type=int, default=DEFAULT_LIMIT, help="每个 HF 集导出上限（默认 80，与已提交产物一致）")
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
        if ds is None:  # DB 不可用等原因跳过（builder 已告警）
            continue
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
