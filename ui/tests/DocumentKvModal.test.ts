import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '../../test/api-mock'
import { uiStubs } from '../../test/helpers'
import DocumentKvModal from './DocumentKvModal.vue'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
  return { api: m.api, isMock: false }
})

describe('DocumentKvModal', () => {
  beforeEach(() => resetApiMocks())

  it('parses JSON values and inserts a document', async () => {
    const w = mount(DocumentKvModal, {
      props: { open: true, projectId: 'p', databaseId: 'db', collection: 'users' },
      global: { stubs: uiStubs }
    })
    await w.get('.sb-ok').trigger('click')
    expect(toast.warning).toHaveBeenCalledWith('请输入 Key')
    await w.get('#doc-key').setValue('age')
    await w.get('#doc-value').setValue('18')
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.db.insert).toHaveBeenCalledWith('p', 'db', 'users', { age: 18 })

    await w.setProps({ open: false })
    await w.setProps({ open: true })
    await w.get('#doc-key').setValue('note')
    await w.get('#doc-value').setValue('plain')
    api.db.insert.mockRejectedValueOnce(new Error('no'))
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('no')
  })
})
