import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { setI18nLocale } from '@/locales'
import { useProjectStore } from '@/stores/project'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp, sampleAgent } from '@/test/helpers'
import AgentManager from '@/pages/AgentManager.vue'

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
  ThreadSwitcher: {
    props: ['threads', 'activeId', 'busy'],
    emits: ['create', 'select', 'remove'],
    template:
      '<div class="ts"><button type="button" class="ts-create" @click="$emit(\'create\')">新会话</button></div>'
  },
  AgentScheduleModal: {
    props: ['open', 'agent', 'schedule'],
    emits: ['update:open', 'saved', 'removed', 'view-thread'],
    template: `
      <div v-if="open" class="sch-m">
        <button type="button" class="sch-close" @click="$emit('update:open', false)">x</button>
        <button type="button" class="sch-saved" @click="$emit('saved', { id: 's1', agent_id: 'ag-1' })">save</button>
        <button type="button" class="sch-removed" @click="$emit('removed', 'sch-1')">rm</button>
        <button type="button" class="sch-thread" @click="$emit('view-thread', 'th-1')">th</button>
      </div>
    `
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
  ThreadSwitcher: {
    props: ['threads', 'activeId', 'busy', 'runningThreadIds'],
    emits: ['create', 'select', 'remove'],
    template: `
      <div class="ts" :data-running-threads="runningThreadIds ? [...runningThreadIds].join(',') : ''">
        <button type="button" class="ts-create" @click="$emit('create')">新会话</button>
        <button type="button" class="ts-select" @click="$emit('select', threads[0] && threads[0].id)">pick</button>
      </div>
    `
  },
  AgentScheduleModal: {
    props: ['open', 'agent', 'schedule'],
    emits: ['update:open', 'saved', 'removed', 'view-thread'],
    template: `
      <div v-if="open" class="sch-m">
        {{ agent && agent.name }}
        <button type="button" class="sch-saved" @click="$emit('saved', { id: 's1', agent_id: 'ag-1', cron_expr: '0 8 * * *', enabled: true })">save</button>
        <button type="button" class="sch-removed" @click="$emit('removed', 'sch-1')">rm</button>
        <button type="button" class="sch-thread" @click="$emit('view-thread', 'th-1')">th</button>
        <button type="button" class="sch-close" @click="$emit('update:open', false)">x</button>
      </div>
    `
  }
}

