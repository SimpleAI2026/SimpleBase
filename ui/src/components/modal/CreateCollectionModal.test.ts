import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '../../test/api-mock'
import { readyDb, uiStubs } from '../../test/helpers'
import CreateCollectionModal from './CreateCollectionModal.vue'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
  return { api: m.api, isMock: false }
})

describe('CreateCollectionModal', () => {
  beforeEach(() => resetApiMocks())

  it('validates collection names and submits', async () => {
    const w = mount(CreateCollectionModal, {
      props: { open: true, projectId: 'p1', database: readyDb },
      global: { stubs: uiStubs }
    })
    await w.get('.sb-ok').trigger('click')
    expect(toast.warning).toHaveBeenCalledWith('请输入集合名称')
    await w.get('#collection-name').setValue('1bad')
    expect(w.text()).toContain('字母开头')
    await w.get('#collection-name').setValue('users')
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(api.db.createCollection).toHaveBeenCalled()
    expect(w.emitted('created')?.[0]).toEqual(['users'])

    api.db.createCollection.mockRejectedValueOnce(new Error('exists'))
    await w.setProps({ open: false })
    await w.setProps({ open: true })
    await w.get('#collection-name').setValue('users')
    await w.get('.sb-ok').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('exists')

    const noDb = mount(CreateCollectionModal, {
      props: { open: true, projectId: 'p1', database: null },
      global: { stubs: uiStubs }
    })
    await noDb.get('#collection-name').setValue('x')
    await noDb.get('.sb-ok').trigger('click')
  })
})
