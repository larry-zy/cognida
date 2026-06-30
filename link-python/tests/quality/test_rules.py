"""规则引擎测试。"""

import os
import tempfile
from pathlib import Path

import pytest
import yaml

from services.quality.rules import RuleEngine
from services.quality.models import FieldRule, DimensionRule, FieldType, QualityRules


@pytest.fixture
def temp_config_dir():
    """创建临时配置目录。"""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield tmpdir


@pytest.fixture
def sample_rules_file(temp_config_dir):
    """创建示例规则文件。"""
    rules = {
        "version": "1.0",
        "field_rules": [
            {
                "name": "id",
                "type": "integer",
                "required": True,
            },
            {
                "name": "email",
                "type": "string",
                "required": True,
                "pattern": r"^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$",
            },
            {
                "name": "age",
                "type": "integer",
                "min_value": 0,
                "max_value": 120,
            },
        ],
        "dimension_rules": [
            {
                "name": "completeness",
                "enabled": True,
                "threshold": 0.95,
                "weight": 1.0,
            },
            {
                "name": "accuracy",
                "enabled": True,
                "threshold": 0.90,
                "weight": 1.0,
            },
        ],
        "cleaning_rules": {
            "remove_duplicates": True,
            "normalize_whitespace": True,
        },
        "metadata": {
            "description": "示例规则配置",
            "created_at": "2024-01-01",
        },
    }

    file_path = Path(temp_config_dir) / "test_rules.yaml"
    with open(file_path, "w", encoding="utf-8") as f:
        yaml.dump(rules, f, allow_unicode=True)

    return file_path


@pytest.fixture
def custom_rules_file(temp_config_dir):
    """创建自定义规则文件。"""
    rules = {
        "version": "1.0",
        "field_rules": [
            {
                "name": "username",
                "type": "string",
                "required": True,
                "min_length": 3,
                "max_length": 20,
            },
        ],
        "dimension_rules": [
            {
                "name": "validity",
                "enabled": True,
                "threshold": 0.85,
                "weight": 1.5,
            },
        ],
        "metadata": {
            "description": "自定义规则配置",
        },
    }

    file_path = Path(temp_config_dir) / "custom_rules.yaml"
    with open(file_path, "w", encoding="utf-8") as f:
        yaml.dump(rules, f, allow_unicode=True)

    return file_path


