// Translate only exact, backend-defined values. Unknown statuses and free-form
// user/model text remain unchanged, including punctuation and whitespace.
const statuses = new Map([
  ["verified_computation", "统计计算已校验，业务语义未验证"],
  ["computed", "已完成统计计算"],
  ["integrity_checked_semantics_unverified", "产物完整性已检查，语义未验证"],
  ["citations_checked_interpretation_unverified", "证据引用已检查，模型解释未验证"],
  ["numeric_agreement_only", "仅数值比对一致，业务语义未验证"],
  ["requires_clarification", "需要进一步核实"],
  ["evidence_resolved_actions_unverified", "证据已读取，行动建议未验证"],
  ["numeric_agreement_actions_unverified", "数值比对一致，行动建议未验证"],
]);

const methods = new Map([
  ["descriptive_statistics", "描述性统计"],
  ["deterministic_statistics", "确定性统计计算"],
  ["goal-directed-llm-v1", "围绕分析目标的模型解读"],
  ["artifact_integrity_and_provenance", "产物完整性与来源检查"],
  ["deterministic_keyed_numeric_comparison", "按指定键和容差进行数值比对"],
  ["authorized_insight_and_retrieved_knowledge_synthesis", "综合已授权的分析报告与检索知识"],
]);

const statements = new Map([
  ["Input snapshot is representative and source semantics have been checked by the data owner.", "假设输入快照具有代表性，且数据负责人已核实来源数据的业务含义。"],
  ["Missing or stale inputs can change the result; descriptive statistics do not establish causality.", "输入缺失或过期可能改变结果；描述性统计不能证明因果关系。"],
  ["Review source definitions and freshness before taking action.", "采取行动前，请核实来源数据的定义与时效性。"],
  ["Source snapshot represents the intended observation period", "假设来源快照涵盖所需的观察时段。"],
  ["Sampling and source quality may limit conclusions", "采样方式和来源数据质量可能限制结论。"],
  ["Descriptive observations only; no causal inference or predictive accuracy is claimed.", "仅提供描述性观察，不作因果推断，也不保证预测准确性。"],
  ["Numeric summaries use finite float64 approximations; exact SQL decimal and identifier values remain unchanged in source and published snapshots.", "数值统计使用有限的 64 位浮点近似值；SQL 小数和标识符的精确原值在来源及发布快照中保持不变。"],
  ["This evaluation verifies artifacts and evidence links; semantic or model accuracy requires a labelled evaluation dataset.", "本次评估检查产物与证据链接；业务语义或模型准确性仍需使用已标注的评估数据集验证。"],
  ["Citations resolve to source evidence; model interpretation is not independently verified.", "引用已关联到来源证据；模型解释尚未经过独立验证。"],
  ["Source excerpts are bounded samples; omitted rows/text may change conclusions.", "来源摘录为有限样本；未包含的数据行或文本可能改变结论。"],
  ["Both paths use the same period, filters, aggregation, units and null semantics; caller must confirm this caliber.", "假设两条路径采用相同的时段、筛选条件、聚合方式、单位及空值规则；这些统计口径须由调用者确认。"],
  ["Result references do not prove independent execution paths.", "结果引用不能证明执行路径相互独立。"],
  ["Agreement can preserve shared upstream errors; this is not causal or semantic validation.", "比对一致仍可能包含共同的上游错误；本次比对不验证因果关系或业务语义。"],
  ["Confirm the metric definition and independent provenance before acting.", "采取行动前，请确认指标定义，并核实来源是否相互独立。"],
  ["Rows are paired by position because no key columns were supplied.", "由于未指定键列，数据行按位置进行配对。"],
  ["Reconcile missing values, units, time windows, filters and source paths before making a decision.", "作出决策前，请核对缺失值、单位、时间窗口、筛选条件和来源路径。"],
  ["Numeric support is inconsistent or absent.", "数值依据不一致或缺失。"],
  ["Caller-proposed actions, assumptions and risks are hypotheses, not conclusions inferred or verified by this tool.", "调用者提出的行动、假设和风险均为待验证假说，并非本工具推导或验证过的结论。"],
  ["Descriptive data and retrieved text do not establish causality or predict action outcomes.", "描述性数据和检索文本不能证明因果关系，也不能预测行动结果。"],
  ["Review the cited evidence and validate each action in a bounded experiment before adoption.", "采纳建议前，请审查引用证据，并在范围受控的实验中逐项验证行动。"],
  ["Numeric verification covers only the supplied result references; their linkage to these insights has not been verified.", "数值校验仅涵盖提供的结果引用；这些结果与当前分析结论的关联尚未验证。"],
  ["No supporting knowledge was retrieved; no domain support is claimed.", "未检索到支持性知识，不能认定结论获得了领域知识支持。"],
  ["Retrieved passages are source evidence; their relevance to proposed actions remains subject to review.", "检索片段仅作为来源证据；它们与所提行动的相关性仍须审查。"],
]);

export const decisionStatusLabel = (value?: string) => value ? statuses.get(value) ?? value : "未提供验证结论";
export const decisionMethodLabel = (value?: string) => value ? methods.get(value) ?? value : "生产者未提供方法说明";
export const decisionText = (value: string) => statements.get(value) ?? value;
