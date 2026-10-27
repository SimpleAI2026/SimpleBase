import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, resetApiMocks } from '@/test/api-mock'
import { mountWithApp, clickText } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const m = await import('@/test/api-mock')
  return { api: m.api }
})

import Sandboxes from '@/pages/Sandboxes.vue'

const sample = {
  id: 'sbx-1', name: 'ci-test', templateId: 'python:3.12-slim', state: 'running', status: 'running', source: 'api',
  cpuCount: 1, memoryMB: 256, idleTimeoutS: 300, metadata: {},
  createdAt: '2026-10-04T00:00:00Z', lastActiveAt: '2026-10-04T00:10:00Z'
}
const available = {
  available: true, backend: 'fake', images: ['python:3.12-slim'], defaultImage: 'python:3.12-slim',
  cpus: 1, memoryMiB: 512, timeoutDefaultS: 300, timeoutMaxS: 3600, maxFileBytes: 1048576, maxOutputBytes: 65536,
  maxPerProject: 5, terminal: true
}
const stubs = {
  SandboxCreateModal: { template: '<div data-modal />' },
  SandboxDrawer: { template: '<div data-drawer />' }
}

describe('Sandboxes', () => {
  beforeEach(() => resetApiMocks())

  it('shows disabled configuration hints', async () => {
    const { wrapper } = await mountWithApp(Sandboxes, { stubs })
    expect(wrapper.text()).toContain('未配置云沙盒')
    expect(wrapper.text()).toContain('backend: local')
    expect(wrapper.text()).not.toContain('新建沙盒')
  })

  it('lists sandboxes, filters and triggers stop', async () => {
    api.sandboxes.capabilities.mockResolvedValue(available)
    api.sandboxes.list.mockResolvedValue([sample, { ...sample, id: 'sbx-2', name: 'agent-test', state: 'paused', status: 'stopped', source: 'agent' }])
    const { wrapper } = await mountWithApp(Sandboxes, { stubs })
    expect(wrapper.text()).toContain('ci-test')
    expect(wrapper.text()).toContain('agent-test')
    // 来源过滤已从原生 <select> 迁到 ui/select（仓库约定：不写原生下拉）；
    // Select 桩不承载真实取值，直接设 v-model。
    const vm = wrapper.vm as unknown as { sourceFilter: string }
    vm.sourceFilter = 'agent'
    await flushPromises()
    expect(wrapper.text()).not.toContain('ci-test')
    expect(wrapper.text()).toContain('agent-test')
    vm.sourceFilter = '__all__'
    await flushPromises()
    await clickText(wrapper, '暂停')
    expect(api.sandboxes.pause).toHaveBeenCalled()
  })

  it('starts, removes and surfaces failed sandbox requests', async () => {
    api.sandboxes.capabilities.mockResolvedValue(available)
    api.sandboxes.list.mockResolvedValue([{ ...sample, state: 'paused', status: 'stopped' }])
    const { wrapper } = await mountWithApp(Sandboxes, { stubs })
    await clickText(wrapper, '恢复')
    await flushPromises()
    expect(api.sandboxes.connect).toHaveBeenCalled()
    await clickText(wrapper, '删除')
    await flushPromises()
    expect(api.sandboxes.remove).toHaveBeenCalled()
    api.sandboxes.list.mockRejectedValueOnce(new Error('读取失败'))
    await clickText(wrapper, '刷新')
    await flushPromises()
    api.sandboxes.connect.mockRejectedValueOnce(new Error('恢复失败'))
    await clickText(wrapper, '恢复')
    await flushPromises()
    wrapper.unmount()
  })
})

