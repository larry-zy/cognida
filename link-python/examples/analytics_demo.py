"""数据分析服务使用示例。

展示数理统计、趋势分析、数据洞察等功能。
"""

import numpy as np
import pandas as pd
from rich.console import Console
from rich.table import Table

from services.analytics import (
    DescriptiveStats,
    DistributionAnalysis,
    CorrelationAnalysis,
    LinearTrendAnalyzer,
    MovingAverageAnalyzer,
    SeasonalityAnalyzer,
    GrowthRateAnalyzer,
    InsightGenerator,
)

console = Console()


def demo_descriptive_stats():
    """描述统计示例。"""
    console.print("\n[bold cyan]1. 描述统计[/bold cyan]")

    # 创建示例数据
    np.random.seed(42)
    data = pd.Series(np.random.normal(100, 15, 100))

    result = DescriptiveStats.describe(data)

    table = Table(title="描述统计结果")
    table.add_column("指标", style="cyan")
    table.add_column("值", style="yellow")

    table.add_row("均值", f"{result.mean:.2f}")
    table.add_row("中位数", f"{result.median:.2f}")
    table.add_row("标准差", f"{result.std:.2f}")
    table.add_row("最小值", f"{result.min:.2f}")
    table.add_row("最大值", f"{result.max:.2f}")
    table.add_row("偏度", f"{result.skewness:.2f}")
    table.add_row("峰度", f"{result.kurtosis:.2f}")
    table.add_row("25% 分位数", f"{result.q25:.2f}")
    table.add_row("75% 分位数", f"{result.q75:.2f}")

    console.print(table)


def demo_distribution_analysis():
    """分布分析示例。"""
    console.print("\n[bold cyan]2. 分布分析[/bold cyan]")

    # 创建正态分布数据
    np.random.seed(42)
    normal_data = pd.Series(np.random.normal(0, 1, 100))

    # 测试多种方法
    methods = ["shapiro", "ks", "normaltest"]

    table = Table(title="正态性检验结果")
    table.add_column("检验方法", style="cyan")
    table.add_column("统计量", style="yellow")
    table.add_column("P值", style="yellow")
    table.add_column("是否正态", style="green")

    for method in methods:
        result = DistributionAnalysis.test_normality(normal_data, method=method)
        table.add_row(
            result.test_type,
            f"{result.test_statistic:.4f}",
            f"{result.p_value:.4f}",
            "是" if result.is_normal else "否",
        )

    console.print(table)


def demo_correlation_analysis():
    """相关性分析示例。"""
    console.print("\n[bold cyan]3. 相关性分析[/bold cyan]")

    # 创建相关数据
    np.random.seed(42)
    df = pd.DataFrame({
        "A": np.random.normal(0, 1, 100),
        "B": np.random.normal(0, 1, 100),
        "C": np.random.normal(0, 1, 100),
    })
    df["D"] = df["A"] * 2 + np.random.normal(0, 0.1, 100)  # D 与 A 强相关

    result = CorrelationAnalysis.compute_correlation(df, method="pearson", threshold=0.7)

    # 显示相关性矩阵
    table = Table(title="Pearson 相关系数矩阵")
    table.add_column("", style="cyan")
    for col in result.matrix.keys():
        table.add_column(col, style="yellow")

    for col1, row_dict in result.matrix.items():
        row = [col1]
        for col2 in result.matrix.keys():
            value = row_dict.get(col2, 0.0)
            style = "red" if abs(value) > 0.7 else "yellow"
            row.append(f"[{style}]{value:.2f}[/{style}]")
        table.add_row(*row)

    console.print(table)

    # 显示显著相关对
    if result.significant_pairs:
        console.print("\n[bold]显著相关对:[/bold]")
        for col1, col2, corr_val in result.significant_pairs:
            console.print(f"  {col1} - {col2}: {corr_val:.4f}")


def demo_trend_analysis():
    """趋势分析示例。"""
    console.print("\n[bold cyan]4. 趋势分析[/bold cyan]")

    # 创建带趋势的数据
    np.random.seed(42)
    t = np.arange(50)
    trend = 2 * t + 100
    noise = np.random.normal(0, 5, 50)
    data = pd.Series(trend + noise)

    result = LinearTrendAnalyzer.analyze(data)

    table = Table(title="线性趋势分析结果")
    table.add_column("指标", style="cyan")
    table.add_column("值", style="yellow")

    direction_map = {"up": "上升", "down": "下降", "flat": "平稳"}
    strength_map = {"strong": "强", "moderate": "中", "weak": "弱"}

    table.add_row("趋势方向", direction_map.get(result.direction, result.direction))
    table.add_row("趋势强度", strength_map.get(result.strength, result.strength))
    table.add_row("斜率", f"{result.slope:.4f}")
    table.add_row("R平方", f"{result.r_squared:.4f}")
    table.add_row("P值", f"{result.p_value:.4e}")

    console.print(table)


