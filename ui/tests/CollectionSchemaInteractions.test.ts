import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs, installJsdomPolyfills, pickSelectOption, realSelectStubs, selectTriggers, closeDialog, dialogEl, inDialog, mountDialog, realDialogStubs } from '@/test/helpers'
import CollectionPanel from '@/components/databases/CollectionPanel.vue'
import CollectionSchemaModal from '@/components/databases/CollectionSchemaModal.vue'
import { readyDb } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

installJsdomPolyfills()

describe('collection schema template interactions', () => {
  beforeEach(() => resetApiMocks())

  it('opens and closes the schema editor from a collection row', async () => {
    const w = await mountDialog(CollectionPanel, { props: { projectId: 'p', database: readyDb }, global: { stubs: { ...uiStubs, ...realDialogStubs } } })
    await flushPromises()
    await inDialog().findAll('button').find((b) => b.text() === '编辑 Schema')!.trigger('click')
    await flushPromises()
    expect(dialogEl()).not.toBeNull()
    await closeDialog()
    expect(dialogEl()).toBeNull()
  })

  it('exercises every field editor, switch and delete callback', async () => {
    api.db.getCollectionSchema.mockResolvedValue({
      name: 'users',
      fields: [{ name: 'state', type: 'select', required: true, unique: true, hidden: true, readonly: true, default: 'open', options: { values: ['open', 'closed'], min: 1, max: 2, pattern: 'o' } }]
    })
    const w = await mountDialog(CollectionSchemaModal, {
      attachTo: document.body,
      props: { open: true, projectId: 'p', databaseId: 'd', collection: 'users' },
      global: { stubs: { ...uiStubs, ...realDialogStubs, ...realSelectStubs } }
    })
    await flushPromises()
    const inputs = inDialog().findAll('input')
    for (const [index, input] of inputs.entries()) await input.setValue(index === 2 ? '"open"' : '1')
    // 真实 Select：把字段类型改成「文本」
    await pickSelectOption(selectTriggers()[0], 'text')
    for (const toggle of inDialog().findAll('[data-slot="switch"]')) await toggle.trigger('click')
    await inDialog().findAll('button').find((b) => b.text() === '删除')!.trigger('click')
    expect(inDialog().text()).toContain('暂无字段')
    await closeDialog()
    expect(w.emitted('update:open')?.at(-1)).toEqual([false])
  })
})
