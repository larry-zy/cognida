import { describe, it, expect } from 'vitest'
import { rebuildSuccessMessage } from '../rebuild'
import type { RebuildGraphResult } from '@/types'

function makeResult(overrides: Partial<RebuildGraphResult> = {}): RebuildGraphResult {
  return {
    total_documents: 5,
    processed_documents: 5,
    skipped_documents: 0,
    failed_documents: 0,
    total_nodes: 10,
    total_relations: 8,
    graph_replaced: true,
    ...overrides
  }
}

describe('rebuildSuccessMessage', () => {
  it('空结果返回默认文案', () => {
    expect(rebuildSuccessMessage(null)).toBe('补建完成')
    expect(rebuildSuccessMessage(undefined)).toBe('补建完成')
  })

  it('替换成功时展示处理进度与去重后写入计数', () => {
    const msg = rebuildSuccessMessage(makeResult({ skipped_documents: 1, failed_documents: 1 }))
    expect(msg).toBe('补建完成：处理 5/5 篇，写入 10 节点、8 关系，跳过 1 篇，失败 1 篇')
  })

  it('存在失败文档时说明未替换且不展示写入计数', () => {
    const msg = rebuildSuccessMessage(
      makeResult({ failed_documents: 2, graph_replaced: false, total_nodes: 0, total_relations: 0 })
    )
    expect(msg).toBe('有 2 篇文档处理失败，为保留现有图谱本次未做任何替换')
    expect(msg).not.toContain('节点')
  })

  it('无失败但未替换（空图）时说明保留旧图', () => {
    expect(rebuildSuccessMessage(makeResult({ graph_replaced: false }))).toBe(
      '未提取到任何图谱数据，已保留现有图谱'
    )
  })
})
