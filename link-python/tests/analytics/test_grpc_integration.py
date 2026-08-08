"""gRPC 服务集成测试。

需要 gRPC 服务器运行。使用 pytest -m grpc 运行这些测试。
"""


import grpc
import pandas as pd
import pytest

from proto import analytics_pb2
from proto import analytics_pb2_grpc
from grpc_service.analytics_servicer import AnalyticsServicer, create_analytics_server

pytestmark = pytest.mark.grpc


@pytest.fixture
def grpc_server():
    """创建测试用 gRPC 服务器"""
    server = create_analytics_server(port=50054)
    server.start()
    yield server
    server.stop(0)


@pytest.fixture
def grpc_channel():
    """创建测试用 gRPC 通道"""
    channel = grpc.insecure_channel("127.0.0.1:50054")
    yield channel
    channel.close()


class TestAnalyticsGrpcService:
    """Analytics gRPC 服务测试"""

    def test_compute_metrics_basic(self, grpc_channel):
        """测试基础指标计算"""
        stub = analytics_pb2_grpc.AnalyticsServiceStub(grpc_channel)

        # 构建测试数据
        proto_df = analytics_pb2.DataFrame()
        proto_df.columns.extend(["A", "B", "C"])
        for i in range(1, 11):
            row = proto_df.rows.add()
            row.values.extend([str(i), str(i * 2), str(i * 1.5)])

        # 构建指标配置
        metric = analytics_pb2.MetricConfig()
        metric.type = analytics_pb2.MetricType.MEAN
        metric.column = "A"

        request = analytics_pb2.MetricsRequest()
        request.data.CopyFrom(proto_df)
        request.metrics.extend([metric])

        # 调用 RPC
        response = stub.ComputeMetrics(request)

        assert response.success
        assert len(response.results) == 1
        assert response.results[0].name == "MEAN_A"
        assert abs(response.results[0].value - 5.5) < 0.01

    def test_analyze_trend(self, grpc_channel):
        """测试趋势分析"""
        stub = analytics_pb2_grpc.AnalyticsServiceStub(grpc_channel)

        # 构建测试数据 - 上升趋势
        proto_df = analytics_pb2.DataFrame()
        proto_df.columns.extend(["value"])
        for i in range(1, 21):
            row = proto_df.rows.add()
            row.values.extend([str(i)])

        request = analytics_pb2.TrendRequest()
        request.data.CopyFrom(proto_df)
        request.value_column = "value"

        # 调用 RPC
        response = stub.AnalyzeTrend(request)

        assert response.success
        assert response.trend.direction == "up"
        assert response.trend.r_squared > 0.9

    def test_discover_insights(self, grpc_channel):
        """测试洞察发现"""
        stub = analytics_pb2_grpc.AnalyticsServiceStub(grpc_channel)

        # 构建测试数据
        proto_df = analytics_pb2.DataFrame()
        proto_df.columns.extend(["A", "B"])
        for i in range(1, 11):
            row = proto_df.rows.add()
            row.values.extend([str(i), str(i * 2)])

        request = analytics_pb2.InsightRequest()
        request.data.CopyFrom(proto_df)
        request.options.include_trend = True
        request.options.include_correlation = True
        request.options.correlation_threshold = 0.9

        # 调用 RPC
        response = stub.DiscoverInsights(request)

        assert response.success
        assert len(response.insights) > 0

        # 检查洞察结构
        for insight in response.insights:
            assert insight.title
            assert insight.description
            assert 0 <= insight.confidence <= 1


class TestDataFrameConversion:
    """DataFrame 转换测试"""

    def test_proto_to_dataframe_empty(self):
        """测试空数据框转换"""
        servicer = AnalyticsServicer()
        proto_df = analytics_pb2.DataFrame()

        df = servicer._proto_to_dataframe(proto_df)

        assert df.empty

    def test_proto_to_dataframe_basic(self):
        """测试基础数据框转换"""
        servicer = AnalyticsServicer()

        proto_df = analytics_pb2.DataFrame()
        proto_df.columns.extend(["A", "B", "C"])
        for i in range(1, 4):
            row = proto_df.rows.add()
            row.values.extend([str(i), str(i * 2), str(i * 1.5)])

        df = servicer._proto_to_dataframe(proto_df)

        assert len(df) == 3
        assert list(df.columns) == ["A", "B", "C"]
        assert df["A"].tolist() == [1, 2, 3]

    def test_dataframe_to_proto_basic(self):
        """测试 pandas DataFrame 转 Proto"""
        servicer = AnalyticsServicer()

        df = pd.DataFrame({
            "A": [1, 2, 3],
            "B": [4, 5, 6]
        })

        proto_df = servicer._dataframe_to_proto(df)

        assert proto_df.row_count == 3
        assert proto_df.col_count == 2
        assert list(proto_df.columns) == ["A", "B"]
        assert len(proto_df.rows) == 3
