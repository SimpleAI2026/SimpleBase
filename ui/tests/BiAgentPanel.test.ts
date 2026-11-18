import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import BiAgentPanel from '@/components/bi/BiAgentPanel.vue'
import { uiStubs } from '@/test/helpers'

// Button 用真实组件，禁用态要落在真实 DOM 上；AgentMarkdown 用真实组件（它会走 DOMPurify）。
const stubs = { ...uiStubs, Button: false, Spinner: false }
const mountPanel = (props: Record<string, unknown> = {}) =>
  mount(BiAgentPanel, { props: { sending: false, ...props } as never, global: { stubs }, slots: { pending: '<div data-testid="slot-pending">cards</div>' } })

const input = (w: ReturnType<typeof mountPanel>) => w.get('[data-testid="bi-agent-input"]')

describe('BiAgentPanel input', () => {
  it('uses the prototype placeholder', () => {
    expect(input(mountPanel()).attributes('placeholder')).toBe('输入图表搭建需求，发送后由 Agent 生成')
  })

  it('Enter sends the trimmed text and clears the box', async () => {
    const wrapper = mountPanel()
    await input(wrapper).setValue('  近 30 天各状态订单  ')
    await input(wrapper).trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('send')).toEqual([['近 30 天各状态订单']])
    expect((input(wrapper).element as HTMLTextAreaElement).value).toBe('')
  })

  it('Shift+Enter inserts a newline instead of sending', async () => {
    const wrapper = mountPanel()
    await input(wrapper).setValue('第一行')
    await input(wrapper).trigger('keydown', { key: 'Enter', shiftKey: true })
    expect(wrapper.emitted('send')).toBeUndefined()
  })

  it('Enter during IME composition confirms the candidate and does not send', async () => {
    const wrapper = mountPanel()
    await input(wrapper).setValue('shi tu')
    await input(wrapper).trigger('keydown', { key: 'Enter', isComposing: true })
    expect(wrapper.emitted('send')).toBeUndefined()
  })

  it('does not send blank input', async () => {
    const wrapper = mountPanel()
    await input(wrapper).setValue('   ')
    await input(wrapper).trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('send')).toBeUndefined()
    expect(wrapper.get('[data-action="send"]').attributes('disabled')).toBeDefined()
  })

  it('the send button sends and is enabled only with text', async () => {
    const wrapper = mountPanel()
    await input(wrapper).setValue('hello')
    expect(wrapper.get('[data-action="send"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('[data-action="send"]').trigger('click')
    expect(wrapper.emitted('send')).toEqual([['hello']])
  })
})

describe('BiAgentPanel running state', () => {
  it('turns the send button into stop while running and locks the input', async () => {
    const wrapper = mountPanel({ sending: true, statusText: '思考中' })
    expect(wrapper.find('[data-action="send"]').exists()).toBe(false)
    expect(input(wrapper).attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="bi-agent-status"]').text()).toContain('思考中')
    await wrapper.get('[data-action="stop"]').trigger('click')
    expect(wrapper.emitted('stop')).toHaveLength(1)
  })

  it('falls back to a generic status while running without status text', () => {
    expect(mountPanel({ sending: true }).get('[data-testid="bi-agent-status"]').text()).toContain('正在生成图表')
  })

  it('Enter while running does nothing', async () => {
    const wrapper = mountPanel({ sending: true })
    await input(wrapper).trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('send')).toBeUndefined()
  })
})

describe('BiAgentPanel disabled', () => {
  it('shows the reason, locks input and send, and never emits', async () => {
    const wrapper = mountPanel({ disabledReason: '系统项目只读，不能生成图表' })
    expect(wrapper.get('[data-testid="bi-agent-disabled"]').text()).toBe('系统项目只读，不能生成图表')
    expect(input(wrapper).attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-action="send"]').attributes('disabled')).toBeDefined()
    await input(wrapper).trigger('keydown', { key: 'Enter' })
    await wrapper.get('[data-action="send"]').trigger('click')
    expect(wrapper.emitted('send')).toBeUndefined()
  })

  it('clears a half-typed draft when it becomes disabled', async () => {
    const wrapper = mountPanel()
    await input(wrapper).setValue('draft')
    await wrapper.setProps({ disabledReason: '没有权限' })
    expect((input(wrapper).element as HTMLTextAreaElement).value).toBe('')
  })

  it('no reason means no hint', () => {
    expect(mountPanel().find('[data-testid="bi-agent-disabled"]').exists()).toBe(false)
  })
})

