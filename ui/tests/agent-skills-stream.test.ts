import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { api, resetApiMocks } from '@/test/api-mock'
import { useAgentConversation } from '@/composables/useAgentConversation'
import type { ChatMsg } from '@/composables/useAiChat'
import AgentComposer from '@/components/agent/AgentComposer.vue'
import AgentToolCard from '@/components/agent/AgentToolCard.vue'
import ConversationView from '@/components/agent/ConversationView.vue'
import { resolveSkillAlias, skillsInText } from '@/utils/agent-skills'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

function makeConv() {
  const messages: ChatMsg[] = []
  const conv = useAgentConversation({
    projectId: () => 'p1',
    threadId: () => 'th-1',
    messages: () => messages
  })
  return { conv, messages }
}

describe('agent skill stream', () => {
  beforeEach(() => {
    resetApiMocks()
    api.agentThreads.streamRun.mockReturnValue({ close: vi.fn() })
    api.agentThreads.cancel.mockResolvedValue({} as never)
    api.agentThreads.confirm.mockResolvedValue(undefined)
  })

  it('replaces streamed arguments with the full tool call', () => {
    const { conv, messages } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: '建库', mentions: [{ agent_id: 'a1' }], skills: ['database'], module: 'database' }, {})
    expect(messages[1]?.module).toBe('database')
    handlers.onToolCallDelta?.('simplebase', '{"argv"', 'c1')
    handlers.onToolCallDelta?.('simplebase', ':[]}', 'c1')
    expect(messages[1]?.toolCalls?.[0]?.arguments).toBe('{"argv":[]}')
    expect(messages[1]?.toolCalls?.[0]?.status).toBe('generating')
    handlers.onToolCall?.('simplebase', '{"argv":["database","list"]}', 'c1')
    expect(messages[1]?.toolCalls).toHaveLength(1)
    expect(messages[1]?.toolCalls?.[0]?.arguments).toBe('{"argv":["database","list"]}')
    expect(messages[1]?.toolCalls?.[0]?.status).toBe('pending')
    handlers.onToolStart?.('simplebase', 'c1')
    expect(messages[1]?.toolCalls?.[0]?.status).toBe('running')
    expect(conv.phase.value).toBe('tool')
  })

  it('marks is_error and opens confirmation', async () => {
    const { conv, messages } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    conv.start({ content: '删库', mentions: [] }, {})
    handlers.onRun?.('run-1')
    handlers.onToolCall?.('simplebase', '{"argv":["database","delete"]}', 'c9')
    handlers.onConfirmationRequired?.('c9', ['database', 'delete', '--id', 'shop'], '将执行破坏性命令')
    expect(conv.phase.value).toBe('awaiting_confirm')
    expect(conv.confirmation.value?.argv).toEqual(['database', 'delete', '--id', 'shop'])
    expect(conv.sending.value).toBe(true)
    await conv.resolveConfirm(false)
    expect(conv.phase.value).not.toBe('awaiting_confirm')
    expect(api.agentThreads.confirm).toHaveBeenCalledWith('p1', 'run-1', 'c9', false)
    handlers.onToolResult?.('simplebase', '{"ok":false}', 'c9', 3, true)
    expect(messages[1]?.toolCalls?.[0]?.is_error).toBe(true)
    expect(messages[1]?.toolCalls?.[0]?.status).toBe('error')
  })

  it('covers skill alias edges and tool-card fallbacks', () => {
    expect(resolveSkillAlias('   ')).toBe('')
    expect(resolveSkillAlias('@')).toBe('')
    expect(resolveSkillAlias('DATABASE')).toBe('database')
    expect(resolveSkillAlias('Nope', ['Nope'])).toBe('')
    expect(resolveSkillAlias('nope')).toBe('')
    expect(skillsInText('@云函数 @云函数 @未知')).toEqual(['gofunction'])

    const pending = mount(AgentToolCard, { props: { tool: { name: '', status: 'pending' } } })
    expect(pending.text()).toContain('正在调用')
    expect(pending.find('[data-status="pending"]').exists()).toBe(true)
    const inferred = mount(AgentToolCard, {
      props: { tool: { name: 'simplebase', content: '{"ok":true,"data":{"columns":["id"],"rows":[[1]],"row_count":1}}' } }
    })
    expect(inferred.find('[data-status="success"]').exists()).toBe(true)
    const raw = mount(AgentToolCard, { props: { tool: { name: 'simplebase', content: 'not-json' } } })
    expect(raw.text()).toContain('not-json')
    const ticking = mount(AgentToolCard, { props: { tool: { name: 'simplebase', status: 'running', duration_ms: 2500 } } })
    expect(ticking.text()).toContain('已运行 2s')
    const quiet = mount(AgentToolCard, { props: { tool: { name: 'simplebase', status: 'running' } } })
    expect(quiet.text()).toContain('正在调用')
    const upper = mount(AgentToolCard, {
      props: { tool: { name: 'readonly_sql', content: '{"Columns":["id"],"Rows":[[1]]}' } }
    })
    expect(upper.text()).toContain('共 1 行')
    const wrapped = mount(AgentToolCard, {
      props: { tool: { name: 'simplebase', content: '{"data":"nope","columns":["n"],"rows":[[2]],"row_count":1}' } }
    })
    expect(wrapped.text()).toContain('n')
    const sandbox = mount(AgentToolCard, {
      props: { tool: { name: 'sandbox_exec', content: '{"stdout":"hi","stderr":"no","exit_code":2}' } }
    })
    expect(sandbox.text()).toContain('exit 2')
    expect(sandbox.text()).toContain('hi')
    expect(sandbox.text()).toContain('no')
    const sandboxUpper = mount(AgentToolCard, {
      props: { tool: { name: 'sandbox_files', content: '{"Stdout":"UP","Stderr":"ERR","ExitCode":3}' } }
    })
    expect(sandboxUpper.text()).toContain('exit 3')
    expect(sandboxUpper.text()).toContain('UP')
    const sandboxDefault = mount(AgentToolCard, {
      props: { tool: { name: 'sandbox_files', content: '{"Stdout":"only"}' } }
    })
    expect(sandboxDefault.text()).toContain('exit 0')
  })

  it('covers confirmation resolution and tool-start fallbacks', async () => {
    const { conv, messages } = makeConv()
    let handlers: Record<string, (...args: unknown[]) => void> = {}
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      handlers = h as Record<string, (...args: unknown[]) => void>
      return { close: vi.fn() }
    })
    await conv.resolveConfirm(true)
    expect(api.agentThreads.confirm).not.toHaveBeenCalled()
    conv.start({ content: '重试', mentions: [], retry_of_run_id: 'old' }, {})
    expect(messages.filter((m) => m.role === 'user')).toHaveLength(0)
    expect(conv.statusText.value).toContain('思考中')
    handlers.onThinking?.(10)
    handlers.onThinking?.(12, '想')
    expect(messages[0]?.thinking).toContain('想')
    handlers.onToolStart?.('simplebase')
    expect(messages[0]?.toolCalls?.[0]?.status).toBe('running')
    handlers.onThinking?.(20, '仍在工具中')
    expect(conv.phase.value).toBe('tool')
    handlers.onToolCallDelta?.('simplebase', '{"a"', 'c2')
    handlers.onToolCall?.('simplebase', '{"a":1}', 'c2')
    handlers.onToolCallDelta?.('simplebase', 'x', 'c2')
    expect(messages[0]?.toolCalls?.find((c) => c.call_id === 'c2')?.arguments).toBe('{"a":1}')
    handlers.onToolStart?.('other', 'missing')
    handlers.onConfirmationRequired?.('missing', ['kv', 'exec'], '确认')
    expect(conv.statusText.value).toBe('等待确认')
    handlers.onConfirmationResolved?.('other', false)
    expect(conv.confirmation.value?.callId).toBe('missing')
    handlers.onConfirmationRequired?.('missing', ['kv', 'exec'], '确认')
    handlers.onConfirmationResolved?.('missing', true)
    expect(conv.confirmation.value).toBeNull()
    expect(conv.phase.value).toBe('tool')
    expect(conv.statusText.value).toContain('执行工具')
    handlers.onToolCall?.('anon', '{}')
    handlers.onToolResult?.('ghost', '{"ok":false}', 'no-such', 0, true)
    handlers.onToolProgress?.('simplebase', 3000, 'c2')
    expect(conv.elapsedMs.value).toBe(3000)
    handlers.onToolResult?.('simplebase', '{"ok":true}', undefined, 1, false)
    handlers.onUsage?.({ duration_ms: 1500, prompt_tokens: 2, completion_tokens: 3, reasoning_tokens: 0, tool_calls: 1 })
    handlers.onEnd?.('stop')
    expect(conv.phase.value).toBe('done')
    expect(conv.statusText.value).toContain('完成')
    handlers.onEnd?.('canceled')
    expect(conv.statusText.value).toBe('已停止')
    api.agentThreads.confirm.mockRejectedValueOnce(new Error('nope'))
    handlers.onRun?.('run-x')
    handlers.onConfirmationRequired?.('c2', [], '')
    await conv.resolveConfirm(true)
    expect(api.agentThreads.confirm).toHaveBeenCalled()
    handlers.onError?.(Object.assign(new Error('失败'), { code: 'x' }))
    expect(conv.phase.value).toBe('error')
    expect(conv.statusText.value).toContain('失败')
  })

  it('stop calls cancel', () => {
    const { conv } = makeConv()
    api.agentThreads.streamRun.mockImplementation((_p, _t, _r, h) => {
      ;(h as { onRun?: (id: string) => void }).onRun?.('run-2')
      return { close: vi.fn() }
    })
    conv.start({ content: '停', mentions: [] }, {})
    conv.stop()
    expect(api.agentThreads.cancel).toHaveBeenCalledWith('p1', 'run-2')
    expect(conv.phase.value).toBe('canceled')
  })

  it('@云函数 is a skill and @Database stays an agent mention', async () => {
    expect(resolveSkillAlias('云函数', ['Database'])).toBe('gofunction')
    expect(resolveSkillAlias('Database', ['Database'])).toBe('')
    expect(resolveSkillAlias('数据库', ['数据库'])).toBe('database')
    expect(skillsInText('@云函数 @定时任务', ['Database'])).toEqual(['gofunction', 'cron'])
    expect(skillsInText('@Database', ['Database'])).toEqual([])

    const wrapper = mount(AgentComposer, {
      props: {
        modelValue: '@云函数 写一个 Hello',
        mentionAgents: [{ id: 'db', name: 'Database', module: 'database' }]
      },
      attachTo: document.body
    })
    await wrapper.get('textarea').setValue('@云函数 写一个 Hello')
    await wrapper.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: false })
    expect(wrapper.emitted('send')?.[0]).toEqual(['@云函数 写一个 Hello', [], ['gofunction']])
    wrapper.unmount()
  })

  it('shows argv in ConfirmAction and a failure badge', async () => {
    const view = mount(ConversationView, {
      props: {
        messages: [{ role: 'assistant', content: '', module: 'database', toolCalls: [] }],
        confirmation: { message: '将执行', argv: ['database', 'delete', '--id', 'shop'] }
      },
      attachTo: document.body
    })
    await flushPromises()
    const prompt = view.findComponent({ name: 'ConfirmAction' })
    expect(prompt.exists()).toBe(true)
    expect(prompt.props('description')).toContain('database delete --id shop')
    expect(prompt.props('open')).toBe(true)
    expect(view.find('[data-agent-icon="databases"]').exists()).toBe(true)
    await prompt.vm.$emit('confirm')
    expect(view.emitted('confirm')).toBeTruthy()
    view.unmount()

    const denied = mount(ConversationView, {
      props: {
        messages: [{ role: 'assistant', content: 'x', module: 'cron' }],
        confirmation: { message: '将执行', argv: [] }
      },
      attachTo: document.body
    })
    await flushPromises()
    expect(denied.find('[data-agent-icon="cron-jobs"]').exists()).toBe(true)
    const denyPrompt = denied.findComponent({ name: 'ConfirmAction' })
    expect(denyPrompt.props('description')).toBe('将执行')
    await denyPrompt.vm.$emit('cancel')
    expect(denied.emitted('deny')).toBeTruthy()
    denied.unmount()

    const card = mount(AgentToolCard, {
      props: { tool: { name: 'simplebase', content: '{"ok":false,"error":{"code":"denied"}}', is_error: true, status: 'error' } }
    })
    expect(card.find('[data-status="error"]').exists()).toBe(true)
    expect(card.text()).toContain('失败')

    const running = mount(AgentToolCard, {
      props: { tool: { name: 'simplebase', status: 'running' } }
    })
    expect(running.find('[data-status="running"]').exists()).toBe(true)
    expect(running.text()).toContain('正在调用')

    const table = mount(AgentToolCard, {
      props: {
        tool: {
          name: 'simplebase',
          status: 'success',
          content: '{"ok":true,"data":{"columns":["amount"],"rows":[[10]],"row_count":1}}'
        }
      }
    })
    expect(table.text()).toContain('amount')
    expect(table.text()).toContain('10')
  })
})
