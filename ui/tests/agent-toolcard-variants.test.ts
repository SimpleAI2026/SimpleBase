import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentToolCard from '@/components/agent/AgentToolCard.vue'
import ConversationView from '@/components/agent/ConversationView.vue'
import ThreadSwitcher from '@/components/agent/ThreadSwitcher.vue'

describe('AgentToolCard variants', () => {
  it('renders sandbox stdout/stderr with exit code after expanding', async () => {
    // 成功态默认折叠（planv6.0 §3.6 #3）：展开后才渲染 stdout/stderr。
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'sandbox_exec', content: '{"stdout":"ok","stderr":"warn","exit_code":0}' } }
    })
    await wrapper.get('[data-tool-summary="sandbox_exec"]').trigger('click')
    expect(wrapper.text()).toContain('exit 0')
    expect(wrapper.text()).toContain('ok')
    expect(wrapper.text()).toContain('warn')
  })

  it('renders truncated flag badge after expanding', async () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'search_logs', content: '...', truncated: true } }
    })
    await wrapper.get('[data-tool-summary="search_logs"]').trigger('click')
    expect(wrapper.text()).toContain('已截断')
  })

  it('renders duration when present', () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'head_object', content: '{}', duration_ms: 120 } }
    })
    expect(wrapper.text()).toContain('120ms')
  })

  it('falls back to raw text for non-json content', async () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'search_logs', content: 'plain text result' } }
    })
    await wrapper.get('[data-tool-summary="search_logs"]').trigger('click')
    expect(wrapper.text()).toContain('plain text result')
  })

  it('pretty-prints json arguments', async () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'readonly_sql', arguments: '{"sql":"SELECT 1"}', content: '{"Columns":[],"Rows":[],"RowCount":0}' } }
    })
    await wrapper.get('[data-tool-summary="readonly_sql"]').trigger('click') // 展开折叠行
    const summary = wrapper.findAll('summary')
    await summary[0].trigger('click') // 参数展开
    expect(wrapper.text()).toContain('SELECT 1')
    expect(wrapper.text()).toContain('共 0 行')
  })
})

describe('ConversationView extra branches', () => {
  it('renders thinking details and canceled marker', () => {
    const wrapper = mount(ConversationView, {
      props: {
        messages: [
          { role: 'assistant', content: '答', thinking: '推理链', canceled: true, toolCalls: [] }
        ],
        sending: false
      }
    })
    expect(wrapper.text()).toContain('思考过程')
    expect(wrapper.text()).toContain('已停止')
  })

  it('follows streaming content, tools and confirmation changes', async () => {
    const wrapper = mount(ConversationView, {
      props: { messages: [{ role: 'assistant' as const, content: '', toolCalls: [] }], sending: true }
    })
    const scroller = wrapper.findComponent({ name: 'MessageScroller' })
    const initial = scroller.props('followKey')
    const messages = [{ role: 'assistant' as const, content: '首句', toolCalls: [] as { name: string; status?: string; content?: string }[] }]
    await wrapper.setProps({ messages })
    expect(scroller.props('followKey')).not.toBe(initial)
    const textKey = scroller.props('followKey')
    await wrapper.setProps({ messages: [{ ...messages[0], toolCalls: [{ name: 'query', status: 'running' }] }] })
    expect(scroller.props('followKey')).not.toBe(textKey)
    const toolKey = scroller.props('followKey')
    await wrapper.setProps({ messages: [{ ...messages[0], toolCalls: [{ name: 'query', status: 'success', content: 'ok' }] }] })
    expect(scroller.props('followKey')).not.toBe(toolKey)
    const resultKey = scroller.props('followKey')
    await wrapper.setProps({ confirmation: { message: '继续？', argv: [] } })
    expect(scroller.props('followKey')).not.toBe(resultKey)
  })

  it('renders empty state via slot', () => {
    const wrapper = mount(ConversationView, {
      props: { messages: [], sending: false },
      slots: { empty: '<div class="custom-empty">空空如也</div>' }
    })
    expect(wrapper.text()).toContain('空空如也')
  })
})

describe('ThreadSwitcher actions', () => {
  it('only exposes the new-thread action (delete moved to the host menu)', () => {
    const wrapper = mount(ThreadSwitcher, {
      props: { threads: [{ id: 't1', title: 'A', created_at: 't', updated_at: 't' }], activeId: 't1' },
      global: { stubs: { ConfirmAction: false } }
    })
    // planv6.0 §3.6 #7：删除会话收进宿主头部的「⋯」菜单。
    expect(wrapper.findAll('button').find((b) => b.text() === '删除')).toBeUndefined()
    expect(wrapper.get('button[aria-label="新会话"]')).toBeTruthy()
  })

  it('hides delete when no active thread', () => {
    const wrapper = mount(ThreadSwitcher, {
      props: { threads: [{ id: 't1', title: 'A', created_at: 't', updated_at: 't' }], activeId: 'other' }
    })
    const del = wrapper.findAll('button').find((b) => b.text() === '删除')
    expect(del).toBeUndefined()
  })
})
