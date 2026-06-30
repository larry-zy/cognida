# Python 评测服务实现错误总结

本文档记录了 Python 评测服务实现过程中遇到的所有错误及解决方案。

---

## 1. 语法错误

### 错误: 字典语法缺少冒号
**文件**: `services/evaluation/graders/builtin/retrieval.py:283`

**错误代码**:
```python
metrics {"map": 0.0},  # 缺少冒号
```

**修复**:
```python
metrics={"map": 0.0},
```

**原因**: Python 字典字面量语法要求键和值之间用冒号分隔。

---

## 2. 导入路径错误

### 错误: register_grader 导入位置错误
**文件**: `services/evaluation/graders/builtin/retrieval.py`, `generation.py`, `semantic.py`, `llm.py`

**错误代码**:
```python
from ...graders.base import (
    BaseGrader,
    GraderMode,
    GraderScore,
    MetricType,
    register_grader,  # 错误：register_grader 在 registry.py 中
)
```

**修复**:
```python
from ...graders.base import (
    BaseGrader,
    GraderMode,
    GraderScore,
    MetricType,
)
from ...graders.registry import register_grader  # 正确
```

**原因**: `register_grader` 装饰器定义在 `registry.py` 中，不在 `base.py`。

---

### 错误: LLM 服务导入路径错误
**文件**: `services/evaluation/graders/builtin/llm.py:16`

**错误代码**:
```python
from ...services.llm.client import LLMClient  # 路径错误
```

**修复**:
```python
from ...llm.client import LLMClient  # 评测服务有独立的 llm 模块
```

**原因**: 评测服务有独立的 `services/evaluation/llm/` 模块，不需要通过 `services.llm` 访问。

---

## 3. Pydantic 模型错误

### 错误: Pydantic 无法识别自定义类型
**文件**: `services/evaluation/runner.py`

**错误信息**:
```
PydanticSchemaGenerationError: Unable to generate pydantic-core schema for <class 'services.evaluation.metrics.retrieval.RetrievalMetrics'>.
```

**原因**: `RetrievalMetrics`、`GenerationMetrics`、`SemanticMetrics`、`LLMJudgeMetrics` 是普通类，Pydantic 无法生成 schema。

**修复**: 转换为 Pydantic BaseModel

```python
# 修复前
class RetrievalMetrics:
    def __init__(self, precision: float, ...):
        self.precision = precision

    def to_dict(self) -> dict:
        return {"precision": self.precision}

# 修复后
from pydantic import BaseModel

class RetrievalMetrics(BaseModel):
    precision: float
    recall: float
    # ...

    model_config = {"arbitrary_types_allowed": True}
```

---

### 错误: .to_dict() 方法不存在
**文件**: `services/evaluation/metrics/retrieval.py`, `generation.py`, `semantic.py`, `llm_judge.py`

**错误信息**:
```
AttributeError: 'RetrievalMetrics' object has no attribute 'to_dict'
```

**原因**: Pydantic BaseModel 使用 `.model_dump()` 而不是 `.to_dict()`。

**修复**:
```python
# 修复前
return metrics.to_dict()

# 修复后
return metrics.model_dump()
```

---

## 4. 函数调用参数错误

### 错误: rouge_n 和 rouge_l 参数类型错误
**文件**: `services/evaluation/graders/builtin/generation.py`

**错误代码**:
```python
rouge_result = rouge_n([reference], [response], n=1)  # 错误：传递了列表
```

**修复**:
```python
rouge_result = rouge_n(reference, response, n=1)  # 正确：传递字符串
```

**原因**: `rouge_n` 和 `rouge_l` 函数签名为 `def rouge_n(reference: str, hypothesis: str)`，不接受列表。

---

### 错误: bleu_at_n 参数类型错误
**文件**: `services/evaluation/graders/builtin/generation.py`

**错误代码**:
```python
bleu_result = bleu_at_n([reference], [response], n=n)  # 错误
```

**修复**:
```python
bleu_result = bleu_at_n(reference, response, n=n)  # 正确
```

---

### 错误: compute_retrieval_metrics 参数错误
**文件**: `services/evaluation/runner.py:418`

**错误代码**:
```python
return compute_retrieval_metrics(
    [retrieved_relevant],  # 错误：嵌套列表
    k=min(self.config.top_k, len(retrieved_relevant)),
)
```

**修复**:
```python
return compute_retrieval_metrics(
    retrieved_relevant,  # 正确：直接传递列表
    k=min(self.config.top_k, len(retrieved_relevant)),
)
```

