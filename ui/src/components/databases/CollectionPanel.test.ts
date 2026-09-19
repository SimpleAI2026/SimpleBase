import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { api, resetApiMocks } from '../../test/api-mock'
import { readyDb, uiStubs } from '../../test/helpers'
import CollectionPanel from './CollectionPanel.vue'

vi.mock('../../services/api', async () => {
  const m = await import('../../test/api-mock')
  return { api: m.api, isMock: false }
})

describe('CollectionPanel', () => {
  beforeEach(() => resetApiMocks())

  it('loads collections and emits actions', async () => {
    api.db.collections.mockResolvedValue(['users'])
    const w = mount(CollectionPanel, {
      props: { projectId: 'p', database: readyDb },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    const vm = w.vm as any
    expect(vm.rows).toEqual([{ name: 'users' }])
    api.db.collections.mockRejectedValueOnce(new Error('coll'))
    await vm.load()
    expect(toast.error).toHaveBeenCalledWith('coll')
    api.db.collections.mockRejectedValueOnce('x')
    await vm.load()
    await w.setProps({ reloadToken: 2, readonly: true })
    await flushPromises()
    w.unmount()
  })
})