describe('Sandboxes interactions', () => {
  beforeEach(() => resetApiMocks())
  const emitStubs = {
    SandboxCreateModal: { props: ['open'], emits: ['update:open', 'saved'], template: '<div data-modal :data-open="open"><button class="modal-saved" @click="$emit(\'saved\')">saved</button><button class="modal-close" @click="$emit(\'update:open\', false)">close</button></div>' },
    SandboxDrawer: {
      props: ['open', 'sandbox'], emits: ['update:open', 'changed'],
      template: '<div data-drawer :data-open="open" :data-id="sandbox && sandbox.id"><button class="drawer-close" @click="$emit(\'update:open\', false)">close</button><button class="drawer-keep" @click="$emit(\'update:open\', true)">keep</button><button class="drawer-changed" @click="$emit(\'changed\')">changed</button></div>'
    }
  }

  it('opens create modal and drawer, reloads on child events and keeps selection fresh', async () => {
    api.sandboxes.capabilities.mockResolvedValue({ ...available, maxFileBytes: 0 })
    api.sandboxes.list.mockResolvedValue([
      { ...sample, source: 'console', lastActiveAt: undefined, endAt: '2026-10-04T01:00:00Z' },
      { ...sample, id: 'sbx-3', name: 'agent-run', source: 'agent', status: 'error' }
    ])
    const { wrapper } = await mountWithApp(Sandboxes, { stubs: emitStubs })
    expect(wrapper.text()).toContain('控制台')
    expect(wrapper.text()).toContain('Agent')
    expect(wrapper.text()).toContain('未启动')
    await clickText(wrapper, '新建沙盒')
    expect(wrapper.get('[data-modal]').attributes('data-open')).toBe('true')
    await wrapper.get('.modal-saved').trigger('click')
    await flushPromises()
    expect(api.sandboxes.list).toHaveBeenCalledTimes(2)
    await wrapper.get('.modal-close').trigger('click')
    expect(wrapper.get('[data-modal]').attributes('data-open')).toBe('false')

    await clickText(wrapper, '打开')
    expect(wrapper.get('[data-drawer]').attributes('data-id')).toBe('sbx-1')
    await wrapper.get('.drawer-changed').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-drawer]').attributes('data-id')).toBe('sbx-1')
    await wrapper.get('.drawer-keep').trigger('click')
    expect(wrapper.get('[data-drawer]').attributes('data-open')).toBe('true')
    await wrapper.get('.drawer-close').trigger('click')
    expect(wrapper.get('[data-drawer]').attributes('data-open')).toBe('false')

    // 打开的沙盒被删除后，刷新时清空选择。
    await clickText(wrapper, '打开')
    api.sandboxes.list.mockResolvedValue([])
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(wrapper.get('[data-drawer]').attributes('data-open')).toBe('false')
    expect(wrapper.text()).toContain('暂无云沙盒')
    wrapper.unmount()
  })

  it('filters by status and reloads when project changes', async () => {
    api.sandboxes.capabilities.mockResolvedValue(available)
    api.sandboxes.list.mockResolvedValue([sample, { ...sample, id: 'sbx-2', name: 'idle-one', state: 'paused', status: 'stopped' }])
    const { wrapper, pinia } = await mountWithApp(Sandboxes, { stubs })
    // 状态过滤同样改为 ui/select：设 v-model 而非原生 select 取值。
    const vm = wrapper.vm as unknown as { statusFilter: string }
    vm.statusFilter = 'paused'
    await flushPromises()
    expect(wrapper.text()).not.toContain('ci-test')
    expect(wrapper.text()).toContain('idle-one')
    const { useProjectStore } = await import('@/stores/project')
    useProjectStore(pinia).setProject('other-project', 'Other')
    await flushPromises()
    expect(api.sandboxes.capabilities).toHaveBeenLastCalledWith('other-project')
    wrapper.unmount()
  })

  it('shows admin project notice and surfaces non-Error failures', async () => {
    api.sandboxes.capabilities.mockResolvedValue(available)
    const admin = await mountWithApp(Sandboxes, { stubs, projectId: 'sb-admin' })
    expect(admin.wrapper.text()).toContain('系统项目不支持云沙盒')
    expect(admin.wrapper.text()).not.toContain('新建沙盒')
    admin.wrapper.unmount()

    api.sandboxes.list.mockResolvedValue([sample])
    const { wrapper } = await mountWithApp(Sandboxes, { stubs })
    api.sandboxes.pause.mockRejectedValueOnce('boom')
    await clickText(wrapper, '暂停')
    await flushPromises()
    api.sandboxes.pause.mockRejectedValueOnce(new Error('暂停失败'))
    await clickText(wrapper, '暂停')
    await flushPromises()
    api.sandboxes.remove.mockRejectedValueOnce('boom')
    await clickText(wrapper, '删除')
    await flushPromises()
    api.sandboxes.remove.mockRejectedValueOnce(new Error('删除失败'))
    await clickText(wrapper, '删除')
    await flushPromises()
    api.sandboxes.list.mockResolvedValue([{ ...sample, state: 'paused', status: 'expired' }])
    await clickText(wrapper, '刷新')
    await flushPromises()
    api.sandboxes.connect.mockRejectedValueOnce('boom')
    await clickText(wrapper, '恢复')
    await flushPromises()
    api.sandboxes.capabilities.mockRejectedValueOnce('boom')
    await clickText(wrapper, '刷新')
    await flushPromises()
    expect(api.sandboxes.pause).toHaveBeenCalledTimes(2)
    expect(api.sandboxes.remove).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('does not list sandboxes when the backend is unavailable', async () => {
    api.sandboxes.capabilities.mockResolvedValue({ ...available, available: false, reason: 'migration v50 has not been applied' })
    const { wrapper } = await mountWithApp(Sandboxes, { stubs })
    await flushPromises()
    expect(api.sandboxes.list).not.toHaveBeenCalled()
    // 不可用时不显示「暂无云沙盒」这种引导式空态
    expect(wrapper.text()).not.toContain('暂无云沙盒')
    wrapper.unmount()
  })

  it('skips loading without a project id', async () => {
    const { wrapper, pinia } = await mountWithApp(Sandboxes, { stubs })
    const { useProjectStore } = await import('@/stores/project')
    const calls = api.sandboxes.capabilities.mock.calls.length
    useProjectStore(pinia).projectId = ''
    await flushPromises()
    expect(api.sandboxes.capabilities.mock.calls.length).toBe(calls)
    wrapper.unmount()
  })
})
