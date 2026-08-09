# 自定义数据清洗器

## 创建自定义清洗器

```python
from ..base import Cleaner
from ...registry import register_cleaner
from ...models import CleaningResult, CleaningOperation

@register_cleaner("my_custom_cleaner")
class MyCustomCleaner(Cleaner):
    """自定义清洗器。"""

    cleaner_name = "my_custom_cleaner"
    description = "我的自定义清洗器"

    def clean(self, data, config=None):
        """实现清洗逻辑。"""
        operations = []
        # ...
        return CleaningResult(
            original_count=len(data),
            cleaned_count=len(cleaned_data),
            removed_count=removed_count,
            operations=operations,
        )
```
