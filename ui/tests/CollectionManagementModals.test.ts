import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ViewCollectionModal from '@/components/databases/ViewCollectionModal.vue'
import CollectionImportModal from '@/components/databases/CollectionImportModal.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs, closeDialog, inDialog, installJsdomPolyfills, mountDialog, realDialogStubs } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

installJsdomPolyfills()

describe('collection management modals', () => {
  beforeEach(() => resetApiMocks())

  it('requires a view dry run before saving inferred fields', async () => {
    api.db.dryRunView.mockResolvedValue({ columns: ['id', 'data'], scanned: 1, fields: [{ name: 'title', type: 'text', required: false, unique: false, hidden: false, readonly: false, options: {} }], items: [] })
    const wrapper = await mountDialog(ViewCollectionModal, { props: { open: true, projectId: 'p', databaseId: 'd' }, global: { stubs: { ...uiStubs, ...realDialogStubs } } })
    await inDialog().findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    expect(api.db.putCollectionSchema).not.toHaveBeenCalled()
    await inDialog().get('input').setValue('active_tasks')
    await inDialog().findAll('button').find((button) => button.text() === '试跑并推断字段')!.trigger('click')
    await flushPromises()
    expect(api.db.dryRunView).toHaveBeenCalled()
    await inDialog().findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.db.putCollectionSchema).toHaveBeenCalledWith('p', 'd', expect.objectContaining({ name: 'active_tasks', schemaMode: 'view' }))
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('supports import dry run and actual import', async () => {
    api.db.importCollection.mockResolvedValue({ dryRun: true, scanned: 1, imported: 0 })
    const wrapper = await mountDialog(CollectionImportModal, { props: { open: true, projectId: 'p', databaseId: 'd', collection: 'tasks' }, global: { stubs: { ...uiStubs, ...realDialogStubs } } })
    const input = inDialog().get('input[type="file"]')
    const file = new File(['title\none\n'], 'tasks.csv', { type: 'text/csv' })
    Object.defineProperty(input.element, 'files', { value: [file] })
    await input.trigger('change')
    await inDialog().findAll('button').find((button) => button.text() === '开始导入')!.trigger('click')
    await flushPromises()
    expect(api.db.importCollection).toHaveBeenCalledWith('p', 'd', 'tasks', file, 'csv', true)
    expect(inDialog().text()).toContain('未写入数据')
    api.db.importCollection.mockResolvedValue({ dryRun: false, scanned: 1, imported: 1 })
    await inDialog().get('[data-slot="switch"]').trigger('click')
    await inDialog().findAll('button').find((button) => button.text() === '开始导入')!.trigger('click')
    await flushPromises()
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('surfaces dry-run, save and import failures and resets on reopen', async () => {
    api.db.dryRunView.mockRejectedValueOnce(new Error('bad query'))
    const view = await mountDialog(ViewCollectionModal, { props: { open: true, projectId: 'p', databaseId: 'd' }, global: { stubs: { ...uiStubs, ...realDialogStubs } } })
    await inDialog().get('input').setValue('v')
    await inDialog().get('textarea').setValue('SELECT id, data, created_at, updated_at FROM tasks')
    await inDialog().findAll('button').find((button) => button.text() === '试跑并推断字段')!.trigger('click')
    await flushPromises()
    expect(inDialog().text()).toContain('bad query')
    api.db.dryRunView.mockResolvedValue({ columns: [], scanned: 0, fields: [], items: [] })
    await inDialog().findAll('button').find((button) => button.text() === '试跑并推断字段')!.trigger('click')
    await flushPromises()
    api.db.putCollectionSchema.mockRejectedValueOnce(new Error('save failed'))
    await inDialog().findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(inDialog().text()).toContain('save failed')
    await view.setProps({ open: false })
    await view.setProps({ open: true })
    await closeDialog()
    view.unmount()

    api.db.importCollection.mockRejectedValueOnce(new Error('bad import'))
    const importer = await mountDialog(CollectionImportModal, { props: { open: true, projectId: 'p', databaseId: 'd', collection: 'tasks' }, global: { stubs: { ...uiStubs, ...realDialogStubs } } })
    const input = inDialog().get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [new File(['{}\n'], 'tasks.ndjson')] })
    await input.trigger('change')
    await inDialog().findAll('button').find((button) => button.text() === '开始导入')!.trigger('click')
    await flushPromises()
    expect(inDialog().text()).toContain('bad import')
    await closeDialog()
  })
})
