import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DataBrowser from '@/pages/DataBrowser.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import { pageVm } from '@/test/page-vm'
import { useProjectStore } from '@/stores/project'
import { useAuthStore } from '@/stores/auth'
import type { CloudAgent, CollectionField } from '@/services/types'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

vi.mock('@/components/bi/BiChart.vue', () => ({
  default: {
    name: 'BiChart',
    props: ['option', 'ariaLabel', 'heightClass'],
    emits: ['select'],
    template: '<div class="bi-chart-stub" />'
  }
}))

const field = (name: string, type: CollectionField['type'], values?: string[]): CollectionField => ({
  name, type, required: false, unique: false, hidden: false, readonly: false, options: values ? { values } : {}
})
const SCHEMA = [field('title', 'text'), field('priority', 'number'), field('state', 'select', ['open', 'closed']), field('published', 'date')]
const chart = (o: Record<string, unknown>) => `<!-- sb:chart ${JSON.stringify({ version: 1, ...o })} -->`
const dataAgent: CloudAgent = {
  id: 'ag-db', name: 'Database', module: 'database', description: '', system_prompt: '', tool_ids: [],
  builtin_key: 'database', team_enabled: false, writable: false, created_at: '', updated_at: ''
}

interface Handlers { onToken?: (t: string) => void; onEnd?: (r: string) => void }
interface Call { req: Record<string, unknown>; handlers: Handlers }
function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useProjectStore().setProject('p1', 'Project')
  Object.defineProperty(useAuthStore(), 'canWrite', { value: true, configurable: true })
  api.databases.list.mockResolvedValue([{ id: 'db1', name: 'Database', status: 'ready', dataModel: 'collection' }])
  api.db.collectionItems.mockResolvedValue([{ name: 'tasks', schemaMode: 'json', fieldCount: 4, documentCount: 3, registered: true }])
  api.db.getCollectionSchema.mockResolvedValue({ name: 'tasks', schemaMode: 'json', fields: SCHEMA })
  api.db.documents.mockResolvedValue({
    rows: [], total: 1,
    items: [{ id: 'r1', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-02T00:00:00Z', data: { title: 'One', state: 'open' } }]
  })
  api.db.aggregate.mockImplementation(async (_p, _d, _c, q) => ({
    columns: q.groupBy ? ['x', 'count'] : ['count'],
    rows: q.groupBy ? [['open', 2], ['closed', 1]] : [[3]],
    truncated: false, scanned: 3, durationMs: 1, requestId: ''
  }))
  api.agents.list.mockResolvedValue([dataAgent])
  api.agentThreads.create.mockResolvedValue({ id: 'th1', title: 't', created_at: '', updated_at: '' })
  api.agentThreads.messages.mockResolvedValue([])
  return mount(DataBrowser, { global: { plugins: [pinia], stubs: uiStubs } })
}

/** 捕获 streamRun 的请求与 handlers，由测试手动驱动流事件。 */
function captureStream(): Call[] {
  const calls: Call[] = []
  api.agentThreads.streamRun.mockImplementation((_p: string, _t: string, req: Record<string, unknown>, handlers: Handlers) => {
    calls.push({ req, handlers })
    return { close: vi.fn() }
  })
  return calls
}

async function ask(wrapper: ReturnType<typeof setup>, text: string) {
  await wrapper.get('[data-testid="bi-agent-input"]').setValue(text)
  await wrapper.get('[data-action="send"]').trigger('click')
  await flushPromises()
}

/** 起页面、发一句话，并让助手回复 content。 */
async function withReply(content: string) {
  const wrapper = setup()
  await flushPromises()
  const calls = captureStream()
  await ask(wrapper, '各状态数量')
  calls[0].handlers.onToken?.(content)
  calls[0].handlers.onEnd?.('stop')
  await flushPromises()
  return { wrapper, calls }
}

beforeEach(() => {
  resetApiMocks()
  window.localStorage.clear()
})

describe('DataBrowser agent wiring', () => {
  it('sends bi_context for the selected collection and preferred type', async () => {
    const wrapper = setup()
    await flushPromises()
    const calls = captureStream()
    wrapper.getComponent({ name: 'BiToolbar' }).vm.$emit('update:chartType', 'stacked_bar')
    await flushPromises()
    await ask(wrapper, '画图')
    expect(calls[0].req.bi_context).toMatchObject({ database_id: 'db1', collection: 'tasks', preferred_type: 'stacked_bar' })
    const names = (calls[0].req.bi_context as { fields: Array<{ name: string }> }).fields.map((x) => x.name)
    expect(names).toEqual(['title', 'priority', 'state', 'published', 'created_at', 'updated_at'])
  })

  it('previews each valid chart with a real aggregate query and leaves the board untouched until confirmed', async () => {
    const { wrapper } = await withReply(`好的\n${chart({ title: '状态占比', type: 'pie', metrics: ['count'], group_by: 'state' })}`)
    const pending = wrapper.get('[data-testid="bi-agent-pending"]')
    expect(pending.find('[data-mode="preview"]').exists()).toBe(true)
    expect(pending.text()).toContain('状态占比')
    // 预览走和看板同一条聚合接口。
    expect(api.db.aggregate.mock.calls.some(([, , , q]) => q.groupBy === 'state' && q.metrics?.join() === 'count')).toBe(true)
    // 加入之前：看板里没有预览卡，也没有写入看板配置。
    expect(wrapper.find('[data-testid="bi-board"] [data-mode="preview"]').exists()).toBe(false)
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).toBeNull()
  })

  it('"add to dashboard" moves the chart onto the board, persists it and clears the preview', async () => {
    const { wrapper } = await withReply(chart({ title: '状态占比', type: 'pie', metrics: ['count'], group_by: 'state' }))
    await wrapper.get('[data-testid="bi-agent-pending"] [data-action="add"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-agent-pending"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="bi-board"] .card h3').map((h) => h.text())).toContain('状态占比')
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).toContain('状态占比')
  })

  it('"discard" drops the preview without touching the board', async () => {
    const { wrapper } = await withReply(chart({ title: 'X', type: 'auto', metrics: ['count'] }))
    await wrapper.get('[data-testid="bi-agent-pending"] [data-action="discard"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-agent-pending"]').exists()).toBe(false)
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).toBeNull()
  })

  it('"adjust" updates the preview only: it is re-previewed and never reaches the board until confirmed', async () => {
    const { wrapper } = await withReply(chart({ title: '待调整', type: 'pie', metrics: ['count'], group_by: 'state' }))
    await wrapper.get('[data-testid="bi-agent-pending"] [data-action="adjust"]').trigger('click')
    await flushPromises()
    expect((wrapper.get('[data-testid="bi-editor"] [data-field="title"]').element as HTMLInputElement).value).toBe('待调整')
    api.db.aggregate.mockClear()
    await wrapper.get('.sb-modal .sb-ok').trigger('click')
    await flushPromises()
    // 保存调整只更新待确认项：预览还在、看板与本地存储都没有变化。
    expect(wrapper.find('[data-testid="bi-agent-pending"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="bi-board"] .card h3').map((h) => h.text())).not.toContain('待调整')
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).toBeNull()
    // 之后用户显式确认才加入。
    await wrapper.get('[data-testid="bi-agent-pending"] [data-action="add"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-agent-pending"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="bi-board"] .card h3').map((h) => h.text())).toContain('待调整')
  })

  it('a rejected block is explained and never becomes a preview', async () => {
    const { wrapper } = await withReply(chart({ title: '幽灵', type: 'bar', metrics: ['count'], group_by: 'nope' }))
    expect(wrapper.find('[data-testid="bi-agent-pending"]').exists()).toBe(false)
    const text = wrapper.get('[data-testid="bi-agent-rejected"]').text()
    expect(text).toContain('幽灵')
    expect(text).toContain('字段不存在')
  })

  it('a failed preview offers "ask the agent to fix" and sends the error as a follow-up', async () => {
    const wrapper = setup()
    await flushPromises()
    const calls = captureStream()
    api.db.aggregate.mockImplementation(async (_p, _d, _c, q) => {
      if (q.filter === 'state="bogus"') throw new Error('filter rejected')
      return { columns: ['count'], rows: [[1]], truncated: false, scanned: 1, durationMs: 1, requestId: '' }
    })
    await ask(wrapper, '画')
    calls[0].handlers.onToken?.(chart({ title: '坏图', type: 'auto', metrics: ['count'], filter: 'state="bogus"' }))
    calls[0].handlers.onEnd?.('stop')
    await flushPromises()
    await wrapper.get('[data-testid="bi-agent-pending"] [data-action="fix"]').trigger('click')
    await flushPromises()
    expect(calls).toHaveLength(2)
    expect(String(calls[1].req.content)).toContain('坏图')
    expect(String(calls[1].req.content)).toContain('filter rejected')
  })

  it('switching collections clears the pending previews', async () => {
    const { wrapper } = await withReply(chart({ title: 'X', type: 'auto', metrics: ['count'] }))
    expect(wrapper.find('[data-testid="bi-agent-pending"]').exists()).toBe(true)
    api.db.collectionItems.mockResolvedValue([
      { name: 'tasks', schemaMode: 'json', fieldCount: 4, documentCount: 3, registered: true },
      { name: 'users', schemaMode: 'json', fieldCount: 1, documentCount: 1, registered: true }
    ])
    api.db.getCollectionSchema.mockResolvedValue({ name: 'users', schemaMode: 'json', fields: [field('email', 'email')] })
    const vm = pageVm(wrapper)
    await vm.reloadCollections()
    vm.selectCollection('users')
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-agent-pending"]').exists()).toBe(false)
  })

  it('restores the previous preview when coming back to the same collection, and keeps other collections clean', async () => {
    const { wrapper } = await withReply(chart({ title: '保留我', type: 'auto', metrics: ['count'] }))
    expect(wrapper.find('[data-testid="bi-agent-pending"]').exists()).toBe(true)
    api.db.collectionItems.mockResolvedValue([
      { name: 'tasks', schemaMode: 'json', fieldCount: 4, documentCount: 3, registered: true },
      { name: 'users', schemaMode: 'json', fieldCount: 1, documentCount: 1, registered: true }
    ])
    const vm = pageVm(wrapper)
    await vm.reloadCollections()
    vm.selectCollection('users')
    await flushPromises()
    expect(wrapper.find('[data-testid="bi-agent-pending"]').exists()).toBe(false)
    api.db.getCollectionSchema.mockResolvedValue({ name: 'tasks', schemaMode: 'json', fields: [field('title', 'text'), field('state', 'select'), field('published', 'date')] })
    vm.selectCollection('tasks')
    await flushPromises()
    expect(wrapper.get('[data-testid="bi-agent-pending"]').text()).toContain('保留我')
  })

  it('reports a missing data assistant instead of creating one', async () => {
    const wrapper = setup()
    await flushPromises()
    api.agents.list.mockResolvedValue([])
    await ask(wrapper, '画')
    expect(wrapper.get('[data-testid="bi-agent-disabled"]').text()).toContain('没有找到数据助手')
    expect(api.agents.create).not.toHaveBeenCalled()
    expect(api.agentThreads.streamRun).not.toHaveBeenCalled()
  })
})

