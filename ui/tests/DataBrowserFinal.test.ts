import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import DataBrowser from '@/pages/DataBrowser.vue'
import { useBiAgent, threadKey } from '@/composables/useBiAgent'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import type { AgentStreamHandlers, CloudAgent, CollectionField } from '@/services/types'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})
vi.mock('@/components/bi/BiChart.vue', () => ({
  default: { name: 'BiChart', props: ['option', 'ariaLabel', 'heightClass'], emits: ['select'], template: '<div class="bi-chart-stub" />' }
}))

const field = (name: string, type: CollectionField['type']): CollectionField => ({
  name, type, required: false, unique: false, hidden: false, readonly: false, options: {}
})
const SCHEMA = [field('title', 'text'), field('state', 'select')]

beforeEach(() => {
  vi.mocked(toast.error).mockClear()
  resetApiMocks()
  window.localStorage.clear()
})

function page(projectId: string | null = 'p1') {
  const pinia = createPinia()
  setActivePinia(pinia)
  if (projectId) useProjectStore().setProject(projectId, 'Project')
  Object.defineProperty(useAuthStore(), 'canWrite', { value: true, configurable: true })
  api.databases.list.mockResolvedValue([{ id: 'db1', name: 'Database', status: 'ready', dataModel: 'collection' }])
  api.db.collectionItems.mockResolvedValue([{ name: 'tasks', schemaMode: 'json', fieldCount: 2, documentCount: 3, registered: true }])
  api.db.getCollectionSchema.mockResolvedValue({ name: 'tasks', schemaMode: 'json', fields: SCHEMA })
  api.db.documents.mockResolvedValue({ rows: [], total: 1, items: [] })
  api.db.aggregate.mockResolvedValue({ columns: ['count'], rows: [[1]], truncated: false, scanned: 1, durationMs: 1, requestId: '' })
  api.agents.list.mockResolvedValue([])
  return mount(DataBrowser, { global: { plugins: [pinia], stubs: uiStubs } })
}

describe('DataBrowser widget capacity', () => {
  /** 预先写入 24 张图表的看板配置，页面加载时按它渲染。 */
  function seedFull() {
    const widgets = Array.from({ length: 24 }, (_, i) => ({ id: `w${i}`, title: `W${i}`, type: 'auto', metrics: ['count'], span: 4 }))
    window.localStorage.setItem('sb_bi_v1:p1:db1:tasks', JSON.stringify({ version: 1, widgets }))
  }

  it('duplicating on a full board tells the user instead of silently doing nothing', async () => {
    seedFull()
    const wrapper = page()
    await flushPromises()
    expect(wrapper.findAll('[data-testid="bi-board"] [data-widget]')).toHaveLength(24)
    await wrapper.get('[data-widget="w0"] [data-action="menu"]').trigger('click')
    await wrapper.get('[data-widget="w0"] [data-action="duplicate"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="bi-board"] [data-widget]')).toHaveLength(24)
    expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('24'))
  })

  it('adding a new chart on a full board reports the limit and does not pretend it worked', async () => {
    seedFull()
    const wrapper = page()
    await flushPromises()
    await wrapper.get('[data-action="add-widget"]').trigger('click')
    await wrapper.get('[data-testid="bi-editor"] [data-field="title"]').setValue('第 25 张')
    await wrapper.get('.sb-modal .sb-ok').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="bi-board"] [data-widget]')).toHaveLength(24)
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).not.toContain('第 25 张')
    expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('24'))
  })
})

