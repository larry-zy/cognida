import type { RebuildGraphResult } from '@/types'

/**
 * 补建成功文案：如实区分「已替换」与「保留旧图未替换」。
 * 节点/关系计数仅在图谱实际写入后才展示，避免把未发生的写入伪装成成功。
 */
export function rebuildSuccessMessage(r: RebuildGraphResult | null | undefined): string {
  if (!r) return '补建完成'
  if (!r.graph_replaced) {
    if (r.failed_documents > 0) {
      return `有 ${r.failed_documents} 篇文档处理失败，为保留现有图谱本次未做任何替换`
    }
    return '未提取到任何图谱数据，已保留现有图谱'
  }
  const failed = r.failed_documents ? `，失败 ${r.failed_documents} 篇` : ''
  const skipped = r.skipped_documents ? `，跳过 ${r.skipped_documents} 篇` : ''
  return (
    `补建完成：处理 ${r.processed_documents}/${r.total_documents} 篇，` +
    `写入 ${r.total_nodes} 节点、${r.total_relations} 关系${skipped}${failed}`
  )
}
