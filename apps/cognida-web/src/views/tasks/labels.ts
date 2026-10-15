const capabilities: Record<string, string> = {
  'lifecycle.data-access': '数据接入', 'lifecycle.data-quality': '质量检查',
  'lifecycle.data-cleaning': '数据清洗', 'lifecycle.data-enrichment': '数据增强',
  'lifecycle.data-asset': '资产发布', 'lifecycle.data-analysis': '数据分析',
  'lifecycle.data-evaluation': '结果评估',
}
const kinds: Record<string, string> = { raw_asset: '原始输入', cleaned_asset: '清洗版本', enriched_asset: '增强版本', published_asset: '已发布资产', quality_report: '质量报告', insight: '分析报告', evaluation: '评估报告', result: '计算结果' }
const statuses: Record<string, string> = { draft: '草稿', ready: '已就绪', running: '进行中', awaiting_approval: '待审批', partially_completed: '部分完成', completed: '已完成', failed: '失败', timed_out: '超时', cancel_requested: '已申请取消', reconcile_required: '待核对', cancelled: '已取消', pending: '待投递', dispatching: '投递中', bound: '已绑定', unknown: '尚未核实' }
const lookup = (labels: Record<string, string>, value: string) => Object.prototype.hasOwnProperty.call(labels, value) ? labels[value]! : value
export const capabilityLabel = (id: string) => lookup(capabilities, id)
export const assetLabel = (kind?: string) => lookup(kinds, kind || '资产')
export const statusLabel = (status: string) => lookup(statuses, status)
