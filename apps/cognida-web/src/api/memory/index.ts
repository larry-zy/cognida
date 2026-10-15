import { http } from '@/utils/request'

export type MemoryCategory = 'fact' | 'preference'
export interface PersonalMemory {
  id: string
  content: string
  category: MemoryCategory
  importance: number
  version: number
  source: 'user' | 'tool'
  access_count: number
  created_at: string
  updated_at: string
}
export interface MemoryInput {
  content: string
  category: MemoryCategory
  importance: number
}
export interface MemoryPage {
  items: PersonalMemory[]
  next_cursor: string
  has_more: boolean
}
export interface MemoryRecall {
  items: { memory: PersonalMemory; score: number; method: string }[]
  method: string
  candidate_limit: number
  truncated: boolean
  note: string
}
export const memoryApi = {
  list: (params: { category?: string; q?: string; cursor?: string; page_size?: number }) =>
    http.get<MemoryPage>('/memories', { params }),
  get: (id: string) => http.get<PersonalMemory>(`/memories/${encodeURIComponent(id)}`),
  create: (input: MemoryInput, key: string) =>
    http.post<PersonalMemory>('/memories', input, { headers: { 'Idempotency-Key': key } }),
  update: (id: string, input: MemoryInput, expectedVersion: number) =>
    http.put<PersonalMemory>(`/memories/${encodeURIComponent(id)}`, { ...input, expected_version: expectedVersion }),
  remove: (id: string, expectedVersion: number) =>
    http.delete(`/memories/${encodeURIComponent(id)}`, { params: { expected_version: expectedVersion } }),
  recall: (query: string) => http.post<MemoryRecall>('/memories/recall', { query, limit: 5 }),
}
