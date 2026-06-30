"""规则引擎。

加载和验证质量规则配置。
"""

import os
from pathlib import Path
from typing import Any

import yaml

from ..models import DimensionRule, FieldRule, FieldType, QualityRules


class RuleEngine:
    """规则引擎。

    加载、验证和合并质量规则配置。
    """

    def __init__(self, config_dir: str | None = None) -> None:
        """初始化规则引擎。

        Args:
            config_dir: 规则配置文件目录
        """
        self.config_dir = config_dir or self._default_config_dir()
        self._cached_rules: dict[str, QualityRules] = {}

    def _default_config_dir(self) -> str:
        """获取默认配置目录。"""
        return str(Path(__file__).parent)

    def load_rules(self, rule_file: str | None = None) -> QualityRules:
        """加载规则配置。

        Args:
            rule_file: 规则文件路径，如果为None则加载默认规则

        Returns:
            规则配置对象
        """
        if rule_file and rule_file in self._cached_rules:
            return self._cached_rules[rule_file]

        # 加载默认规则
        default_rules = self._load_yaml_file("default_rules.yaml")

        # 加载特定规则并合并
        if rule_file:
            specific_rules = self._load_yaml_file(rule_file)
            merged_rules = self._merge_rules(default_rules, specific_rules)
        else:
            merged_rules = default_rules

        # 验证规则
        self._validate_rules(merged_rules)

        # 缓存规则
        cache_key = rule_file or "default"
        self._cached_rules[cache_key] = merged_rules

        return merged_rules

    def _load_yaml_file(self, filename: str) -> QualityRules:
        """加载YAML规则文件。

        Args:
            filename: 文件名

        Returns:
            规则配置对象
        """
        file_path = Path(self.config_dir) / filename

        if not file_path.exists():
            return QualityRules()

        with open(file_path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f)

        return self._parse_rules(data)

    def _parse_rules(self, data: dict[str, Any]) -> QualityRules:
        """解析规则数据。

        Args:
            data: YAML数据

        Returns:
            规则配置对象
        """
        field_rules = []
        for field_data in data.get("field_rules", []):
            field_rules.append(self._parse_field_rule(field_data))

        dimension_rules = []
        for dim_data in data.get("dimension_rules", []):
            dimension_rules.append(self._parse_dimension_rule(dim_data))

        return QualityRules(
            version=data.get("version", "1.0"),
            field_rules=field_rules,
            dimension_rules=dimension_rules,
            cleaning_rules=data.get("cleaning_rules", {}),
            metadata=data.get("metadata", {}),
        )

    def _parse_field_rule(self, data: dict[str, Any]) -> FieldRule:
        """解析字段规则。

        Args:
            data: 字段规则数据

        Returns:
            字段规则对象
        """
        return FieldRule(
            name=data["name"],
            type=FieldType(data.get("type", "string")),
            required=data.get("required", False),
            pattern=data.get("pattern"),
            min_value=data.get("min_value"),
            max_value=data.get("max_value"),
            min_length=data.get("min_length"),
            max_length=data.get("max_length"),
            allowed_values=data.get("allowed_values"),
            description=data.get("description", ""),
        )

    def _parse_dimension_rule(self, data: dict[str, Any]) -> DimensionRule:
        """解析维度规则。

        Args:
            data: 维度规则数据

        Returns:
            维度规则对象
        """
        return DimensionRule(
            name=data["name"],
            enabled=data.get("enabled", True),
            threshold=data.get("threshold", 70.0),
            weight=data.get("weight", 1.0),
            config=data.get("config", {}),
        )

    def _merge_rules(
        self, default: QualityRules, specific: QualityRules
    ) -> QualityRules:
        """合并规则配置。

        Args:
            default: 默认规则
            specific: 特定规则

        Returns:
            合并后的规则
        """
        # 合并字段规则（特定规则覆盖默认规则）
        field_rules_map = {r.name: r for r in default.field_rules}
        for rule in specific.field_rules:
            field_rules_map[rule.name] = rule

        # 合并维度规则
        dimension_rules_map = {r.name: r for r in default.dimension_rules}
        for rule in specific.dimension_rules:
            dimension_rules_map[rule.name] = rule

        # 合并清洗规则
        cleaning_rules = {**default.cleaning_rules, **specific.cleaning_rules}

        # 合并元数据
        metadata = {**default.metadata, **specific.metadata}

        return QualityRules(
            version=specific.version or default.version,
            field_rules=list(field_rules_map.values()),
            dimension_rules=list(dimension_rules_map.values()),
            cleaning_rules=cleaning_rules,
            metadata=metadata,
        )

    def _validate_rules(self, rules: QualityRules) -> None:
        """验证规则配置。

        Args:
            rules: 规则配置

        Raises:
            ValueError: 规则无效时
        """
        # 验证字段规则
        for rule in rules.field_rules:
            if not rule.name:
                raise ValueError("Field rule must have a name")

            if rule.min_value is not None and rule.max_value is not None:
                if rule.min_value > rule.max_value:
                    raise ValueError(
                        f"Field '{rule.name}': min_value cannot be greater than max_value"
                    )

            if rule.min_length is not None and rule.max_length is not None:
                if rule.min_length > rule.max_length:
                    raise ValueError(
                        f"Field '{rule.name}': min_length cannot be greater than max_length"
                    )

        # 验证维度规则
        for rule in rules.dimension_rules:
            if not rule.name:
                raise ValueError("Dimension rule must have a name")

            if not 0 <= rule.threshold <= 100:
                raise ValueError(
                    f"Dimension '{rule.name}': threshold must be between 0 and 100"
                )

            if rule.weight < 0:
                raise ValueError(f"Dimension '{rule.name}': weight cannot be negative")

    def list_available_rule_files(self) -> list[str]:
        """列出可用的规则文件。

        Returns:
            规则文件名列表
        """
        rule_files = []
        for file in Path(self.config_dir).glob("*.yaml"):
            if file.name != "default_rules.yaml":
                rule_files.append(file.name)
        return sorted(rule_files)

    def reload_rules(self, rule_file: str | None = None) -> QualityRules:
        """重新加载规则。

        Args:
            rule_file: 规则文件名

        Returns:
            规则配置对象
        """
        cache_key = rule_file or "default"
        if cache_key in self._cached_rules:
            del self._cached_rules[cache_key]
        return self.load_rules(rule_file)


__all__ = ["RuleEngine"]