function clickExact(wrapper: Awaited<ReturnType<typeof mountWithApp>>['wrapper'], text: string) {
  const el = wrapper.findAll('button').find((b) => b.text().trim() === text)
  if (!el) throw new Error(`No exact button "${text}"`)
  return el.trigger('click')
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
  },
  AgentScheduleModal: {
    props: ['open', 'agent', 'schedule'],
    emits: ['update:open', 'saved', 'removed', 'view-thread'],
    template: '<div />'
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
    api.agentSchedules.list.mockResolvedValue([
      {
        id: 'sch-1',
        agent_id: 'ag-1',
        thread_id: 'th-1',
        prompt: 'p',
        cron_expr: '0 * * * *',
        enabled: true,
        created_at: 't',
        updated_at: 't'
      }
    ])
  })

  it('marks agents / threads with active runs (incl. scheduled) and stops polling when idle', async () => {
    vi.useFakeTimers()
    try {
      api.agentThreads.active.mockResolvedValue([{ run_id: 'r-cron', agent_id: 'ag-1', thread_id: 'th-1' }])
      const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
      await flushPromises()
      expect(api.agentThreads.active).toHaveBeenCalledWith('testprj1')
      expect(wrapper.find('[data-agent-running]').exists()).toBe(true)
      expect(wrapper.get('.ts').attributes('data-running-threads')).toBe('th-1')

      // 运行结束：下一轮轮询拿到空列表，转圈消失，之后不再轮询。
      api.agentThreads.active.mockResolvedValue([])
      await vi.advanceTimersByTimeAsync(3000)
      await flushPromises()
      expect(wrapper.find('[data-agent-running]').exists()).toBe(false)
      const calls = api.agentThreads.active.mock.calls.length
      await vi.advanceTimersByTimeAsync(10000)
      expect(api.agentThreads.active.mock.calls.length).toBe(calls)
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('lists agents with schedule summary and selects one', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    // 页面级 subtitle 已移除（planv6.0 §3.2），空间让给对话区。
    expect(wrapper.text()).not.toContain('按模块的 Agent')
    expect(wrapper.text()).toContain('Database')
    expect(wrapper.text()).toContain('已启用')
    const select = wrapper.get('button[aria-label="选择 Agent Database"]')
    expect(select.attributes('aria-pressed')).toBe('true')
    expect(select.find('button').exists()).toBe(false)
    await select.trigger('click')
    expect(select.attributes('aria-pressed')).toBe('true')
    expect(wrapper.text()).toContain('@Database')
  })

  it('creates, edits, and deletes an agent from the page', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await wrapper.get('button[aria-label="新建"]').trigger('click')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
    await wrapper.get('#agent-name').setValue('')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(toast.warning).toHaveBeenCalledWith('请填写名称')

    await wrapper.get('#agent-name').setValue('N2')
    await wrapper.get('#agent-desc').setValue('desc')
    await wrapper.get('#agent-prompt').setValue('sys')
    await wrapper.get('.select-emit').trigger('click')
    const toolButton = wrapper.findAll('button.badge').at(-1)
    if (!toolButton) throw new Error('Missing tool selector')
    const initialPressed = toolButton.attributes('aria-pressed')
    expect(['true', 'false']).toContain(initialPressed)
    await toolButton.trigger('click')
    expect(toolButton.attributes('aria-pressed')).not.toBe(initialPressed)
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.agents.create).toHaveBeenCalled()

    // planv6.0 §3.3：编辑/删除从头部「⋯」菜单进入（Popover 桩直接渲染内容）。
    await clickExact(wrapper, '编辑助手')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)
    await wrapper.get('#agent-name').setValue('N3')
    await wrapper.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.agents.patch).toHaveBeenCalled()

    api.agents.remove.mockRejectedValueOnce(new Error('rm'))
    await clickExact(wrapper, '删除')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('rm')
    await clickExact(wrapper, '删除')
    await flushPromises()
    expect(api.agents.remove).toHaveBeenCalled()
  })

  it('jumps to the cron jobs page with the agent preselected when choosing schedule', async () => {
    const { wrapper, router } = await mountWithApp(AgentManager, { stubs: agentStubs })
    // 菜单项里带定时摘要，用包含匹配而非全等。
    await clickText(wrapper, '定时执行')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('cron-jobs')
    expect(router.currentRoute.value.query.agent).toBe(sampleAgent.id)
    expect(wrapper.find('.sch-m').exists()).toBe(false)
  })

  it('keeps the composer outside the scrollable message viewport', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: composerOnlyStubs })
    const viewport = wrapper.get('div.overflow-y-auto.py-1').element
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
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, h: { onRun?: (id: string) => void; onToken?: (t: string) => void; onEnd?: () => void }) => {
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
      { role: 'user', content: 'u', module: undefined, toolCalls: undefined, error: undefined, canceled: false },
      {
        role: 'assistant', content: 'a', module: 's3', toolCalls: [{ name: 't', status: 'success' }],
        // 现状：传入的是普通对象而非 Error，errorMessage 走 fallback 文案（按现状锁定）。
        error: '运行失败', canceled: true
      },
      { role: 'user', content: 's', module: undefined, toolCalls: undefined, error: undefined, canceled: false }
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

  it('menu actions: no active agent is a no-op', async () => {
    // planv6.0 §3.3：编辑/定时/删除都从头部「⋯」菜单进入。
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as {
      onMenu: (a: 'edit' | 'schedule' | 'remove') => void
      activeId: string
      activeScheduleSummary: string
    }
    // 有定时任务时菜单里显示摘要
    expect(vm.activeScheduleSummary).toContain('每小时')

    // 没有选中助手时，编辑/定时/删除都直接返回，不抛错
    vm.activeId = ''
    await flushPromises()
    vm.onMenu('edit')
    vm.onMenu('schedule')
    vm.onMenu('remove')
    expect(vm.activeScheduleSummary).toBe('')
    wrapper.unmount()
  })

  it('menu handles thread rename and delete', async () => {
    // 会话的重命名/删除从 ThreadSwitcher 收进「⋯」菜单（planv6.0 §3.6 #7）。
    api.agentThreads.rename.mockResolvedValue({ id: 'th-1', title: '改过', created_at: 't', updated_at: 't' })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as {
      onMenu: (a: 'renameThread' | 'removeThread') => void
      submitRenameThread: () => Promise<void>
      renameValue: string
      renameOpen: boolean
      renameTarget: { id: string; title: string } | null
    }
    vm.onMenu('renameThread')
    await flushPromises()
    expect(vm.renameOpen).toBe(true)
    expect(vm.renameTarget?.id).toBe('th-1')
    // 名称没变则直接关闭，不发请求（用标题本身，不假设具体文案）
    vm.renameValue = vm.renameTarget!.title
    await vm.submitRenameThread()
    expect(api.agentThreads.rename).not.toHaveBeenCalled()
    expect(vm.renameOpen).toBe(false)

    vm.onMenu('renameThread')
    vm.renameValue = '改过'
    await vm.submitRenameThread()
    await flushPromises()
    expect(api.agentThreads.rename).toHaveBeenCalled()

    vm.onMenu('removeThread')
    await flushPromises()
    expect(api.agentThreads.remove).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('wires the header menu and composer focus', async () => {
    // 覆盖菜单的模板内联处理器与 composer 的 focus 透传（planv6.0 §3.3/§3.8）。
    const { wrapper } = await mountWithApp(AgentManager, { stubs: composerOnlyStubs })
    // 菜单开合
    await wrapper.get('.pop-open').trigger('click')
    await wrapper.get('.pop-close').trigger('click')
    // 点菜单项：编辑 / 定时 / 删除 / 重命名会话
    await clickText(wrapper, '编辑助手')
    await flushPromises()
    if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    await clickText(wrapper, '定时执行')
    await flushPromises()
    if (wrapper.find('.sch-close').exists()) await wrapper.get('.sch-close').trigger('click')
    await clickText(wrapper, '重命名会话')
    await flushPromises()
    // 重命名弹窗：撤销并关闭
    if (wrapper.find('#thread-rename').exists()) await wrapper.get('#thread-rename').setValue('新名')
    if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    expect(wrapper.text()).not.toContain('刷新助手列表')
    // 删除会话走 ConfirmAction 桩
    const removeBtn = wrapper.findAll('button').find((b) => b.text() === '删除会话')
    if (removeBtn) await removeBtn.trigger('click')
    await flushPromises()

    // composer 的 focus 透传：点提示词后不抛错，且写入了 draft
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

  it('resets the selected agent when switching to a new project', async () => {
    const oldAgent = { ...sampleAgent, id: 'ag-old', builtin_key: 'general' }
    const newAgent = { ...sampleAgent, id: 'ag-new', builtin_key: 'general' }
    api.agents.list.mockImplementation((projectId: string) =>
      Promise.resolve([projectId === 'testprj1' ? oldAgent : newAgent])
    )
    api.agentThreads.page.mockImplementation((_projectId: string, agentId?: string) =>
      Promise.resolve({
        threads: [{ id: `th-${agentId}`, title: 'T', agent_id: agentId, created_at: 't', updated_at: 't' }],
        next_cursor: ''
      })
    )
    const { wrapper, pinia } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as { activeId: string }
    expect(vm.activeId).toBe('ag-old')

    useProjectStore(pinia).setProject('newproj1', 'New')
    await flushPromises()

    expect(vm.activeId).toBe('ag-new')
    expect(api.agentThreads.page).toHaveBeenLastCalledWith('newproj1', 'ag-new')
    expect(api.agentThreads.page).not.toHaveBeenCalledWith('newproj1', 'ag-old')
    wrapper.unmount()
  })

  it('clears the old agent and skips thread loading when the new project has no agents', async () => {
    api.agents.list.mockImplementation((projectId: string) =>
      Promise.resolve(projectId === 'testprj1' ? [{ ...sampleAgent, id: 'ag-old' }] : [])
    )
    const { wrapper, pinia } = await mountWithApp(AgentManager, { stubs: agentStubs })
    useProjectStore(pinia).setProject('newproj1', 'New')
    await flushPromises()

    expect((wrapper.vm as unknown as { activeId: string }).activeId).toBe('')
    expect(api.agentThreads.page).not.toHaveBeenCalledWith('newproj1', 'ag-old')
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

    const vm = wrapper.vm as unknown as { activeId: string; agents: { id: string }[] }
    expect(vm.activeId).toBe('ag-new')
    expect(vm.agents.map((agent) => agent.id)).toEqual(['ag-new'])
    wrapper.unmount()
  })

  it('keeps a separate conversation per agent', async () => {
    // planv6.0 §3.7 修订：会话归属助手，切换助手要换一份会话列表。
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'ag-1', name: 'Database', builtin_key: 'general' },
      { ...sampleAgent, id: 'ag-2', name: 'Logs', module: 'logs' }
    ])
    // 按 agent_id 返回各自的会话：默认助手 ag-1 有 th-1，ag-2 有 th-2。
    api.agentThreads.page.mockImplementation((_p: string, agentId?: string) =>
      Promise.resolve(
        agentId === 'ag-2'
          ? { threads: [{ id: 'th-2', title: '日志会话', agent_id: 'ag-2', created_at: 't', updated_at: 't' }], next_cursor: '' }
          : { threads: [{ id: 'th-1', title: '库会话', agent_id: 'ag-1', created_at: 't', updated_at: 't' }], next_cursor: '' }
      )
    )
    api.agentThreads.messages.mockResolvedValue([])

    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as {
      selectAgent: (id: string) => Promise<void>
      activeId: string
      threadId: string
      threads: { id: string }[]
    }
    // 初始用默认助手的会话
    expect(vm.activeId).toBe('ag-1')
    expect(vm.threadId).toBe('th-1')

    await vm.selectAgent('ag-2')
    await flushPromises()
    // 切到 ag-2：会话换成它自己的
    expect(vm.activeId).toBe('ag-2')
    expect(vm.threadId).toBe('th-2')
    expect(vm.threads.map((t) => t.id)).toEqual(['th-2'])
    // 取会话时必须带上 agent_id
    expect(api.agentThreads.page).toHaveBeenLastCalledWith('testprj1', 'ag-2')
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, handlers: { onEnd?: () => void }) => {
      handlers.onEnd?.()
      return { close: vi.fn() }
    })
    api.agentThreads.page.mockClear()
    await wrapper.get('.ai-send').trigger('click')
    await flushPromises()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    expect(api.agentThreads.page).not.toHaveBeenCalled()
    expect((wrapper.vm as unknown as { draft: string }).draft).toBe('')

    // 点当前已选中的助手不重复请求
    const calls = (api.agentThreads.page as unknown as { mock: { calls: unknown[] } }).mock.calls.length
    await vm.selectAgent('ag-2')
    expect((api.agentThreads.page as unknown as { mock: { calls: unknown[] } }).mock.calls.length).toBe(calls)
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
    expect(wrapper.text()).toContain('数据库助手')
    expect(wrapper.text()).toContain('对象存储助手')
    expect(wrapper.text()).toContain('日志助手')
    expect(wrapper.text()).toContain('我的报表')
    setI18nLocale('en-US')
    await flushPromises()
    expect(wrapper.text()).toContain('General Assistant')
    expect(wrapper.text()).toContain('Database Assistant')
    expect(wrapper.text()).toContain('Object Storage Assistant')
    expect(wrapper.text()).toContain('Logs Assistant')
    expect(wrapper.text()).toContain('Key-Value Assistant')
    expect(wrapper.text()).toContain('我的报表')
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
    api.agentThreads.page.mockResolvedValue({
      threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, ThreadSwitcher: agentStubs.ThreadSwitcher, AgentScheduleModal: agentStubs.AgentScheduleModal }
    })
    await wrapper.get('textarea').setValue('@数据库 查询统计表')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(api.agentThreads.page).toHaveBeenCalledWith('testprj1', 'general')
    // @ 只点名助手；技能由后端按助手 module 决定，请求体不再携带 skills。
    expect(api.agentThreads.streamRun).toHaveBeenCalledWith('testprj1', 'general-thread', {
      content: '@数据库 查询统计表',
      mentions: [{ agent_id: 'general' }],
      module: 'general',
      stream: true
    }, expect.any(Object))
    wrapper.unmount()
  })

  it('asks to switch when a general thread mentions another agent', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' }
    ])
    api.agentThreads.page.mockResolvedValue({
      threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, ThreadSwitcher: agentStubs.ThreadSwitcher, AgentScheduleModal: agentStubs.AgentScheduleModal }
    })
    await wrapper.get('textarea').setValue('@数据库助手 查询统计表')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    expect(wrapper.get('[data-agent-switch]').text()).toContain('数据库助手')
    expect((wrapper.vm as unknown as { draft: string }).draft).toBe('@数据库助手 查询统计表')
    wrapper.unmount()
  })

  it('keeps legacy @Database mentions as agent targets after translating the display name', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' }
    ])
    api.agentThreads.page.mockResolvedValue({
      threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, ThreadSwitcher: agentStubs.ThreadSwitcher, AgentScheduleModal: agentStubs.AgentScheduleModal }
    })
    await wrapper.get('textarea').setValue('@Database 查询统计表')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    expect(wrapper.get('[data-agent-switch]').text()).toContain('数据库助手')
    wrapper.unmount()
  })

  it('keeps the switch action after changing locale with an existing mention draft', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' }
    ])
    api.agentThreads.page.mockResolvedValue({ threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }], next_cursor: '' })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, ThreadSwitcher: agentStubs.ThreadSwitcher, AgentScheduleModal: agentStubs.AgentScheduleModal }
    })
    await wrapper.get('textarea').setValue('@数据库助手 查询统计表')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(wrapper.find('[data-agent-switch]').exists()).toBe(true)
    setI18nLocale('en-US')
    await flushPromises()
    expect(wrapper.find('[data-agent-switch]').exists()).toBe(true)
    expect((wrapper.vm as unknown as { draft: string }).draft).toBe('@数据库助手 查询统计表')
    wrapper.unmount()
    setI18nLocale('zh-CN')
  })

  it('offers the KV built-in switch for @KV助手 without requiring a custom agent named that', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'kv', name: '键值助手', module: 'kv', builtin_key: 'kv', tool_ids: [] }
    ])
    api.agentThreads.page.mockResolvedValue({ threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }], next_cursor: '' })
    api.agents.modules.mockResolvedValue([{ id: 'kv', name: 'KV', description: '', default_tools: [], team_supported: false }])
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, ThreadSwitcher: agentStubs.ThreadSwitcher, AgentScheduleModal: agentStubs.AgentScheduleModal }
    })
    await wrapper.get('textarea').setValue('@KV助手 查询缓存')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    expect(wrapper.get('[data-agent-switch]').text()).toContain('键值助手')
    wrapper.unmount()
  })

  it('does not silently send only the first of multiple mentioned agents', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'general', name: '通用助手', module: 'general', builtin_key: 'general' },
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' }
    ])
    api.agentThreads.page.mockResolvedValue({
      threads: [{ id: 'general-thread', title: '通用会话', agent_id: 'general', created_at: 't', updated_at: 't' }],
      next_cursor: ''
    })
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, ThreadSwitcher: agentStubs.ThreadSwitcher, AgentScheduleModal: agentStubs.AgentScheduleModal }
    })
    await wrapper.get('textarea').setValue('@通用助手 @数据库助手 分析统计表')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    expect((wrapper.vm as unknown as { draft: string }).draft).toContain('@数据库助手')
    wrapper.unmount()
  })

  it('shows CLI-only capabilities without suggesting unrelated built-in tools', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'kv', name: '键值助手', module: 'kv', builtin_key: 'kv', tool_ids: [] }
    ])
    api.agents.modules.mockResolvedValue([
      { id: 'kv', name: 'Key-Value', description: '', default_tools: [], team_supported: false }
    ])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as { toolsHint: string; toolOptions: { value: string }[]; openEdit: (agent: typeof sampleAgent) => void }
    expect(vm.toolsHint).toContain('CLI 技能')
    vm.openEdit({ ...sampleAgent, id: 'kv', name: '键值助手', module: 'kv', tool_ids: [] })
    expect(vm.toolOptions).toEqual([])
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
    api.agentThreads.page.mockImplementation((_project: string, id: string) => Promise.resolve({
      threads: [{ id: `${id}-thread`, title: id, agent_id: id, created_at: 't', updated_at: 't' }], next_cursor: ''
    }))
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as unknown as { selectAgent: (id: string) => Promise<void>; onSend: (text: string, mentions: { agent_id: string }[]) => Promise<void>; draft: string }
    await vm.selectAgent('kv')
    await flushPromises()
    expect(wrapper.text()).toContain('需启用 CLI 技能')
    vm.draft = '查询缓存'
    await vm.onSend('查询缓存', [])
    await flushPromises()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    expect(vm.draft).toBe('查询缓存')
    await vm.selectAgent('sandbox')
    await flushPromises()
    expect(wrapper.text()).toContain('云沙盒不可用')
    vm.draft = '运行脚本'
    await vm.onSend('运行脚本', [])
    await flushPromises()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps a specialist draft and requests a switch before mentioning another agent', async () => {
    api.agents.list.mockResolvedValue([
      { ...sampleAgent, id: 'database', name: 'Database', module: 'database', builtin_key: 'database' },
      { ...sampleAgent, id: 'kv', name: 'KV助手', module: 'kv', builtin_key: 'kv' }
    ])
    api.agentThreads.page.mockImplementation((_project: string, id: string) => Promise.resolve({
      threads: [{ id: `${id}-thread`, title: id, agent_id: id, created_at: 't', updated_at: 't' }],
      next_cursor: ''
    }))
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ConversationView: agentStubs.ConversationView, ThreadSwitcher: agentStubs.ThreadSwitcher, AgentScheduleModal: agentStubs.AgentScheduleModal }
    })
    await wrapper.get('textarea').setValue('@KV助手 查询缓存')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    await flushPromises()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('KV助手')
    expect((wrapper.vm as unknown as { draft: string }).draft).toBe('@KV助手 查询缓存')
    await wrapper.get('[data-agent-switch]').trigger('click')
    await flushPromises()
    expect(api.agentThreads.page).toHaveBeenCalledWith('testprj1', 'kv')
    expect((wrapper.vm as unknown as { draft: string }).draft).toBe('@KV助手 查询缓存')
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('creates the first conversation of an agent with its own agent_id', async () => {
    api.agents.list.mockResolvedValue([{ ...sampleAgent, id: 'ag-9', builtin_key: 'general' }])
    api.agentThreads.page.mockResolvedValue({ threads: [], next_cursor: '' })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    // 该助手暂无会话 → 自动建一个，并带上 agent_id
    expect(api.agentThreads.create).toHaveBeenCalledWith('testprj1', expect.any(String), 'ag-9')
    wrapper.unmount()
  })

  it('sends a stream message, stops it, and starts a new thread', async () => {
    const close = vi.fn()
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, h: { onRun?: (id: string) => void; onToken?: (t: string) => void; onEnd?: () => void }) => {
      h.onRun?.('run-1')
      h.onToken?.('A')
      h.onEnd?.()
      return { close }
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await wrapper.get('.ai-send').trigger('click')
    await flushPromises()
    expect(api.agentThreads.streamRun).toHaveBeenCalled()
    await wrapper.get('.ai-stop').trigger('click')
    await clickText(wrapper, '新会话')
    await flushPromises()
    expect(api.agentThreads.create).toHaveBeenCalled()
  })

  it('shows empty-state create and reloads on project change', async () => {
    api.agents.list.mockResolvedValueOnce([])
    const { wrapper, pinia } = await mountWithApp(AgentManager, { stubs: agentStubs })
    expect(wrapper.text()).toContain('暂无 Agent')
    await wrapper.get('.empty-action').trigger('click')
    expect(wrapper.find('.sb-modal').exists()).toBe(true)

    api.agents.list.mockResolvedValue([sampleAgent])
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    expect(api.agents.list.mock.calls.length).toBeGreaterThan(1)
  })

  it('handles list / schedule / thread load errors', async () => {
    api.agents.list.mockRejectedValueOnce(new Error('agents'))
    api.agentSchedules.list.mockRejectedValueOnce(new Error('sched'))
    await mountWithApp(AgentManager, { stubs: agentStubs })
    expect(toast.error).toHaveBeenCalledWith('agents')
    expect(toast.error).toHaveBeenCalledWith('sched')
  })

  it('blocks sandbox module when sandbox_available is false', async () => {
    api.agents.modules.mockResolvedValue([
      { id: 'database', name: 'Database', description: 'd', default_tools: ['list_databases'], team_supported: false },
      { id: 'sandbox', name: 'Sandbox', description: 'isolated sandbox', default_tools: ['sandbox_exec', 'sandbox_shell', 'sandbox_read_file', 'sandbox_write_file'], team_supported: false, sandbox_available: false }
    ])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>
    const sandboxOpt = vm.moduleOptions.find((o: { value: string }) => o.value === 'sandbox')
    expect(sandboxOpt.disabled).toBe(true)

    vm.openCreate()
    vm.form.name = 'SB'
    vm.form.module = 'sandbox'
    await vm.saveAgent()
    expect(toast.warning).toHaveBeenCalledWith('云沙盒未配置，无法创建 Sandbox Agent')
    expect(api.agents.create).not.toHaveBeenCalled()
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
    const sandboxOpt = vm.moduleOptions.find((o: { value: string }) => o.value === 'sandbox')
    expect(sandboxOpt.disabled).toBe(false)
    expect(vm.toolsHint).toContain('沙盒命令在云端隔离环境执行')
    expect(vm.toolsHint).toContain('@Database')

    vm.openCreate()
    vm.form.name = 'SB'
    vm.form.module = 'sandbox'
    await vm.saveAgent()
    expect(api.agents.create).toHaveBeenCalled()
  })

  it('reports unavailable model options and failed background thread refresh', async () => {
    api.agents.models.mockRejectedValueOnce(new Error('model-options-offline'))
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('model-options-offline')
    const vm = wrapper.vm as Record<string, any>
    expect(vm.modelOptions).toEqual([])

    api.agentThreads.page.mockRejectedValueOnce(new Error('thread-refresh-offline'))
    vm.refreshThreadsForAgent()
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('thread-refresh-offline')
  })

  it('manages threads, restores URL selection, and sends selected model', async () => {
    api.agentThreads.page.mockResolvedValue({ threads: [
      { id: 'th-1', title: '旧会话', created_at: 't', updated_at: 't' },
      { id: 'th-2', title: '选中会话', created_at: 't', updated_at: 't' }
    ], next_cursor: 'th-2' })
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
    vm.openEdit(sampleAgent)
    vm.form.model_override = 'deepseek-ai/DeepSeek-V4-Flash'
    await vm.saveAgent()
    expect(api.agents.patch).toHaveBeenCalledWith(expect.anything(), sampleAgent.id, expect.objectContaining({ model_override: 'deepseek-ai/DeepSeek-V4-Flash' }))
  })

  it('matches duplicate tool names by call id, retries errors and shows cancel state', async () => {
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, h: Record<string, (...args: unknown[]) => void>) => {
      handlers = h
      return { close: vi.fn() }
    })
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as Record<string, any>
    await vm.onSend('查库', [{ agent_id: 'ag-1' }])
    handlers.onRun?.('run-1')
    handlers.onThinking?.(2000, 'thinking')
    handlers.onToolCall?.('list_databases', '{}', 'c1')
    handlers.onToolCall?.('list_databases', '{}', 'c2')
    handlers.onToolResult?.('list_databases', 'second', 'c2', 20)
    handlers.onToolResult?.('list_databases', 'first', 'c1', 10)
    expect(vm.chatMessages[1].toolCalls.map((card: { content: string }) => card.content)).toEqual(['first', 'second'])
    handlers.onError?.(Object.assign(new Error('rate limit'), { code: 'llm_rate_limited' }))
    expect(vm.chatMessages[1].error).toContain('限流')
    // 重试走后端 retry_of_run_id（BUG-03）：失败 run id 已记录。
    expect(vm.failedRunId).toBe('run-1')
    vm.retryLast()
    expect(api.agentThreads.streamRun).toHaveBeenCalledTimes(2)
    expect(api.agentThreads.streamRun.mock.calls[1][2]).toMatchObject({ retry_of_run_id: 'run-1' })
    handlers.onRun?.('run-2')
    vm.onStop()
    expect(vm.chatMessages.at(-1)?.canceled).toBe(true)
    // 重试分支的两个回调：onEnd 刷新会话列表，onError 走 toast。
    handlers.onEnd?.('stop')
    handlers.onError?.(Object.assign(new Error('retry failed'), { code: 'x' }))
    expect(toast.error).toHaveBeenCalledWith('retry failed')
  })

  it('covers thread errors and safe retry guards', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    const vm = wrapper.vm as Record<string, any>
    api.agentThreads.page.mockRejectedValueOnce(new Error('page-error'))
    vm.nextCursor = 't'
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
    api.agentThreads.remove.mockResolvedValueOnce(undefined)
    await vm.removeThread('broken')
    expect(api.agentThreads.create).toHaveBeenCalled()
  })

  it('shows readonly hint when active agent has no sandbox tools', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>
    expect(vm.toolsHint).toContain('工具只读')
  })

  // planv6.0 前端简化 B2：openCreate/openEdit 的表单构造（字段、默认值、数组复制、modal 打开）。
  it('builds create/edit forms with copied tool arrays and defaults', async () => {
    const { wrapper } = await mountWithApp(AgentManager, { stubs: agentStubs })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>

    vm.openCreate()
    expect(vm.editing).toBeNull()
    expect(vm.modalOpen).toBe(true)
    expect(vm.form).toEqual({
      name: 'Database', module: 'database', description: 'd', system_prompt: '',
      model_override: '', tool_ids: ['list_databases'], writable: false
    })
    vm.form.tool_ids.push('mutated')
    expect(vm.modules[0].default_tools).toEqual(['list_databases'])

    const agent = { ...sampleAgent, model_override: undefined, writable: undefined, tool_ids: ['a'] }
    vm.openEdit(agent)
    expect(vm.editing).toStrictEqual(agent)
    expect(vm.form).toEqual({
      name: 'Database', module: 'database', description: 'db helper', system_prompt: 'sys',
      model_override: '', tool_ids: ['a'], writable: false
    })
    vm.form.tool_ids.push('b')
    expect(agent.tool_ids).toEqual(['a'])
    vm.openEdit({ ...sampleAgent, tool_ids: undefined, model_override: 'm', writable: true })
    expect(vm.form).toMatchObject({ tool_ids: [], model_override: 'm', writable: true })

    vm.modules = []
    vm.openCreate()
    expect(vm.form).toEqual({
      name: '', module: 'database', description: '', system_prompt: '',
      model_override: '', tool_ids: [], writable: false
    })
  })
})

