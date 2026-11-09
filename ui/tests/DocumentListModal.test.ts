import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import DocumentListModal from '@/components/modal/DocumentListModal.vue'
import type { DbRow } from '@/services/types'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

interface DocumentListVm {
  docFields(row: DbRow): Record<string, unknown>
  removeRow(id: string): Promise<void>
  load(): Promise<void>
}

function page(rows: DbRow[]) {
  return { rows, items: [] }
}

function mountModal(readonly = false) {
  return mount(DocumentListModal, {
    props: { open: true, projectId: 'p', databaseId: 'db', collection: 'users', readonly },
    global: { stubs: uiStubs }
  })
}

describe('DocumentListModal (数据管理 / 文档)', () => {
  beforeEach(() => resetApiMocks())

  it('loads rows, emits add-document, and deletes from the table', async () => {
    api.db.documents.mockResolvedValue(page([{ id: 'r1', name: 'Ada' }]))
    const w = mountModal()
    await flushPromises()
    expect(w.text()).toContain('r1')
    expect(w.findAll('th').find((el) => el.text() === '数据')?.classes()).toContain('text-left')
    await w.findAll('button').find((b) => b.text().includes('新增文档'))!.trigger('click')
    expect(w.emitted('add-document')).toBeTruthy()
    await w.findAll('button').find((b) => b.text().includes('删除'))!.trigger('click')
    await flushPromises()
    expect(api.db.remove).toHaveBeenCalled()
    await w.findAll('button').find((b) => b.text().includes('刷新'))!.trigger('click')
    await flushPromises()
    await w.get('.pager-next').trigger('click')
  })

  it('hides write actions in readonly and shows empty copy', async () => {
    api.db.documents.mockResolvedValue(page([]))
    const w = mountModal(true)
    await flushPromises()
    expect(w.text()).toContain('暂时未查询到数据')
    expect(w.text()).not.toContain('新增文档')
    expect(w.text()).not.toContain('删除')
  })

  it('strips system fields and surfaces action errors', async () => {
    api.db.documents.mockResolvedValue(page([{ id: 'r1', name: 'Ada' }]))
    const w = mountModal()
    await flushPromises()
    const vm = w.vm as unknown as DocumentListVm
    expect(vm.docFields({ id: 'r1', name: 'Ada', created_at: 'x', updated_at: 'y' })).toEqual({ name: 'Ada' })
    await vm.removeRow('r1')
    expect(api.db.remove).toHaveBeenCalled()
    api.db.remove.mockRejectedValueOnce(new Error('rm'))
    await vm.removeRow('r1')
    expect(toast.error).toHaveBeenCalledWith('rm')
    api.db.documents.mockRejectedValueOnce(new Error('load'))
    await vm.load()
    expect(toast.error).toHaveBeenCalledWith('load')
    await w.setProps({ databaseId: '', collection: '' })
    await vm.load()
  })

  it('passes filter, sort, fields and offset to the documents API', async () => {
    api.db.documents.mockResolvedValue(page([{ id: 'r1', title: 'A' }]))
    const w = mountModal()
    await flushPromises()
    const inputs = w.findAll('input')
    await inputs[0].setValue('priority>=2')
    await inputs[1].setValue('title')
    await inputs[2].setValue('id,title')
    await w.findAll('button').find((b) => b.text() === '查询')!.trigger('click')
    await flushPromises()
    expect(api.db.documents).toHaveBeenLastCalledWith('p', 'db', 'users', expect.objectContaining({
      filter: 'priority>=2', sort: 'title', fields: 'id,title', count: true, offset: 0
    }))
  })

  it('patches, batches and truncates through JSON editors and confirmations', async () => {
    api.db.documents.mockResolvedValue(page([{ id: 'r1', name: 'Ada' }]))
    const w = mountModal()
    await flushPromises()
    await w.findAll('button').find((b) => b.text() === '编辑')!.trigger('click')
    await flushPromises()
    const patchArea = w.find('textarea')
    await patchArea.setValue('{"name":"Grace"}')
    await w.findAll('button').find((b) => b.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.db.patch).toHaveBeenCalledWith('p', 'db', 'users', 'r1', { name: 'Grace' })

    await w.findAll('button').find((b) => b.text() === '批量写入')!.trigger('click')
    await w.find('textarea').setValue('[{"name":"A"}]')
    await w.findAll('button').find((b) => b.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.db.batch).toHaveBeenCalledWith('p', 'db', 'users', [{ name: 'A' }])

    await w.findAll('button').find((b) => b.text() === '清空集合')!.trigger('click')
    await flushPromises()
    expect(api.db.truncate).toHaveBeenCalledWith('p', 'db', 'users')
  })
})
