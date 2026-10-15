import { describe, expect, it } from 'vitest'
import { activeNavigationPath, navigationGroups } from './navigation'

describe('workspace navigation', () => {
  it('marks only the most specific route, including the legacy data-agent URL', () => {
    expect(activeNavigationPath('/analysis/chat/session-1')).toBe('/analysis/chat')
    expect(activeNavigationPath('/analysis')).toBe('/analysis')
    expect(activeNavigationPath('/tasks')).toBe('/tasks')
    expect(activeNavigationPath('/data-agent/session-1')).toBe('/analysis/chat')
    expect(activeNavigationPath('/knowledge/book-1')).toBe('/knowledge')
    expect(activeNavigationPath('/knowledge-other')).toBeUndefined()
  })
  it('keeps all workflow destinations unique and available', () => {
    const paths = navigationGroups.flatMap(group => group.items.map(item => item.path))
    expect(new Set(paths).size).toBe(paths.length)
    for (const path of ['/home', '/tasks', '/collection-tasks', '/datasource', '/pipelines', '/experts', '/knowledge', '/kb-assistant', '/subscriptions', '/analysis']) expect(paths).toContain(path)
  })
})