describe('DataBrowser toolbar and board actions', () => {
  it('opens every data-action modal', async () => {
    const wrapper = setup()
    await flushPromises()
    for (const action of ['view', 'import', 'schema']) {
      await wrapper.get(`[data-action="${action}"]`).trigger('click')
      expect(wrapper.find('.sb-modal').exists(), action).toBe(true)
      await wrapper.get('.sb-modal-close').trigger('click')
    }
  })

  it('adding a widget opens a blank editor and saving puts it on the board', async () => {
    const wrapper = setup()
    await flushPromises()
    const before = wrapper.findAll('[data-testid="bi-board"] [data-widget]').length
    await wrapper.get('[data-action="add-widget"]').trigger('click')
    await wrapper.get('[data-testid="bi-editor"] [data-field="title"]').setValue('我的图')
    await wrapper.get('.sb-modal .sb-ok').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="bi-board"] [data-widget]').length).toBe(before + 1)
    expect(window.localStorage.getItem('sb_bi_v1:p1:db1:tasks')).toContain('我的图')
  })

  it('editing a board widget updates it in place', async () => {
    const wrapper = setup()
    await flushPromises()
    await wrapper.get('[data-widget="kpi-total"] [data-action="menu"]').trigger('click')
    await wrapper.get('[data-widget="kpi-total"] [data-action="edit"]').trigger('click')
    await wrapper.get('[data-testid="bi-editor"] [data-field="title"]').setValue('改名后')
    await wrapper.get('.sb-modal .sb-ok').trigger('click')
    await flushPromises()
    const titles = wrapper.findAll('[data-testid="bi-board"] .card h3').map((h) => h.text())
    expect(titles.filter((t) => t === '改名后')).toHaveLength(1)
    expect(wrapper.find('[data-widget="kpi-total"]').exists()).toBe(true)
  })

  it('appends a field to the filter, reuses a history entry and clears everything', async () => {
    const wrapper = setup()
    await flushPromises()
    await wrapper.get('[data-chip="field"]').trigger('click')
    expect((wrapper.get('input[data-field="filter"]').element as HTMLInputElement).value).toBe('title')
    await wrapper.get('input[data-field="filter"]').setValue('state="open"')
    await wrapper.get('[data-action="apply"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-action="clear"]').trigger('click')
    await flushPromises()
    api.db.aggregate.mockClear()
    await wrapper.get('[data-chip="history"]').trigger('click')
    await flushPromises()
    expect(api.db.aggregate.mock.calls.some(([, , , q]) => q.filter === 'state="open"')).toBe(true)
  })
})
