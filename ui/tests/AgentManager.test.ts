import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { setI18nLocale } from '@/locales'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp, sampleAgent } from '@/test/helpers'
import AgentManager from '@/pages/AgentManager.vue'
import type { AgentStreamHandlers } from '@/services/types'

const generalTag = {
  id: 'general', builtinKey: 'general', name: '通用', agentIds: ['ag-1', 'general', 'database', 'kv', 's3'], prompts: [],
  unavailableAgentIds: [], routingPrompt: '', threadKinds: ['chat', 'alert', 'statistics', 'scheduled'], version: 0
}

function routeOneAgent(handlers: AgentStreamHandlers, agentId = 'ag-1') {
  handlers.onRoute?.({ status: 'done', steps: [{ taskId: 'task-1', kind: 'agent', agentId, instruction: 'test' }] })
  handlers.onFrameContext?.({ runId: 'run-1', agentId })
}

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

const interactionAgentStubs = {
  AgentComposer: {
    props: ['sending', 'mentionAgents', 'modelValue'],
    emits: ['send', 'stop', 'update:modelValue'],
    template:
      '<div class="ai"><button type="button" class="ai-stop" @click="$emit(\'stop\')">stop</button><slot /></div>'
  },
  ConversationView: {
    props: ['messages', 'sending'],
    template: '<div class="conv" />'
  },
  AgentWorkspaceSidebar: {
    props: ['tags', 'groupPages', 'section', 'threadId', 'runningThreadIds', 'pendingByGroup', 'pending', 'error'],
    emits: ['select-section', 'select-thread', 'create-tag', 'create-thread', 'load-more', 'rename-thread', 'remove-thread', 'edit-tag', 'archive-tag', 'add-members', 'create-member-agent', 'reload'],
    template: `
      <div class="ws">
        <button type="button" class="ws-create-thread" @click="$emit('create-thread', section)">新会话</button>
        <button type="button" class="ws-select-thread" @click="$emit('select-thread', firstThread, section)">pick</button>
        <button type="button" class="ws-create-tag" @click="$emit('create-tag')">新建标签</button>
      </div>
    `,
    computed: {
      firstThread(): string {
        const pages = (this as unknown as { groupPages: Record<string, { threads: { id: string }[] }>; section: string }).groupPages
        const page = pages ? pages[(this as unknown as { section: string }).section] : undefined
        return page && page.threads[0] ? page.threads[0].id : ''
      }
    }
  }
}


const agentStubs = {
  AgentComposer: {
    props: ['sending', 'mentionAgents', 'modelValue'],
    emits: ['send', 'stop', 'update:modelValue'],
    template: `
      <div class="ai-chat">
        <button type="button" class="ai-send" @click="$emit('send', 'hello', mentionAgents && mentionAgents[0] ? [{ agent_id: mentionAgents[0].id }] : [])">send</button>
        <button type="button" class="ai-blank" @click="$emit('send', '   ', [])">blank</button>
        <button type="button" class="ai-stop" @click="$emit('stop')">stop</button>
      </div>
    `
  },
  ConversationView: {
    props: ['messages', 'sending', 'canRetry'],
    emits: ['retry'],
    template: '<div class="conv" />'
  },
  AgentWorkspaceSidebar: {
    props: ['tags', 'groupPages', 'section', 'threadId', 'runningThreadIds', 'pendingByGroup', 'pending', 'error'],
    emits: ['select-section', 'select-thread', 'create-tag', 'create-thread', 'load-more', 'rename-thread', 'remove-thread', 'edit-tag', 'archive-tag', 'add-members', 'create-member-agent', 'reload'],
    template: `
      <div class="ws" :data-running-threads="runningThreadIds ? [...runningThreadIds].join(',') : ''" :data-section="section">
        <button type="button" class="ws-create-thread" @click="$emit('create-thread', section)">新会话</button>
        <button type="button" class="ws-select-thread" @click="$emit('select-thread', firstThread, section)">pick</button>
      </div>
    `,
    computed: {
      firstThread(): string {
        const self = this as unknown as {
          groupPages: Record<string, { threads: { id: string }[] }>
          section: string
        }
        const page = self.groupPages ? self.groupPages[self.section] : undefined
        return page && page.threads[0] ? page.threads[0].id : ''
      }
    }
  }
}


/**
 * 只桩输入区，保留真实的 ConversationView / 欢迎页：
 * 这两条用例验证的正是「消息流是否真的渲染出来」。
 */
const composerOnlyStubs = {
  AgentComposer: {
    props: ['sending', 'mentionAgents', 'modelValue'],
    emits: ['send', 'stop', 'update:modelValue'],
    template: '<div class="ai"><button type="button" class="ai-send" @click="$emit(\'send\', \'hi\', [], [])">send</button></div>',
    methods: { focus: () => undefined }
  }
}

