import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { knowledgeApi } from '@/api/knowledge'
import type { KnowledgeBase } from '@/types'
import { useAuthStore } from '@/stores/auth'

// 知识库列表属「跨视图共享域数据」：助手选库、图谱列表、知识库管理、评测建任务等多处都要读。
// 过去各视图各自 knowledgeApi.getList() + 本地 ref，导致重复请求、状态不一致（〔FE-10〕）。
// 此 store 作为单一事实源，带 TTL 缓存 + in-flight 去重：TTL 内复用缓存，避免频繁切换页面重复拉取；
// 变更（建/删）后由调用方 refresh() 失效缓存。
const CACHE_TTL = 60_000 // 缓存有效期 1 分钟

export const useKnowledgeStore = defineStore('knowledge', () => {
  const auth = useAuthStore()
  const knowledgeBases = ref<KnowledgeBase[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  // 缓存时间戳与并发去重句柄不需要响应式，用闭包变量而非 ref。
  let fetchedAt = 0
  let inflight: Promise<KnowledgeBase[]> | null = null
  let generation = 0

  function reset(): void {
    generation++
    knowledgeBases.value = []
    fetchedAt = 0
    inflight = null
    loading.value = false
    error.value = null
  }

  // Cache entries and outstanding responses belong to one authenticated identity.
  // Flush synchronously so the next view cannot read the preceding tenant's data.
  // Watch primitive sources separately: refreshing a profile/tenant object for
  // the same identity must not discard an in-flight knowledge-base response.
  watch([() => auth.user?.id, () => auth.currentTenant?.id, () => auth.isAuthenticated], reset, { flush: 'sync' })

  // 后端列表可能走分页信封 {items,total}，也可能直接返回数组，统一归一。
  function normalize(data: unknown): KnowledgeBase[] {
    if (Array.isArray(data)) return data as KnowledgeBase[]
    const items = (data as { items?: KnowledgeBase[] } | null)?.items
    return Array.isArray(items) ? items : []
  }

  async function fetchList(requestGeneration: number): Promise<KnowledgeBase[]> {
    loading.value = true
    error.value = null
    try {
      const res = await knowledgeApi.getList()
      if (requestGeneration !== generation) return knowledgeBases.value
      knowledgeBases.value = res.data ? normalize(res.data) : []
      fetchedAt = Date.now()
      return knowledgeBases.value
    } catch (e) {
      if (requestGeneration !== generation) return knowledgeBases.value
      error.value = e instanceof Error ? e.message : '加载知识库列表失败'
      throw e
    } finally {
      if (requestGeneration === generation) loading.value = false
    }
  }

  /**
   * 加载知识库列表（带缓存）。
   * - TTL 内且非 force：直接返回缓存，不发请求；
   * - 已有在途请求且非 force：复用同一 Promise，避免并发重复拉取；
   * - force：强制重新拉取（用于建/删后刷新）。
   */
  function loadKnowledgeBases(opts?: { force?: boolean }): Promise<KnowledgeBase[]> {
    const force = opts?.force === true
    const fresh = fetchedAt > 0 && Date.now() - fetchedAt < CACHE_TTL
    if (!force && fresh) return Promise.resolve(knowledgeBases.value)
    if (!force && inflight) return inflight
    const requestGeneration = ++generation
    inflight = fetchList(requestGeneration).finally(() => {
      if (requestGeneration === generation) inflight = null
    })
    return inflight
  }

  // refresh：强制刷新（建/删知识库后调用）。
  function refresh(): Promise<KnowledgeBase[]> {
    return loadKnowledgeBases({ force: true })
  }

  // invalidate：仅使缓存失效，下次读取自然重新拉取。
  function invalidate(): void {
    fetchedAt = 0
  }

  return { knowledgeBases, loading, error, loadKnowledgeBases, refresh, invalidate, reset }
})
