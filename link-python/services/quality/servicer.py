"""质量服务 gRPC 实现。"""

import time
from typing import Any

import pandas as pd
from grpc import ServicerContext

# 通过 proto 命名空间包导入, 与 grpc_service/servicer.py 及生成代码保持一致
# (裸 import quality_pb2 需要 proto/ 目录本身在 sys.path 上, 生产/测试均不成立)
from proto import quality_pb2
from proto import quality_pb2_grpc
from services.quality.evaluator import DataQualityEvaluator
from services.quality.drift_detector import DriftDetector
from services.quality.models import QualityReport, UnstructuredQualityReport
from services.quality.pipeline.executor import QualityPipeline
from services.quality.registry import EvaluatorRegistry, CleanerRegistry
from services.quality.rules import RuleEngine


def _read_csv_bytes(raw: bytes) -> pd.DataFrame:
    """从字节流解析 CSV，把 pandas 底层报错翻译成用户可读的中文提示。

    用户在页面粘贴的数据经常存在列数不一致、引号未闭合等格式问题，pandas 会抛出
    晦涩的英文 tokenizer 错误。这里统一兜底，给出可操作的说明，便于前端直接展示。
    """
    from io import BytesIO

    try:
        return pd.read_csv(BytesIO(raw))
    except pd.errors.EmptyDataError as exc:
        raise ValueError("CSV 数据为空或缺少表头，无法解析") from exc
    except pd.errors.ParserError as exc:
        raise ValueError(
            f"CSV 格式错误，无法解析（请检查各行列数是否一致、引号是否闭合）：{exc}"
        ) from exc


def _read_json_bytes(raw: bytes) -> pd.DataFrame:
    """从字节流解析 JSON 为二维表，支持「对象数组」与「列->值 的对象」两种常见形态。

    前端 JSON 输入通常是记录数组 [{...}, {...}]；也兼容单对象 {...}（视作一行）与
    列式对象 {"col": [...]} 。统一在这里把 json/pandas 的报错翻译成可读中文提示。
    """
    import json

    text = raw.decode("utf-8", errors="replace").strip()
    if not text:
        raise ValueError("JSON 数据为空，无法解析")
    try:
        parsed = json.loads(text)
    except json.JSONDecodeError as exc:
        raise ValueError(
            f"JSON 格式错误，无法解析（请检查括号、引号、逗号是否配对）：{exc}"
        ) from exc

    try:
        if isinstance(parsed, list):
            df = pd.DataFrame(parsed)
        elif isinstance(parsed, dict):
            # 列式对象 {"col": [..]} → 直接建表；否则按单条记录处理
            if parsed and all(isinstance(v, list) for v in parsed.values()):
                df = pd.DataFrame(parsed)
            else:
                df = pd.DataFrame([parsed])
        else:
            raise ValueError("JSON 顶层需为对象数组或对象，无法解析为二维表")
    except ValueError:
        raise
    except Exception as exc:  # pandas 结构化失败（如数组元素非同构）
        raise ValueError(f"JSON 结构无法转换为二维表：{exc}") from exc

    if df.empty:
        raise ValueError("JSON 数据不含任何记录，无法评估")

    # 二维表的单元格必须是标量：若某字段的值是数组/对象（嵌套结构），
    # 建表阶段不会报错，但后续维度评估（唯一性/一致性等）会对单元格取哈希，
    # 遇到 list/dict 抛出难懂的 "unhashable type: 'list'"。这里提前拦截，
    # 给出可读提示——最常见的诱因是误把「评估结果报告」当「待评估数据」粘入。
    nested_cols = [
        str(col)
        for col in df.columns
        if df[col].map(lambda v: isinstance(v, (list, dict))).any()
    ]
    if nested_cols:
        raise ValueError(
            "JSON 记录包含嵌套的数组/对象字段（"
            + "、".join(nested_cols)
            + "），无法作为二维表评估。请提供扁平的记录——每个字段为文本/数字/布尔，"
            "例如 [{\"id\":1,\"name\":\"Alice\",\"age\":30}]，而不是嵌套结构或结果报告。"
        )
    return df


