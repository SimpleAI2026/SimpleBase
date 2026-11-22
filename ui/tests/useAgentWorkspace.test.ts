// planv6.7 v2 §4：工作台取数（每群独立分页、展开懒加载、旧响应不覆盖）。
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
})