describe('AgentManager (云助手)', () => {
  beforeEach(() => {
    setI18nLocale('zh-CN')
    resetApiMocks()
    api.agents.list.mockResolvedValue([sampleAgent])
    api.agents.modules.mockResolvedValue([
      { id: 'database', name: 'Database', description: 'd', default_tools: ['list_databases'], team_supported: false },
      { id: 's3', name: 'S3', description: 's', default_tools: ['list_objects'], team_supported: false }
    ])
    api.agentThreads.list.mockResolvedValue([{ id: 'th-1', title: 'T', created_at: 't', updated_at: 't' }])
    api.agentThreads.messages.mockResolvedValue([])
    // 分区制取数走 pageBy，默认分区为 #通用。
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [{ id: 'th-1', title: 'T', agent_id: 'ag-1', tag_id: 'general', dispatch_mode: 'collaborative', thread_kind: 'chat', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    api.agentTags.list.mockResolvedValue([generalTag])
    // planv6.6 DC-5：定时摘要改来自 cronjobs（targetKind=agent）。
    api.cronjobs.list.mockResolvedValue([
      {
        id: 'sch-1',
        name: 'sch-1',
        description: '',
        targetKind: 'agent',
        agentId: 'ag-1',
        agentPrompt: 'p',
        scheduleKind: 'cron',
        cronExpr: '0 * * * *',
        funcFile: '',
        funcExport: '',
        inputJson: '{}',
        enabled: true,
        lastStatus: '',
        lastError: '',
        runCount: 0,
        targetMissing: false,
        createdAt: 't',
        updatedAt: 't'
      }
    ])
  })

  it('marks threads with active runs (incl. scheduled) and stops polling when idle', async () => {
    vi.useFakeTimers()
    try {
      api.agentThreads.active.mockResolvedValue([{ run_id: 'r-cron', agent_id: 'ag-1', thread_id: 'th-1' }])
      const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
      await flushPromises()
      expect(api.agentThreads.active).toHaveBeenCalledWith('testprj1')
      expect(wrapper.get('.ws').attributes('data-running-threads')).toBe('th-1')

      // 运行结束：下一轮轮询拿到空列表，转圈消失，之后不再轮询。
      api.agentThreads.active.mockResolvedValue([])
      await vi.advanceTimersByTimeAsync(3000)
      await flushPromises()
      expect(wrapper.get('.ws').attributes('data-running-threads')).toBe('')
      const calls = api.agentThreads.active.mock.calls.length
      await vi.advanceTimersByTimeAsync(10000)
      expect(api.agentThreads.active.mock.calls.length).toBe(calls)
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('loads workspace sections and selects the first thread', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    expect(api.agentThreads.pageBy).toHaveBeenCalledWith('testprj1', expect.objectContaining({ tagId: 'general' }))
    const vm = wrapper.vm as unknown as { section: string; threadId: string }
    expect(vm.section).toBe('general')
    expect(vm.threadId).toBe('th-1')
    // 会话绑定助手的徽标在头部可见（planv6.7 AGC-NAV）。
    expect(wrapper.find('[data-thread-agent-badge]').exists()).toBe(true)
  })

  it('keeps the composer outside the scrollable message viewport', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: composerOnlyStubs })
    const viewport = wrapper.get('div.overflow-y-auto').element
    const region = viewport.closest('[data-slot="async-region"]')
    expect(region).not.toBeNull()
    expect(Array.from(region!.classList)).toEqual(expect.arrayContaining(['flex', 'flex-1', 'flex-col', 'min-h-0']))
    expect(wrapper.get('.ai').element.parentElement).toBe(region!.parentElement)
    wrapper.unmount()
  })

  it('renders the first message of an empty thread without refreshing', async () => {
    // planv6.0 §0.1 第 3 条回归：此前空会话的 SbAsyncRegion 停在 showEmpty，
    // ConversationView 从未挂载，发完第一条消息也看不到任何内容。
    api.agentThreads.messages.mockResolvedValue([])
    api.agentThreads.streamTurn.mockImplementation((_p: string, _t: string, _r: unknown, h: AgentStreamHandlers) => {
      routeOneAgent(h)
      h.onRun?.('run-1')
      h.onToken?.('第一条回复')
      h.onEnd?.()
      return { close: vi.fn() }
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: composerOnlyStubs })
    // 空会话显示欢迎页
    expect(wrapper.text()).toContain('你好，我是')
    await wrapper.get('.ai-send').trigger('click')
    await flushPromises()
    // 消息立即可见，无需刷新或切换会话
    expect(wrapper.text()).toContain('第一条回复')
    expect(wrapper.text()).not.toContain('你好，我是')
    wrapper.unmount()
  })

  // planv6.0 前端简化 C3：锁定加载会话时各消息字段的映射（role/工具卡/错误/取消态）。
  it('maps persisted thread messages into chat bubbles with restored states', async () => {
    api.agents.list.mockResolvedValue([{ ...sampleAgent, id: 'ag-x', module: 's3' }])
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [{ id: 'th-1', title: 'T', agent_id: 'ag-x', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    api.agentThreads.messages.mockResolvedValue([
      { id: 'm1', role: 'user', content: 'u', created_at: 't' },
      {
        id: 'm2', role: 'assistant', content: 'a', agent_id: 'ag-x',
        tool_calls: [{ name: 't', status: 'success' }], created_at: 't',
        run_status: 'canceled', error_code: 'E_TIMEOUT'
      },
      { id: 'm3', role: 'system', content: 's', created_at: 't' }
    ])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: composerOnlyStubs })
    await flushPromises()
    const vm = wrapper.vm as unknown as { chatMessages: Array<Record<string, unknown>> }
    expect(vm.chatMessages).toEqual([
      { role: 'user', content: 'u', module: undefined, agentId: undefined, routeKind: undefined, toolCalls: undefined, error: undefined, canceled: false },
      {
        role: 'assistant', content: 'a', module: 's3', agentId: 'ag-x', routeKind: undefined,
        toolCalls: [{ name: 't', status: 'success' }],
        // 现状：传入的是普通对象而非 Error，errorMessage 走 fallback 文案（按现状锁定）。
        error: '运行失败', canceled: true
      },
      { role: 'user', content: 's', module: undefined, agentId: undefined, routeKind: undefined, toolCalls: undefined, error: undefined, canceled: false }
    ])
    wrapper.unmount()
  })

  it('discards a stale response when the thread changed mid-flight', async () => {
    api.agentThreads.messages.mockImplementation(async (_p: string, id: string) => {
      if (id === 'slow') {
        await new Promise((r) => setTimeout(r, 20))
        return [{ id: 'm', role: 'user', content: 'old', created_at: 't' }]
      }
      return []
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: composerOnlyStubs })
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      chatMessages: Array<Record<string, unknown>>
      loadThreadMessages: (id: string) => Promise<boolean>
      threadId: string
    }
    vm.threadId = 'slow'
    const stale = vm.loadThreadMessages('slow')
    vm.threadId = 'other'
    expect(await stale).toBe(false)
    expect(vm.chatMessages).toEqual([])
    wrapper.unmount()
  })

  it('fills the composer from a welcome prompt without sending', async () => {
    // planv6.0 §3.5：点提示词只写入输入框并聚焦，回车才发送。
    api.agentThreads.messages.mockResolvedValue([])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: composerOnlyStubs })
    const prompt = wrapper.findAll('button[data-prompt-index]')
    expect(prompt.length).toBeGreaterThan(0)
    const text = prompt[0].text()
    await prompt[0].trigger('click')
    await flushPromises()
    expect((wrapper.vm as unknown as { draft: string }).draft).toBe(text)
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps composer focus passthrough after removing header actions', async () => {
    // planv6.7 v2：头部「⋯」菜单已下线（会话操作全部收进左栏会话菜单）；
    // 这里只保留输入区的 focus / draft 透传回归。
    const { wrapper } = await mountWithApp(AgentManager, { stubs: composerOnlyStubs })
    const vm = wrapper.vm as unknown as { pickPrompt: (t: string) => void; draft: string }
    vm.pickPrompt('填进去')
    await flushPromises()
    expect(vm.draft).toBe('填进去')
    wrapper.unmount()
  })

  it('reloadThread reloads the current agent threads without a thread', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as { reloadThread: () => void; threadId: string }
    vm.reloadThread()
    await flushPromises()
    vm.threadId = ''
    vm.reloadThread()
    await flushPromises()
    wrapper.unmount()
  })

  it('resets the workspace section and threads when switching to a new project', async () => {
    const oldAgent = { ...sampleAgent, id: 'ag-old', builtin_key: 'general' }
    const newAgent = { ...sampleAgent, id: 'ag-new', builtin_key: 'general' }
    api.agents.list.mockImplementation((projectId: string) =>
      Promise.resolve([projectId === 'testprj1' ? oldAgent : newAgent])
    )
    api.agentThreads.pageBy.mockImplementation((_projectId: string, opts?: { tagId?: string }) =>
      Promise.resolve({
        threads: [{ id: `th-${opts?.tagId || 'general'}`, title: 'T', agent_id: 'ag-new', created_at: 't', updated_at: 't' }],
        next_cursor: ''
      })
    )
    const { wrapper, pinia } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as { section: string; tags: { id: string }[] }
    expect(vm.section).toBe('general')

    useProjectStore(pinia).setProject('newproj1', 'New')
    await flushPromises()

    // 分区制：切项目回到 #通用，标签与线程清空重载。
    expect(vm.section).toBe('general')
    expect(api.agentThreads.pageBy).toHaveBeenLastCalledWith('newproj1', expect.objectContaining({ tagId: 'general' }))
    wrapper.unmount()
  })

  it('loads an empty workspace when the new project has no agents', async () => {
    api.agents.list.mockImplementation((projectId: string) =>
      Promise.resolve(projectId === 'testprj1' ? [{ ...sampleAgent, id: 'ag-old' }] : [])
    )
    api.agentThreads.pageBy.mockImplementation((projectId: string) =>
      Promise.resolve(projectId === 'testprj1'
        ? { threads: [{ id: 'th-1', title: 'T', agent_id: 'ag-old', created_at: 't', updated_at: 't' }], next_cursor: '' }
        : { threads: [], next_cursor: '' })
    )
    const { wrapper, pinia } = await mountWithApp(AgentManager, { stubs: agentStubs })
    useProjectStore(pinia).setProject('newproj1', 'New')
    await flushPromises()

    const vm = wrapper.vm as unknown as { agents: unknown[]; threadId: string; sectionThreads: unknown[] }
    expect(vm.agents).toEqual([])
    expect(vm.threadId).toBe('')
    expect(vm.sectionThreads).toEqual([])
    wrapper.unmount()
  })

  it('ignores an old project agent list that resolves after switching', async () => {
    let resolveOld!: (agents: Array<typeof sampleAgent>) => void
    const oldAgent = { ...sampleAgent, id: 'ag-old', builtin_key: 'general' }
    const newAgent = { ...sampleAgent, id: 'ag-new', builtin_key: 'general' }
    api.agents.list.mockImplementation((projectId: string) => {
      if (projectId === 'testprj1') {
        return new Promise<Array<typeof sampleAgent>>((resolve) => { resolveOld = resolve })
      }
      return Promise.resolve([newAgent])
    })
    const { wrapper, pinia } = await mountWithApp(AgentManager, { stubs: agentStubs })
    useProjectStore(pinia).setProject('newproj1', 'New')
    await flushPromises()
    resolveOld([oldAgent])
    await flushPromises()

    const vm = wrapper.vm as unknown as { agents: { id: string }[] }
    expect(vm.agents.map((agent) => agent.id)).toEqual(['ag-new'])
    wrapper.unmount()
  })

  it('keeps a separate thread list per section and reloads list after runs', async () => {
    // planv6.7 AGC-NAV：会话归属分区；切换分区换列表，run 结束刷新当前分区。
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'ag-1', name: 'Database', builtin_key: 'general' },
      { ...sampleAgent, id: 'ag-2', name: 'Logs', module: 'logs' }
    ])
    api.agentTags.list.mockResolvedValue([
      { id: 'tag-1', name: '报表', agentIds: ['ag-2'], prompts: [], version: 1 }
    ])
    api.agentThreads.pageBy.mockImplementation((_p: string, opts?: { tagId?: string }) =>
      Promise.resolve(
        opts?.tagId === 'tag-1'
          ? { threads: [{ id: 'th-2', title: '日志会话', agent_id: 'ag-2', tag_id: 'tag-1', created_at: 't', updated_at: 't' }], next_cursor: '' }
          : { threads: [{ id: 'th-1', title: '库会话', agent_id: 'ag-1', created_at: 't', updated_at: 't' }], next_cursor: '' }
      )
    )
    api.agentThreads.messages.mockResolvedValue([])

    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as {
      selectSection: (id: string) => Promise<void>
      section: string
      threadId: string
      sectionThreads: { id: string }[]
    }
    // 初始在 #通用
    expect(vm.section).toBe('general')
    expect(vm.threadId).toBe('th-1')

    await vm.selectSection('tag-1')
    await flushPromises()
    // 切到标签：会话换成该标签的
    expect(vm.section).toBe('tag-1')
    expect(vm.threadId).toBe('th-2')
    expect(vm.sectionThreads.map((t) => t.id)).toEqual(['th-2'])
    expect(api.agentThreads.pageBy).toHaveBeenLastCalledWith('testprj1', expect.objectContaining({ tagId: 'tag-1' }))
    // 重复选同分区不重复请求
    const calls = (api.agentThreads.pageBy as unknown as { mock: { calls: unknown[] } }).mock.calls.length
    await vm.selectSection('tag-1')
    expect((api.agentThreads.pageBy as unknown as { mock: { calls: unknown[] } }).mock.calls.length).toBe(calls)
    wrapper.unmount()
  })

  it('shows built-in names in both locales without changing their stored names or ids', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' },
      { ...sampleAgent, id: 's3', name: 'S3', module: 's3', builtin_key: 's3' },
      { ...sampleAgent, id: 'logs', name: 'Logs', module: 'logs', builtin_key: 'logs' },
      { ...sampleAgent, id: 'kv', name: '键值助手', module: 'kv', builtin_key: 'kv' },
      { ...sampleAgent, id: 'custom', name: '我的报表', module: 'database', builtin_key: '' }
    ])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as { agentNamesById: Record<string, string> }
    expect(vm.agentNamesById.database).toBe('数据库助手')
    expect(vm.agentNamesById.s3).toBe('对象存储助手')
    expect(vm.agentNamesById.logs).toBe('日志助手')
    expect(vm.agentNamesById.custom).toBe('我的报表')
    setI18nLocale('en-US')
    await flushPromises()
    const vmEn = wrapper.vm as unknown as { agentNamesById: Record<string, string> }
    expect(vmEn.agentNamesById.general).toBe('General Assistant')
    expect(vmEn.agentNamesById.database).toBe('Database Assistant')
    expect(vmEn.agentNamesById.s3).toBe('Object Storage Assistant')
    expect(vmEn.agentNamesById.logs).toBe('Logs Assistant')
    expect(vmEn.agentNamesById.kv).toBe('Key-Value Assistant')
    expect(vmEn.agentNamesById.custom).toBe('我的报表')
    expect(api.agents.patch).not.toHaveBeenCalled()
    expect((wrapper.vm as unknown as { agents: { id: string; name: string }[] }).agents[1]).toEqual(expect.objectContaining({ id: 'database', name: 'Database' }))
    wrapper.unmount()
    setI18nLocale('zh-CN')
  })

  it('sends @数据库 from a general thread without treating it as an agent or a skill', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' }
    ])
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, AgentWorkspaceSidebar: agentStubs.AgentWorkspaceSidebar }
    })
    await wrapper.get('textarea').setValue('@数据库 查询统计表')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(api.agentThreads.pageBy).toHaveBeenCalledWith('testprj1', expect.objectContaining({ tagId: 'general' }))
    // @ 只点名助手；技能由后端按助手 module 决定，请求体不再携带 skills。
    // planv6.7 AGC-ROUTE：固定助手会话点名别的助手 → 按 mention 路由直接发送。
    expect(api.agentThreads.streamRun).toHaveBeenCalledWith('testprj1', 'general-thread', {
      content: '@数据库 查询统计表',
      mentions: [{ agent_id: 'general' }],
      module: 'general',
      stream: true
    }, expect.any(Object))
    wrapper.unmount()
  })

  it('routes to the mentioned agent directly instead of asking to switch', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' }
    ])
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, AgentWorkspaceSidebar: agentStubs.AgentWorkspaceSidebar }
    })
    await wrapper.get('textarea').setValue('@数据库助手 查询统计表')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    // planv6.7 AGC-ROUTE：不再弹「切换助手」条，点名即按 mention 发送。
    expect(api.agentThreads.streamRun).toHaveBeenCalled()
    expect(wrapper.find('[data-agent-switch]').exists()).toBe(false)
    expect((wrapper.vm as unknown as { draft: string }).draft).toBe('')
    wrapper.unmount()
  })

  it('keeps legacy @Database mentions as agent targets after translating the display name', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' }
    ])
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, AgentWorkspaceSidebar: agentStubs.AgentWorkspaceSidebar }
    })
    await wrapper.get('textarea').setValue('@Database 查询统计表')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    // 旧名 @Database 仍解析到数据库助手（mention 匹配兼容 legacy 名）。
    expect(api.agentThreads.streamRun).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps the routed mention after changing locale with an existing mention draft', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' }
    ])
    api.agentThreads.pageBy.mockResolvedValue({ threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }], next_cursor: '' })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, AgentWorkspaceSidebar: agentStubs.AgentWorkspaceSidebar }
    })
    await wrapper.get('textarea').setValue('@数据库助手 查询统计表')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(api.agentThreads.streamRun).toHaveBeenCalled()
    setI18nLocale('en-US')
    await flushPromises()
    expect(api.agentThreads.streamRun).toHaveBeenCalledTimes(1)
    expect((wrapper.vm as unknown as { draft: string }).draft).toBe('')
    wrapper.unmount()
    setI18nLocale('zh-CN')
  })

  it('routes @KV助手 to the kv built-in agent without requiring a custom agent named that', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'kv', name: '键值助手', module: 'kv', builtin_key: 'kv', tool_ids: [] }
    ])
    api.agentThreads.pageBy.mockResolvedValue({ threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }], next_cursor: '' })
    api.agents.modules.mockResolvedValue([{ id: 'kv', name: 'KV', description: '', default_tools: [], team_supported: false }])
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, AgentWorkspaceSidebar: agentStubs.AgentWorkspaceSidebar }
    })
    await wrapper.get('textarea').setValue('@KV助手 查询缓存')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    // planv6.7 AGC-ROUTE：@KV助手 直接按 mention 路由发送到键值助手。
    expect(api.agentThreads.streamRun).toHaveBeenCalledWith('testprj1', 'general-thread', expect.objectContaining({
      mentions: [{ agent_id: 'kv' }]
    }), expect.any(Object))
    wrapper.unmount()
  })

  it('does not silently send only the first of multiple mentioned agents', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' }
    ])
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, AgentWorkspaceSidebar: agentStubs.AgentWorkspaceSidebar }
    })
    await wrapper.get('textarea').setValue('@通用助手 @数据库助手 分析统计表')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    expect((wrapper.vm as unknown as { draft: string }).draft).toContain('@数据库助手')
    wrapper.unmount()
  })

  it('shows the same writable hint for CLI-only assistants', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'kv', name: '键值助手', module: 'kv', builtin_key: 'kv', tool_ids: [] }
    ])
    api.agents.modules.mockResolvedValue([
      { id: 'kv', name: 'Key-Value', description: '', default_tools: [], team_supported: false }
    ])
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [{ id: 'th-kv', title: 'KV', agent_id: 'kv', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as { toolsHint: string }
    expect(vm.toolsHint).toContain('工具可读写')
    expect(vm.toolsHint).not.toContain('工具只读')
    wrapper.unmount()
  })

  it('explains why a previously seeded CLI assistant or sandbox assistant cannot run after capability shutdown', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'kv', name: '键值助手', module: 'kv', builtin_key: 'kv', tool_ids: [] },
      { ...sampleAgent, id: 'sandbox', name: '沙盒助手', module: 'sandbox', builtin_key: 'sandbox', tool_ids: ['sandbox_exec'] }
    ])
    api.agents.modules.mockResolvedValue([
      { id: 'general', name: 'General', description: '', default_tools: [], team_supported: false },
      { id: 'sandbox', name: 'Sandbox', description: '', default_tools: [], team_supported: false, sandbox_available: false }
    ])
    // 分区制：#通用 里放两条固定不同助手的线程。
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [
        { id: 'kv-thread', title: 'kv', agent_id: 'kv', created_at: 't', updated_at: 't' },
        { id: 'sandbox-thread', title: 'sandbox', agent_id: 'sandbox', created_at: 't', updated_at: 't' }
      ],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as { selectThread: (id: string) => Promise<void>; onSend: (text: string, mentions: { agent_id: string }[]) => Promise<void>; draft: string }
    await vm.selectThread('kv-thread')
    await flushPromises()
    expect(wrapper.text()).toContain('需启用 CLI 技能')
    vm.draft = '查询缓存'
    await vm.onSend('查询缓存', [])
    await flushPromises()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    expect(vm.draft).toBe('查询缓存')
    await vm.selectThread('sandbox-thread')
    await flushPromises()
    expect(wrapper.text()).toContain('云沙盒不可用')
    vm.draft = '运行脚本'
    await vm.onSend('运行脚本', [])
    await flushPromises()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('routes a specialist draft mentioning another agent directly', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' },
      { ...sampleAgent, id: 'kv', name: 'KV助手', module: 'kv', builtin_key: 'kv' }
    ])
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [{ id: 'database-thread', title: 'db', agent_id: 'database', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, AgentWorkspaceSidebar: agentStubs.AgentWorkspaceSidebar }
    })
    await wrapper.get('textarea').setValue('@KV助手 查询缓存')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    // planv6.7 AGC-ROUTE：点名即路由，不再要求切换。
    expect(api.agentThreads.streamRun).toHaveBeenCalledWith('testprj1', 'database-thread', expect.objectContaining({
      mentions: [{ agent_id: 'kv' }]
    }), expect.any(Object))
    expect((wrapper.vm as unknown as { draft: string }).draft).toBe('')
    wrapper.unmount()
  })

  it('does not auto-create a thread when the section is empty', async () => {
    api.agents.list.mockResolvedValue([{ ...sampleAgent, id: 'ag-9', builtin_key: 'general' }])
    api.agentThreads.pageBy.mockResolvedValue({ threads: [], next_cursor: '' })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    // planv6.7 AGC-NAV-5：分区无会话展示空态总览，不自动建会话。
    expect(api.agentThreads.create).not.toHaveBeenCalled()
    const vm = wrapper.vm as unknown as { threadId: string }
    expect(vm.threadId).toBe('')
    wrapper.unmount()
  })

  it('sends a stream message, stops it, and starts a new thread', async () => {
    const close = vi.fn()
    api.agentThreads.streamTurn.mockImplementation((_p: string, _t: string, _r: unknown, h: AgentStreamHandlers) => {
      routeOneAgent(h)
      h.onRun?.('run-1')
      h.onToken?.('A')
      h.onEnd?.()
      return { close }
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await wrapper.get('.ai-send').trigger('click')
    await flushPromises()
    expect(api.agentThreads.streamTurn).toHaveBeenCalled()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    await wrapper.get('.ai-stop').trigger('click')
    await clickText(wrapper, '新会话')
    await flushPromises()
    expect(api.agentThreads.create).toHaveBeenCalled()
  })

  it('shows empty-state and reloads on project change', async () => {
    api.agents.list.mockResolvedValueOnce([])
    const { wrapper, pinia } = await mountWithApp(AgentManager, { stubs: agentStubs })
    // 分区制：无助手时默认通用分区空态文案可见。
    expect(wrapper.text()).toContain('通用')
    api.agents.list.mockResolvedValue([sampleAgent])
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.agents.list.mock.calls.length).toBeGreaterThan(1)
  })

  it('handles list load errors', async () => {
    api.agents.list.mockRejectedValueOnce(new Error('agents'))
    await mountWithApp(AgentManager, { stubs: agentStubs })
    expect(toast.error).toHaveBeenCalledWith('agents')
  })

  it('allows sandbox module and shows sandbox hint when available', async () => {
    api.agents.modules.mockResolvedValue([
      { id: 'sandbox', name: 'Sandbox', description: 'isolated sandbox', default_tools: ['sandbox_exec', 'sandbox_shell', 'sandbox_read_file', 'sandbox_write_file'], team_supported: false, sandbox_available: true }
    ])
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, tool_ids: ['sandbox_exec', 'sandbox_shell'] }
    ])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>
    expect(vm.toolsHint).toContain('沙盒命令在云端隔离环境执行')
    expect(vm.toolsHint).toContain('@Database')
    wrapper.unmount()
  })

  it('reports unavailable model options and swallows failed background thread refresh', async () => {
    api.agents.models.mockRejectedValueOnce(new Error('model-options-offline'))
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('model-options-offline')
    const vm = wrapper.vm as Record<string, any>
    expect(vm.modelOptions).toEqual([])

    // planv6.7：后台刷新失败静默（下一次动作会重新拉列表）。
    api.agentThreads.pageBy.mockRejectedValueOnce(new Error('thread-refresh-offline'))
    vm.refreshThreadsForAgent()
    await flushPromises()
    expect(toast.error).not.toHaveBeenCalledWith('thread-refresh-offline')
  })

  it('manages threads, restores URL selection, and sends selected model', async () => {
    api.agentThreads.pageBy.mockResolvedValue({ threads: [
      { id: 'th-1', title: '旧会话', agent_id: 'ag-1', created_at: 't', updated_at: 't' },
      { id: 'th-2', title: '选中会话', agent_id: 'ag-1', created_at: 't', updated_at: 't' }
    ], next_cursor: '' })
    const { wrapper, router } = await mountWithApp(AgentManager, { stubs: agentStubs, path: '/?thread=th-2' })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>
    expect(vm.threadId).toBe('th-2')
    await vm.selectThread('th-1')
    await flushPromises()
    expect(router.currentRoute.value.query.thread).toBe('th-1')
    await vm.renameThread('th-1', '新标题')
    expect(api.agentThreads.rename).toHaveBeenCalledWith(expect.anything(), 'th-1', '新标题')
    await vm.removeThread('th-1')
    expect(api.agentThreads.remove).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('matches duplicate tool names by call id in a collaborative turn', async () => {
    let handlers: AgentStreamHandlers = {}
    api.agentThreads.streamTurn.mockImplementation((_p: string, _t: string, _r: unknown, h: AgentStreamHandlers) => {
      handlers = h
      return { close: vi.fn() }
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as Record<string, any>
    await vm.onSend('查库', [{ agent_id: 'ag-1' }])
    routeOneAgent(handlers)
    handlers.onRun?.('run-1')
    handlers.onThinking?.(2000, 'thinking')
    handlers.onToolStart?.('list_databases', 'c1')
    handlers.onToolStart?.('list_databases', 'c2')
    handlers.onToolResult?.('list_databases', 'second', 'c2', 20)
    handlers.onToolResult?.('list_databases', 'first', 'c1', 10)
    expect(vm.chatMessages[1].toolCalls.map((card: { content: string }) => card.content)).toEqual(['first', 'second'])
    handlers.onError?.(new Error('retry failed'))
    expect(toast.error).toHaveBeenCalledWith('retry failed')
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
  })

  it('covers thread errors and safe retry guards', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as Record<string, any>
    api.agentThreads.pageBy.mockRejectedValueOnce(new Error('page-error'))
    vm.sectionCursor = 't'
    vm.loadMoreSectionThreads()
    await flushPromises()
    api.agentThreads.rename.mockRejectedValueOnce(new Error('rename-error'))
    await vm.renameThread('t', 'x')
    expect(toast.error).toHaveBeenCalledWith('rename-error')
    api.agentThreads.remove.mockRejectedValueOnce(new Error('remove-error'))
    await vm.removeThread('th-1')
    expect(toast.error).toHaveBeenCalledWith('remove-error')
    await vm.selectThread('')
    api.agentThreads.messages.mockRejectedValueOnce(new Error('messages-error'))
    await vm.selectThread('broken')
    expect(api.agentThreads.messages).not.toHaveBeenCalledWith('testprj1', 'broken')
    await vm.selectThread(vm.threadId)
    expect(toast.error).toHaveBeenCalledWith('messages-error')
    // 无失败 run 时重试是安全的空操作（BUG-03）。
    vm.failedRunId = ''
    vm.retryLast()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    // 分区制：删除当前会话后分区还有别的线程 → 切到首条而非自动建会话。
    api.agentThreads.remove.mockResolvedValueOnce(undefined)
    await vm.removeThread('broken')
    expect(api.agentThreads.create).not.toHaveBeenCalled()
  })

  it('shows writable hint when active agent has no sandbox tools', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>
    expect(vm.toolsHint).toContain('工具可读写')
    expect(vm.toolsHint).not.toContain('工具只读')
  })

  it('AgentManager empty-state is safe without agents', async () => {
    api.agents.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: interactionAgentStubs })
    await flushPromises()
    // 分区空态渲染不报错即可（planv6.7 AGC-NAV-5）。
    expect(wrapper.find('.ws').exists()).toBe(true)
    wrapper.unmount()
  })


  it('AgentManager surfaces non-Error failures from list and thread loading', async () => {
    api.agentThreads.pageBy.mockResolvedValue({ threads: [{ id: 'th-1', title: 'T', agent_id: 'ag-1', tag_id: 'general', dispatch_mode: 'collaborative', thread_kind: 'chat', created_at: 't', updated_at: 't' }], next_cursor: '' })
    const { wrapper } = await mountWithApp(AgentManager)
    const vm = wrapper.vm as unknown as {
      loadModules: () => Promise<void>
      loadAgents: () => Promise<void>
      loadSectionThreads: () => Promise<void>
      selectThread: (id: string) => Promise<void>
      resetThread: () => Promise<void>
    }
    api.agents.modules.mockRejectedValueOnce('mods')
    await vm.loadModules()
    api.agents.list.mockRejectedValueOnce('agents')
    await vm.loadAgents()
    api.agentThreads.pageBy.mockRejectedValueOnce('threads')
    await vm.loadSectionThreads()
    api.agentThreads.messages.mockRejectedValueOnce('msgs')
    await vm.selectThread('th-x')
    api.agentThreads.create.mockRejectedValueOnce('th')
    await vm.resetThread()
    wrapper.unmount()
  })

  it('covers extended AgentManager helpers', async () => {
    const agents = mount(AgentManager, {
      global: { plugins: [createPinia()], stubs: { default: true } },
      shallow: true
    })
    await flushPromises()
    const avm = agents.vm as any
    await avm.bootstrap?.()
    await avm.selectThread?.('th')
    await avm.loadModules?.()
    await avm.loadAgents?.()
    api.agents.list.mockRejectedValueOnce(new Error('x'))
    await avm.loadAgents?.()
    await avm.loadThreads?.()
    await avm.resetThread?.()
    avm.onStop?.()
    api.agentThreads.streamRun.mockReturnValue({ close: vi.fn() })
    await avm.onSend?.('hi', [{ agent_id: 'a' }])
    agents.unmount()
  })

  it('does not render a manual chat or plan mode switch', async () => {
    const { wrapper } = await mountWithApp(AgentManager)
    await flushPromises()
    expect(wrapper.find('[data-chat-plan-mode]').exists()).toBe(false)
    expect(wrapper.find('[data-plan-progress]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('offers group settings and member management for #通用', async () => {
    const { wrapper } = await mountWithApp(AgentManager)
    await flushPromises()
    expect(wrapper.find('[data-sidebar-group-more="general"]').exists()).toBe(true)
    await wrapper.get('[data-sidebar-group-add="general"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-group-add-existing="general"]').exists()).toBe(true)
    expect(wrapper.find('[data-group-create-agent="general"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('starts home prompts as a collaborative thread in #通用', async () => {
    sessionStorage.clear()
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs, path: '/?starter=dashboard' })
    await flushPromises()
    expect(api.agentThreads.create).toHaveBeenCalledWith('testprj1', {
      title: '销售数据看板', tagId: 'general', dispatchMode: 'collaborative', threadKind: 'chat', source: 'starter'
    })
    expect(api.agentThreads.streamTurn).toHaveBeenCalledWith('testprj1', 'th-1', expect.objectContaining({
      content: expect.any(String), mentions: [], threadKind: 'chat'
    }), expect.any(Object))
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    wrapper.unmount()
  })

})


