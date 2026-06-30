"""评测服务 Pydantic 模型。"""

from typing import Any, Literal, Optional
from pydantic import BaseModel, Field, field_validator


# ============================================================
# 通用基础模型
# ============================================================

class EvaluateMode(BaseModel):
    """评测模式。"""
    mode: Literal["llm", "agent", "rag"] = Field(
        ..., description="评测模式: llm/agent/rag"
    )


class LLMJudgeConfig(BaseModel):
    """LLM-as-Judge 配置。"""
    dimensions: list[str] = Field(
        default_factory=lambda: ["accuracy", "completeness", "clarity", "relevance"],
        description="评分维度"
    )
    questions: list[str] = Field(
        default_factory=list,
        description="原始问题列表，用于 Judge 评分"
    )


# ============================================================
# LLM 评测模型
# ============================================================

class LLMEvaluateRequest(BaseModel):
    """LLM 评测请求。"""
    references: list[str] = Field(..., description="参考答案列表", min_length=1)
    outputs: list[str] = Field(..., description="模型输出列表", min_length=1)
    metrics: list[str] = Field(
        default_factory=lambda: ["rouge", "bleu"],
        description="评测指标列表"
    )
    llm_judge_config: Optional[LLMJudgeConfig] = Field(
        default=None,
        description="LLM-as-Judge 配置"
    )

    @field_validator("outputs")
    @classmethod
    def check_length(cls, v, info):
        if "references" in info.data and len(v) != len(info.data["references"]):
            raise ValueError("outputs 和 references 长度必须相同")
        return v


class LLMEvaluateResponse(BaseModel):
    """LLM 评测响应。"""
    rouge: Optional[dict[str, float]] = Field(None, description="ROUGE 指标")
    bleu: Optional[dict[str, float]] = Field(None, description="BLEU 指标")
    semantic: Optional[dict[str, float]] = Field(None, description="语义相似度")
    llm_judge: Optional[dict[str, float]] = Field(None, description="LLM-as-Judge 评分")
    error: Optional[str] = Field(None, description="错误信息")


# ============================================================
# Agent 评测模型
# ============================================================

class TrajectoryStep(BaseModel):
    """Agent 轨迹步骤。"""
    step: int = Field(..., description="步骤序号")
    action: Literal["thought", "tool_call", "observation", "finish"] = Field(
        ..., description="动作类型"
    )
    tool: Optional[str] = Field(None, description="工具名称")
    input: Optional[str] = Field(None, description="工具输入")
    content: Optional[str] = Field(None, description="内容/结果")


class AgentReference(BaseModel):
    """Agent 评测参考。"""
    final_answer: str = Field(..., description="期望的最终答案")
    tools_used: list[str] = Field(default_factory=list, description="期望使用的工具")
    expected_steps: list[str] = Field(default_factory=list, description="期望的推理步骤")


class AgentOutput(BaseModel):
    """Agent 输出。"""
    final_answer: str = Field(..., description="Agent 给出的最终答案")
    trajectory: list[TrajectoryStep] = Field(..., description="Agent 轨迹")
    tools_used: list[str] = Field(default_factory=list, description="实际使用的工具")
    total_steps: Optional[int] = Field(None, description="总步骤数")
    execution_time: Optional[float] = Field(None, description="执行时间（秒）")


class AgentEvaluateRequest(BaseModel):
    """Agent 评测请求。"""
    references: list[AgentReference] = Field(..., description="期望参考列表", min_length=1)
    outputs: list[AgentOutput] = Field(..., description="Agent 输出列表", min_length=1)
    metrics: list[str] = Field(
        default_factory=lambda: ["answer_accuracy", "tool_selection"],
        description="评测指标列表"
    )
    llm_judge_config: Optional[LLMJudgeConfig] = Field(
        default=None,
        description="LLM-as-Judge 配置"
    )

    @field_validator("outputs")
    @classmethod
    def check_length(cls, v, info):
        if "references" in info.data and len(v) != len(info.data["references"]):
            raise ValueError("outputs 和 references 长度必须相同")
        return v


class ToolMetric(BaseModel):
    """工具选择指标。"""
    precision: float = Field(..., description="精确率")
    recall: float = Field(..., description="召回率")
    f1: float = Field(..., description="F1 分数")


class TrajectoryMetric(BaseModel):
    """轨迹匹配指标。"""
    exact_match: float = Field(..., description="精确匹配率")
    similarity: float = Field(..., description="语义相似度")


