import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import CollectionPanel from '@/components/databases/CollectionPanel.vue'
import CollectionSchemaModal from '@/components/databases/CollectionSchemaModal.vue'
import { readyDb } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

describe('collection schema template interactions', () => {
  beforeEach(() => resetApiMocks())

  it('opens and closes the schema editor from a collection row', async () => {
    const w = mount(CollectionPanel, { props: { projectId: 'p', database: readyDb }, global: { stubs: uiStubs } })
    await flushPromises()
    await w.findAll('button').find((b) => b.text() === '编辑 Schema')!.trigger('click')
    await flushPromises()
    expect(w.find('.sb-modal').exists()).toBe(true)
    await w.get('.sb-modal-close').trigger('click')
    expect(w.find('.sb-modal').exists()).toBe(false)
  })

  it('exercises every field editor, switch and delete callback', async () => {
    api.db.getCollectionSchema.mockResolvedValue({
      name: 'users',
      fields: [{ name: 'state', type: 'select', required: true, unique: true, hidden: true, readonly: true, default: 'open', options: { values: ['open', 'closed'], min: 1, max: 2, pattern: 'o' } }]
    })
    const w = mount(CollectionSchemaModal, {
      props: { open: true, projectId: 'p', databaseId: 'd', collection: 'users' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const inputs = w.findAll('input')
    for (const [index, input] of inputs.entries()) await input.setValue(index === 2 ? '"open"' : '1')
    await w.get('.select-emit').trigger('click')
    for (const toggle of w.findAll('.switch')) await toggle.trigger('click')
    await w.findAll('button').find((b) => b.text() === '删除')!.trigger('click')
    expect(w.text()).toContain('暂无字段')
    await w.get('.sb-modal-close').trigger('click')
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
  })
})
