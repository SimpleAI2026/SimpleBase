import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs, cancelModal, inDialog, installJsdomPolyfills, modalOkDisabled, mountDialog, okModal, realDialogStubs } from '@/test/helpers'
import DocumentKvModal from '@/components/modal/DocumentKvModal.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

installJsdomPolyfills()

describe('DocumentKvModal', () => {
  beforeEach(() => resetApiMocks())

  it('parses JSON values and inserts a document', async () => {
    const w = await mountDialog(DocumentKvModal, {
      props: { open: true, projectId: 'p', databaseId: 'db', collection: 'users' },
      global: { stubs: { ...uiStubs, ...realDialogStubs } }
    })
    // Key 为空时「确定」禁用；提示只能由回车触发。
    expect(modalOkDisabled()).toBe(true)
    await inDialog().get('#doc-key').trigger('keydown.enter')
    expect(toast.warning).toHaveBeenCalledWith('请输入 Key')
    await inDialog().get('#doc-key').setValue('age')
    await inDialog().get('#doc-value').setValue('18')
    expect(modalOkDisabled()).toBe(false)
    await okModal()
    await flushPromises()
    expect(api.db.insert).toHaveBeenCalledWith('p', 'db', 'users', { age: 18 })

    await w.setProps({ open: false })
    await w.setProps({ open: true })
    await inDialog().get('#doc-key').setValue('note')
    await inDialog().get('#doc-value').setValue('plain')
    api.db.insert.mockRejectedValueOnce(new Error('no'))
    await okModal()
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('no')
  })
  it('closes on cancel', async () => {
    const w = await mountDialog(DocumentKvModal, { props: { open: true, projectId: 'p', databaseId: 'db', collection: 'users' }, global: { stubs: { ...uiStubs, ...realDialogStubs } } })
    await cancelModal()
    expect(w.emitted('update:open')).toBeTruthy()
    w.unmount()
  })

})
