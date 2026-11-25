// 会话导航：跨群先切分区再选会话；取数期间切群时旧响应不落地；新建 / 删除会话的竞态与收尾。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, ref } from 'vue'
import type { Ref } from 'vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { SECTION_PAGE_SIZE, useThreadNavigation } from '@/composables/useThreadNavigation'
import { useAgentCollaboration } from '@/composables/useAgentCollaboration'
import type { UseThreadNavigationOptions } from '@/composables/useThreadNavigation'
import type { AgentTag, AgentThread, ChatMsg } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function tag(id: string): AgentTag {
  return { id, name: id, agentIds: [], prompts: [], unavailableAgentIds: [], routingPrompt: '', threadKinds: ['chat'], version: 1 }
}

function thread(id: string, tagId?: string): AgentThread {
  return { id, title: id, tag_id: tagId, created_at: 't', updated_at: 't' }
}

function setup(sectionId = 'g1') {
  const calls: string[] = []
  const projectId = ref('p1')
  const section = ref(sectionId)
  const threadId = ref('')
  const chatMessages: Ref<ChatMsg[]> = ref([])
  /** 当前分区的会话列表：随 removeThread / unshiftThread 变化，和真实 workspace 一样。 */
  const list = ref<AgentThread[]>([thread('t1', sectionId), thread('t2', sectionId)])
  const sectionThreads = computed(() => list.value)
  const ws = {
    loadFirstPage: vi.fn(async (sec: string, _size: number, isCurrent: () => boolean) => {
      calls.push(`first:${sec}`)
      return isCurrent() ? [thread('t1', sec)] : null
    }),
    loadMore: vi.fn(async () => undefined),
    pageOf: vi.fn(() => ({ error: '' })),
    unshiftThread: vi.fn((sec: string, th: AgentThread) => {
      calls.push(`unshift:${sec}:${th.id}`)
      list.value = [th, ...list.value]
    }),
    replaceThread: vi.fn(),
    removeThread: vi.fn((sec: string, id: string) => {
      calls.push(`remove:${sec}:${id}`)
      list.value = list.value.filter((th) => th.id !== id)
    }),
    refreshFirstPage: vi.fn()
  }
  const conv = {
    clear: vi.fn(() => calls.push('detach')),
    attach: vi.fn()
  }
  const load = {
    run: vi.fn(async (task: () => Promise<boolean>) => task()),
    settle: vi.fn()
  }
  const opts: UseThreadNavigationOptions = {
    projectId: () => projectId.value,
    tags: ref([tag('g1'), tag('g2')]),
    section,
    threadId,
    chatMessages,
    sectionThreads,
    sectionCursor: computed(() => ''),
    sending: computed(() => false),
    ws: ws as unknown as UseThreadNavigationOptions['ws'],
    conv: conv as unknown as UseThreadNavigationOptions['conv'],
    load,
    queryThread: () => '',
    replaceQuery: (patch) => calls.push(`query:${JSON.stringify(patch)}`),
    moduleOf: () => undefined,
    stop: () => undefined
  }
  const nav = useThreadNavigation(opts)
  return { nav, calls, projectId, section, threadId, chatMessages, ws, conv, load, opts }
}

