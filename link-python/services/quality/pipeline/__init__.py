"""端到端质量流程编排。

提供质量评估和清洗的端到端流程编排能力。
"""

from .executor import QualityPipeline
from .hooks import PipelineHook, HookManager
from .nodes import PipelineNode

__all__ = ["QualityPipeline", "PipelineHook", "HookManager", "PipelineNode"]