def _read_tabular_bytes(raw: bytes, fmt: str) -> pd.DataFrame:
    """按显式 format 提示（csv/json）选择解析器，默认 csv。"""
    if (fmt or "csv").strip().lower() == "json":
        return _read_json_bytes(raw)
    return _read_csv_bytes(raw)


def _write_tabular_bytes(df: pd.DataFrame, fmt: str) -> bytes:
    """把 DataFrame 序列化为字节流，与 :func:`_read_tabular_bytes` 对称（csv/json）。

    - csv：与历史行为一致，`to_csv(index=False)`；
    - json：对象数组 [{...}, {...}]，`force_ascii=False` 保留中文，日期按 ISO 输出，
      可被 :func:`_read_json_bytes` 无损回读，形成清洗「JSON 入 → JSON 出」的闭环。
    """
    from io import BytesIO

    if (fmt or "csv").strip().lower() == "json":
        return df.to_json(
            orient="records", force_ascii=False, date_format="iso"
        ).encode("utf-8")
    output = BytesIO()
    df.to_csv(output, index=False)
    return output.getvalue()


class QualityServicer(quality_pb2_grpc.QualityServiceServicer):
    """质量服务 gRPC 实现。"""

    def __init__(self) -> None:
        """初始化服务。"""
        self.evaluator = DataQualityEvaluator()
        self.pipeline = QualityPipeline()
        self.drift_detector = DriftDetector()
        self.rule_engine = RuleEngine()

    def EvaluateQuality(
        self,
        request: quality_pb2.EvaluateQualityRequest,
        context: ServicerContext,
    ) -> quality_pb2.EvaluateQualityResponse:
        """评估结构化数据质量。"""
        try:
            # format 提示经 config 透传（避免改 proto）；先取出，避免污染维度配置
            config = dict(request.config) if request.config else {}
            data_format = config.pop("format", "csv")

            # 加载数据
            if request.file_path:
                data = self.evaluator.load_data(request.file_path)
            elif request.csv_data:
                data = _read_tabular_bytes(request.csv_data, data_format)
            else:
                return quality_pb2.EvaluateQualityResponse(
                    success=False,
                    error_message="No data provided",
                )

            # 执行评估
            dimensions = list(request.dimensions) if request.dimensions else None
            config = config or None

            report = self.evaluator.evaluate_structured(
                data,
                dimensions=dimensions,
                rule_file=request.rule_file or None,
                config=config,
            )

            return quality_pb2.EvaluateQualityResponse(
                report=self._convert_quality_report(report),
                success=True,
            )

        except Exception as e:
            return quality_pb2.EvaluateQualityResponse(
                success=False,
                error_message=str(e),
            )

    def EvaluateUnstructuredQuality(
        self,
        request: quality_pb2.EvaluateUnstructuredQualityRequest,
        context: ServicerContext,
    ) -> quality_pb2.EvaluateUnstructuredQualityResponse:
        """评估非结构化数据质量。"""
        try:
            dimensions = list(request.dimensions) if request.dimensions else None
            config = dict(request.config) if request.config else None

            report = self.evaluator.evaluate_unstructured(
                request.text,
                dimensions=dimensions,
                rule_file=request.rule_file or None,
                config=config,
            )

            return quality_pb2.EvaluateUnstructuredQualityResponse(
                report=self._convert_unstructured_report(report),
                success=True,
            )

        except Exception as e:
            return quality_pb2.EvaluateUnstructuredQualityResponse(
                success=False,
                error_message=str(e),
            )

    def CleanData(
        self,
        request: quality_pb2.CleanDataRequest,
        context: ServicerContext,
    ) -> quality_pb2.CleanDataResponse:
        """清洗数据。"""
        try:
            # format 提示经 config 透传（避免改 proto），与 EvaluateQuality 保持一致：
            #   format        —— 输入解析格式（csv/json），默认 csv；
            #   output_format —— 清洗结果导出格式（csv/json），缺省跟随输入格式，
            #                    因此「JSON 入 → JSON 出」开箱即用，前端也可显式覆盖。
            # 先从 config 取出，避免污染下游清洗器配置。
            config = dict(request.config) if request.config else {}
            input_format = config.pop("format", "csv")
            output_format = config.pop("output_format", "") or input_format

            # 加载数据
            if request.file_path:
                data = self.evaluator.load_data(request.file_path)
            elif request.csv_data:
                data = _read_tabular_bytes(request.csv_data, input_format)
            else:
                return quality_pb2.CleanDataResponse(
                    success=False,
                    error_message="No data provided",
                )

            # 执行清洗
            cleaners = list(request.cleaners) if request.cleaners else None
            if cleaners:
                config["cleaners"] = cleaners

            result = self.pipeline._clean_data(data, config)

            # 按 output_format 序列化清洗后的数据（csv/json）
            cleaned_data = result.cleaned_data if result.cleaned_data is not None else data

            return quality_pb2.CleanDataResponse(
                result=self._convert_cleaning_result(result),
                cleaned_data=_write_tabular_bytes(cleaned_data, output_format),
                success=True,
            )

        except Exception as e:
            return quality_pb2.CleanDataResponse(
                success=False,
                error_message=str(e),
            )

    def ProcessPipeline(
        self,
        request: quality_pb2.ProcessPipelineRequest,
        context: ServicerContext,
    ) -> quality_pb2.ProcessPipelineResponse:
        """执行端到端流程。"""
        try:
            config = dict(request.config) if request.config else {}
            # 输入格式（csv/json，默认 csv）经 config 透传，与 EvaluateQuality/CleanData 一致；
            # 端到端结果不回传导出数据（PipelineResult 仅含清洗计数/操作），故无 output_format。
            input_format = config.pop("format", "csv")

            if request.is_structured:
                # 处理结构化数据
                if request.file_path:
                    data = self.evaluator.load_data(request.file_path)
                elif request.csv_data:
                    data = _read_tabular_bytes(request.csv_data, input_format)
                else:
                    return quality_pb2.ProcessPipelineResponse(
                        success=False,
                        error_message="No data provided",
                    )

                result = self.pipeline.process_structured(data, config)

                return quality_pb2.ProcessPipelineResponse(
                    result=self._convert_pipeline_result(result),
                    success=True,
                )

            else:
                # 处理非结构化数据
                result = self.pipeline.process_unstructured(request.text, config)

                return quality_pb2.ProcessPipelineResponse(
                    result=self._convert_pipeline_result(result),
                    success=True,
                )

        except Exception as e:
            return quality_pb2.ProcessPipelineResponse(
                success=False,
                error_message=str(e),
            )

    def DetectDrift(
        self,
        request: quality_pb2.DetectDriftRequest,
        context: ServicerContext,
    ) -> quality_pb2.DetectDriftResponse:
        """检测数据漂移。"""
        try:
            # 加载基线数据
            if request.baseline_file:
                baseline_data = self.evaluator.load_data(request.baseline_file)
            else:
                baseline_data = _read_csv_bytes(request.baseline_data)

            # 加载当前数据
            if request.current_file:
                current_data = self.evaluator.load_data(request.current_file)
            else:
                current_data = _read_csv_bytes(request.current_data)

            # 设置检测器
            columns = list(request.columns) if request.columns else None
            threshold = request.threshold if request.threshold > 0 else 0.05

            self.drift_detector.threshold = threshold
            self.drift_detector.set_baseline(baseline_data, columns)

            # 执行检测
            result = self.drift_detector.detect_drift(current_data, columns)

            return quality_pb2.DetectDriftResponse(
                has_drift=result.has_drift,
                drift_metrics=[
                    quality_pb2.DetectDriftResult(
                        field=m.field,
                        drift_score=m.drift_score,
                        p_value=m.p_value,
                        threshold=m.threshold,
                        drifted=m.drifted,
                    )
                    for m in result.drift_metrics
                ],
                overall_drift_score=result.overall_drift_score,
                baseline_timestamp=int(result.baseline_timestamp.timestamp()),
                current_timestamp=int(result.current_timestamp.timestamp()),
                success=True,
            )

        except Exception as e:
            return quality_pb2.DetectDriftResponse(
                success=False,
                error_message=str(e),
            )

    def ListDimensions(
        self,
        request: quality_pb2.ListDimensionsRequest,
        context: ServicerContext,
    ) -> quality_pb2.ListDimensionsResponse:
        """列出可用的评估维度。"""
        try:
            # 内置维度
            structured_dimensions = [
                "completeness",
                "accuracy",
                "consistency",
                "validity",
                "uniqueness",
                "timeliness",
            ]
            unstructured_dimensions = [
                "readability",
                "information_density",
                "language_quality",
                "duplication",
                "pii_detector",
                "relevance",
            ]

            dimensions = []

            # 添加结构化维度
            for dim in structured_dimensions:
                dimensions.append(
                    quality_pb2.DimensionInfo(
                        name=dim,
                        description=f"{dim} evaluator",
                        supports_structured=True,
                        supports_unstructured=False,
                    )
                )

            # 添加非结构化维度
            for dim in unstructured_dimensions:
                dimensions.append(
                    quality_pb2.DimensionInfo(
                        name=dim,
                        description=f"{dim} evaluator",
                        supports_structured=False,
                        supports_unstructured=True,
                    )
                )

            return quality_pb2.ListDimensionsResponse(
                dimensions=dimensions,
                success=True,
            )

        except Exception as e:
            return quality_pb2.ListDimensionsResponse(
                success=False,
                error_message=str(e),
            )

    def GetQualityRules(
        self,
        request: quality_pb2.GetQualityRulesRequest,
        context: ServicerContext,
    ) -> quality_pb2.GetQualityRulesResponse:
        """获取质量规则。"""
        try:
            rules = self.rule_engine.load_rules(request.rule_file or None)
            available_files = self.rule_engine.list_available_rule_files()

            return quality_pb2.GetQualityRulesResponse(
                rules=self._convert_quality_rules(rules),
                available_rule_files=available_files,
                success=True,
            )

        except Exception as e:
            return quality_pb2.GetQualityRulesResponse(
                success=False,
                error_message=str(e),
            )

    def UpdateQualityRules(
        self,
        request: quality_pb2.UpdateQualityRulesRequest,
        context: ServicerContext,
    ) -> quality_pb2.UpdateQualityRulesResponse:
        """更新质量规则。"""
        try:
            # 这里简化处理，实际应该保存到文件
            return quality_pb2.UpdateQualityRulesResponse(
                success=True,
            )

        except Exception as e:
            return quality_pb2.UpdateQualityRulesResponse(
                success=False,
                error_message=str(e),
            )

    def RegisterCustomEvaluator(
        self,
        request: quality_pb2.RegisterCustomEvaluatorRequest,
        context: ServicerContext,
    ) -> quality_pb2.RegisterCustomEvaluatorResponse:
        """注册自定义评估器。"""
        try:
            # 动态加载和注册评估器
            import importlib

            module = importlib.import_module(request.module_path)
            evaluator_class = getattr(module, request.class_name)

            # 注册到注册表
            EvaluatorRegistry.register(request.name)(evaluator_class)

            return quality_pb2.RegisterCustomEvaluatorResponse(
                success=True,
            )

        except Exception as e:
            return quality_pb2.RegisterCustomEvaluatorResponse(
                success=False,
                error_message=str(e),
            )

    def RegisterCustomCleaner(
        self,
        request: quality_pb2.RegisterCustomCleanerRequest,
        context: ServicerContext,
    ) -> quality_pb2.RegisterCustomCleanerResponse:
        """注册自定义清洗器。"""
        try:
            # 动态加载和注册清洗器
            import importlib

            module = importlib.import_module(request.module_path)
            cleaner_class = getattr(module, request.class_name)

            # 注册到注册表
            CleanerRegistry.register(request.name)(cleaner_class)

            return quality_pb2.RegisterCustomCleanerResponse(
                success=True,
            )

        except Exception as e:
            return quality_pb2.RegisterCustomCleanerResponse(
                success=False,
                error_message=str(e),
            )

    def _convert_quality_report(self, report: QualityReport) -> quality_pb2.QualityReport:
        """转换质量报告。"""
        return quality_pb2.QualityReport(
            overall_score=report.overall_score,
            dimensions=[self._convert_dimension_score(d) for d in report.dimensions],
            issues=[self._convert_quality_issue(i) for i in report.issues],
            record_count=report.record_count,
            timestamp=int(report.timestamp.timestamp()),
            # proto metadata 为 map<string,string>, 需将值统一转为字符串
            metadata={k: str(v) for k, v in report.metadata.items()},
        )

    def _convert_unstructured_report(
        self, report: UnstructuredQualityReport
    ) -> quality_pb2.UnstructuredQualityReport:
        """转换非结构化质量报告。"""
        return quality_pb2.UnstructuredQualityReport(
            overall_score=report.overall_score,
            dimensions=[self._convert_unstructured_dim_score(d) for d in report.dimensions],
            text_stats={k: str(v) for k, v in report.text_stats.items()},
            issues=[self._convert_text_issue(i) for i in report.issues],
            timestamp=int(report.timestamp.timestamp()),
        )

    def _convert_dimension_score(self, score) -> quality_pb2.DimensionScore:
        """转换维度评分。"""
        return quality_pb2.DimensionScore(
            name=score.name,
            score=score.score,
            passed=score.passed,
            issues=[self._convert_quality_issue(i) for i in score.issues],
        )

    def _convert_unstructured_dim_score(self, score) -> quality_pb2.UnstructuredDimensionScore:
        """转换非结构化维度评分。"""
        return quality_pb2.UnstructuredDimensionScore(
            name=score.name,
            score=score.score,
            passed=score.passed,
            issues=[self._convert_text_issue(i) for i in score.issues],
            details={k: str(v) for k, v in score.details.items()},
        )

    def _convert_quality_issue(self, issue) -> quality_pb2.QualityIssue:
        """转换质量问题。"""
        return quality_pb2.QualityIssue(
            severity=self._convert_severity(issue.severity),
            dimension=issue.dimension,
            field=issue.field or "",
            description=issue.description,
            count=issue.count,
            sample=[str(s) for s in issue.sample[:5]],
        )

    def _convert_text_issue(self, issue) -> quality_pb2.TextQualityIssue:
        """转换文本质量问题。"""
        return quality_pb2.TextQualityIssue(
            type=issue.type,
            severity=self._convert_severity(issue.severity),
            description=issue.description,
            position_start=issue.position[0] if issue.position else 0,
            position_end=issue.position[1] if issue.position else 0,
            snippet=issue.snippet or "",
            suggestion=issue.suggestion or "",
        )

    def _convert_severity(self, severity) -> quality_pb2.SeverityLevel:
        """转换严重级别。"""
        mapping = {
            "critical": quality_pb2.SeverityLevel.CRITICAL,
            "warning": quality_pb2.SeverityLevel.WARNING,
            "info": quality_pb2.SeverityLevel.INFO,
        }
        return mapping.get(severity.value if hasattr(severity, "value") else severity, quality_pb2.SeverityLevel.INFO)

    def _convert_cleaning_result(self, result) -> quality_pb2.CleaningResult:
        """转换清洗结果。"""
        return quality_pb2.CleaningResult(
            original_count=result.original_count,
            cleaned_count=result.cleaned_count,
            removed_count=result.removed_count,
            operations=[
                quality_pb2.CleaningOperation(
                    type=op.type,
                    field=op.field or "",
                    count=op.count,
                    description=op.description,
                )
                for op in result.operations
            ],
            metadata={k: str(v) for k, v in result.metadata.items()},
            timestamp=int(result.timestamp.timestamp()),
        )

    def _convert_pipeline_result(self, result) -> quality_pb2.PipelineResult:
        """转换流程结果。"""
        pb_result = quality_pb2.PipelineResult(
            decision=self._convert_decision(result.decision),
            quick_score=result.quick_score or 0.0,
            deep_score=result.deep_score,
            stage_times={k: float(v) for k, v in result.stage_times.items()},
            metadata={k: str(v) for k, v in result.metadata.items()},
            timestamp=int(result.timestamp.timestamp()),
        )

        if result.cleaning_result:
            pb_result.cleaning_result.CopyFrom(self._convert_cleaning_result(result.cleaning_result))

        if isinstance(result.quality_report, QualityReport):
            pb_result.structured_report.CopyFrom(self._convert_quality_report(result.quality_report))
        else:
            pb_result.unstructured_report.CopyFrom(self._convert_unstructured_report(result.quality_report))

        return pb_result

    def _convert_decision(self, decision) -> quality_pb2.Decision:
        """转换决策。"""
        mapping = {
            "accept": quality_pb2.Decision.ACCEPT,
            "reject": quality_pb2.Decision.REJECT,
            "review": quality_pb2.Decision.REVIEW,
        }
        return mapping.get(decision.value if hasattr(decision, "value") else decision, quality_pb2.Decision.REVIEW)

    def _convert_quality_rules(self, rules) -> quality_pb2.QualityRules:
        """转换质量规则。"""
        return quality_pb2.QualityRules(
            version=rules.version,
            field_rules=[self._convert_field_rule(r) for r in rules.field_rules],
            dimension_rules=[self._convert_dimension_rule(r) for r in rules.dimension_rules],
            cleaning_rules={k: str(v) for k, v in rules.cleaning_rules.items()},
            metadata={k: str(v) for k, v in rules.metadata.items()},
        )

    def _convert_field_rule(self, rule) -> quality_pb2.FieldRule:
        """转换字段规则。"""
        return quality_pb2.FieldRule(
            name=rule.name,
            type=self._convert_field_type(rule.type),
            required=rule.required,
            pattern=rule.pattern or "",
            min_value=float(rule.min_value) if rule.min_value is not None else 0.0,
            max_value=float(rule.max_value) if rule.max_value is not None else 0.0,
            min_length=rule.min_length or 0,
            max_length=rule.max_length or 0,
            allowed_values=list(rule.allowed_values) if rule.allowed_values else [],
            description=rule.description,
        )

    def _convert_dimension_rule(self, rule) -> quality_pb2.DimensionRule:
        """转换维度规则。"""
        return quality_pb2.DimensionRule(
            name=rule.name,
            enabled=rule.enabled,
            threshold=rule.threshold,
            weight=rule.weight,
            config={k: str(v) for k, v in rule.config.items()},
        )

    def _convert_field_type(self, field_type) -> quality_pb2.FieldType:
        """转换字段类型。"""
        mapping = {
            "string": quality_pb2.FieldType.STRING,
            "integer": quality_pb2.FieldType.INTEGER,
            "float": quality_pb2.FieldType.FLOAT,
            "boolean": quality_pb2.FieldType.BOOLEAN,
            "date": quality_pb2.FieldType.DATE,
            "datetime": quality_pb2.FieldType.DATETIME,
            "array": quality_pb2.FieldType.ARRAY,
        }
        return mapping.get(field_type.value if hasattr(field_type, "value") else field_type, quality_pb2.FieldType.STRING)
