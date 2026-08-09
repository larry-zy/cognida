"""配置管理模块。"""

from functools import lru_cache
from typing import Literal

from pydantic import Field, field_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """应用配置。"""

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        case_sensitive=False,
        extra="ignore",  # 忽略额外的环境变量
    )

    # 应用配置
    app_name: str = Field(default="cognida-python", description="应用名称")
    app_env: Literal["dev", "test", "prod"] = Field(default="dev", description="运行环境")
    log_level: Literal["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"] = Field(
        default="INFO", description="日志级别"
    )

    # API 配置（HTTP REST API，用于调试和管理）
    api_host: str = Field(default="0.0.0.0", description="API 监听地址")
    api_port: int = Field(default=8000, description="API 监听端口")

    # MCP 配置
    # 默认与部署真源 dev.sh(py-mcp 以 http 模式监听 :3100)对齐：Go 侧 data_analysis 工具
    # (correlation/insight)固定 POST http://localhost:3100/mcp，若这里默认回退到 stdio/:3000，
    # 缺省启动的 MCP 服务既不开 HTTP 也不在 3100，工具调用 → 连不上/404，Agent 退回手写 SQL。
    mcp_mode: Literal["stdio", "http"] = Field(
        default="http", description="MCP 运行模式"
    )
    mcp_host: str = Field(default="0.0.0.0", description="MCP HTTP 监听地址")
    mcp_port: int = Field(default=3100, description="MCP HTTP 端口")

    # gRPC 配置
    grpc_enabled: bool = Field(default=True, description="是否启用 gRPC 服务")
    grpc_port: int = Field(default=50051, description="gRPC 监听端口")
    grpc_max_workers: int = Field(default=10, description="gRPC 最大工作线程数")

    # Go 服务配置
    go_service_url: str = Field(
        default="http://localhost:8080", description="Go 服务地址"
    )

    # LLM 配置
    llm_provider: str = Field(default="openai", description="LLM 提供商")
    llm_api_key: str = Field(default="", description="LLM API 密钥")
    llm_base_url: str = Field(
        default="https://api.openai.com/v1", description="LLM API 地址"
    )
    llm_model: str = Field(default="gpt-4", description="LLM 模型名称")

    # 存储配置（可选，Python 服务直接连接）
    milvus_host: str = Field(default="localhost", description="Milvus 地址")
    milvus_port: int = Field(default=19530, description="Milvus 端口")
    neo4j_uri: str = Field(default="bolt://localhost:7687", description="Neo4j URI")

    # 文件上传配置（服务从 services/cognida-python/ 启动，../../var/uploads 指向仓库根
    # var/uploads，与 Go 侧 UPLOAD_DIR 共享同一目录）
    upload_base_dir: str = Field(default="../../var/uploads", description="共享上传目录")
    allowed_paths: list[str] = Field(
        default=["../../var/uploads"], description="允许访问的文件路径列表"
    )

    # 数据库配置（可选）
    database_url: str | None = Field(default=None, description="数据库连接字符串")

    # Redis 配置（可选）
    redis_url: str | None = Field(default=None, description="Redis 连接字符串")

    @field_validator("app_env")
    @classmethod
    def validate_app_env(cls, v: str) -> str:
        """验证运行环境。"""
        valid_envs = {"dev", "test", "prod"}
        if v not in valid_envs:
            raise ValueError(f"app_env 必须是 {valid_envs} 之一")
        return v

    @property
    def is_dev(self) -> bool:
        """是否为开发环境。"""
        return self.app_env == "dev"

    @property
    def is_prod(self) -> bool:
        """是否为生产环境。"""
        return self.app_env == "prod"


@lru_cache
def get_settings() -> Settings:
    """获取配置单例。"""
    return Settings()