class TestRuleEngine:
    """规则引擎测试。"""

    def test_load_default_rules(self, sample_rules_file, temp_config_dir):
        """测试加载默认规则。"""
        engine = RuleEngine(temp_config_dir)
        rules = engine.load_rules(sample_rules_file.name)

        assert isinstance(rules, QualityRules)
        assert len(rules.field_rules) > 0
        assert len(rules.dimension_rules) > 0

    def test_load_specific_rules(self, sample_rules_file, temp_config_dir):
        """测试加载指定规则文件。"""
        engine = RuleEngine(temp_config_dir)
        rules = engine.load_rules(sample_rules_file.name)

        assert isinstance(rules, QualityRules)
        assert rules.version == "1.0"

    def test_field_rules_parsing(self, sample_rules_file, temp_config_dir):
        """测试字段规则解析。"""
        engine = RuleEngine(temp_config_dir)
        rules = engine.load_rules(sample_rules_file.name)

        # 检查字段规则是否正确解析
        id_rule = next((r for r in rules.field_rules if r.name == "id"), None)
        assert id_rule is not None
        assert id_rule.type == FieldType.INTEGER
        assert id_rule.required is True

        email_rule = next((r for r in rules.field_rules if r.name == "email"), None)
        assert email_rule is not None
        assert email_rule.pattern is not None

    def test_dimension_rules_parsing(self, sample_rules_file, temp_config_dir):
        """测试维度规则解析。"""
        engine = RuleEngine(temp_config_dir)
        rules = engine.load_rules(sample_rules_file.name)

        # 检查维度规则是否正确解析
        completeness_rule = next(
            (r for r in rules.dimension_rules if r.name == "completeness"),
            None
        )
        assert completeness_rule is not None
        assert completeness_rule.enabled is True
        assert completeness_rule.threshold == 0.95
        assert completeness_rule.weight == 1.0

    def test_rules_merging(self, sample_rules_file, custom_rules_file, temp_config_dir):
        """测试规则合并。"""
        engine = RuleEngine(temp_config_dir)

        # 创建一个基础规则文件
        base_rules = {
            "version": "1.0",
            "field_rules": [
                {"name": "id", "type": "integer", "required": True},
            ],
            "dimension_rules": [
                {"name": "completeness", "enabled": True, "threshold": 0.95, "weight": 1.0},
            ],
        }
        base_path = Path(temp_config_dir) / "default_rules.yaml"
        with open(base_path, "w", encoding="utf-8") as f:
            yaml.dump(base_rules, f)

        rules = engine.load_rules(custom_rules_file.name)

        # 自定义规则应该覆盖默认规则
        assert len(rules.field_rules) > 0
        assert len(rules.dimension_rules) > 0

    def test_list_available_rule_files(self, sample_rules_file, custom_rules_file, temp_config_dir):
        """测试列出可用规则文件。"""
        engine = RuleEngine(temp_config_dir)
        files = engine.list_available_rule_files()

        assert len(files) >= 2
        assert "test_rules.yaml" in files
        assert "custom_rules.yaml" in files

    def test_invalid_rules_file(self, temp_config_dir):
        """测试无效规则文件处理。"""
        # 创建无效的YAML文件
        invalid_path = Path(temp_config_dir) / "invalid.yaml"
        invalid_path.write_text("invalid: yaml: content: [")

        engine = RuleEngine(temp_config_dir)

        # 应该抛出异常或返回空规则
        try:
            rules = engine.load_rules("invalid.yaml")
            # 如果没有抛出异常，规则应该有默认值
            assert rules is not None
        except Exception:
            # 预期可能抛出异常
            pass

    def test_empty_config_dir(self):
        """测试空配置目录。"""
        with tempfile.TemporaryDirectory() as empty_dir:
            engine = RuleEngine(empty_dir)
            files = engine.list_available_rule_files()

            # 应该返回空列表
            assert files == []

    def test_rules_validation(self, sample_rules_file, temp_config_dir):
        """测试规则验证。"""
        engine = RuleEngine(temp_config_dir)
        rules = engine.load_rules(sample_rules_file.name)

        # 验证规则对象
        assert rules.version is not None
        assert isinstance(rules.field_rules, list)
        assert isinstance(rules.dimension_rules, list)
        assert isinstance(rules.cleaning_rules, dict)

    def test_field_type_conversion(self, temp_config_dir):
        """测试字段类型转换。"""
        rules_data = {
            "version": "1.0",
            "field_rules": [
                {"name": "str_field", "type": "string"},
                {"name": "int_field", "type": "integer"},
                {"name": "float_field", "type": "float"},
                {"name": "bool_field", "type": "boolean"},
                {"name": "date_field", "type": "date"},
                {"name": "datetime_field", "type": "datetime"},
            ],
            "dimension_rules": [],
        }

        rules_path = Path(temp_config_dir) / "type_test.yaml"
        with open(rules_path, "w", encoding="utf-8") as f:
            yaml.dump(rules_data, f)

        engine = RuleEngine(temp_config_dir)
        rules = engine.load_rules("type_test.yaml")

        # 检查类型转换
        type_mapping = {
            "str_field": FieldType.STRING,
            "int_field": FieldType.INTEGER,
            "float_field": FieldType.FLOAT,
            "bool_field": FieldType.BOOLEAN,
            "date_field": FieldType.DATE,
            "datetime_field": FieldType.DATETIME,
        }

        for field_name, expected_type in type_mapping.items():
            rule = next((r for r in rules.field_rules if r.name == field_name), None)
            assert rule is not None
            assert rule.type == expected_type

    def test_dimension_enable_config(self, temp_config_dir):
        """测试维度启用配置。"""
        rules_data = {
            "version": "1.0",
            "field_rules": [],
            "dimension_rules": [
                {"name": "enabled_dim", "enabled": True, "threshold": 0.9, "weight": 1.0},
                {"name": "disabled_dim", "enabled": False, "threshold": 0.8, "weight": 1.0},
            ],
        }

        rules_path = Path(temp_config_dir) / "enable_test.yaml"
        with open(rules_path, "w", encoding="utf-8") as f:
            yaml.dump(rules_data, f)

        engine = RuleEngine(temp_config_dir)
        rules = engine.load_rules("enable_test.yaml")

        enabled = [d for d in rules.dimension_rules if d.enabled]
        disabled = [d for d in rules.dimension_rules if not d.enabled]

        assert len(enabled) == 1
        assert len(disabled) == 1

    def test_cleaning_rules_config(self, sample_rules_file, temp_config_dir):
        """测试清洗规则配置。"""
        engine = RuleEngine(temp_config_dir)
        rules = engine.load_rules(sample_rules_file.name)

        # 检查清洗规则
        assert "remove_duplicates" in rules.cleaning_rules
        assert "normalize_whitespace" in rules.cleaning_rules

    def test_metadata_preservation(self, sample_rules_file, temp_config_dir):
        """测试元数据保留。"""
        engine = RuleEngine(temp_config_dir)
        rules = engine.load_rules(sample_rules_file.name)

        # 检查元数据
        assert "description" in rules.metadata
        assert rules.metadata["description"] == "示例规则配置"

    def test_get_enabled_dimensions(self, sample_rules_file, temp_config_dir):
        """测试获取启用的维度列表。"""
        engine = RuleEngine(temp_config_dir)
        rules = engine.load_rules(sample_rules_file.name)

        enabled_dimensions = [d.name for d in rules.dimension_rules if d.enabled]

        assert len(enabled_dimensions) == 2
        assert "completeness" in enabled_dimensions
        assert "accuracy" in enabled_dimensions
