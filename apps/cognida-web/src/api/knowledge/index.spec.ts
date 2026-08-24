import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/utils/request', () => ({
  http: { post }
}))

import { knowledgeApi } from './index'

describe('knowledgeApi.rebuildGraph', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('uses a five-minute timeout without changing the global request timeout', () => {
    knowledgeApi.rebuildGraph('kb-1')

    expect(post).toHaveBeenCalledWith('/knowledge-bases/kb-1/graph/rebuild', undefined, {
      timeout: 5 * 60 * 1000
    })
  })
})
