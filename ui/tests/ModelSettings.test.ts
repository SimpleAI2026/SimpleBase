import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ModelSettings from '@/pages/ModelSettings.vue'
import { resetApiMocks } from '@/test/api-mock'
import { clickTab, mountWithApp } from '@/test/helpers'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

const extraStubs = {
  ModelDefaultsPanel: { template: '<div class="panel-models" />' },
  ModelTraceList: { template: '<div class="panel-traces" />' },
  McpServersPanel: { template: '<div class="panel-mcp" />' },
  ApprovalsPanel: { template: '<div class="panel-approvals" />' },
  TraceDetailModal: {
    emits: ['close'],
    template: '<div class="trace-modal" @click="$emit(\'close\')" />'
  }
}

describe('ModelSettings', () => {
  beforeEach(() => {
    resetApiMocks()
  })

  it('renders four tabs in order and switches between them', async () => {
    const { wrapper } = await mountWithApp(ModelSettings, {
      projectId: 'p1',
      path: '/models',
      stubs: extraStubs
    })
    // 页签顺序固定：模型 → MCP → 审批队列 → Trace。
    const triggers = wrapper.findAll('[role="tab"]')
    expect(triggers.map((x) => x.text())).toEqual(['模型', 'MCP', '审批队列', 'Trace'])
    expect(wrapper.find('.panel-models').exists()).toBe(true)

    const vm = wrapper.vm as unknown as {
      activeTab: string
      onTabChange: (t: string) => void
      openDetail: (id: string) => void
    }
    // 真实页签触发器：点「MCP」
    await clickTab(wrapper, 'mcp')
    await flushPromises()
    expect(vm.activeTab).toBe('mcp')
    expect(wrapper.find('.panel-mcp').exists()).toBe(true)

    vm.onTabChange('approvals')
    await flushPromises()
    expect(vm.activeTab).toBe('approvals')
    expect(wrapper.find('.panel-approvals').exists()).toBe(true)

    vm.onTabChange('traces')
    await flushPromises()
    expect(vm.activeTab).toBe('traces')
    vm.openDetail('trace-1')

    vm.onTabChange('models')
    await flushPromises()
    expect(vm.activeTab).toBe('models')
  })

  it('reads the tab from the query string', async () => {
    for (const [query, want] of [
      ['mcp', 'mcp'],
      ['approvals', 'approvals'],
      ['traces', 'traces'],
      ['nope', 'models']
    ] as const) {
      const { wrapper } = await mountWithApp(ModelSettings, {
        projectId: 'p1',
        path: `/models?tab=${query}`,
        stubs: extraStubs
      })
      expect((wrapper.vm as unknown as { activeTab: string }).activeTab).toBe(want)
      wrapper.unmount()
    }
  })

  it('closes the trace detail modal', async () => {
    const { wrapper } = await mountWithApp(ModelSettings, {
      projectId: 'p1',
      path: '/models',
      stubs: extraStubs
    })
    const vm = wrapper.vm as unknown as { detailTraceId?: string; openDetail: (id: string) => void }
    vm.openDetail('trace-9')
    await flushPromises()
    expect(vm.detailTraceId).toBe('trace-9')
    await wrapper.get('.trace-modal').trigger('click')
    await flushPromises()
    expect(vm.detailTraceId).toBeUndefined()
  })

})