describe('DataBrowser remove confirmation', () => {
  it('removing a widget asks first; cancelling keeps it, confirming deletes it and persists', async () => {
    const wrapper = page()
    await flushPromises()
    await wrapper.get('[data-widget="kpi-total"] [data-action="menu"]').trigger('click')
    await wrapper.get('[data-widget="kpi-total"] [data-action="remove"]').trigger('click')
    await flushPromises()

    // 受控 ConfirmAction：弹出时标题带上图表名。共享桩点击即确认，所以先取消再确认，分别断言。
    const confirms = wrapper.findAllComponents(uiStubs.ConfirmAction)
    const removeConfirm = confirms.find((c) => String(c.props('title')).includes('总记录'))!
    expect(removeConfirm).toBeDefined()
    expect(removeConfirm.props('title')).toBe('删除图表「总记录」？')

    removeConfirm.vm.$emit('update:open', false)
    await flushPromises()
    expect(wrapper.find('[data-widget="kpi-total"]').exists()).toBe(true)

    await wrapper.get('[data-widget="kpi-total"] [data-action="menu"]').trigger('click')
    await wrapper.get('[data-widget="kpi-total"] [data-action="remove"]').trigger('click')
    await flushPromises()
    wrapper.findAllComponents(uiStubs.ConfirmAction).find((c) => String(c.props('title')).includes('总记录'))!.vm.$emit('confirm')
    await flushPromises()
    expect(wrapper.find('[data-widget="kpi-total"]').exists()).toBe(false)
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).not.toContain('kpi-total')
  })

  it('confirming with nothing selected is a harmless no-op', async () => {
    const wrapper = page()
    await flushPromises()
    const before = wrapper.findAll('[data-testid="bi-board"] [data-widget]').length
    // 未选中任何图表时，删除确认的标题里图表名为空：「删除图表「」？」。
    const confirm = wrapper.findAllComponents(uiStubs.ConfirmAction).find((c) => c.props('title') === '删除图表「」？')!
    expect(confirm).toBeDefined()
    confirm.vm.$emit('confirm')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="bi-board"] [data-widget]').length).toBe(before)
  })
})

describe('DataBrowser without a project', () => {
  it('shows nothing and makes no data requests', async () => {
    const wrapper = page(null)
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-board"]').exists()).toBe(false)
    expect(api.db.aggregate).not.toHaveBeenCalled()
    expect(api.db.collectionItems).not.toHaveBeenCalled()
  })
})

describe('useBiAgent storage failures', () => {
  const agent: CloudAgent = {
    id: 'ag', name: 'Database', module: 'database', description: '', system_prompt: '', tool_ids: [],
    builtin_key: 'database', team_enabled: false, writable: false, created_at: '', updated_at: ''
  }
  const scope = { projectId: 'p1', databaseId: 'db1', collection: 'orders' }
  const brokenStorage = (): Storage => ({
    get length() { return 0 }, clear: () => undefined, key: () => null,
    getItem: () => null,
    setItem: () => { throw new Error('quota') },
    removeItem: () => { throw new Error('denied') }
  })

  it('still creates the thread and runs when the session id cannot be persisted', async () => {
    api.agents.list.mockResolvedValue([agent])
    api.agentThreads.create.mockResolvedValue({ id: 'th', title: 't', created_at: '', updated_at: '' })
    const calls: Array<Record<string, unknown>> = []
    api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, req: Record<string, unknown>, _h: AgentStreamHandlers) => {
      calls.push(req)
      return { close: vi.fn() }
    })
    const bi = useBiAgent({ scope: () => scope, fields: () => [field('amount', 'number')], preferredType: () => 'auto', storage: brokenStorage() })
    await bi.send('画图')
    expect(calls).toHaveLength(1)
    expect(bi.failure.value).toBe('')
  })

  it('"new chat" survives a storage that refuses to delete', async () => {
    const bi = useBiAgent({ scope: () => scope, fields: () => [], preferredType: () => 'auto', storage: brokenStorage() })
    await expect(bi.newChat()).resolves.toBeUndefined()
    expect(bi.messages.value).toEqual([])
  })

  it('threadKey is stable per scope', () => {
    expect(threadKey(scope)).toBe('sb_bi_thread_v1:p1:db1:orders')
  })
})
