# Quality Service 测试错误修复记录

修复日期: 2026-05-04

## 概述

本次修复了 Quality Service (services/quality) 中的 163 个测试用例，解决了从测试初始化到评分公式计算的各类问题。

---

## 同类问题在其他模块的排查结果

### 数据清洗组件 (services/quality/cleaning) 发现的问题

#### ✅ 已修复: pii_masker.py 邮箱正则边界问题
**文件**: `services/quality/cleaning/pii_masker.py:29`

**问题**: 与 `unstructured/pii_detector.py` 相同的 `\b` 边界符问题。

**修复前**:
```python
EMAIL_PATTERN = re.compile(r"\b[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\b")
```

**修复后**:
```python
EMAIL_PATTERN = re.compile(r"(?<![a-zA-Z0-9._%+-])[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(?![a-zA-Z0-9._%+-])")
```

#### ✅ 已检查: 其他潜在问题

| 问题类型 | 检查结果 | 说明 |
|---------|---------|------|
| 变量未定义 | ✅ 无问题 | 所有变量在使用前都有定义 |
| datetime 时区问题 | ✅ 无问题 | format_converter 不涉及时区 |
| frozen dataclass 修改 | ✅ 无问题 | 清洗模块无 frozen dataclass |
| 排序问题 | ✅ 无问题 | 清洗模块无排序操作 |
| YAML 编码 | ✅ 无问题 | 使用 yaml.safe_load |

---

## 错误分类与解决方案

### 1. 变量未定义错误

#### 错误: NameError: name 'text_length' is not defined
**文件**: `services/quality/unstructured/readability.py:91`

**原因**: 在检查文本长度时使用了未定义的 `text_length` 变量。

**解决**:
```python
# 添加变量定义
text_length = len(text)
min_text_length = config.get("min_text_length", 20)
if text_length < min_text_length:
```

---

### 2. 测试代码错误

#### 错误: NameError: name 'drifted' is not defined
**文件**: `tests/quality/test_drift_detector.py`

**原因**: 测试中使用了 `drifted` 变量，但 fixture 名称是 `drifted_data`。

**解决**:
```python
# 修改前
result = detector.detect_drift(drifted)

# 修改后
result = detector.detect_drift(drifted_data)
```

#### 错误: FrozenInstanceError: cannot assign to field 'unique'
**文件**: `tests/quality/test_dimensions.py`

**原因**: 尝试修改 frozen dataclass 的字段。

**解决**:
```python
# 修改前
rules = [FieldRule(name="id", type=FieldType.INTEGER)]
rules[0].unique = True  # 错误

# 修改后
rules = [FieldRule(name="id", type=FieldType.INTEGER, unique=True)]  # 正确
```

---

### 3. 类型不匹配错误

#### 错误: TypeError: Cannot subtract tz-naive and tz-aware datetime-like objects
**文件**: `services/quality/dimensions/timeliness.py`

**原因**: 混合使用时区感知和非时区感知的 datetime 对象进行减法运算。

**解决**:
```python
# 确保 now_for_calc 总是 tz-naive
if oldest_date.tzinfo is not None:
    oldest_date = oldest_date.tz_localize(None)
now_for_calc = now.replace(tzinfo=None)
oldest_age_days = (now_for_calc - oldest_date).days
```

#### 错误: isinstance(numpy.bool_, bool) 返回 False
**文件**: 多个测试文件

**原因**: numpy 的 bool 类型与 Python 内置 bool 不同。

**解决**:
```python
# 修改前
assert isinstance(dimension.passed, bool)

# 修改后
assert dimension.passed in (True, False)
```

---

### 4. 数据结构问题

#### 错误: baseline_stats 初始值应为 None
**文件**: `services/quality/drift_detector.py`

**原因**: 初始值设为 `{}` 导致测试期望 `None` 时失败。

**解决**:
```python
# 修改前
self._baseline_stats: dict[str, Any] = {}

# 修改后
self._baseline_stats: dict[str, Any] | None = None
```

#### 错误: get_summary() 返回类型不匹配
**文件**: `services/quality/models.py`

**原因**: 测试期望返回字典，但实现返回字符串。

**解决**:
```python
def get_summary(self) -> dict[str, Any]:
    return {
        "overall_drift_score": self.overall_drift_score,
        "drifted_field_count": sum(1 for m in self.drift_metrics if m.drifted),
        "total_field_count": len(self.drift_metrics),
        "has_drift": self.has_drift,
    }
```

---

### 5. 正则表达式问题

#### 错误: 邮箱检测在中英文混合文本中失败
**文件**: `services/quality/unstructured/pii_detector.py`

**原因**: `\b` 词边界符在中文字符后不起作用。

**解决**:
```python
# 修改前
EMAIL_PATTERN = re.compile(r"\b[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}\b")

# 修改后
EMAIL_PATTERN = re.compile(r"(?<![a-zA-Z0-9._%+-])[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(?![a-zA-Z0-9._%+-])")
```

