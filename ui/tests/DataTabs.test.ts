import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { readyDb, uiStubs } from '@/test/helpers'
import DataTabs from '@/components/databases/DataTabs.vue'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

describe('DataTabs (数据库数据页签)', () => {
  beforeEach(() => resetApiMocks())

  it('默认展示集合文档页签并加载集合（Key-Value 已改项目级，不在此挂载）', async () => {
    api.db.collections.mockResolvedValue(['users'])
    const w = mount(DataTabs, {
      props: { projectId: 'p', database: readyDb },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(api.db.collections).toHaveBeenCalled()
    expect(w.text()).toContain('集合文档')
    expect(w.text()).toContain('users')
    expect(w.text()).not.toContain('Key-Value')
    w.unmount()
  })

  it('不再触发任何 KV 调用', async () => {
    api.db.collections.mockResolvedValue(['users'])
    const w = mount(DataTabs, {
      props: { projectId: 'p', database: readyDb },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    expect(api.kv.exec).not.toHaveBeenCalled()
    w.unmount()
  })

  it('集合文档事件透传', async () => {
    api.db.collections.mockResolvedValue(['users'])
    const w = mount(DataTabs, {
      props: { projectId: 'p', database: readyDb },
      global: { stubs: uiStubs }
    })
    await flushPromises()
    await w.findAll('button').find((b) => b.text().includes('查看数据'))!.trigger('click')
    expect(w.emitted('view-data')?.[0]).toEqual(['users'])
    await w.findAll('button').find((b) => b.text().includes('新增文档'))!.trigger('click')
    expect(w.emitted('add-document')?.[0]).toEqual(['users'])
    w.unmount()
  })
})
