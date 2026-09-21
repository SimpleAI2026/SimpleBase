import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '../../test/api-mock'
import { uiStubs } from '../../test/helpers'
import DocumentListModal from './DocumentListModal.vue'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
  return { api: m.api, isMock: false }
})

describe('DocumentListModal (数据库管理 / 文档)', () => {
  beforeEach(() => resetApiMocks())

  it('loads rows, emits add-document, and deletes from the table', async () => {
    api.db.rows.mockResolvedValue([{ id: 'r1', name: 'Ada' }])
    const w = mount(DocumentListModal, {
      props: { open: true, projectId: 'p', databaseId: 'db', collection: 'users' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(w.text()).toContain('r1')
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
    api.db.rows.mockResolvedValue([])
    const w = mount(DocumentListModal, {
      props: { open: true, projectId: 'p', databaseId: 'db', collection: 'users', readonly: true },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(w.text()).toContain('暂时未查询到数据')
    expect(w.text()).not.toContain('新增文档')
    expect(w.text()).not.toContain('删除')
  })

  it('loads, strips id fields, and deletes rows', async () => {
    api.db.rows.mockResolvedValue([{ id: 'r1', name: 'Ada' }])
    const w = mount(DocumentListModal, {
      props: { open: true, projectId: 'p', databaseId: 'db', collection: 'users' },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.docFields({ id: 'r1', name: 'Ada' })).toEqual({ name: 'Ada' })
    await vm.removeRow('r1')
    expect(api.db.remove).toHaveBeenCalled()
    api.db.remove.mockRejectedValueOnce(new Error('rm'))
    await vm.removeRow('r1')
    expect(toast.error).toHaveBeenCalledWith('rm')
    api.db.rows.mockRejectedValueOnce(new Error('load'))
    await vm.load()
    expect(toast.error).toHaveBeenCalledWith('load')
    await w.setProps({ databaseId: '', collection: '' })
    await vm.load()
    await w.setProps({ open: true, databaseId: 'db', collection: 'c', reloadToken: 3, readonly: true })
    await flushPromises()
    w.unmount()
  })
})
