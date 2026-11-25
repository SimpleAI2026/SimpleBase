import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { readyDb, uiStubs, cancelModal, inDialog, installJsdomPolyfills, modalOkDisabled, mountDialog, okModal, realDialogStubs } from '@/test/helpers'
import CreateCollectionModal from '@/components/modal/CreateCollectionModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

installJsdomPolyfills()

describe('CreateCollectionModal', () => {
  beforeEach(() => resetApiMocks())

  it('validates collection names and submits', async () => {
    const w = await mountDialog(CreateCollectionModal, {
      props: { open: true, projectId: 'p1', database: readyDb },
      global: { stubs: { ...uiStubs, ...realDialogStubs } }
    })
    // 名称为空 / 非法时「确定」禁用；空名称的提示只能由回车触发。
    expect(modalOkDisabled()).toBe(true)
    await inDialog().get('#collection-name').trigger('keydown.enter')
    expect(toast.warning).toHaveBeenCalledWith('请输入集合名称')
    await inDialog().get('#collection-name').setValue('1bad')
    expect(inDialog().text()).toContain('字母开头')
    expect(modalOkDisabled()).toBe(true)
    await inDialog().get('#collection-name').setValue('users')
    expect(modalOkDisabled()).toBe(false)
    await okModal()
    await flushPromises()
    expect(api.db.createCollection).toHaveBeenCalled()
    expect(w.emitted('created')?.[0]).toEqual(['users'])

    api.db.createCollection.mockRejectedValueOnce(new Error('exists'))
    await w.setProps({ open: false })
    await w.setProps({ open: true })
    await inDialog().get('#collection-name').setValue('users')
    await okModal()
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('exists')

    // 同一页面同时只有一个真实对话框：先卸载上一个再挂新的
    w.unmount()
    const noDb = await mountDialog(CreateCollectionModal, {
      props: { open: true, projectId: 'p1', database: null },
      global: { stubs: { ...uiStubs, ...realDialogStubs } }
    })
    await inDialog().get('#collection-name').setValue('x')
    // 没有选中数据库时即使名称合法也不能提交
    expect(modalOkDisabled()).toBe(true)
  })
  it('closes on cancel', async () => {
    const w = await mountDialog(CreateCollectionModal, { props: { open: true, projectId: 'p', database: readyDb }, global: { stubs: { ...uiStubs, ...realDialogStubs } } })
    await cancelModal()
    expect(w.emitted('update:open')).toBeTruthy()
    w.unmount()
  })

})