def demo_moving_average():
    """移动平均示例。"""
    console.print("\n[bold cyan]5. 移动平均[/bold cyan]")

    # 创建波动数据
    np.random.seed(42)
    data = pd.Series(np.random.normal(100, 10, 20))

    sma5 = MovingAverageAnalyzer.sma(data, window=5)
    ema5 = MovingAverageAnalyzer.ema(data, span=5)

    table = Table(title="移动平均结果 (最近5个值)")
    table.add_column("原始值", style="cyan")
    table.add_column("SMA(5)", style="yellow")
    table.add_column("EMA(5)", style="green")

    for i in range(max(0, len(data) - 5), len(data)):
        table.add_row(
            f"{data.iloc[i]:.2f}",
            f"{sma5.iloc[i]:.2f}" if not pd.isna(sma5.iloc[i]) else "N/A",
            f"{ema5.iloc[i]:.2f}" if not pd.isna(ema5.iloc[i]) else "N/A",
        )

    console.print(table)


def demo_seasonality():
    """季节性分析示例。"""
    console.print("\n[bold cyan]6. 季节性分析[/bold cyan]")

    # 创建带季节性的数据
    np.random.seed(42)
    t = np.arange(100)
    seasonal = 10 * np.sin(2 * np.pi * t / 12)
    trend_line = 0.5 * t
    noise = np.random.normal(0, 2, 100)
    data = pd.Series(trend_line + seasonal + noise)

    result = SeasonalityAnalyzer.decompose(data, period=12)

    table = Table(title="季节性分析结果")
    table.add_column("指标", style="cyan")
    table.add_column("值", style="yellow")

    table.add_row("存在季节性", "是" if result.has_seasonality else "否")
    table.add_row("周期", str(result.period) if result.period else "N/A")
    table.add_row("季节性强度", f"{result.strength:.4f}")

    console.print(table)


def demo_growth_rate():
    """增长率分析示例。"""
    console.print("\n[bold cyan]7. 增长率分析[/bold cyan]")

    # 创建增长数据
    data = pd.Series([100, 110, 121, 133.1, 146.41])  # 10% 复合增长

    period_rate = GrowthRateAnalyzer.period_over_period(data, periods=1)
    cagr = GrowthRateAnalyzer.cagr(data, years=4)

    table = Table(title="增长率分析结果")
    table.add_column("周期", style="cyan")
    table.add_column("值", style="yellow")
    table.add_column("环比增长率", style="green")

    for i in range(1, len(data)):
        table.add_row(
            f"t={i}",
            f"{data.iloc[i]:.2f}",
            f"{period_rate.iloc[i] * 100:.2f}",
        )

    console.print(table)
    console.print(f"\n[green]复合年增长率 (CAGR): {cagr * 100:.2f}[/green]")


def demo_insights():
    """洞察生成示例。"""
    console.print("\n[bold cyan]8. 数据洞察[/bold cyan]")

    # 创建带异常和趋势的数据
    np.random.seed(42)
    trend_data = list(np.linspace(100, 200, 20))
    normal_data = trend_data + list(np.random.normal(0, 5, 20).tolist())

    # 添加异常值
    normal_data[5] = 300  # 突增
    normal_data[15] = 50  # 突降

    df = pd.DataFrame({
        "sales": normal_data,
        "visitors": [x * 10 + np.random.normal(0, 50) for x in normal_data],
    })

    generator = InsightGenerator()
    insights = generator.generate(
        df,
        options={
            "include_trend": True,
            "include_anomaly": True,
            "include_correlation": True,
            "correlation_threshold": 0.9,
        },
    )

    table = Table(title="数据洞察结果")
    table.add_column("严重程度", style="cyan")
    table.add_column("类型", style="yellow")
    table.add_column("标题", style="green")
    table.add_column("描述", style="white")
    table.add_column("置信度", style="magenta")

    for insight in insights[:10]:  # 只显示前10条
        table.add_row(
            str(insight.severity),
            str(insight.type),
            insight.title,
            insight.description[:50] + "..."
            if len(insight.description) > 50
            else insight.description,
            f"{insight.confidence:.2f}",
        )

    console.print(table)


def demo_all():
    """运行所有示例。"""
    console.print("[bold green]=== 数据分析服务示例 ===[/bold green]")

    demo_descriptive_stats()
    demo_distribution_analysis()
    demo_correlation_analysis()
    demo_trend_analysis()
    demo_moving_average()
    demo_seasonality()
    demo_growth_rate()
    demo_insights()

    console.print("\n[bold green]=== 示例完成 ===[/bold green]")


if __name__ == "__main__":
    demo_all()
