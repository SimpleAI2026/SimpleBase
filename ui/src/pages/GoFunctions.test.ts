import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import { ADMIN_PROJECT_ID } from '../stores/project'
import { api, resetApiMocks } from '../test/api-mock'
import { clickText, mountWithApp, sampleGoFn } from '../test/helpers'

vi.mock('../services/api', async () => {
  const m = await import('../test/api-mock')
  return { api: m.api, isMock: false }
})

import GoFunctions from './GoFunctions.vue'

const modalStub = {
  GoFunctionModal: {
    props: ['open', 'mode', 'target'],
    emits: ['saved', 'update:open'],
    template:
      '<div v-if="open" class="gf-modal"><button type="button" class="gf-close" @click="$emit(\'update:open\', false)">x</button>{{ mode }} {{ target && target.name }}</div>'
  }
}

describe('GoFunctions (云函数)', () => {
  beforeEach(() => {
    resetApiMocks()
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
    api.gofunctions.list.mockResolvedValue([sampleGoFn, { ...sampleGoFn, id: 'gf-2', name: 'empty', file: 'empty.go', exports: [] }])
  })

  it('lists functions and copies invoke paths', async () => {
    const { wrapper } = await mountWithApp(GoFunctions, { stubs: modalStub })
    expect(wrapper.text()).toContain('hello.go')
    await wrapper.findAll('.badge')[0].trigger('click')
    await flushPromises()
    expect(navigator.clipboard.writeText).toHaveBeenCalled()
    expect(toast.success).toHaveBeenCalled()

    await clickText(wrapper, '复制路径')
    await flushPromises()

    const emptyRowBtn = wrapper.findAll('button').filter((b) => b.text().includes('复制路径'))[1]
    await emptyRowBtn.trigger('click')
    expect(toast.error).toHaveBeenCalledWith('该云函数没有导出函数')

    vi.mocked(navigator.clipboard.writeText).mockRejectedValueOnce(new Error('denied'))
    await wrapper.findAll('.badge')[0].trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('复制失败')
  })

  it('opens create/edit/view modals and deletes', async () => {
    const { wrapper } = await mountWithApp(GoFunctions, { stubs: modalStub })
    await clickText(wrapper, '新建云函数')
    expect(wrapper.get('.gf-modal').text()).toContain('create')
    await clickText(wrapper, '编辑')
    expect(wrapper.get('.gf-modal').text()).toContain('edit')
    await clickText(wrapper, '查看')
    expect(wrapper.get('.gf-modal').text()).toContain('view')
    await wrapper.get('.gf-close').trigger('click')

    await clickText(wrapper, '删除')
    await flushPromises()
    expect(api.gofunctions.remove).toHaveBeenCalled()
    api.gofunctions.remove.mockRejectedValueOnce(new Error('rm'))
    await clickText(wrapper, '删除')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('rm')
    await wrapper.get('.pager-next').trigger('click')
  })

  it('hides write actions on admin project and handles list errors', async () => {
    api.gofunctions.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(GoFunctions, {
      projectId: ADMIN_PROJECT_ID,
      stubs: modalStub
    })
    expect(wrapper.text()).toContain('系统项目不支持云函数')
    expect(wrapper.text()).not.toContain('新建云函数')

    api.gofunctions.list.mockRejectedValueOnce(new Error('list'))
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(toast.error).toHaveBeenCalledWith('list')
  })

  it('reloads when the project changes', async () => {
    const { pinia } = await mountWithApp(GoFunctions, { stubs: modalStub })
    const { useProjectStore } = await import('../stores/project')
    useProjectStore(pinia).setProject('00000000-0000-0000-0000-000000000003')
    await flushPromises()
    expect(api.gofunctions.list.mock.calls.length).toBeGreaterThan(1)
  })

  it('shows the empty catalog for a normal project', async () => {
    api.gofunctions.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(GoFunctions, { stubs: modalStub })
    expect(wrapper.text()).toContain('还没有云函数')
    expect(wrapper.text()).toContain('新建云函数')
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(api.gofunctions.list.mock.calls.length).toBeGreaterThan(1)
  })
})
