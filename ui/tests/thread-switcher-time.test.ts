import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { relativeTime } from '@/utils/relative-time'
import ThreadSwitcher from '@/components/agent/ThreadSwitcher.vue'

describe('relativeTime', () => {
  it('formats fresh/minutes/hours/days', () => {
    expect(relativeTime(new Date().toISOString())).toBe('刚刚')
    expect(relativeTime(new Date(Date.now() - 5 * 60_000).toISOString())).toBe('5分钟前')
    expect(relativeTime(new Date(Date.now() - 3 * 3_600_000).toISOString())).toBe('3小时前')
    expect(relativeTime(new Date(Date.now() - 2 * 86_400_000).toISOString())).toBe('2天前')
  })

  it('returns empty for invalid date', () => {
    expect(relativeTime('not-a-date')).toBe('')
  })
})

describe('ThreadSwitcher render', () => {
  const passthroughStubs = {
    Select: { props: ['modelValue'], template: '<div class="sel"><slot /></div>' },
    SelectTrigger: { template: '<div class="sel-tr"><slot /></div>' },
    SelectValue: { props: ['placeholder'], template: '<span class="sel-val">{{ placeholder || "选择会话" }}</span>' },
    SelectContent: { template: '<div class="sel-ct"><slot /></div>' },
    SelectGroup: { template: '<div class="sel-gr"><slot /></div>' },
    SelectItem: { props: ['value'], template: '<div class="sel-item"><slot /></div>' }
  }

  it('renders thread title with relative time and action buttons', () => {
    const wrapper = mount(ThreadSwitcher, {
      props: { threads: [{ id: 't1', title: '会话A', created_at: 't', updated_at: new Date().toISOString() }], activeId: 't1' },
      global: { stubs: passthroughStubs }
    })
    expect(wrapper.text()).toContain('会话A')
    expect(wrapper.text()).toContain('刚刚')
    expect(wrapper.text()).toContain('新会话')
    expect(wrapper.text()).toContain('删除')
  })

  it('emits create and remove on button click', async () => {
    const wrapper = mount(ThreadSwitcher, {
      props: { threads: [{ id: 't1', title: '会话A', created_at: 't', updated_at: new Date().toISOString() }], activeId: 't1' },
      global: { stubs: passthroughStubs }
    })
    await wrapper.findAll('button').find((b) => b.text() === '新会话')!.trigger('click')
    expect(wrapper.emitted('create')).toHaveLength(1)
  })

  it('renders without active thread match', () => {
    const wrapper = mount(ThreadSwitcher, {
      props: { threads: [], activeId: 'x' },
      global: { stubs: passthroughStubs }
    })
    expect(wrapper.text()).toContain('新会话')
    expect(wrapper.text()).not.toContain('删除')
  })
})