describe('useThreadNavigation', () => {
  beforeEach(() => {
    resetApiMocks()
    api.agentThreads.messages.mockResolvedValue([])
    api.agentThreads.runs.mockResolvedValue([])
  })

  it('跨群选会话：先断开当前 run、切分区并拉首屏，再选中会话', async () => {
    const { nav, calls, section, threadId } = setup('general')
    api.agentThreads.messages.mockImplementation(async (_p: string, id: string) => {
      calls.push(`messages:${id}`)
      return []
    })
    await nav.selectThreadInGroup('t1', 'g1')

    expect(section.value).toBe('g1')
    expect(threadId.value).toBe('t1')
    expect(calls[0]).toBe('detach')
    // 首屏在消息之前；首屏里选中 t1 与随后的 selectThread 各加载一次消息、各写一次地址栏。
    expect(calls).toEqual([
      'detach', 'query:{"tag":"g1"}', 'first:g1', 'query:{"thread":"t1"}', 'messages:t1',
      'detach', 'query:{"thread":"t1"}', 'messages:t1'
    ])
  })

  it('取数期间切群：workspace 守卫丢弃旧响应（返回 null）时不选中会话、不改地址栏里的 thread', async () => {
    const { nav, calls, section, threadId, ws } = setup('general')
    let loaded = 0
    api.agentThreads.messages.mockImplementation(async () => {
      loaded += 1
      return []
    })
    ws.loadFirstPage.mockResolvedValue(null)
    await nav.selectSection('g1')

    expect(ws.loadFirstPage).toHaveBeenCalledWith('g1', SECTION_PAGE_SIZE, expect.any(Function))
    expect(section.value).toBe('g1')
    expect(threadId.value).toBe('')
    expect(loaded).toBe(0)
    expect(calls.some((c) => c.startsWith('query:') && c.includes('"thread":"'))).toBe(false)
  })

  it('新建会话期间项目切换：旧响应不插入列表、不改当前选中', async () => {
    const { nav, calls, projectId, threadId, ws } = setup('g1')
    api.agentThreads.create.mockImplementation(async () => {
      projectId.value = 'p2'
      return thread('new', 'g1')
    })
    await nav.resetThread()

    expect(api.agentThreads.create).toHaveBeenCalledWith('p1', expect.objectContaining({ tagId: 'g1', source: 'workspace' }))
    expect(ws.unshiftThread).not.toHaveBeenCalled()
    expect(threadId.value).toBe('')
    expect(calls.some((c) => c.startsWith('unshift:'))).toBe(false)
  })

  it('新建会话：插到当前分区顶部并选中', async () => {
    const { nav, calls, threadId, ws, load } = setup('g1')
    api.agentThreads.create.mockResolvedValue(thread('new', 'g1'))
    await nav.resetThread()

    expect(ws.unshiftThread).toHaveBeenCalledWith('g1', expect.objectContaining({ id: 'new' }))
    expect(threadId.value).toBe('new')
    expect(load.settle).toHaveBeenCalledWith(false)
    expect(calls).toContain('query:{"thread":"new"}')
  })

  it('删除当前会话：先停 run，再切到分区首条', async () => {
    const { nav, threadId, conv, ws, calls } = setup('g1')
    threadId.value = 't1'
    api.agentThreads.remove.mockImplementation(async () => {
      calls.push('api-remove')
    })
    api.agentThreads.messages.mockImplementation(async (_p: string, id: string) => {
      calls.push(`messages:${id}`)
      return []
    })
    await nav.removeThread('t1')

    expect(api.agentThreads.remove).toHaveBeenCalledWith('p1', 't1')
    expect(ws.removeThread).toHaveBeenCalledWith('g1', 't1')
    expect(calls).toEqual(['api-remove', 'remove:g1:t1', 'detach', 'query:{"thread":"t2"}', 'messages:t2'])
    expect(conv.clear).toHaveBeenCalledTimes(1)
    expect(threadId.value).toBe('t2')
  })

  it('选中不存在的会话：保持原选中，不加载消息', async () => {
    const { nav, threadId } = setup('g1')
    threadId.value = 't2'
    api.agentThreads.get.mockResolvedValue(undefined)
    let loaded = 0
    api.agentThreads.messages.mockImplementation(async () => {
      loaded += 1
      return []
    })
    await nav.selectThread('missing')

    expect(threadId.value).toBe('t2')
    expect(loaded).toBe(0)
  })

  it('首屏按 SECTION_PAGE_SIZE 取数；没有游标时不请求下一页', async () => {
    const { nav, ws, section } = setup('g1')
    await nav.loadSectionThreads()
    expect(ws.loadFirstPage).toHaveBeenCalledWith('g1', SECTION_PAGE_SIZE, expect.any(Function))

    await nav.loadMoreSectionThreads()
    expect(ws.loadMore).not.toHaveBeenCalled()
    expect(section.value).toBe('g1')
  })

  it('回到协作会话按最新轮次续接，切换时只断开订阅', async () => {
    const { nav, opts, threadId, chatMessages } = setup('g1')
    threadId.value = 't1'
    const collab = useAgentCollaboration({ projectId: () => 'p1', messages: () => chatMessages.value })
    opts.collab = collab
    opts.isCollaborative = () => true
    const detach = vi.spyOn(collab, 'detach')
    const turn = {
      id: 'turn-1', threadId: 't1', status: 'running' as const, version: 1,
      createdAt: 't', updatedAt: 't', tasks: [], lastEventSeq: 4
    }
    api.agentThreads.turns.mockResolvedValue([turn])
    api.agentTurns.get.mockResolvedValue(turn)
    api.agentThreads.attachTurn.mockReturnValue({ close: vi.fn() })
    await nav.loadThreadMessages('t1')
    expect(api.agentThreads.turns).toHaveBeenCalledWith('p1', 't1', 1)
    await vi.waitFor(() => expect(api.agentTurns.get).toHaveBeenCalledWith('p1', 'turn-1'))
    await vi.waitFor(() => expect(api.agentThreads.attachTurn).toHaveBeenCalledWith('p1', 'turn-1', 4, expect.any(Object)))
    nav.detachRun()
    expect(detach).toHaveBeenCalled()
  })
})
