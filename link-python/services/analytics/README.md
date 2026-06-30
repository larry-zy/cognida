# 数据分析服务 (Analytics Service)

提供数理统计、趋势分析、数据洞察等核心分析能力。

## 功能概览

### 数理统计 (`statistics.py`)

- **描述统计**: 均值、中位数、标准差、分位数、偏度、峰度
- **分布分析**: 正态性检验 (Shapiro-Wilk, KS, D'Agostino)
- **相关性分析**: Pearson、Spearman、Kendall 相关性
- **假设检验**: 独立样本 t 检验、卡方检验

### 趋势分析 (`trend.py`)

- **线性趋势**: 趋势检测、强度评估、显著性检验
- **移动平均**: SMA、EMA、双指数平滑
- **季节分解**: STL 分解、周期检测、季节性强度
- **增长率**: 周期增长率、同比增长率、CAGR

### 数据洞察 (`insight.py`)

- **趋势洞察**: 上升/下降/平稳趋势、加速检测
- **异常洞察**: 突增/突降异常、离群值检测
- **相关性洞察**: 强相关发现、关系分析
- **整合生成**: 自动生成排序后的洞察列表

### 数据验证 (`validation.py`)

- **数据验证**: 数据框验证、列验证、数值列验证
- **时序检测**: 时间列检测、规则间隔检测
- **数据质量**: 缺失值统计、类型检查、质量报告

## 使用示例

### 基础统计

```python
from services.analytics import DescriptiveStats
import pandas as pd

data = pd.Series([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])
result = DescriptiveStats.describe(data)

print(f"均值: {result.mean}")
print(f"标准差: {result.std}")
print(f"中位数: {result.median}")
```

### 趋势分析

```python
from services.analytics import LinearTrendAnalyzer
import pandas as pd

data = pd.Series(range(1, 11))
result = LinearTrendAnalyzer.analyze(data)

print(f"方向: {result.direction}")
print(f"强度: {result.strength}")
print(f"R²: {result.r_squared}")
```

### 洞察生成

```python
from services.analytics import InsightGenerator
import pandas as pd

df = pd.DataFrame({
    "sales": [100, 110, 105, 120, 130, 125, 140, 150, 145, 160],
    "visitors": [1000, 1100, 1050, 1200, 1300, 1250, 1400, 1500, 1450, 1600]
})

generator = InsightGenerator()
insights = generator.generate(df)

for insight in insights:
    print(f"[{insight.severity}] {insight.title}")
    print(f"  {insight.description}")
```

### 数据验证

```python
from services.analytics.validation import DataValidator
import pandas as pd

df = pd.DataFrame({"A": [1, 2, 3, 4, 5]})

# 验证数据框
DataValidator.validate_dataframe(df, min_rows=3)

# 验证数值列
series = DataValidator.validate_numeric_column(df, "A")
```

## gRPC 服务

### 启动服务

```bash
# 独立启动数据分析服务
python -m grpc_service.analytics_servicer
```

### Proto 定义

参见 `link-go/api/proto/analytics.proto`

### 服务端口

- 默认端口: `50053`

### RPC 方法

- `ComputeMetrics`: 计算统计指标
- `AnalyzeTrend`: 分析趋势
- `DiscoverInsights`: 发现洞察

## 配置

### 数据量限制

```python
from services.analytics.validation import DEFAULT_MAX_ROWS, DEFAULT_MIN_ROWS

# 默认配置
DEFAULT_MAX_ROWS = 100_000  # 最大行数
DEFAULT_MIN_ROWS = 3         # 最小行数
```

### 异常检测方法

- `iqr`: 四分位距法 (默认)
- `zscore`: Z-score 法

## 依赖

- `pandas >= 2.0.0`
- `numpy >= 1.24.0`
- `scipy >= 1.10.0`
- `statsmodels >= 0.14.0`
- `scikit-learn >= 1.3.0`

## 开发

### 运行测试

```bash
# 运行所有测试
pytest tests/analytics/ -v

# 运行特定测试文件
pytest tests/analytics/test_statistics.py -v

# 运行集成测试
pytest tests/analytics/test_grpc_integration.py -v
```

## 注意事项

1. **数据量限制**: 默认最大支持 10 万行数据
2. **数值类型**: 统计分析仅支持数值型变量
3. **时序数据**: 趋势分析依赖时间序列，需正确指定时间列
4. **异常检测**: IQR 方法对异常值敏感，可根据业务调整阈值