describe('BiAgentPanel results', () => {
  it('shows failures with a retry that emits', async () => {
    const wrapper = mountPanel({ failure: '模型额度已用完', canRetry: true })
    expect(wrapper.get('[data-testid="bi-agent-failure"]').text()).toContain('模型额度已用完')
    await wrapper.get('[data-action="retry"]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
    expect(mountPanel({ failure: 'x', canRetry: false }).find('[data-action="retry"]').exists()).toBe(false)
  })

  it('renders the assistant prose as sanitized markdown', () => {
    const wrapper = mountPanel({ notes: '**重点**<img src=x onerror=alert(1)><script>alert(1)</script>' })
    const html = wrapper.get('[data-testid="bi-agent-notes"]').html()
    expect(html).toContain('<strong>重点</strong>')
    expect(html).not.toContain('onerror')
    expect(html).not.toContain('<script')
  })

  it('lists rejected blocks with a human reason and a raw code fallback', () => {
    const wrapper = mountPanel({
      rejected: [
        { title: '幽灵图', reason: 'field_unknown' },
        { title: '堆积', reason: 'stacked_needs_additive' },
        { title: '版本', reason: 'version_unsupported' },
        { title: '奇怪', reason: 'brand_new_code' }
      ]
    })
    const text = wrapper.get('[data-testid="bi-agent-rejected"]').text()
    expect(text).toContain('图表「幽灵图」未采用：字段不存在或未登记')
    expect(text).toContain('只支持记录数与求和')
    expect(text).toContain('图表配置版本不受支持')
    // 没有文案的未知原因码原样展示，保证用户至少看得到原因。
    expect(text).toContain('brand_new_code')
  })

  it('offers "ask again" when the reply had no usable block', async () => {
    const wrapper = mountPanel({ noBlock: true, lastPrompt: '画图' })
    expect(wrapper.get('[data-testid="bi-agent-noblock"]').text()).toContain('没有产出可用的图表配置')
    await wrapper.get('[data-action="ask-again"]').trigger('click')
    expect(wrapper.emitted('ask-again')).toHaveLength(1)
    // 没发过请求、正在运行或已失败时不提示。
    expect(mountPanel({ noBlock: true, lastPrompt: '' }).find('[data-testid="bi-agent-noblock"]').exists()).toBe(false)
    expect(mountPanel({ noBlock: true, lastPrompt: 'x', sending: true }).find('[data-testid="bi-agent-noblock"]').exists()).toBe(false)
    expect(mountPanel({ noBlock: true, lastPrompt: 'x', failure: 'boom' }).find('[data-testid="bi-agent-noblock"]').exists()).toBe(false)
  })

  it('renders the pending slot with a title and new-chat button only when there is something pending', async () => {
    const pending = mountPanel({ hasPending: true, hasHistory: true })
    expect(pending.get('[data-testid="bi-agent-pending"]').text()).toContain('Agent 生成的图表')
    expect(pending.find('[data-testid="slot-pending"]').exists()).toBe(true)
    await pending.get('[data-action="new-chat"]').trigger('click')
    expect(pending.emitted('new-chat')).toHaveLength(1)

    const idle = mountPanel()
    expect(idle.find('[data-testid="bi-agent-pending"]').exists()).toBe(false)
    expect(idle.find('[data-action="new-chat"]').exists()).toBe(false)
  })

  it('offers a new chat when there is history even without pending results', async () => {
    const wrapper = mountPanel({ hasHistory: true })
    expect(wrapper.find('[data-testid="bi-agent-pending"]').exists()).toBe(false)
    await wrapper.get('[data-action="new-chat"]').trigger('click')
    expect(wrapper.emitted('new-chat')).toHaveLength(1)
  })
})
