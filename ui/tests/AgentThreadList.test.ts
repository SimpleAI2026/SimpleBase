import { describe, expect, it, vi } from 'vitest'
import { mountWithApp } from '@/test/helpers'
import AgentThreadList from '@/components/ai/AgentThreadList.vue'

const threads = [
  { id: 't1', title: '第一段', last_message_preview: 'hello', created_at: new Date().toISOString(), updated_at: new Date().toISOString() },
  { id: 't2', title: '第二段', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }
]

describe('AgentThreadList', () => {
  it('selects, creates, renames, and exposes delete confirmation', async () => {
    const { wrapper } = await mountWithApp(AgentThreadList, { props: { threads, activeId: 't1', nextCursor: 't2' }, stubs: { ConfirmAction: { template: '<div><slot /></div>' } } })
    expect(wrapper.text()).toContain('hello')
    expect(wrapper.get('button[aria-pressed="true"]').text()).toContain('第一段')
    await wrapper.get('button[aria-pressed="false"]').trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual(['t2'])
    await wrapper.findAll('button').find((button) => button.text() === '新会话')?.trigger('click')
    expect(wrapper.emitted('create')).toHaveLength(1)
    await wrapper.get('[aria-label="重命名会话 第一段"]').trigger('click')
    await wrapper.get('[aria-label="重命名 第一段"]').setValue('新标题')
    await wrapper.findAll('button').find((button) => button.text() === '保存')?.trigger('click')
    expect(wrapper.emitted('rename')?.[0]).toEqual(['t1', '新标题'])
    expect(wrapper.get('[aria-label="删除会话 第一段"]').exists()).toBe(true)
    await wrapper.findAll('button').find((button) => button.text() === '加载更多')?.trigger('click')
    expect(wrapper.emitted('more')).toHaveLength(1)
    vi.restoreAllMocks()
  })
})
