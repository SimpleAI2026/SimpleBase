import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ViewCollectionModal from '@/components/databases/ViewCollectionModal.vue'
import CollectionImportModal from '@/components/databases/CollectionImportModal.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

describe('collection management modals', () => {
  beforeEach(() => resetApiMocks())

  it('requires a view dry run before saving inferred fields', async () => {
    api.db.dryRunView.mockResolvedValue({ columns: ['id', 'data'], scanned: 1, fields: [{ name: 'title', type: 'text', required: false, unique: false, hidden: false, readonly: false, options: {} }], items: [] })
    const wrapper = mount(ViewCollectionModal, { props: { open: true, projectId: 'p', databaseId: 'd' }, global: { stubs: uiStubs } })
    await wrapper.findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    expect(api.db.putCollectionSchema).not.toHaveBeenCalled()
    await wrapper.get('input').setValue('active_tasks')
    await wrapper.findAll('button').find((button) => button.text() === '试跑并推断字段')!.trigger('click')
    await flushPromises()
    expect(api.db.dryRunView).toHaveBeenCalled()
    await wrapper.findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.db.putCollectionSchema).toHaveBeenCalledWith('p', 'd', expect.objectContaining({ name: 'active_tasks', schemaMode: 'view' }))
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('supports import dry run and actual import', async () => {
    api.db.importCollection.mockResolvedValue({ dryRun: true, scanned: 1, imported: 0 })
    const wrapper = mount(CollectionImportModal, { props: { open: true, projectId: 'p', databaseId: 'd', collection: 'tasks' }, global: { stubs: uiStubs } })
    const input = wrapper.get('input[type="file"]')
    const file = new File(['title\none\n'], 'tasks.csv', { type: 'text/csv' })
    Object.defineProperty(input.element, 'files', { value: [file] })
    await input.trigger('change')
    await wrapper.findAll('button').find((button) => button.text() === '开始导入')!.trigger('click')
    await flushPromises()
    expect(api.db.importCollection).toHaveBeenCalledWith('p', 'd', 'tasks', file, 'csv', true)
    expect(wrapper.text()).toContain('未写入数据')
    api.db.importCollection.mockResolvedValue({ dryRun: false, scanned: 1, imported: 1 })
    await wrapper.get('.switch').trigger('click')
    await wrapper.findAll('button').find((button) => button.text() === '开始导入')!.trigger('click')
    await flushPromises()
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('surfaces dry-run, save and import failures and resets on reopen', async () => {
    api.db.dryRunView.mockRejectedValueOnce(new Error('bad query'))
    const view = mount(ViewCollectionModal, { props: { open: true, projectId: 'p', databaseId: 'd' }, global: { stubs: uiStubs } })
    await view.get('input').setValue('v')
    await view.get('textarea').setValue('SELECT id, data, created_at, updated_at FROM tasks')
    await view.findAll('button').find((button) => button.text() === '试跑并推断字段')!.trigger('click')
    await flushPromises()
    expect(view.text()).toContain('bad query')
    api.db.dryRunView.mockResolvedValue({ columns: [], scanned: 0, fields: [], items: [] })
    await view.findAll('button').find((button) => button.text() === '试跑并推断字段')!.trigger('click')
    await flushPromises()
    api.db.putCollectionSchema.mockRejectedValueOnce(new Error('save failed'))
    await view.findAll('button').find((button) => button.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(view.text()).toContain('save failed')
    await view.setProps({ open: false })
    await view.setProps({ open: true })
    await view.get('.sb-modal-close').trigger('click')

    api.db.importCollection.mockRejectedValueOnce(new Error('bad import'))
    const importer = mount(CollectionImportModal, { props: { open: true, projectId: 'p', databaseId: 'd', collection: 'tasks' }, global: { stubs: uiStubs } })
    const input = importer.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [new File(['{}\n'], 'tasks.ndjson')] })
    await input.trigger('change')
    await importer.findAll('button').find((button) => button.text() === '开始导入')!.trigger('click')
    await flushPromises()
    expect(importer.text()).toContain('bad import')
    await importer.get('.sb-modal-close').trigger('click')
  })
})
