import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import FunctionTriggers from '@/pages/FunctionTriggers.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { clickText, mountWithApp } from '@/test/helpers'
import { ADMIN_PROJECT_ID } from '@/stores/project'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

const trigger = {
  id: 't1',
  projectId: 'p1',
  kind: 'http' as const,
  target: '',
  events: ['http'],
  funcFile: 'handler',
  funcExport: 'Handle',
  filter: 'status="open"',
  enabled: true,
  createdAt: '',
  updatedAt: ''
}
const extraStubs = {
  FunctionTriggerModal: {
    props: ['open', 'projectId'],
    emits: ['saved', 'update:open'],
    template: '<div v-if="open" class="trigger-modal"><button class="modal-save" @click="$emit(\'saved\')">save</button><button class="modal-close" @click="$emit(\'update:open\', false)">close</button></div>'
  }
}

describe('FunctionTriggers', () => {
  beforeEach(() => {
    resetApiMocks()
    api.triggers.list.mockResolvedValue([trigger])
    api.triggers.patch.mockResolvedValue({ ...trigger, enabled: false })
  })

  it('uses the cron-style card and supports create, toggle, invoke and delete', async () => {
    const { wrapper } = await mountWithApp(FunctionTriggers, { projectId: 'p1', stubs: extraStubs })
    expect(wrapper.text()).toContain('触发器列表')
    expect(wrapper.text()).toContain('共 1 个触发器')
    expect(wrapper.text()).toContain('handler.Handle')

    const header = wrapper.get('.card-header')
    const actions = header.findAll('button').map((button) => button.text())
    expect(actions).toEqual(['刷新', '新建触发器'])
    await clickText(wrapper, '新建触发器')
    expect(wrapper.find('.trigger-modal').exists()).toBe(true)
    await wrapper.get('.modal-close').trigger('click')
    expect(wrapper.find('.trigger-modal').exists()).toBe(false)

    await clickText(wrapper, '调用')
    await wrapper.find('.confirm-action').trigger('click')
    await flushPromises()
    expect(api.triggers.invoke).toHaveBeenCalledWith('p1', 't1', {})

    await wrapper.get('.switch').trigger('click')
    await flushPromises()
    expect(api.triggers.patch).toHaveBeenCalledWith('p1', 't1', { enabled: false })

    await clickText(wrapper, '删除')
    await wrapper.findAll('.confirm-action').at(-1)!.trigger('click')
    await flushPromises()
    expect(api.triggers.remove).toHaveBeenCalledWith('p1', 't1')

    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(api.triggers.list.mock.calls.length).toBeGreaterThan(2)
    await wrapper.get('.pager-next').trigger('click')
  })

  it('opens create from empty state and surfaces operation failures', async () => {
    api.triggers.list.mockResolvedValueOnce([])
    const { wrapper } = await mountWithApp(FunctionTriggers, { projectId: 'p1', stubs: extraStubs })
    await wrapper.get('.empty-action').trigger('click')
    expect(wrapper.find('.trigger-modal').exists()).toBe(true)

    api.triggers.patch.mockRejectedValueOnce(new Error('toggle failed'))
    api.triggers.invoke.mockRejectedValueOnce(new Error('invoke failed'))
    api.triggers.remove.mockRejectedValueOnce(new Error('delete failed'))
    const vm = wrapper.vm as unknown as {
      toggle: (item: typeof trigger, enabled: boolean) => Promise<void>
      invoke: (item: typeof trigger) => Promise<void>
      remove: (item: typeof trigger) => Promise<void>
    }
    await vm.toggle(trigger, false)
    await vm.invoke(trigger)
    await vm.remove(trigger)

    api.triggers.list.mockRejectedValueOnce(new Error('offline'))
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(wrapper.text()).toContain('offline')
  })

  it('shows an admin empty state without requesting triggers and reloads on project change', async () => {
    const { wrapper, pinia } = await mountWithApp(FunctionTriggers, {
      projectId: ADMIN_PROJECT_ID,
      stubs: extraStubs
    })
    expect(api.triggers.list).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('系统项目不支持事件触发器')
    expect(wrapper.text()).not.toContain('新建触发器')
    ;(wrapper.vm as unknown as { openCreate: () => void }).openCreate()
    expect(wrapper.find('.trigger-modal').exists()).toBe(false)

    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('p2', 'Second')
    await flushPromises()
    expect(api.triggers.list).toHaveBeenCalledWith('p2')
  })

  it('handles empty responses, stale lists and an enabled update', async () => {
    const { wrapper, pinia } = await mountWithApp(FunctionTriggers, { projectId: 'p1', stubs: extraStubs })
    const vm = wrapper.vm as unknown as {
      load: () => Promise<void>
      toggle: (item: typeof trigger, enabled: boolean) => Promise<void>
    }
    api.triggers.list.mockResolvedValueOnce(undefined)
    await vm.load()
    expect(wrapper.text()).toContain('暂无触发器')

    api.triggers.patch.mockResolvedValueOnce({ ...trigger, enabled: true })
    await vm.toggle(trigger, true)

    let resolveOld!: (items: typeof trigger[]) => void
    api.triggers.list.mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
    const staleLoad = vm.load()
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('p2', 'Second')
    await flushPromises()
    resolveOld([])
    await staleLoad
    expect(wrapper.text()).toContain('handler.Handle')
  })
})
