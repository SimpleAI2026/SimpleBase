import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ADMIN_PROJECT_ID, DEFAULT_PROJECT_ID, useProjectStore } from './project'

vi.mock('../services/api', () => ({
  api: {
    projects: {
      list: vi.fn()
    }
  }
}))

import { api } from '../services/api'

describe('useProjectStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(api.projects.list).mockReset()
  })

  it('defaults project id and exposes admin getter', () => {
    localStorage.removeItem('sb_project_id')
    setActivePinia(createPinia())
    const store = useProjectStore()
    expect(store.id).toBe(DEFAULT_PROJECT_ID)
    expect(localStorage.getItem('sb_project_id')).toBe(DEFAULT_PROJECT_ID)
    store.setProject(ADMIN_PROJECT_ID)
    expect(store.isAdmin).toBe(true)
  })

  it('ignores empty set/remember and tracks history', () => {
    const store = useProjectStore()
    store.setProject('   ')
    store.remember('  ')
    store.setProject('proj-a', 'Alpha')
    store.setProject('proj-b')
    store.remember('proj-a')
    expect(store.history[0]).toBe('proj-a')
    expect(store.history).toContain('proj-b')
    expect(store.displayName).toBe('proj-b')
  })

  it('resolves display name from explicit name or project list', async () => {
    vi.mocked(api.projects.list).mockResolvedValue([
      { id: DEFAULT_PROJECT_ID, name: '商城后台', createdAt: 't' }
    ])
    const store = useProjectStore()
    store.setProject(DEFAULT_PROJECT_ID)
    await store.loadProjects()
    expect(store.projects).toHaveLength(1)
    expect(store.current?.name).toBe('商城后台')
    expect(store.displayName).toBe('商城后台')
    store.projectName = '  Custom  '
    expect(store.displayName).toBe('Custom')
  })

  it('tolerates list failures and invalid history JSON', async () => {
    localStorage.setItem('sb_project_id_history', '{not-json')
    setActivePinia(createPinia())
    vi.mocked(api.projects.list).mockRejectedValue(new Error('boom'))
    const store = useProjectStore()
    expect(store.history).toEqual([])
    await store.loadProjects()
    expect(store.projects).toEqual([])
    expect(store.loading).toBe(false)
  })

  it('toggles the create modal', () => {
    const store = useProjectStore()
    store.openCreateModal()
    expect(store.createModalOpen).toBe(true)
    store.closeCreateModal()
    expect(store.createModalOpen).toBe(false)
  })

  it('covers history non-array JSON, empty names, and non-array project lists', async () => {
    localStorage.removeItem('sb_project_id_history')
    localStorage.setItem('sb_project_id_history', '{}')
    setActivePinia(createPinia())
    expect(useProjectStore().history).toEqual([])

    localStorage.setItem('sb_project_id_history', JSON.stringify(['', 'kept']))
    setActivePinia(createPinia())
    expect(useProjectStore().history).toEqual(['kept'])

    vi.mocked(api.projects.list).mockResolvedValueOnce(null as never)
    const store = useProjectStore()
    store.projectId = ''
    store.projectName = ''
    expect(store.displayName).toBe('选择项目')
    await store.loadProjects()
    expect(store.projects).toEqual([])

    store.setProject(DEFAULT_PROJECT_ID)
    vi.mocked(api.projects.list).mockResolvedValueOnce([{ id: DEFAULT_PROJECT_ID, name: '', createdAt: 't' }])
    await store.loadProjects()
    expect(store.projectName).toBe('')
    store.setProject(DEFAULT_PROJECT_ID)
    expect(store.projectName).toBe('')
  })
})
