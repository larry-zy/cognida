"""数据分析 gRPC 服务实现。

提供数理统计、趋势分析、数据洞察等分析能力。
"""

import asyncio
from typing import TYPE_CHECKING

import grpc
import numpy as np
import pandas as pd

from core import get_logger
from services.analytics import (
    DescriptiveStats,
    DistributionAnalysis,
    CorrelationAnalysis,
    HypothesisTest,
    LinearTrendAnalyzer,
    MovingAverageAnalyzer,
    SeasonalityAnalyzer,
    GrowthRateAnalyzer,
    InsightGenerator,
)

from proto import analytics_pb2
from proto import analytics_pb2_grpc

if TYPE_CHECKING:
    from grpc import ServicerContext


class AnalyticsServicer(analytics_pb2_grpc.AnalyticsServiceServicer):
    """数据分析 gRPC 服务实现"""

    def __init__(self) -> None:
        self._logger = get_logger(__name__)

    def _proto_to_dataframe(
        self,
        proto_df: "analytics_pb2.DataFrame"
    ) -> pd.DataFrame:
        """将 Proto DataFrame 转换为 pandas DataFrame

        Args:
            proto_df: Proto DataFrame 消息

        Returns:
            pd.DataFrame: 转换后的数据框
        """
        if not proto_df.rows:
            return pd.DataFrame()

        # 构建数据字典
        data = {col: [] for col in proto_df.columns}

        for row in proto_df.rows:
            for i, value in enumerate(row.values):
                if i < len(proto_df.columns):
                    col = proto_df.columns[i]
                    data[col].append(value)

        # 尝试转换数值列
        for col in data:
            try:
                data[col] = pd.to_numeric(data[col])
            except (ValueError, TypeError):
                # 保持为字符串
                pass

        return pd.DataFrame(data)

    def _dataframe_to_proto(
        self,
        df: pd.DataFrame
    ) -> "analytics_pb2.DataFrame":
        """将 pandas DataFrame 转换为 Proto DataFrame

        Args:
            df: pandas 数据框

        Returns:
            analytics_pb2.DataFrame: Proto 数据框消息
        """
        proto_rows = []
        for _, row in df.iterrows():
            proto_row = analytics_pb2.Row()
            for val in row.values:
                proto_row.values.append(str(val))
            proto_rows.append(proto_row)

        return analytics_pb2.DataFrame(
            columns=list(df.columns),
            rows=proto_rows,
            row_count=len(df),
            col_count=len(df.columns)
        )

    def ComputeMetrics(
        self,
        request: "analytics_pb2.MetricsRequest",
        context: "grpc.ServicerContext"
    ) -> "analytics_pb2.MetricsResponse":
        """计算统计指标"""
        self._logger.info("收到 ComputeMetrics 请求")

        try:
            result = asyncio.run(self._compute_metrics_async(request))
            return result
        except Exception as e:
            self._logger.error("ComputeMetrics 处理失败", error=str(e))
            return analytics_pb2.MetricsResponse(
                success=False,
                error=str(e)
            )

    async def _compute_metrics_async(
        self,
        request: "analytics_pb2.MetricsRequest"
    ) -> "analytics_pb2.MetricsResponse":
        """异步计算统计指标"""
        # 转换数据
        df = self._proto_to_dataframe(request.data)
        if df.empty:
            return analytics_pb2.MetricsResponse(
                success=False,
                error="输入数据为空"
            )

        results = []

        for metric_config in request.metrics:
            try:
                result = await self._compute_single_metric(df, metric_config)
                results.append(result)
            except Exception as e:
                self._logger.warning(
                    "计算指标失败",
                    metric_type=metric_config.type,
                    error=str(e)
                )

        return analytics_pb2.MetricsResponse(
            success=True,
            results=results
        )

    async def _compute_single_metric(
        self,
        df: pd.DataFrame,
        config: "analytics_pb2.MetricConfig"
    ) -> "analytics_pb2.MetricResult":
        """计算单个指标

        Args:
            df: 输入数据框
            config: 指标配置

        Returns:
            MetricResult: 指标结果
        """
        metric_type = config.type
        column = config.column

        if metric_type == analytics_pb2.MetricType.COUNT:
            series = df[column].dropna() if column else df.iloc[:, 0].dropna()
            value = float(len(series))

        elif metric_type == analytics_pb2.MetricType.MEAN:
            series = df[column].dropna()
            value = float(series.mean())

        elif metric_type == analytics_pb2.MetricType.MEDIAN:
            series = df[column].dropna()
            value = float(series.median())

        elif metric_type == analytics_pb2.MetricType.STD:
            series = df[column].dropna()
            value = float(series.std())

        elif metric_type == analytics_pb2.MetricType.VAR:
            series = df[column].dropna()
            value = float(series.var())

        elif metric_type == analytics_pb2.MetricType.MIN:
            series = df[column].dropna()
            value = float(series.min())

        elif metric_type == analytics_pb2.MetricType.MAX:
            series = df[column].dropna()
            value = float(series.max())

        elif metric_type == analytics_pb2.MetricType.Q25:
            series = df[column].dropna()
            value = float(series.quantile(0.25))

        elif metric_type == analytics_pb2.MetricType.Q50:
            series = df[column].dropna()
            value = float(series.quantile(0.50))

        elif metric_type == analytics_pb2.MetricType.Q75:
            series = df[column].dropna()
            value = float(series.quantile(0.75))

        elif metric_type == analytics_pb2.MetricType.IQR:
            series = df[column].dropna()
            q25 = float(series.quantile(0.25))
            q75 = float(series.quantile(0.75))
            value = q75 - q25

        elif metric_type == analytics_pb2.MetricType.SKEWNESS:
            series = df[column].dropna()
            value = float(series.skew())

        elif metric_type == analytics_pb2.MetricType.KURTOSIS:
            series = df[column].dropna()
            value = float(series.kurtosis())

        elif metric_type == analytics_pb2.MetricType.PERCENTILE:
            series = df[column].dropna()
            percentile = float(config.params.get("percentile", 50))
            value = float(series.quantile(percentile / 100))

        elif metric_type == analytics_pb2.MetricType.CORRELATION:
            column2 = config.params.get("column2")
            if column and column2 and column in df.columns and column2 in df.columns:
                corr = df[[column, column2]].corr().iloc[0, 1]
                value = float(corr) if not pd.isna(corr) else 0.0
            else:
                value = 0.0

        else:
            value = 0.0

        return analytics_pb2.MetricResult(
            name=f"{metric_type}_{column}" if column else str(metric_type),
            value=value,
            details={}
        )

    def AnalyzeTrend(
        self,
        request: "analytics_pb2.TrendRequest",
        context: "grpc.ServicerContext"
    ) -> "analytics_pb2.TrendResponse":
        """分析趋势"""
        self._logger.info("收到 AnalyzeTrend 请求")

        try:
            result = asyncio.run(self._analyze_trend_async(request))
            return result
        except Exception as e:
            self._logger.error("AnalyzeTrend 处理失败", error=str(e))
            return analytics_pb2.TrendResponse(
                success=False,
                error=str(e)
            )

    async def _analyze_trend_async(
        self,
        request: "analytics_pb2.TrendRequest"
    ) -> "analytics_pb2.TrendResponse":
        """异步分析趋势"""
        df = self._proto_to_dataframe(request.data)

        if df.empty or request.value_column not in df.columns:
            return analytics_pb2.TrendResponse(
                success=False,
                error=f"数据为空或列不存在: {request.value_column}"
            )

        series = df[request.value_column].dropna()

        # 线性趋势分析
        trend_result = LinearTrendAnalyzer.analyze(series)

        proto_trend = analytics_pb2.TrendResult(
            direction=trend_result.direction,
            strength=trend_result.strength,
            slope=trend_result.slope,
            r_squared=trend_result.r_squared,
            p_value=trend_result.p_value,
            ci_lower=trend_result.confidence_interval[0],
            ci_upper=trend_result.confidence_interval[1]
        )

        # 季节分解
        period = request.options.period if request.options else None
        seasonality_result = SeasonalityAnalyzer.decompose(series, period=period)

        proto_seasonality = analytics_pb2.SeasonalityResult(
            has_seasonality=seasonality_result.has_seasonality,
            period=seasonality_result.period or 0,
            strength=seasonality_result.strength,
            seasonal=seasonality_result.seasonal_component,
            trend=seasonality_result.trend_component,
            residual=seasonality_result.residual
        )

        # 预测
        forecast = []
        if request.options and request.options.forecast:
            steps = request.options.forecast_steps or 5
            forecast = LinearTrendAnalyzer.forecast(series, steps)

        return analytics_pb2.TrendResponse(
            success=True,
            trend=proto_trend,
            seasonality=proto_seasonality,
            forecast=forecast
        )

    def DiscoverInsights(
        self,
        request: "analytics_pb2.InsightRequest",
        context: "grpc.ServicerContext"
    ) -> "analytics_pb2.InsightResponse":
        """发现洞察"""
        self._logger.info("收到 DiscoverInsights 请求")

        try:
            result = asyncio.run(self._discover_insights_async(request))
            return result
        except Exception as e:
            self._logger.error("DiscoverInsights 处理失败", error=str(e))
            return analytics_pb2.InsightResponse(
                success=False,
                error=str(e)
            )

    async def _discover_insights_async(
        self,
        request: "analytics_pb2.InsightRequest"
    ) -> "analytics_pb2.InsightResponse":
        """异步发现洞察"""
        df = self._proto_to_dataframe(request.data)

        if df.empty:
            return analytics_pb2.InsightResponse(
                success=False,
                error="输入数据为空"
            )

        # 构建选项
        options = {
            "include_trend": request.options.include_trend,
            "include_anomaly": request.options.include_anomaly,
            "include_correlation": request.options.include_correlation,
            "correlation_threshold": request.options.correlation_threshold or 0.7,
            "anomaly_method": request.options.anomaly_method or "iqr",
            "anomaly_threshold": request.options.anomaly_threshold or 3.0,
        }

        # 生成洞察
        generator = InsightGenerator()
        insights = generator.generate(df, options=options)

        # 转换为 Proto 格式
        proto_insights = []
        for insight in insights:
            proto_insight = analytics_pb2.Insight(
                type=str(insight.type),
                severity=str(insight.severity),
                title=insight.title,
                description=insight.description,
                confidence=insight.confidence,
                evidence={str(k): str(v) for k, v in insight.evidence.items()},
                recommendations=insight.recommendations,
                affected_metrics=insight.affected_metrics
            )
            proto_insights.append(proto_insight)

        return analytics_pb2.InsightResponse(
            success=True,
            insights=proto_insights
        )


def create_analytics_server(port: int = 50053):
    """创建数据分析 gRPC 服务器

    Args:
        port: 监听端口

    Returns:
        gRPC 服务器实例
    """
    from concurrent import futures

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))

    # 添加 Analytics 服务
    analytics_pb2_grpc.add_AnalyticsServiceServicer_to_server(
        AnalyticsServicer(), server
    )

    server.add_insecure_port(f'127.0.0.1:{port}')
    return server


async def serve_analytics_grpc(port: int = 50053) -> None:
    """启动数据分析 gRPC 服务器

    Args:
        port: 监听端口
    """
    import threading

    logger = get_logger(__name__)

    server = create_analytics_server(port)

    logger.info(f"启动数据分析 gRPC 服务器，端口: {port}")

    def start_server():
        server.start()
        server.wait_for_termination()

    server_thread = threading.Thread(target=start_server, daemon=True)
    server_thread.start()

    logger.info("数据分析 gRPC 服务器已启动")

    try:
        while server_thread.is_alive():
            await asyncio.sleep(1)
    except KeyboardInterrupt:
        logger.info("正在关闭数据分析 gRPC 服务器...")
        server.stop(0)
