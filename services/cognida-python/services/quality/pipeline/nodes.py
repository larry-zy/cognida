"""Pipeline 自定义节点。

允许在 Pipeline 中插入自定义处理逻辑。
"""

from abc import ABC, abstractmethod

from .hooks import PipelineContext


class PipelineNode(ABC):
    """Pipeline 节点抽象基类。

    定义可插入 Pipeline 的自定义处理节点。
    """

    node_name: str = ""
    description: str = ""

    @abstractmethod
    def execute(self, context: PipelineContext) -> PipelineContext:
        """执行节点逻辑。

        Args:
            context: Pipeline 上下文

        Returns:
            更新后的上下文
        """

    def __repr__(self) -> str:
        """字符串表示。"""
        return f"{self.__class__.__name__}(name={self.node_name})"


class DataEnrichmentNode(PipelineNode):
    """数据增强节点。

    从外部数据源增强数据。
    """

    node_name = "data_enrichment"
    description = "从外部数据源增强数据"

    def __init__(
        self,
        enrichment_func: callable,
        enrichment_key: str = "enriched_data",
    ) -> None:
        """初始化数据增强节点。

        Args:
            enrichment_func: 增强函数，接收 data 返回增强后的数据
            enrichment_key: 存储增强结果的键名
        """
        self.enrichment_func = enrichment_func
        self.enrichment_key = enrichment_key

    def execute(self, context: PipelineContext) -> PipelineContext:
        """执行数据增强。"""
        try:
            enriched = self.enrichment_func(context.data)
            context.metadata[self.enrichment_key] = enriched
        except Exception as e:
            context.metadata[f"{self.enrichment_key}_error"] = str(e)
        return context


class DataValidationNode(PipelineNode):
    """数据验证节点。

    在处理前验证数据格式和内容。
    """

    node_name = "data_validation"
    description = "验证数据格式和内容"

    def __init__(
        self,
        validation_func: callable,
        fail_on_error: bool = False,
    ) -> None:
        """初始化数据验证节点。

        Args:
            validation_func: 验证函数，接收 data 返回 (is_valid, error_message)
            fail_on_error: 验证失败时是否抛出异常
        """
        self.validation_func = validation_func
        self.fail_on_error = fail_on_error

    def execute(self, context: PipelineContext) -> PipelineContext:
        """执行数据验证。"""
        is_valid, error_message = self.validation_func(context.data)
        context.metadata["validation_result"] = {
            "is_valid": is_valid,
            "error_message": error_message,
        }

        if not is_valid and self.fail_on_error:
            raise ValueError(f"Data validation failed: {error_message}")

        return context


class DataTransformNode(PipelineNode):
    """数据转换节点。

    对数据进行转换处理。
    """

    node_name = "data_transform"
    description = "转换数据格式或内容"

    def __init__(
        self,
        transform_func: callable,
    ) -> None:
        """初始化数据转换节点。

        Args:
            transform_func: 转换函数，接收 data 返回转换后的数据
        """
        self.transform_func = transform_func

    def execute(self, context: PipelineContext) -> PipelineContext:
        """执行数据转换。"""
        try:
            transformed = self.transform_func(context.data)
            context.data = transformed
            context.metadata["transformed"] = True
        except Exception as e:
            context.metadata["transform_error"] = str(e)
        return context


class ConditionalNode(PipelineNode):
    """条件节点。

    根据条件决定是否执行子节点。
    """

    node_name = "conditional"
    description = "根据条件执行子节点"

    def __init__(
        self,
        condition_func: callable,
        child_node: PipelineNode,
    ) -> None:
        """初始化条件节点。

        Args:
            condition_func: 条件函数，接收 context 返回 bool
            child_node: 条件满足时执行的子节点
        """
        self.condition_func = condition_func
        self.child_node = child_node

    def execute(self, context: PipelineContext) -> PipelineContext:
        """执行条件判断。"""
        if self.condition_func(context):
            return self.child_node.execute(context)
        return context


class RetryNode(PipelineNode):
    """重试节点。

        失败时自动重试。
    """

    node_name = "retry"
    description = "失败时自动重试"

    def __init__(
        self,
        child_node: PipelineNode,
        max_retries: int = 3,
        backoff_factor: float = 1.0,
    ) -> None:
        """初始化重试节点。

        Args:
            child_node: 要执行的子节点
            max_retries: 最大重试次数
            backoff_factor: 退避因子
        """
        self.child_node = child_node
        self.max_retries = max_retries
        self.backoff_factor = backoff_factor

    def execute(self, context: PipelineContext) -> PipelineContext:
        """执行带重试的节点。"""
        import time

        last_error: Exception | None = None

        for attempt in range(self.max_retries + 1):
            try:
                return self.child_node.execute(context)
            except Exception as e:
                last_error = e
                if attempt < self.max_retries:
                    sleep_time = self.backoff_factor * (2**attempt)
                    time.sleep(sleep_time)

        raise RuntimeError(f"Retry failed after {self.max_retries} attempts") from last_error
