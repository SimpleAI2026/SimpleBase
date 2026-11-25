// 首页「一句话建应用」的搭建会话：深链只触发一次；创建期间项目切换不发送；无效入口只清 query。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { Ref } from 'vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { useStarterThread } from '@/composables/useStarterThread'
import type { AgentThread, ChatMsg } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function thread(id: string): AgentThread {
  return { id, title: id, tag_id: 'general', created_at: 't', updated_at: 't' }
}

function setup(starterQuery: () => unknown = () => 'dashboard') {
  const calls: string[] = []
  const projectId = ref('p1')
  const section = ref('alerts')
  const threadId = ref('old')
  const chatMessages: Ref<ChatMsg[]> = ref([{ role: 'user', content: '旧消息' }])
  const addThread = vi.fn((th: AgentThread) => calls.push(`add:${th.id}`))
  const startTurn = vi.fn((id: string, _content: string) => calls.push(`turn:${id}`))
  const starter = useStarterThread({
    projectId: () => projectId.value,
    starterQuery,
    section,
    threadId,
    chatMessages,
    addThread,
    replaceQuery: (patch) => calls.push(`query:${JSON.stringify(patch)}`),
    settleThread: (hasData) => calls.push(`settle:${hasData}`),
    startTurn
  })
  return { ...starter, calls, projectId, section, threadId, chatMessages, addThread, startTurn }
}

describe('useStarterThread', () => {
  beforeEach(() => {
    resetApiMocks()
    sessionStorage.clear()
    api.agentThreads.create.mockResolvedValue(thread('s1'))
    api.s3.getSite.mockResolvedValue({ baseUrl: 'https://p.example.com', publishable: true })
  })

  it('首次进入：切到 #通用、新建协作会话并自动发送一次种子提示词；再次进入不重复发送', async () => {
    const s = setup()
    await s.maybeStartStarter()

    expect(api.agentThreads.create).toHaveBeenCalledTimes(1)
    expect(api.agentThreads.create).toHaveBeenCalledWith('p1', expect.objectContaining({
      tagId: 'general', dispatchMode: 'collaborative', threadKind: 'chat', source: 'starter'
    }))
    expect(s.section.value).toBe('general')
    expect(s.threadId.value).toBe('s1')
    expect(s.chatMessages.value).toEqual([])
    expect(s.calls).toContain('add:s1')
    expect(s.calls).toContain('settle:false')
    expect(s.calls).toContain('query:{"thread":"s1"}')
    // 发送前先消费 query：刷新或再次进入都不会再发。
    expect(s.calls.indexOf('query:{}')).toBeLessThan(s.calls.indexOf('add:s1'))
    expect(s.startTurn).toHaveBeenCalledTimes(1)
    const [id, seed] = s.startTurn.mock.calls[0]
    expect(id).toBe('s1')
    expect(seed).toContain('项目 ID=p1')
    expect(seed).toContain('站点基址=https://p.example.com')
    expect(seed).toContain('站点可发布=true')

    await s.maybeStartStarter()
    expect(api.agentThreads.create).toHaveBeenCalledTimes(1)
    expect(s.startTurn).toHaveBeenCalledTimes(1)
  })

  it('创建会话期间项目切换：不插入会话、不发送', async () => {
    const s = setup()
    api.agentThreads.create.mockImplementation(async () => {
      s.projectId.value = 'p2'
      return thread('late')
    })
    await s.maybeStartStarter()

    expect(s.addThread).not.toHaveBeenCalled()
    expect(s.startTurn).not.toHaveBeenCalled()
    expect(s.threadId.value).toBe('')
  })

  it('取站点信息期间切走了会话：不再发送', async () => {
    const s = setup()
    api.s3.getSite.mockImplementation(async () => {
      s.threadId.value = 'somewhere-else'
      return { publishable: true }
    })
    await s.maybeStartStarter()

    expect(s.addThread).toHaveBeenCalledWith(expect.objectContaining({ id: 's1' }))
    expect(s.startTurn).not.toHaveBeenCalled()
  })

  it('站点不可发布时，种子提示词写明原因', async () => {
    const s = setup()
    api.s3.getSite.mockResolvedValue({ publishable: false, reason: 'site_domain_unconfigured' })
    await s.maybeStartStarter()

    expect(s.starterSite.value).toEqual({ publishable: false, reason: 'site_domain_unconfigured' })
    expect(s.startTurn.mock.calls[0][1]).toContain('站点可发布=false (site_domain_unconfigured)')
    expect(s.buildStarterSeed('dashboard')).toContain('站点基址=-')
  })

  it('模板不存在或没有项目：只清掉 query，不建会话', async () => {
    const unknown = setup(() => 'no-such-template')
    await unknown.maybeStartStarter()
    expect(unknown.calls).toEqual(['query:{}'])
    expect(api.agentThreads.create).not.toHaveBeenCalled()

    const noProject = setup()
    noProject.projectId.value = ''
    noProject.calls.length = 0
    await noProject.maybeStartStarter()
    expect(noProject.calls).toEqual(['query:{}'])
    expect(api.agentThreads.create).not.toHaveBeenCalled()
  })

  it('没有 starter query 时什么都不做', async () => {
    const s = setup(() => undefined)
    await s.maybeStartStarter()
    expect(s.calls).toEqual([])
    expect(api.agentThreads.create).not.toHaveBeenCalled()
  })
})