---

### 错误: compute_generation_metrics 参数错误
**文件**: `services/evaluation/runner.py:439`

**错误代码**:
```python
return compute_generation_metrics(
    references=[reference],  # 错误：函数不接受此参数名
    outputs=[generated],
)
```

**修复**:
```python
return compute_generation_metrics(
    reference=reference,
    hypothesis=generated,
)
```

**原因**: 函数签名为 `compute_generation_metrics(reference: str, hypothesis: str)`。

---

### 错误: compute_semantic_metrics_async 参数名错误
**文件**: `services/evaluation/runner.py:462`

**错误代码**:
```python
result = await compute_semantic_metrics_async(
    references=[reference],
    outputs=[generated],  # 错误：应该是 hypotheses
)
```

**修复**:
```python
result = await compute_semantic_metrics_async(
    references=[reference],
    hypotheses=[generated],  # 正确
)
```

---

## 5. 属性访问错误

### 错误: RetrievalMetrics.map 不存在
**文件**: `services/evaluation/runner.py:621`

**错误信息**:
```
AttributeError: 'RetrievalMetrics' object has no attribute 'map'
```

**错误代码**:
```python
map=sum(m.map for m in metrics if m.map) / count,  # 错误
```

**修复**:
```python
map_score=sum(m.map_score for m in metrics if m.map_score) / count,  # 正确
```

**原因**: 属性名为 `map_score` 而不是 `map`（`map` 是 Python 内置函数）。

---

## 6. Demo 导入路径错误

### 错误: 模块路径错误
**文件**: `examples/evaluation_demo.py:6`

**错误信息**:
```
ModuleNotFoundError: No module named 'services.evaluation.runners'
```

**错误代码**:
```python
from services.evaluation.runners import EvaluationRunner, EvaluationConfig
```

**修复**:
```python
from services.evaluation.runner import EvaluationRunner, EvaluationConfig
```

**原因**: 文件名为 `runner.py`（单数），不是 `runners.py`。

---

## 7. 测试问题

### 错误: 标点符号测试失败
**文件**: `tests/test_tokenizer.py:82-86`

**错误信息**:
```
AssertionError: assert "!" in result
```

**错误代码**:
```python
text = "Hello, 世界! 这是个测试。"  # ! 是 ASCII 感叹号
result = remove_punctuation(text, keep_chinese_punct=True)
assert "!" in result  # 失败：ASCII ! 总是被移除
```

**修复**:
```python
text = "Hello, 世界！这是个测试。"  # ！是中文全角感叹号
result = remove_punctuation(text, keep_chinese_punct=True)
assert "！" in result  # 通过
```

**原因**: `remove_punctuation` 中的 `keep_chinese_punct` 只保留中文全角标点（`！`），不保留 ASCII 标点（`!`）。

---

## 8. Protobuf 版本不匹配

### 错误: Protobuf runtime 版本不兼容
**错误信息**:
```
google.protobuf.runtime_version.VersionError: Detected incompatible Protobuf Gencode/Runtime versions
when loading proto/evaluation.proto: gencode 6.31.1 runtime 5.29.6.
```

**修复**:
```bash
python scripts/generate_grpc.py  # 重新生成 gRPC 代码
```

**原因**: 生成的 protobuf 代码与运行时版本不匹配，需要重新生成。

---

## 9. 缺失依赖

### 错误: jieba 未安装
**文件**: `tests/test_tokenizer.py`

**错误信息**:
```
ImportError: jieba is required for Chinese tokenization. Install with: pip install jieba
```

**修复**:
```bash
pip install jieba
```

---

### 错误: sentence_transformers 未安装
**文件**: `services/evaluation/metrics/semantic.py`

**错误信息**:
```
ImportWarning: No module named 'sentence_transformers'
```

**说明**: 这是可选依赖，系统会降级到 TF-IDF 余弦相似度。完整功能需要安装：

```bash
pip install sentence-transformers
```

---

## 经验总结

1. **类型注解很重要**: 函数签名的类型注解帮助快速发现参数类型错误
2. **Pydantic BaseModel vs 普通类**: 在需要 schema 验证时，应使用 Pydantic BaseModel
3. **导入路径要清晰**: 相对导入时要确认正确的模块路径
4. **Python 命名规范**: 避免使用 Python 内置函数/关键字作为属性名（如 `map`）
5. **中文标点符号**: 中文全角标点（`！。，`）与 ASCII 标点（`!.,`）是不同的字符
6. **依赖管理**: 明确区分必需依赖和可选依赖