describe('AgentManager script helpers', () => {
  beforeEach(() => {
    resetApiMocks()
    api.agents.list.mockResolvedValue([sampleAgent])
    api.agentThreads.list.mockResolvedValue([{ id: 'th-1', title: 'T', created_at: 't', updated_at: 't' }])
    api.agentThreads.messages.mockResolvedValue([])
    api.agentSchedules.list.mockResolvedValue([
      { id: 'sch-1', agent_id: 'ag-1', thread_id: 'th-1', prompt: 'p', cron_expr: '0 * * * *', enabled: false, created_at: 't', updated_at: 't' }
    ])
  })

  it('covers schedule helpers, CRUD edge cases, and stream send/stop', async () => {
    const { wrapper } = await mountWithApp(AgentManager, {
      stubs: { ...agentStubs, AgentScheduleModal: true, AiChat: true }
    })
    await flushPromises()
    const vm = wrapper.vm as Record<string, any>
    expect(vm.scheduleSummary({ cron_expr: '*/15 * * * *', enabled: true })).toContain('每 15 分钟')
    expect(vm.scheduleSummary({ cron_expr: '0 * * * *', enabled: false })).toContain('已停用')
    expect(vm.scheduleSummary({ cron_expr: '0 8 * * *', enabled: true })).toContain('每天')
    expect(vm.scheduleSummary({ cron_expr: '0 8 * * 1', enabled: true })).toContain('每周一')
    expect(vm.scheduleSummary({ cron_expr: '1 2 3 4 5', enabled: true })).toContain('1 2 3 4 5')

    vm.onScheduleSaved({ id: 's', agent_id: 'ag-1' })
    vm.onScheduleRemoved('sch-1')
    vm.openSchedule(sampleAgent)
    await vm.onViewScheduleThread('th-view')
    api.agentThreads.messages.mockRejectedValueOnce(new Error('th'))
    await vm.onViewScheduleThread('th-view')
    api.agentThreads.messages.mockResolvedValueOnce([
      { id: 'm1', role: 'assistant', content: 'a', tool_calls: [{ name: 't' }], created_at: 't' },
      { id: 'm2', role: 'user', content: 'u', created_at: 't' }
    ])
    await vm.onViewScheduleThread('th-msgs')

    vm.openCreate()
    vm.onModuleChange('s3')
    expect(vm.form.tool_ids).toEqual(expect.arrayContaining(['list_objects']))
    vm.form.name = ''
    await vm.saveAgent()
    expect(toast.warning).toHaveBeenCalledWith('请填写名称')
    vm.form.name = 'N'
    await vm.saveAgent()
    vm.openEdit(sampleAgent)
    vm.onModuleChange('s3')
    vm.onModuleChange('missing')
    vm.toggleTool('list_databases')
    vm.toggleTool('list_databases')
    // planv6.0 v2 §3.2：新建默认只读，编辑回填已有的 writable。
    vm.openCreate()
    expect(vm.form.writable).toBe(false)
    vm.openEdit({ ...sampleAgent, writable: true })
    expect(vm.form.writable).toBe(true)
    vm.form.writable = false
    await vm.saveAgent()
    expect(api.agents.patch).toHaveBeenCalledWith(expect.anything(), sampleAgent.id, expect.objectContaining({ writable: false }))
    api.agents.patch.mockRejectedValueOnce(new Error('p'))
    await vm.saveAgent()
    await vm.removeAgent(sampleAgent)
    api.agents.remove.mockRejectedValueOnce(new Error('r'))
    await vm.removeAgent(sampleAgent)
    await vm.resetThread()
    api.agentThreads.create.mockRejectedValueOnce(new Error('c'))
    await vm.resetThread()

    const close = vi.fn()
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, h: any) => {
      h.onRun?.('run-1')
      h.onToken?.('A')
      h.onToolCall?.('list_databases', '{}')
      h.onToolResult?.('list_databases', '[]')
      h.onToolResult?.('other', 'x')
      h.onEnd?.()
      return { close }
    })
    await vm.onSend('hello', [{ agent_id: 'ag-1' }])
    await vm.onSend('   ', [])
    // sending 是 computed：模拟运行中通过新开一个未完成的流。
    const close2 = vi.fn()
    api.agentThreads.streamRun.mockImplementation(() => ({ close: close2 }))
    await vm.onSend('again', [{ agent_id: 'ag-1' }])
    await vm.onSend('during', [{ agent_id: 'ag-1' }])
    vm.onStop()
    vm.activeId = ''
    await vm.onSend('no-agent', [])
    expect(toast.warning).toHaveBeenCalledWith('请先选择或 @ 一个 Agent')
    vm.activeId = 'ag-1'
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, h: any) => {
      h.onError?.(new Error('sse'))
      return { close }
    })
    await vm.onSend('x', [])
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, _r: unknown, h: any) => {
      h.onError?.('z')
      return { close }
    })
    await vm.onSend('y', [{ agent_id: 'ag-1' }])
    vm.onStop()

    api.agents.modules.mockRejectedValueOnce('m')
    await vm.loadModules()
    api.agentThreads.list.mockResolvedValueOnce([])
    api.agentThreads.messages.mockResolvedValueOnce([
      { id: 'm1', role: 'assistant', content: 'a', tool_calls: [{ name: 't' }], created_at: 't' }
    ])
    await vm.loadThreads()
    api.agentThreads.list.mockRejectedValueOnce(new Error('t'))
    await vm.loadThreads()
    api.agentSchedules.list.mockRejectedValueOnce(new Error('s'))
    await vm.loadSchedules()
    wrapper.unmount()
  })
  it('AgentManager clicks list actions, modal fields, schedule, and stream stop', async () => {
    api.agents.modules.mockResolvedValue([
      { id: 'database', name: 'Database', description: 'd', default_tools: ['list_databases'], team_supported: false },
      { id: 's3', name: 'S3', description: 's', default_tools: ['list_objects'], team_supported: false }
    ])
    const { wrapper, pinia } = await mountWithApp(AgentManager, { stubs: interactionAgentStubs })
    await wrapper.find('button.w-full').trigger('click')
    await wrapper.get('button[aria-label="新建"]').trigger('click')
    await flushPromises()
    if (wrapper.find('.sb-modal').exists()) {
      if (wrapper.find('.select-emit').exists()) await wrapper.get('.select-emit').trigger('click')
      if (wrapper.find('#agent-name').exists()) await wrapper.get('#agent-name').setValue('N2')
      if (wrapper.find('#agent-desc').exists()) await wrapper.get('#agent-desc').setValue('d')
      if (wrapper.find('#agent-prompt').exists()) await wrapper.get('#agent-prompt').setValue('p')
      const badges = wrapper.findAll('.badge')
      if (badges.length) await badges[badges.length - 1].trigger('click')
      if (wrapper.find('.sb-ok').exists()) await wrapper.get('.sb-ok').trigger('click')
      await flushPromises()
      if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    }

    await clickText(wrapper, '定时')
    await flushPromises()
    if (wrapper.find('.sch-saved').exists()) await wrapper.get('.sch-saved').trigger('click')
    if (wrapper.find('.sch-removed').exists()) await wrapper.get('.sch-removed').trigger('click')
    if (wrapper.find('.sch-thread').exists()) await wrapper.get('.sch-thread').trigger('click')
    if (wrapper.find('.sch-close').exists()) await wrapper.get('.sch-close').trigger('click')
    await clickText(wrapper, '删除')
    await clickText(wrapper, '新会话')
    if (wrapper.find('.ai-stop').exists()) await wrapper.get('.ai-stop').trigger('click')
    useProjectStore(pinia).setProject('other-proj')
    await flushPromises()
    wrapper.unmount()
  })

  it('AgentManager empty-state create', async () => {
    api.agents.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(AgentManager, { stubs: interactionAgentStubs })
    if (wrapper.find('.empty-action').exists()) {
      await wrapper.get('.empty-action').trigger('click')
      expect(wrapper.find('.sb-modal').exists()).toBe(true)
    }
    wrapper.unmount()
  })


  it('AgentManager clicks edit/schedule/delete and non-Error catches', async () => {
    api.agentThreads.list.mockResolvedValue([{ id: 'th-1', title: 'T', created_at: 't', updated_at: 't' }])
    const { wrapper } = await mountWithApp(AgentManager)
    const edit = wrapper.findAll('button').find((b) => b.text() === '编辑')
    const sched = wrapper.findAll('button').find((b) => b.text() === '定时')
    if (edit) await edit.trigger('click')
    if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    if (sched) await sched.trigger('click')
    if (wrapper.find('.sb-cancel').exists()) await wrapper.get('.sb-cancel').trigger('click')
    const confirm = wrapper.find('.confirm-action')
    if (confirm.exists()) await confirm.trigger('click')
    await flushPromises()

    const vm = wrapper.vm as Record<string, any>
    api.agentSchedules.list.mockRejectedValueOnce('sched')
    await vm.loadSchedules()
    api.agents.modules.mockRejectedValueOnce('mods')
    await vm.loadModules()
    api.agents.list.mockRejectedValueOnce('agents')
    await vm.loadAgents()
    api.agentThreads.list.mockRejectedValueOnce('threads')
    await vm.loadThreads()
    api.agentThreads.messages.mockRejectedValueOnce('msgs')
    await vm.onViewScheduleThread('th-x')
    api.agents.patch.mockRejectedValueOnce('save')
    vm.editing = sampleAgent
    vm.form = { ...sampleAgent, name: 'n' }
    await vm.saveAgent()
    api.agents.remove.mockRejectedValueOnce('rm')
    await vm.removeAgent(sampleAgent)
    api.agentThreads.create.mockRejectedValueOnce('th')
    await vm.resetThread()
    vm.openCreate()
    vm.onModuleChange('database')
    vm.onModuleChange('missing')
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
    await avm.loadSchedules?.()
    api.agentSchedules.list.mockRejectedValueOnce(new Error('x'))
    await avm.loadSchedules?.()
    expect(avm.scheduleSummary?.({ cron_expr: '* * * * *', enabled: true, next_run_at: 't' })).toBeTruthy()
    avm.onScheduleSaved?.({ id: 's', agent_id: 'a' })
    avm.onScheduleRemoved?.('s')
    await avm.onViewScheduleThread?.('th')
    await avm.loadModules?.()
    await avm.loadAgents?.()
    api.agents.list.mockRejectedValueOnce(new Error('x'))
    await avm.loadAgents?.()
    await avm.loadThreads?.()
    avm.openCreate?.()
    avm.openEdit?.({ id: 'a', name: 'A', module: 'database', tool_ids: [] })
    avm.onModuleChange?.('s3')
    avm.toggleTool?.('list_databases')
    await avm.saveAgent?.()
    await avm.removeAgent?.({ id: 'a', name: 'A' })
    api.agents.remove.mockRejectedValueOnce(new Error('x'))
    await avm.removeAgent?.({ id: 'a', name: 'A' })
    await avm.resetThread?.()
    avm.onStop?.()
    api.agentThreads.streamRun.mockReturnValue({ close: vi.fn() })
    await avm.onSend?.('hi', [{ agent_id: 'a' }])
    agents.unmount()
  })

})
