const capabilities: Record<string, string> = {
  'governance.feature-engineering': '特征工程', 'governance.tagging': '知识标签候选',
  'governance.classification': '分类预标注', 'governance.named_entity': '实体预标注',
  'governance.qa': '问答样本标注',
  'lifecycle.data-access': '数据接入', 'lifecycle.data-quality': '质量检查',
  'lifecycle.data-cleaning': '数据清洗', 'lifecycle.data-enrichment': '数据增强',
  'lifecycle.data-asset': '资产发布', 'lifecycle.data-analysis': '数据分析',
  'lifecycle.data-evaluation': '结果评估',
}
const kinds: Record<string, string> = { raw_asset: '原始输入', cleaned_asset: '清洗版本', enriched_asset: '增强版本', published_asset: '已发布资产', quality_report: '质量报告', insight: '分析报告', evaluation: '评估报告', result: '计算结果' }
const profiles: Record<string, string> = {
  'quality-default-v1; min_score=70': '质量检查，最低分数 70',
  'clean-trim-dedup-v1; keep=first; min_score=70': '去除首尾空白、重复记录保留首次出现；质量阈值 70',
  'enrich-rules-v1; keywords,summary,sensitivity': '规则提取关键词（最多 10 个）、摘要（最多 300 字符）与敏感度',
  'artifact-integrity-and-provenance-v1; no model accuracy claims': '核对产物完整性与来源；不代表模型准确率或语义已验证',
}
const statuses: Record<string, string> = { succeeded: '已成功', queued: '已排队', waiting: '等待依赖', blocked: '等待依赖', skipped: '已跳过', quarantined: '已隔离', draft: '草稿', ready: '已就绪', running: '进行中', awaiting_approval: '待审批', partially_completed: '部分完成', completed: '已完成', failed: '失败', timed_out: '超时', cancel_requested: '已申请取消', reconcile_required: '待核对', cancelled: '已取消', pending: '待投递', dispatching: '投递中', bound: '已绑定', unknown: '尚未核实' }
const lookup = (labels: Record<string, string>, value: string) => Object.prototype.hasOwnProperty.call(labels, value) ? labels[value]! : value
export const capabilityLabel = (id: string) => lookup(capabilities, id)
export const assetLabel = (kind?: string) => lookup(kinds, kind || '资产')
export const statusLabel = (status: string) => lookup(statuses, status)
export const executionProfileLabel = (profile: string) => lookup(profiles, profile)
