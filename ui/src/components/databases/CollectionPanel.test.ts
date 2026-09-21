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

describe('CollectionPanel (数据库管理 / 集合)', () => {
  beforeEach(() => resetApiMocks())

  it('loads collections and emits view / add / create actions', async () => {
    api.db.collections.mockResolvedValue(['users'])
    const w = mount(CollectionPanel, {
      props: { projectId: 'p', database: readyDb },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(w.text()).toContain('users')
    await w.findAll('button').find((b) => b.text().includes('查看数据'))!.trigger('click')
    expect(w.emitted('view-data')?.[0]).toEqual(['users'])
    await w.findAll('button').find((b) => b.text().includes('新增文档'))!.trigger('click')
    expect(w.emitted('add-document')?.[0]).toEqual(['users'])

    const vm = w.vm as any
    api.db.collections.mockRejectedValueOnce(new Error('coll'))
    await vm.load()
    expect(toast.error).toHaveBeenCalledWith('coll')
    api.db.collections.mockRejectedValueOnce('x')
    await vm.load()
    expect(toast.error).toHaveBeenCalledWith('集合加载失败')
    await w.setProps({ reloadToken: 2, readonly: true })
    await flushPromises()
    expect(w.text()).not.toContain('新增文档')
    w.unmount()
  })

  it('emits create-collection from the empty state unless readonly', async () => {
    api.db.collections.mockResolvedValue([])
    const w = mount(CollectionPanel, {
      props: { projectId: 'p', database: readyDb },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(w.text()).toContain('暂无集合')
    await w.get('.empty-action').trigger('click')
    expect(w.emitted('create-collection')).toBeTruthy()

    const ro = mount(CollectionPanel, {
      props: { projectId: 'p', database: readyDb, readonly: true },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(ro.text()).toContain('暂无数据表')
    expect(ro.find('.empty-action').exists()).toBe(false)
    ro.unmount()
  })
})
