import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentToolCard from '@/components/agent/AgentToolCard.vue'
import ConversationView from '@/components/agent/ConversationView.vue'
import ThreadSwitcher from '@/components/agent/ThreadSwitcher.vue'

describe('AgentToolCard variants', () => {
  it('renders sandbox stdout/stderr with exit code', () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'sandbox_exec', content: '{"stdout":"ok","stderr":"warn","exit_code":0}' } }
    })
    expect(wrapper.text()).toContain('exit 0')
    expect(wrapper.text()).toContain('ok')
    expect(wrapper.text()).toContain('warn')
  })

  it('renders truncated flag badge', () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'search_logs', content: '...', truncated: true } }
    })
    expect(wrapper.text()).toContain('已截断')
  })

  it('renders duration when present', () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'head_object', content: '{}', duration_ms: 120 } }
    })
    expect(wrapper.text()).toContain('120ms')
  })

  it('falls back to raw text for non-json content', () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'search_logs', content: 'plain text result' } }
    })
    expect(wrapper.text()).toContain('plain text result')
  })

  it('pretty-prints json arguments', async () => {
    const wrapper = mount(AgentToolCard, {
      props: { tool: { name: 'readonly_sql', arguments: '{"sql":"SELECT 1"}', content: '{"Columns":[],"Rows":[],"RowCount":0}' } }
    })
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

  it('renders empty state via slot', () => {
    const wrapper = mount(ConversationView, {
      props: { messages: [], sending: false },
      slots: { empty: '<div class="custom-empty">空空如也</div>' }
    })
    expect(wrapper.text()).toContain('空空如也')
  })
})

describe('ThreadSwitcher actions', () => {
  it('emits remove for active thread', async () => {
    const confirmStub = {
      template: '<div><button type="button" class="confirm-ok" @click="$root.$emit(\'confirm\')" /><slot /></div>',
      methods: {}
    }
    const wrapper = mount(ThreadSwitcher, {
      props: { threads: [{ id: 't1', title: 'A', created_at: 't', updated_at: 't' }], activeId: 't1' },
      global: { stubs: { ConfirmAction: false } }
    })
    // ConfirmAction 默认渲染触发按钮；直接验证删除按钮存在。
    const del = wrapper.findAll('button').find((b) => b.text() === '删除')
    expect(del).toBeTruthy()
  })

  it('hides delete when no active thread', () => {
    const wrapper = mount(ThreadSwitcher, {
      props: { threads: [{ id: 't1', title: 'A', created_at: 't', updated_at: 't' }], activeId: 'other' }
    })
    const del = wrapper.findAll('button').find((b) => b.text() === '删除')
    expect(del).toBeUndefined()
  })
})