class StepMetric(BaseModel):
    """步骤效率指标。"""
    optimal_ratio: float = Field(..., description="最优步骤比率")
    avg_steps: float = Field(..., description="平均步骤数")
    optimal_steps: float = Field(..., description="最优步骤数")


class AgentEvaluateResponse(BaseModel):
    """Agent 评测响应。"""
    answer_accuracy: Optional[float] = Field(None, description="答案准确率")
    tool_selection: Optional[ToolMetric] = Field(None, description="工具选择指标")
    trajectory_match: Optional[TrajectoryMetric] = Field(None, description="轨迹匹配指标")
    step_efficiency: Optional[StepMetric] = Field(None, description="步骤效率指标")
    llm_judge: Optional[dict[str, float]] = Field(None, description="LLM-as-Judge 评分")
    error: Optional[str] = Field(None, description="错误信息")


# ============================================================
# RAG 评测模型
# ============================================================

class Document(BaseModel):
    """文档。"""
    doc_id: str = Field(..., description="文档 ID")
    content: str = Field(..., description="文档内容")
    metadata: Optional[dict[str, str]] = Field(default=None, description="元数据")


class RAGReference(BaseModel):
    """RAG 评测参考。"""
    answer: str = Field(..., description="参考答案")
    relevant_docs: list[str] = Field(..., description="相关文档 ID 列表")


class RAGOutput(BaseModel):
    """RAG 输出。"""
    answer: str = Field(..., description="生成的答案")
    retrieved_docs: list[str] = Field(..., description="检索到的文档 ID 列表")


class RAGEvaluateRequest(BaseModel):
    """RAG 评测请求。"""
    questions: list[str] = Field(..., description="问题列表", min_length=1)
    references: list[RAGReference] = Field(..., description="参考列表", min_length=1)
    outputs: list[RAGOutput] = Field(..., description="输出列表", min_length=1)
    corpus: list[Document] = Field(default_factory=list, description="文档语料")
    metrics: list[str] = Field(
        default_factory=lambda: ["retrieval_precision", "faithfulness"],
        description="评测指标列表"
    )
    llm_judge_config: Optional[LLMJudgeConfig] = Field(
        default=None,
        description="LLM-as-Judge 配置"
    )

    @field_validator("outputs")
    @classmethod
    def check_length(cls, v, info):
        if "references" in info.data and len(v) != len(info.data["references"]):
            raise ValueError("outputs 和 references 长度必须相同")
        if "questions" in info.data and len(v) != len(info.data["questions"]):
            raise ValueError("outputs 和 questions 长度必须相同")
        return v


class RAGMetric(BaseModel):
    """RAG 特有指标。"""
    faithfulness: float = Field(..., description="忠实度")
    context_relevance: float = Field(..., description="上下文相关性")
    noise_ratio: float = Field(..., description="噪声比例")


class RAGEvaluateResponse(BaseModel):
    """RAG 评测响应。"""
    answer_quality: Optional[dict[str, float]] = Field(None, description="答案质量指标")
    retrieval: Optional[dict[str, float]] = Field(None, description="检索指标")
    rag_specific: Optional[RAGMetric] = Field(None, description="RAG 特有指标")
    llm_judge: Optional[dict[str, float]] = Field(None, description="LLM-as-Judge 评分")
    error: Optional[str] = Field(None, description="错误信息")


# ============================================================
# 通用评测请求/响应
# ============================================================

class EvaluateRequest(BaseModel):
    """通用评测请求。"""
    mode: Literal["llm", "agent", "rag"] = Field(..., description="评测模式")
    data: dict[str, Any] = Field(..., description="评测数据（根据模式不同）")
    metrics: list[str] = Field(..., description="评测指标列表", min_length=1)
    llm_judge_config: Optional[LLMJudgeConfig] = Field(
        default=None,
        description="LLM-as-Judge 配置"
    )


class EvaluateResponse(BaseModel):
    """通用评测响应。"""
    mode: str = Field(..., description="评测模式")
    result: dict[str, Any] = Field(..., description="评测结果")
    error: Optional[str] = Field(None, description="错误信息")


# ============================================================
# 健康检查
# ============================================================

class HealthResponse(BaseModel):
    """健康检查响应。"""
    status: str = Field(..., description="服务状态")
    version: str = Field(default="1.0.0", description="版本号")
