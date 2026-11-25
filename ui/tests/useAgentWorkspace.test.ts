// 工作台取数：每群独立分页、展开懒加载、旧响应不覆盖。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { useAgentWorkspace } from '@/composables/useAgentWorkspace'
import type { AgentTag, AgentThread } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function tag(id: string, name: string): AgentTag {
  return {
    id,
    name,
    agentIds: [],
    prompts: [],
    unavailableAgentIds: [],
    routingPrompt: '',
    threadKinds: ['chat'],
    version: 1
  }
}

function thread(id: string, tagId?: string): AgentThread {
  return { id, title: id, tag_id: tagId, created_at: 't', updated_at: 't' }
}

describe('useAgentWorkspace', () => {
  beforeEach(() => {
    resetApiMocks()
    api.agentTags.list.mockResolvedValue([tag('general', '通用'), tag('g1', '告警')])
  })

  it('加载群列表并容错非数组响应', async () => {
    const ws = useAgentWorkspace({ projectId: () => 'p1' })
    api.agentTags.list.mockResolvedValueOnce({ nope: true })
    expect(await ws.loadTags()).toEqual([])
    api.agentTags.list.mockResolvedValueOnce([tag('g1', '告警')])
    const list = await ws.loadTags()
    expect(list).toHaveLength(1)
    expect(ws.tags.value[0].id).toBe('g1')
  })

  it('每个群独立分页：互不覆盖游标与列表', async () => {
    const ws = useAgentWorkspace({ projectId: () => 'p1' })
    api.agentThreads.pageBy
      .mockResolvedValueOnce({ threads: [thread('t1', 'g1')], next_cursor: 'c1' })
      .mockResolvedValueOnce({ threads: [thread('t2', 'general')], next_cursor: '' })
    await ws.loadThreads('g1')
    await ws.loadThreads('general')
    expect(ws.pageOf('g1').threads.map((t) => t.id)).toEqual(['t1'])
    expect(ws.pageOf('g1').cursor).toBe('c1')
    expect(ws.pageOf('g1').hasMore).toBe(true)
    expect(ws.pageOf('general').threads.map((t) => t.id)).toEqual(['t2'])
    expect(ws.pageOf('general').hasMore).toBe(false)
    expect(api.agentThreads.pageBy).toHaveBeenNthCalledWith(1, 'p1', expect.objectContaining({ tagId: 'g1' }))
  })

  it('加载更多去重保序，且无游标时不请求', async () => {
    const ws = useAgentWorkspace({ projectId: () => 'p1' })
    api.agentThreads.pageBy
      .mockResolvedValueOnce({ threads: [thread('t1', 'g1')], next_cursor: 'c1' })
      .mockResolvedValueOnce({ threads: [thread('t1', 'g1'), thread('t2', 'g1')], next_cursor: '' })
    await ws.loadThreads('g1')
    const merged = await ws.loadMore('g1')
    expect(merged.map((t) => t.id)).toEqual(['t1', 't2'])
    const calls = api.agentThreads.pageBy.mock.calls.length
    await ws.loadMore('g1')
    expect(api.agentThreads.pageBy.mock.calls.length).toBe(calls)
  })

  it('并行预取多个群：各自响应都落到自己的分页（互不丢弃）', async () => {
    const ws = useAgentWorkspace({ projectId: () => 'p1' })
    api.agentThreads.pageBy.mockImplementation((_p: string, q: { tagId: string }) => {
      if (q.tagId === 'g1') return Promise.resolve({ threads: [thread('t1', 'g1')], next_cursor: '' })
      return Promise.resolve({ threads: [thread('t2', 'g2')], next_cursor: 'c2' })
    })
    await ws.prefetchAll(['g1', 'g2'])
    expect(ws.pageOf('g1').threads.map((t) => t.id)).toEqual(['t1'])
    expect(ws.pageOf('g2').threads.map((t) => t.id)).toEqual(['t2'])
    expect(ws.pageOf('g2').hasMore).toBe(true)
  })

  it('请求失败时保留旧列表并给出错误', async () => {
    const ws = useAgentWorkspace({ projectId: () => 'p1' })
    api.agentThreads.pageBy.mockResolvedValueOnce({ threads: [thread('t1', 'g1')], next_cursor: '' })
    await ws.loadThreads('g1')
    api.agentThreads.pageBy.mockRejectedValueOnce(new Error('boom'))
    await ws.loadThreads('g1')
    expect(ws.pageOf('g1').threads.map((t) => t.id)).toEqual(['t1'])
    expect(ws.pageOf('g1').error).toBe('boom')
  })

  it('本地插入/移除/替换会话', async () => {
    const ws = useAgentWorkspace({ projectId: () => 'p1' })
    api.agentThreads.pageBy.mockResolvedValue({ threads: [thread('t1', 'g1')], next_cursor: '' })
    await ws.loadThreads('g1')
    ws.unshiftThread('g1', thread('t0', 'g1'))
    expect(ws.pageOf('g1').threads.map((t) => t.id)).toEqual(['t0', 't1'])
    ws.unshiftThread('g1', thread('t0', 'g1'))
    expect(ws.pageOf('g1').threads).toHaveLength(2)
    ws.replaceThread('g1', { ...thread('t1', 'g1'), title: '改名' })
    expect(ws.pageOf('g1').threads.find((t) => t.id === 't1')?.title).toBe('改名')
    ws.removeThread('g1', 't1')
    expect(ws.pageOf('g1').threads.map((t) => t.id)).toEqual(['t0'])
  })

  it('refreshGroup 保留本地新建会话', async () => {
    const ws = useAgentWorkspace({ projectId: () => 'p1' })
    api.agentThreads.pageBy.mockResolvedValueOnce({ threads: [thread('t1', 'g1')], next_cursor: '' })
    await ws.loadThreads('g1')
    ws.unshiftThread('g1', thread('local', 'g1'))
    api.agentThreads.pageBy.mockResolvedValueOnce({ threads: [thread('t1', 'g1')], next_cursor: '' })
    await ws.refreshGroup('g1')
    expect(ws.pageOf('g1').threads.map((t) => t.id)).toEqual(['t1', 'local'])
  })

  it('会话所属群只使用真实标签，不映射旧的无标签会话', () => {
    const ws = useAgentWorkspace({ projectId: () => 'p1' })
    expect(ws.tagIdOfThread(thread('t1'))).toBe('')
    expect(ws.tagIdOfThread(thread('t1', 'general'))).toBe('general')
    expect(ws.tagIdOfThread(thread('t1', 'g1'))).toBe('g1')
    expect(ws.tagIdOfThread(undefined)).toBe('')
  })
  it('setPage / appendPage / mergeFirstPage：当前群与其他群共用同一份分页', () => {
    const ws = useAgentWorkspace({ projectId: () => 'p1' })
    ws.setPage('g1', { threads: [thread('t1', 'g1'), thread('t2', 'g1')], next_cursor: 'c1' })
    expect(ws.pageOf('g1')).toMatchObject({ cursor: 'c1', hasMore: true })

    // 追加：按 id 去重保序，并更新游标。
    ws.appendPage('g1', { threads: [thread('t2', 'g1'), thread('t3', 'g1')], next_cursor: '' })
    expect(ws.pageOf('g1').threads.map((t) => t.id)).toEqual(['t1', 't2', 't3'])
    expect(ws.pageOf('g1')).toMatchObject({ cursor: '', hasMore: false })

    // 合并首屏：服务端结果在前，保留本地新建但服务端尚未收录的会话。
    ws.unshiftThread('g1', thread('local', 'g1'))
    ws.mergeFirstPage('g1', { threads: [thread('t1', 'g1'), thread('t9', 'g1')], next_cursor: 'c9' })
    expect(ws.pageOf('g1').threads.map((t) => t.id)).toEqual(['t1', 't9', 'local', 't2', 't3'])
    expect(ws.pageOf('g1')).toMatchObject({ cursor: 'c9', hasMore: true })

    // 畸形响应一律按空页处理。
    ws.setPage('g2', { threads: undefined, next_cursor: undefined } as never)
    expect(ws.pageOf('g2')).toMatchObject({ threads: [], cursor: '', hasMore: false })
  })

  it('非 Error 抛出（含字符串）时错误文案统一走 i18n，不再是英文硬编码', async () => {
    const ws = useAgentWorkspace({ projectId: () => 'p1' })
    api.agentThreads.pageBy.mockRejectedValueOnce({ weird: true })
    await ws.loadThreads('g1')
    expect(ws.pageOf('g1').error).toBe('请求失败')
    api.agentThreads.pageBy.mockRejectedValueOnce('plain text')
    await ws.loadThreads('g1')
    expect(ws.pageOf('g1').error).toBe('请求失败')
    api.agentThreads.pageBy.mockRejectedValueOnce(new Error('boom'))
    await ws.loadThreads('g1')
    expect(ws.pageOf('g1').error).toBe('boom')
  })

  describe('当前群首屏 / 续页 / 刷新（页面不再自己做竞态判断）', () => {
    const deferred = <T,>() => {
      let resolve!: (v: T) => void
      const promise = new Promise<T>((r) => { resolve = r })
      return { promise, resolve }
    }

    it('loadFirstPage：按传入条数取数，落地后返回线程；loading 起落正确', async () => {
      const ws = useAgentWorkspace({ projectId: () => 'p1' })
      api.agentThreads.pageBy.mockResolvedValueOnce({ threads: [thread('a', 'g1')], next_cursor: 'c1' })
      const pending = ws.loadFirstPage('g1', 50, () => true)
      expect(ws.pageOf('g1').loading).toBe(true)
      expect((await pending)?.map((th) => th.id)).toEqual(['a'])
      expect(api.agentThreads.pageBy).toHaveBeenCalledWith('p1', { tagId: 'g1', limit: 50 })
      expect(ws.pageOf('g1')).toMatchObject({ loading: false, cursor: 'c1', hasMore: true })
    })

    it('loadFirstPage：期间项目切换 / 群不再是当前群 / 被更新的一次覆盖，旧响应不落地', async () => {
      let project = 'p1'
      const ws = useAgentWorkspace({ projectId: () => project })
      const d1 = deferred<{ threads: AgentThread[]; next_cursor: string }>()
      api.agentThreads.pageBy.mockReturnValueOnce(d1.promise)
      const first = ws.loadFirstPage('g1', 50, () => true)
      project = 'p2'
      d1.resolve({ threads: [thread('stale', 'g1')], next_cursor: '' })
      expect(await first).toBeNull()
      expect(ws.pageOf('g1').threads).toEqual([])

      project = 'p1'
      const d2 = deferred<{ threads: AgentThread[]; next_cursor: string }>()
      api.agentThreads.pageBy.mockReturnValueOnce(d2.promise)
      let current = true
      const second = ws.loadFirstPage('g1', 50, () => current)
      current = false
      d2.resolve({ threads: [thread('stale2', 'g1')], next_cursor: '' })
      expect(await second).toBeNull()
      expect(ws.pageOf('g1').threads).toEqual([])

      const d3 = deferred<{ threads: AgentThread[]; next_cursor: string }>()
      const d4 = deferred<{ threads: AgentThread[]; next_cursor: string }>()
      api.agentThreads.pageBy.mockReturnValueOnce(d3.promise).mockReturnValueOnce(d4.promise)
      const older = ws.loadFirstPage('g1', 50, () => true)
      const newer = ws.loadFirstPage('g1', 50, () => true)
      d4.resolve({ threads: [thread('new', 'g1')], next_cursor: '' })
      expect((await newer)?.map((th) => th.id)).toEqual(['new'])
      d3.resolve({ threads: [thread('old', 'g1')], next_cursor: '' })
      expect(await older).toBeNull()
      expect(ws.pageOf('g1').threads.map((th) => th.id)).toEqual(['new'])
    })

    it('loadFirstPage：取数失败原样抛给页面（由页面 toast），loading 复位', async () => {
      const ws = useAgentWorkspace({ projectId: () => 'p1' })
      api.agentThreads.pageBy.mockRejectedValueOnce(new Error('boom'))
      await expect(ws.loadFirstPage('g1', 50, () => true)).rejects.toThrow('boom')
      expect(ws.pageOf('g1').loading).toBe(false)
    })

    it('loadMore：带游标与传入条数，去重追加；期间群被重载则丢弃旧响应', async () => {
      const ws = useAgentWorkspace({ projectId: () => 'p1' })
      ws.setPage('g1', { threads: [thread('a', 'g1')], next_cursor: 'c1' })
      api.agentThreads.pageBy.mockResolvedValueOnce({ threads: [thread('a', 'g1'), thread('b', 'g1')], next_cursor: '' })
      await ws.loadMore('g1', 50)
      expect(api.agentThreads.pageBy).toHaveBeenLastCalledWith('p1', { tagId: 'g1', limit: 50, cursor: 'c1' })
      expect(ws.pageOf('g1').threads.map((th) => th.id)).toEqual(['a', 'b'])
      expect(ws.pageOf('g1').hasMore).toBe(false)

      ws.setPage('g2', { threads: [thread('x', 'g2')], next_cursor: 'c2' })
      const d = deferred<{ threads: AgentThread[]; next_cursor: string }>()
      api.agentThreads.pageBy.mockReturnValueOnce(d.promise)
      const more = ws.loadMore('g2')
      api.agentThreads.pageBy.mockResolvedValueOnce({ threads: [thread('fresh', 'g2')], next_cursor: '' })
      await ws.loadThreads('g2')
      d.resolve({ threads: [thread('late', 'g2')], next_cursor: '' })
      await more
      expect(ws.pageOf('g2').threads.map((th) => th.id)).toEqual(['fresh'])
    })

    it('loadMore 失败只写 page.error，不影响已有列表', async () => {
      const ws = useAgentWorkspace({ projectId: () => 'p1' })
      ws.setPage('g1', { threads: [thread('a', 'g1')], next_cursor: 'c1' })
      api.agentThreads.pageBy.mockRejectedValueOnce(new Error('page-error'))
      await ws.loadMore('g1')
      expect(ws.pageOf('g1').error).toBe('page-error')
      expect(ws.pageOf('g1').threads).toHaveLength(1)
    })

    it('refreshFirstPage：合并而不覆盖本地新建的会话；失败静默；切项目后丢弃', async () => {
      let project = 'p1'
      const ws = useAgentWorkspace({ projectId: () => project })
      ws.setPage('g1', { threads: [thread('local', 'g1'), thread('a', 'g1')], next_cursor: '' })
      api.agentThreads.pageBy.mockResolvedValueOnce({ threads: [thread('a', 'g1'), thread('b', 'g1')], next_cursor: 'c' })
      await ws.refreshFirstPage('g1', 50)
      expect(ws.pageOf('g1').threads.map((th) => th.id)).toEqual(['a', 'b', 'local'])
      expect(ws.pageOf('g1').cursor).toBe('c')

      api.agentThreads.pageBy.mockRejectedValueOnce(new Error('offline'))
      await expect(ws.refreshFirstPage('g1', 50)).resolves.toBeUndefined()
      expect(ws.pageOf('g1').threads).toHaveLength(3)

      const d = deferred<{ threads: AgentThread[]; next_cursor: string }>()
      api.agentThreads.pageBy.mockReturnValueOnce(d.promise)
      const refreshing = ws.refreshFirstPage('g1', 50)
      project = 'p2'
      d.resolve({ threads: [thread('stale', 'g1')], next_cursor: '' })
      await refreshing
      expect(ws.pageOf('g1').threads.map((th) => th.id)).not.toContain('stale')
    })
  })
})
