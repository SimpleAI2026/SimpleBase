import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs, realDialogStubs, inDialog, installJsdomPolyfills, mountDialog, clickAlertDialog, realAlertDialogStubs } from '@/test/helpers'
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

async function mountModal(readonly = false) {
  return await mountDialog(DocumentListModal, {
    props: { open: true, projectId: 'p', databaseId: 'db', collection: 'users', readonly },
    global: { stubs: { ...uiStubs, ...realDialogStubs, ...realAlertDialogStubs } }
  })
}

installJsdomPolyfills()

describe('DocumentListModal (数据管理 / 文档)', () => {
  beforeEach(() => resetApiMocks())

  it('loads rows, emits add-document, and deletes from the table', async () => {
    api.db.documents.mockResolvedValue(page([{ id: 'r1', name: 'Ada' }]))
    const w = await mountModal()
    await flushPromises()
    expect(inDialog().text()).toContain('r1')
    expect(inDialog().findAll('th').find((el) => el.text() === '数据')?.classes()).toContain('text-left')
    await inDialog().findAll('button').find((b) => b.text().includes('新增文档'))!.trigger('click')
    expect(w.emitted('add-document')).toBeTruthy()
    await inDialog().findAll('button').find((b) => b.text().includes('删除'))!.trigger('click')
    await flushPromises()
    // 真实 ConfirmAction：先弹确认，点「确定」才删除
    expect(api.db.remove).not.toHaveBeenCalled()
    await clickAlertDialog('确定')
    expect(api.db.remove).toHaveBeenCalled()
    await inDialog().findAll('button').find((b) => b.text().includes('刷新'))!.trigger('click')
    await flushPromises()
    await inDialog().get('.pager-next').trigger('click')
  })

  it('hides write actions in readonly and shows empty copy', async () => {
    api.db.documents.mockResolvedValue(page([]))
    const w = await mountModal(true)
    await flushPromises()
    expect(inDialog().text()).toContain('暂时未查询到数据')
    expect(inDialog().text()).not.toContain('新增文档')
    expect(inDialog().text()).not.toContain('删除')
  })

  it('strips system fields and surfaces action errors', async () => {
    api.db.documents.mockResolvedValue(page([{ id: 'r1', name: 'Ada' }]))
    const w = await mountModal()
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
    const w = await mountModal()
    await flushPromises()
    const inputs = inDialog().findAll('input')
    await inputs[0].setValue('priority>=2')
    await inputs[1].setValue('title')
    await inputs[2].setValue('id,title')
    await inDialog().findAll('button').find((b) => b.text() === '查询')!.trigger('click')
    await flushPromises()
    expect(api.db.documents).toHaveBeenLastCalledWith('p', 'db', 'users', expect.objectContaining({
      filter: 'priority>=2', sort: 'title', fields: 'id,title', count: true, offset: 0
    }))
  })

  it('patches, batches and truncates through JSON editors and confirmations', async () => {
    api.db.documents.mockResolvedValue(page([{ id: 'r1', name: 'Ada' }]))
    const w = await mountModal()
    await flushPromises()
    await inDialog().findAll('button').find((b) => b.text() === '编辑')!.trigger('click')
    await flushPromises()
    const patchArea = inDialog().find('textarea')
    await patchArea.setValue('{"name":"Grace"}')
    await inDialog().findAll('button').find((b) => b.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.db.patch).toHaveBeenCalledWith('p', 'db', 'users', 'r1', { name: 'Grace' })

    await inDialog().findAll('button').find((b) => b.text() === '批量写入')!.trigger('click')
    await inDialog().find('textarea').setValue('[{"name":"A"}]')
    await inDialog().findAll('button').find((b) => b.text() === '保存')!.trigger('click')
    await flushPromises()
    expect(api.db.batch).toHaveBeenCalledWith('p', 'db', 'users', [{ name: 'A' }])

    await inDialog().findAll('button').find((b) => b.text() === '清空集合')!.trigger('click')
    await flushPromises()
    // 清空是破坏性操作：先弹确认，取消不动，确定才执行
    expect(api.db.truncate).not.toHaveBeenCalled()
    await clickAlertDialog('取消')
    expect(api.db.truncate).not.toHaveBeenCalled()
    await inDialog().findAll('button').find((b) => b.text() === '清空集合')!.trigger('click')
    await flushPromises()
    await clickAlertDialog('确定')
    expect(api.db.truncate).toHaveBeenCalledWith('p', 'db', 'users')
  })
})
