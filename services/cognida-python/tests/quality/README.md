# 质量服务测试文档

## 测试目录结构

```
tests/quality/
├── __init__.py                 # 测试包初始化
├── conftest.py                 # pytest配置
├── fixtures/                   # 测试数据
│   ├── sample_data.csv        # 示例结构化数据
│   └── text_samples.txt       # 示例文本数据
├── test_dimensions.py          # 结构化评估器测试
├── test_unstructured.py        # 非结构化评估器测试
├── test_cleaning.py            # 清洗模块测试
├── test_rules.py               # 规则引擎测试
├── test_drift_detector.py      # 漂移检测测试
├── test_pipeline.py            # 流程编排测试
├── test_evaluator.py           # 评估器集成测试
├── test_servicer.py            # gRPC服务测试
└── test_e2e.py                 # 端到端测试
```

## 运行测试

### 运行所有测试

```bash
# 使用pytest
pytest tests/quality

# 或使用测试脚本
python scripts/run_quality_tests.py
```

### 运行特定测试文件

```bash
pytest tests/quality/test_dimensions.py
pytest tests/quality/test_evaluator.py
```

### 运行特定测试类或方法

```bash
# 运行特定测试类
pytest tests/quality/test_dimensions.py::TestCompletenessEvaluator

# 运行特定测试方法
pytest tests/quality/test_dimensions.py::TestCompletenessEvaluator::test_evaluate_completeness
```

### 按标记运行

```bash
# 只运行单元测试
pytest tests/quality -m unit

# 只运行集成测试
pytest tests/quality -m integration

# 跳过慢速测试
pytest tests/quality -m "not slow"
```

### 生成覆盖率报告

```bash
# 使用测试脚本
python scripts/run_quality_tests.py --coverage

# 或直接使用pytest
pytest tests/quality --cov=services.quality --cov-report=html
```

## 测试说明

### 1. 结构化评估器测试 (test_dimensions.py)

测试各个结构化数据评估维度：

- **CompletenessEvaluator**: 空值检测、必填字段检测
- **AccuracyEvaluator**: 范围检查、模式验证
- **ConsistencyEvaluator**: 跨字段逻辑规则检查
- **ValidityEvaluator**: 类型验证、长度验证
- **UniquenessEvaluator**: 重复检测
- **TimelinessEvaluator**: 时效性检查

### 2. 非结构化评估器测试 (test_unstructured.py)

测试各个非结构化文本评估维度：

- **ReadabilityEvaluator**: 编码检测、乱码检测、语言检测
- **InformationDensityEvaluator**: 词数统计、有效词比例、停用词比例
- **LanguageQualityEvaluator**: 标点符号检测、重复词检测
- **DuplicationEvaluator**: 精确重复、近似重复检测
- **PIIDetector**: 手机号、邮箱、身份证号、地址检测
- **RelevanceEvaluator**: 关键词匹配、主题分类

### 3. 清洗模块测试 (test_cleaning.py)

测试数据清洗功能：

- **TextCleaner**: HTML标签移除、空白字符标准化、编码修复
- **DedupProcessor**: 精确去重、模糊去重、MinHash去重
- **Denoiser**: 广告移除、垃圾内容检测、模板移除
- **FormatConverter**: 编码转换、日期/数字格式标准化
- **PIIMasker**: 手机号、邮箱、身份证号脱敏

### 4. 规则引擎测试 (test_rules.py)

测试规则配置和加载：

- YAML规则文件加载
- 字段规则解析
- 维度规则解析
- 规则合并
- 规则验证

### 5. 漂移检测测试 (test_drift_detector.py)

测试数据漂移检测：

- 基线设置
- KS检验计算
- 漂移检测
- 阈值配置

### 6. 流程编排测试 (test_pipeline.py)

测试端到端流程：

- 结构化数据流程
- 非结构化数据流程
- 钩子系统
- 决策逻辑

### 7. 评估器集成测试 (test_evaluator.py)

测试评估器主入口：

- 数据加载（CSV/JSON/Excel/Parquet）
- 结构化数据评估
- 非结构化数据评估
- 建议生成

### 8. gRPC服务测试 (test_servicer.py)

测试gRPC服务实现：

- EvaluateQuality RPC
- EvaluateUnstructuredQuality RPC
- CleanData RPC
- ProcessPipeline RPC
- DetectDrift RPC
- ListDimensions RPC
- GetQualityRules RPC

### 9. 端到端测试 (test_e2e.py)

完整场景测试：

- 客户数据质量场景
- 电商产品数据场景
- 日志数据场景
- 问卷调查数据场景
- 文件操作往返测试
- 性能测试

## 测试数据

测试数据位于 `tests/quality/fixtures/` 目录：

- `sample_data.csv`: 包含各种质量问题的示例数据
- `text_samples.txt`: 包含各种情况的文本样本

## 添加新测试

1. 在相应的测试文件中添加测试类或测试方法
2. 使用 `@pytest.fixture` 创建共享的测试数据
3. 使用 `assert` 语句验证结果
4. 使用适当的标记（`@pytest.mark.unit`, `@pytest.mark.integration`, `@pytest.mark.slow`）

示例：

```python
import pytest
from services.quality.dimensions.completeness import CompletenessEvaluator

@pytest.mark.unit
class TestMyNewEvaluator:
    """我的新评估器测试。"""

    @pytest.fixture
    def sample_data(self):
        """创建测试数据。"""
        return pd.DataFrame({"col1": [1, 2, 3]})

    def test_evaluation(self, sample_data):
        """测试评估功能。"""
        evaluator = CompletenessEvaluator()
        result = evaluator.evaluate(sample_data)

        assert result.name == "completeness"
        assert 0 <= result.score <= 100
```

## CI/CD集成

在CI/CD流程中运行测试：

```bash
# 完整测试套件
python scripts/run_quality_tests.py --coverage

# 快速测试（跳过慢速测试）
python scripts/run_quality_tests.py --fast
```
