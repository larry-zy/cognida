import { http } from '@/utils/request'
import type { AssetRef, Invocation, Page } from '@/api/lifecycle'
import type { CollectionTask, CollectionTaskListResponse } from '@/api/collection'

export interface OverviewPage<T> {
  items: T[]
  hasMore: boolean
}

function lifecyclePage<T>(data: Page<T> | undefined): OverviewPage<T> {
  if (!data || !Array.isArray(data.items)) throw new Error('服务未返回有效列表，请重试')
  return { items: data.items, hasMore: Boolean(data.has_more || data.next_cursor) }
}

// Overview reads existing, authorized lists. It never derives global totals
// from a page or starts work while the user is looking at the workspace.
export const overviewApi = {
  async runs(signal?: AbortSignal) {
    const response = await http.get<Page<Invocation>>('/lifecycle/runs', { params: { page_size: 50 }, signal })
    return lifecyclePage(response.data)
  },
  async invocations(signal?: AbortSignal) {
    const response = await http.get<Page<Invocation>>('/lifecycle/invocations', { params: { page_size: 50 }, signal })
    return lifecyclePage(response.data)
  },
  async assets(signal?: AbortSignal) {
    const response = await http.get<Page<AssetRef>>('/lifecycle/assets', { params: { kind: 'published_asset', page_size: 20 }, signal })
    return lifecyclePage(response.data)
  },
  async collection(signal?: AbortSignal): Promise<OverviewPage<CollectionTask>> {
    const response = await http.get<CollectionTaskListResponse>('/collection-tasks', { signal })
    if (!response.data || !Array.isArray(response.data.tasks)) throw new Error('服务未返回有效采集列表，请重试')
    // The collection API currently bounds the list at 100, without a cursor.
    return { items: response.data.tasks, hasMore: response.data.tasks.length >= 100 }
  }
}
