# Python 语言规范检查

## 1. 命名规范

### 模块名
- ✅ 小写、短、无下划线：`user.py`, `agent.py`
- ❌ 驼峰：`UserService.py`
- ❌ 下划线：`user_service.py`

### 类名
- ✅ 驼峰命名：`UserService`, `DocumentParser`
- ❌ 下划线：`user_service`

### 函数和变量
- ✅ 小写+下划线：`get_user()`, `user_cache`
- ❌ 驼峰：`getUser()`, `userCache`

### 常量
- ✅ 大写+下划线：`MAX_RETRIES`, `DEFAULT_TIMEOUT`
- ❌ 小写：`max_retries`

### 私有成员
- ✅ 单下划线前缀：`_internal_func`, `_private_var`
- ❌ 双下划线（除非必要）：`__private`

## 2. 类型注解

### 函数必须有类型注解
```python
# ✅ 正确
def get_user(user_id: str) -> Optional[User]:
    return db.query(user_id)

# ❌ 错误 - 缺少类型注解
def get_user(user_id):
    return db.query(user_id)
```

### 复杂类型使用 typing
```python
# ✅ 正确
from typing import List, Dict, Optional

def process_documents(docs: List[Document]) -> Dict[str, Any]:
    return {"results": [...]}

# ❌ 错误
def process_documents(docs):
    return {"results": [...]}
```

## 3. 异常处理

### 不使用裸 except
```python
# ❌ 错误
try:
    risky_operation()
except:
    pass

# ✅ 正确
try:
    risky_operation()
except ValueError as e:
    logger.error(f"Invalid value: {e}")
except Exception as e:
    logger.error(f"Unexpected error: {e}")
    raise
```

### 自定义异常
```python
# ✅ 正确
class UserNotFoundError(Exception):
    """用户未找到异常"""
    pass

class ValidationError(Exception):
    """数据验证异常"""
    pass
```

## 4. 资源管理

### 使用 Context Manager
```python
# ✅ 正确
with open("file.txt", "r") as f:
    data = f.read()

# ❌ 错误 - 可能资源泄漏
f = open("file.txt", "r")
data = f.read()
# 忘记关闭
```

### 数据库连接
```python
# ✅ 正确
def get_user(user_id: str) -> User:
    with get_db_session() as session:
        return session.query(User).filter_by(id=user_id).one()

# ❌ 错误
def get_user(user_id: str) -> User:
    session = get_db_session()
    return session.query(User).filter_by(id=user_id).one()
    # session 未关闭
```

## 5. 分层架构

### Python 服务架构
```
services/cognida-python/
├── grpc/           # gRPC 服务层
├── services/       # 业务逻辑层
├── core/           # 核心模块
└── config/         # 配置管理
```

### 依赖方向
- grpc → services → core
- services → core
- core 无依赖

## 6. 日志规范

### 使用 logger 而非 print
```python
# ✅ 正确
import logging
logger = logging.getLogger(__name__)

logger.info("Processing document")
logger.error(f"Failed to process: {error}")

# ❌ 错误
print("Processing document")
```

### 日志级别
- DEBUG: 详细调试信息
- INFO: 一般信息
- WARNING: 警告但不影响运行
- ERROR: 错误但可继续
- CRITICAL: 严重错误需立即处理

## 7. 文档字符串

### 函数必须有 docstring
```python
# ✅ 正确
def parse_document(file_path: str) -> Document:
    """解析文档文件并返回 Document 对象。

    Args:
        file_path: 文档文件路径

    Returns:
        Document: 解析后的文档对象

    Raises:
        FileNotFoundError: 文件不存在
        ParseError: 文档格式错误
    """
    pass

# ❌ 错误 - 缺少 docstring
def parse_document(file_path: str) -> Document:
    pass
```

## 8. 测试规范

### 使用 pytest
```python
# ✅ 正确
import pytest

def test_user_creation():
    user = User(name="test")
    assert user.name == "test"

@pytest.mark.parametrize("input,expected", [
    ("valid@email.com", True),
    ("invalid", False),
])
def test_email_validation(input, expected):
    assert validate_email(input) == expected
```

### Fixtures
```python
# ✅ 正确
@pytest.fixture
def test_db():
    db = create_test_db()
    yield db
    cleanup_test_db(db)

def test_user_repository(test_db):
    repo = UserRepository(test_db)
    # 测试代码
```

## 9. 配置管理

### 使用配置文件
```python
# ✅ 正确 - config/settings.py
from pydantic import BaseSettings

class Settings(BaseSettings):
    database_url: str
    api_key: str
    debug: bool = False

    class Config:
        env_file = ".env"

settings = Settings()

# ❌ 错误 - 硬编码
DATABASE_URL = "mysql://localhost:3306/cognida"
API_KEY = "sk-xxxxx"
```

## 10. 性能注意

### 避免全局变量
```python
# ❌ 错误
cache = {}  # 全局状态

# ✅ 正确 - 使用类或依赖注入
class CacheService:
    def __init__(self):
        self._cache = {}
```

### 避免过早优化
- ✅ 先写清晰的代码
- ✅ 性能测试后再优化

## 11. 安全注意

### SQL 注入
```python
# ❌ 危险
query = f"SELECT * FROM users WHERE name = '{user_name}'"

# ✅ 正确
query = "SELECT * FROM users WHERE name = %s"
cursor.execute(query, (user_name,))
```

### 敏感信息
```python
# ❌ 错误 - 硬编码密钥
API_KEY = "sk-xxxxx"

# ✅ 正确 - 从环境变量读取
API_KEY = os.getenv("API_KEY")
```

## 12. 常见错误模式

### 忘记 return
```python
# ❌ 错误 - 忘记 return
def get_user(user_id: str) -> Optional[User]:
    if user_id == "admin":
        return admin_user
    # 忘记返回 None

# ✅ 正确
def get_user(user_id: str) -> Optional[User]:
    if user_id == "admin":
        return admin_user
    return None
```

### 可变默认参数
```python
# ❌ 错误 - 可变默认参数
def process_items(items=[]):
    items.append("processed")
    return items

# ✅ 正确
def process_items(items=None):
    if items is None:
        items = []
    items.append("processed")
    return items
```

## 13. 导入顺序

```python
# 1. 标准库
import os
import sys
from typing import Optional

# 2. 第三方库
import grpc
from pydantic import BaseSettings

# 3. 项目内部
from link.python.core import exceptions
from link.python.services import document
```

## 14. 编译前检查

### 必须通过
```bash
# 类型检查
mypy services/cognida-python/

# Lint
flake8 services/cognida-python/
black --check services/cognida-python/

# 测试
pytest services/cognida-python/tests/
```