---

### 6. YAML 编码问题

#### 错误: UnicodeDecodeError: 'utf-8' codec can't decode byte 0xbf
**文件**: `tests/quality/test_e2e.py`

**原因**: `write_text()` 方法可能在某些情况下产生编码问题。

**解决**:
```python
# 修改前
rules_path.write_text(rules_data)

# 修改后
rules_path.write_bytes(rules_data.encode('utf-8'))
```

---

### 7. 评分公式问题

#### 问题: 短文本评分过高
**文件**: `services/quality/unstructured/readability.py`

**解决**: 使非常短的文本（小于最小长度一半）成为 CRITICAL 问题。
```python
severity = SeverityLevel.CRITICAL if text_length < min_text_length / 2 else SeverityLevel.WARNING
```

#### 问题: 信息密度评分过低
**文件**: `services/quality/unstructured/information_density.py`

**解决**: 改进评分公式，使用更合理的权重分配。
```python
density_bonus = valid_ratio * 0.7 + (1 - stop_ratio) * 0.3
density_score = base_score * max(0.3, density_bonus)
```

#### 问题: 重复检测未检测到内部重复
**文件**: `services/quality/unstructured/duplication.py`

**解决**: 添加内部重复检测。
```python
def _detect_internal_duplicates(self, text: str) -> int:
    sentences = re.split(r'[。！？\n.!?]+', text)
    sentences = [s.strip() for s in sentences if s.strip()]
    # 检测重复句子...
```

---

### 8. 排序问题

#### 错误: 问题未按严重级别排序
**文件**: `services/quality/evaluator.py`

**原因**: 使用字符串值排序导致字母顺序而非优先级顺序。

**解决**:
```python
# 修改前
all_issues.sort(key=lambda i: (i.severity.value, -i.count), reverse=True)

# 修改后
severity_priority = {"critical": 1, "warning": 2, "info": 3}
all_issues.sort(key=lambda i: (severity_priority.get(i.severity.value, 99), -i.count))
```

---

### 9. 关键词匹配问题

#### 问题: 关键词匹配率过低
**文件**: `services/quality/unstructured/relevance.py`

**原因**: 依赖 jieba 分词结果进行集合交集，可能因分词不准确导致漏检。

**解决**: 使用子串匹配代替分词集合匹配。
```python
# 修改前
matched_keywords = words_set & set(expected_keywords)

# 修改后
matched_keywords = [kw for kw in expected_keywords if kw in text]
```

---

### 10. 缺少导入

#### 错误: NameError: name 'QualityReport' is not defined
**文件**: `tests/quality/test_e2e.py`

**解决**:
```python
from services.quality.models import QualityReport
```

---

## 测试结果

### 修复前
- 163 个测试用例
- 约 56 个失败

### 修复后
- 163 个测试用例
- **全部通过** ✅

---

## 经验总结

1. **数据类型一致性**: 处理 datetime、numpy 类型时要注意类型转换和比较
2. **Frozen Dataclass**: 创建新实例而非修改现有实例
3. **正则表达式边界**: 中英文混合场景下 `\b` 不可靠，使用 `(?<!...)` 和 `(?!...)` 代替
4. **YAML 文件写入**: 使用 `write_bytes()` 配合 UTF-8 编码更可靠
5. **测试变量名**: 确保测试中使用的变量名与 fixture 名称一致
6. **评分公式**: 需要考虑边界情况和合理权重分配
7. **类型注解**: 返回类型要与实际返回值一致

---

## 全模块排查结果

### 已检查的模块
- ✅ `services/quality/cleaning/*` - 数据清洗模块
- ✅ `services/quality/dimensions/*` - 维度评估模块
- ✅ `services/quality/unstructured/*` - 非结构化评估模块
- ✅ `services/quality/rules/*` - 规则引擎模块
- ✅ `services/quality/models.py` - 数据模型
- ✅ `services/document/*` - 文档服务（无相关代码模式）

### 排查的问题类型
| 问题类型 | 覆盖范围 | 发现数量 | 状态 |
|---------|---------|---------|------|
| 变量未定义 | 所有模块 | 1 | ✅ 已修复 |
| datetime 时区 | 所有模块 | 1 | ✅ 已修复 |
| 正则 `\b` 边界 | 所有模块 | 2 | ✅ 已修复 |
| frozen dataclass | 所有模块 | 1 | ✅ 已修复 |
| numpy bool 类型 | 测试代码 | 多处 | ✅ 已修复 |
| YAML 编码 | 测试代码 | 1 | ✅ 已修复 |
| 评分公式 | 评估模块 | 3 | ✅ 已修复 |
| 排序逻辑 | 评估模块 | 1 | ✅ 已修复 |

### 未发现的问题
- ❌ 无其他 datetime 时区混用问题
- ❌ 无其他 frozen dataclass 修改问题
- ❌ 无其他 YAML 写入编码问题
- ❌ 无其他变量未定义问题
