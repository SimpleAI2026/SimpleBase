import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentCard from '@/components/agent/AgentCard.vue'
import AgentToolCard from '@/components/agent/AgentToolCard.vue'
import ConversationView from '@/components/agent/ConversationView.vue'
import ThreadSwitcher from '@/components/agent/ThreadSwitcher.vue'

const sampleAgent = {
  id: 'ag-1', name: 'Database', module: 'database', description: '查库助手',
  system_prompt: '', tool_ids: ['list_databases'], builtin_key: 'general',
  team_enabled: false, created_at: 't', updated_at: 't'
}

describe('AgentCard', () => {
  it('renders the compact row and emits select', async () => {
    // planv6.0 §3.3：条目收窄成 图标 + 名称 + 一行描述；
    // 内置标记改为小圆点，编辑/定时/删除移到宿主头部的「⋯」菜单。
    const wrapper = mount(AgentCard, { props: { agent: sampleAgent, active: true } })
    expect(wrapper.text()).toContain('Database')
    expect(wrapper.text()).toContain('查库助手')
    const row = wrapper.get('button[aria-label="选择 Agent Database"]')
    expect(row.attributes('aria-pressed')).toBe('true')
    expect(row.find('[data-agent-icon="databases"]').exists()).toBe(true)
    await row.trigger('click')
    expect(wrapper.emitted('select')).toHaveLength(1)
    // 操作按钮不再出现在条目里
    expect(wrapper.findAll('button')).toHaveLength(1)
  })
})

describe('AgentToolCard', () => {
  it('folds failed results by default and marks them red (BUG-02)', async () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'readonly_sql', content: '{"error":"write not allowed","is_error":true}' } }
    })
    const summary = wrapper.get('[data-tool-summary="readonly_sql"]')
    expect(summary.attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('[data-status="error"]').classes()).toContain('border-destructive/40')
    expect(summary.find('.text-destructive').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('write not allowed')
    await summary.trigger('click')
    expect(wrapper.text()).toContain('失败')
    expect(wrapper.text()).toContain('write not allowed')
  })

  it('detects error structure without is_error flag', async () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'list_databases', content: '{"error":"boom"}' } }
    })
    expect(wrapper.get('[data-tool-summary="list_databases"]').attributes('aria-expanded')).toBe('false')
    await wrapper.get('[data-tool-summary="list_databases"]').trigger('click')
    expect(wrapper.text()).toContain('失败')
  })

  it('renders running state without content', () => {
    const wrapper = mount(AgentToolCard, { props: { tool: { name: 'search_logs', arguments: '{"q":"x"}' } } })
    expect(wrapper.text()).toContain('正在调用')
    expect(wrapper.text()).toContain('search_logs')
  })

  it('renders sql table result', async () => {
    // 成功态默认折叠成一行摘要，展开后才是表格（planv6.0 §3.6 #3）。
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'readonly_sql', content: '{"Columns":["id"],"Rows":[[1]],"RowCount":1}' } }
    })
    expect(wrapper.text()).toContain('共 1 行')
    expect(wrapper.text()).not.toContain('id')
    await wrapper.get('[data-tool-summary="readonly_sql"]').trigger('click')
    expect(wrapper.text()).toContain('id')
  })
})

describe('ConversationView', () => {
  it('renders user/assistant bubbles, tool cards and loading', () => {
    const wrapper = mount(ConversationView, {
      props: {
        messages: [
          { role: 'user', content: '查库' },
          { role: 'assistant', content: '结果', toolCalls: [{ name: 'list_databases', content: '[]' }] }
        ],
        sending: true,
        canRetry: false
      }
    })
    expect(wrapper.text()).toContain('查库')
    expect(wrapper.text()).toContain('list_databases')
    expect(wrapper.find('[role="status"][aria-label="正在加载"]').exists()).toBe(true)
    const circles = wrapper.findAll('.size-7.rounded-full')
    expect(circles.length).toBe(2)
    const userIcon = wrapper.get('[data-user-avatar] svg')
    expect(userIcon.classes()).toContain('size-4')
    // planv6.0 v2 D-A1：头像恒为模块图标，忙碌态不再占用头像位。
    const assistantIcon = wrapper.get('[data-agent-icon] svg')
    expect(assistantIcon.classes()).not.toContain('animate-spin')
    expect(wrapper.find('[data-typing-dots]').exists()).toBe(true)
    expect(wrapper.findAll('[role="status"][aria-label="正在加载"]')).toHaveLength(1)
  })

  it('keeps the module avatar and stops the typing dots after completion', async () => {
    const wrapper = mount(ConversationView, {
      props: { messages: [{ role: 'assistant', content: '完成', module: 'database' }], sending: true }
    })
    expect(wrapper.get('[data-agent-icon="databases"] svg').classes()).not.toContain('animate-spin')
    expect(wrapper.find('[data-typing-dots]').exists()).toBe(true)
    await wrapper.setProps({ sending: false })
    expect(wrapper.find('[data-typing-dots]').exists()).toBe(false)
    // 待确认时由确认卡承担状态，不再显示三点。
    await wrapper.setProps({ sending: true, confirmation: { callId: 'c1', argv: [], message: '确认' } })
    expect(wrapper.find('[data-typing-dots]').exists()).toBe(false)
    expect(wrapper.get('[data-agent-icon="databases"] svg').exists()).toBe(true)
  })

  it('renders assistant markdown safely while preserving user text', () => {
    const wrapper = mount(ConversationView, {
      props: { messages: [
        { role: 'user', content: '**输入**' },
        { role: 'assistant', content: '**结果**\n\n- 第一项\n\n<script>alert(1)</script>' }
      ] }
    })
    expect(wrapper.get('strong').text()).toBe('结果')
    expect(wrapper.get('li').text()).toBe('第一项')
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.text()).toContain('**输入**')
  })

  it('shows error text with retry button when canRetry', async () => {
    const wrapper = mount(ConversationView, {
      props: {
        messages: [{ role: 'assistant', content: '', error: '超时' }],
        sending: false,
        canRetry: true
      }
    })
    expect(wrapper.text()).toContain('超时')
    const btn = wrapper.findAll('button').find((b) => b.text() === '重试')
    expect(btn).toBeTruthy()
    await btn!.trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })
})

describe('ThreadSwitcher', () => {
  it('lists threads and emits create', async () => {
    const wrapper = mount(ThreadSwitcher, {
      props: { threads: [{ id: 't1', title: '会话一', created_at: 't', updated_at: new Date().toISOString() }], activeId: 't1' }
    })
    // 新建改为图标按钮（planv6.0 §3.6 #7），文案在 aria-label 上。
    const create = wrapper.get('button[aria-label="新会话"]')
    await create.trigger('click')
    expect(wrapper.emitted('create')).toHaveLength(1)
  })
})