describe('AgentManager conversation cleanup', () => {
  beforeEach(() => {
    setI18nLocale('zh-CN')
    resetApiMocks()
    api.agents.list.mockResolvedValue([sampleAgent])
    api.agents.modules.mockResolvedValue([
      { id: 'database', name: 'Database', description: 'd', default_tools: ['list_databases'], team_supported: false }
    ])
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [{ id: 'th-1', title: 'T', agent_id: 'ag-1', tag_id: 'general', dispatch_mode: 'collaborative', thread_kind: 'chat', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    api.agentTags.list.mockResolvedValue([generalTag])
    api.agentThreads.messages.mockResolvedValue([])
    api.cronjobs.list.mockResolvedValue([])
  })

  it('tracks and resolves a pending confirmation in a collaborative turn', async () => {
    let handlers: AgentStreamHandlers = {}
    api.agentThreads.streamTurn.mockImplementation((_project: string, _thread: string, _body: unknown, next: AgentStreamHandlers) => {
      handlers = next
      return { close: vi.fn() }
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as {
      collab: { confirmationList: { value: { callId: string }[] } }
      onSend: (text: string, mentions: { agent_id: string }[]) => Promise<void>
    }
    await vm.onSend('hello', [])
    routeOneAgent(handlers)
    handlers.onRun?.('run-1')
    handlers.onConfirmationRequired?.('call-1', ['site', 'publish'], 'confirm')
    expect(vm.collab.confirmationList.value[0]?.callId).toBe('call-1')
    handlers.onConfirmationResolved?.('call-1', true)
    expect(vm.collab.confirmationList.value).toEqual([])
    wrapper.unmount()
  })

  it('sends multiple mentioned agents in a group thread through /turns', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' },
      { ...sampleAgent, id: 's3', name: 'S3', module: 's3', builtin_key: 's3' }
    ])
    // 群会话：服务端把 auto 读成 collaborative，这里直接给新形态。
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [
        {
          id: 'group-thread',
          title: '协作会话',
          tag_id: 'g1',
          dispatch_mode: 'collaborative',
          thread_kind: 'chat',
          created_at: 't',
          updated_at: 't'
        }
      ],
      next_cursor: ''
    })
    api.agentTags.list.mockResolvedValue([
      {
        id: 'g1',
        name: '协作群',
        agentIds: ['general', 'database', 's3'],
        prompts: [],
        unavailableAgentIds: [],
        routingPrompt: '',
        threadKinds: ['chat'],
        version: 1
      }
    ])
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, AgentWorkspaceSidebar: agentStubs.AgentWorkspaceSidebar }
    })
    const vm = wrapper.vm as unknown as {
      selectSection: (id: string) => Promise<void>
      onSend: (text: string, mentions: { agent_id: string }[]) => Promise<void>
    }
    await vm.selectSection('g1')
    await vm.onSend('核对订单和文件', [{ agent_id: 'database' }, { agent_id: 's3' }])
    await flushPromises()
    // v2：多目标走协作轮次，而不是 /runs 的单助手通道。
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    expect(api.agentThreads.streamTurn).toHaveBeenCalledWith(
      'testprj1',
      'group-thread',
      expect.objectContaining({
        content: '核对订单和文件',
        mentions: [
          { kind: 'agent', id: 'database' },
          { kind: 'agent', id: 's3' }
        ]
      }),
      expect.any(Object)
    )
    wrapper.unmount()
  })

  it('rejects multiple mentions in a fixed-agent legacy thread', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' }
    ])
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [{ id: 'legacy-thread', title: '老会话', agent_id: 'general', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, AgentWorkspaceSidebar: agentStubs.AgentWorkspaceSidebar }
    })
    const vm = wrapper.vm as unknown as {
      onSend: (text: string, mentions: { agent_id: string }[]) => Promise<void>
    }
    await vm.onSend('两个助手一起看', [{ agent_id: 'general' }, { agent_id: 'database' }])
    await flushPromises()
    // /runs 只支持单目标：明确拒绝，不静默只发第一个。
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    expect(api.agentThreads.streamTurn).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('shows the routing card for a collaborative turn', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' }
    ])
    api.agentThreads.pageBy.mockResolvedValue({
      threads: [
        {
          id: 'group-thread',
          title: '协作会话',
          tag_id: 'g1',
          dispatch_mode: 'collaborative',
          created_at: 't',
          updated_at: 't'
        }
      ],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: { AgentComposer: agentStubs.AgentComposer } })
    const vm = wrapper.vm as unknown as {
      collab: {
        syncTasks: (tasks: unknown[]) => void
        tasks: { value: unknown[] }
      }
    }
    vm.collab.syncTasks([
      {
        id: 't1', turnId: 'turn-1', seq: 1, targetKind: 'agent', targetId: 'database',
        instruction: '核对订单', status: 'running', attempt: 0, dependsOn: [], version: 1, createdAt: 't'
      },
      {
        id: 't2', turnId: 'turn-1', seq: 2, targetKind: 'human', targetId: 'u1',
        instruction: '确认异常', status: 'waiting_human', assigneeId: 'u1', attempt: 0,
        dependsOn: [], version: 1, createdAt: 't'
      }
    ])
    await flushPromises()
    expect(wrapper.find('[data-agent-dispatch-card]').exists()).toBe(true)
    expect(wrapper.findAll('[data-dispatch-step]')).toHaveLength(2)
    expect(wrapper.find('[data-agent-human-task]').exists()).toBe(true)
    wrapper.unmount()
  })
})
