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
  it('renders builtin badge, module label and emits actions', async () => {
    const wrapper = mount(AgentCard, { props: { agent: sampleAgent, active: true } })
    expect(wrapper.text()).toContain('Database')
    expect(wrapper.text()).toContain('内置')
    expect(wrapper.text()).toContain('数据库') // database 模块标签
    await wrapper.get('button[aria-label="选择 Agent Database"]').trigger('click')
    expect(wrapper.emitted('select')).toHaveLength(1)
    const buttons = wrapper.findAll('button')
    await buttons.find((b) => b.text() === '编辑')!.trigger('click')
    expect(wrapper.emitted('edit')).toHaveLength(1)
  })
})

describe('AgentToolCard', () => {
  it('marks error results (BUG-02)', () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'readonly_sql', content: '{"error":"write not allowed","is_error":true}' } }
    })
    expect(wrapper.text()).toContain('失败')
    expect(wrapper.text()).toContain('write not allowed')
  })

  it('detects error structure without is_error flag', () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'list_databases', content: '{"error":"boom"}' } }
    })
    expect(wrapper.text()).toContain('失败')
  })

  it('renders running state without content', () => {
    const wrapper = mount(AgentToolCard, { props: { tool: { name: 'search_logs', arguments: '{"q":"x"}' } } })
    expect(wrapper.text()).toContain('正在调用')
    expect(wrapper.text()).toContain('search_logs')
  })

  it('renders sql table result', () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'readonly_sql', content: '{"Columns":["id"],"Rows":[[1]],"RowCount":1}' } }
    })
    expect(wrapper.text()).toContain('共 1 行')
    expect(wrapper.text()).toContain('id')
  })
})

describe('ConversationView', () => {
  it('renders user/assistant bubbles, tool cards and cursor', () => {
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
    expect(wrapper.text()).toContain('▍')
    const circles = wrapper.findAll('.size-7.rounded-full')
    expect(circles.length).toBe(2)
    const userIcon = wrapper.get('[data-user-avatar] svg')
    expect(userIcon.classes()).toContain('size-4')
    const assistantIcon = wrapper.get('[data-agent-icon] svg')
    expect(assistantIcon.classes()).toContain('size-4')
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
    const create = wrapper.findAll('button').find((b) => b.text().includes('新会话'))
    await create!.trigger('click')
    expect(wrapper.emitted('create')).toHaveLength(1)
  })
})
