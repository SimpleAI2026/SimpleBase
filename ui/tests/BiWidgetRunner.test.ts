import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { toast } from 'vue-sonner'
import BiWidgetRunner from '@/components/bi/BiWidgetRunner.vue'
import { api, resetApiMocks } from '@/test/api-mock'
import { uiStubs } from '@/test/helpers'
import type { BiWidget, CollectionField } from '@/services/types'

vi.mock('@/services/api', async () => {
  const module = await import('@/test/api-mock')
  return { api: module.api }
})

// 图表容器依赖 canvas：这里只关心 Runner 传入的 option 与点击透传。
vi.mock('@/components/bi/BiChart.vue', () => ({
  default: {
    name: 'BiChart',
    props: ['option', 'ariaLabel', 'heightClass'],
    emits: ['select'],
    template: '<div class="bi-chart-stub" :data-series="(option.series || []).map((s) => s.type).join(\',\')"><button type="button" class="bi-chart-click" @click="$emit(\'select\', { name: clickName, seriesName: \'\', value: 1 })">click</button></div>',
    data: () => ({ clickName: (globalThis as { __clickName?: string }).__clickName ?? 'x' })
  }
}))

const f = (name: string, type: CollectionField['type']): CollectionField => ({
  name, type, required: false, unique: false, options: {}, hidden: false, readonly: false
})
const fields = [f('amount', 'number'), f('qty', 'number'), f('status', 'select'), f('title', 'text'), f('placed', 'date')]
const target = { projectId: 'p1', databaseId: 'db1', collection: 'orders' }
const widget = (over: Partial<BiWidget> = {}): BiWidget => ({ id: 'w1', title: 'T', type: 'auto', metrics: ['count'], span: 6, ...over })

function agg(columns: string[], rows: unknown[][]) {
  return { columns, rows, truncated: false, scanned: rows.length, durationMs: 1, requestId: '' }
}

async function mountRunner(w: BiWidget, click = 'x', props: Record<string, unknown> = {}) {
  ;(globalThis as { __clickName?: string }).__clickName = click
  const wrapper = mount(BiWidgetRunner, {
    props: { widget: w, target, fields, globalFilter: '', refreshTick: 0, ...props } as never,
    global: { stubs: { ...uiStubs, Button: false, Spinner: false } }
  })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  resetApiMocks()
  api.db.aggregate.mockResolvedValue(agg(['x', 'count'], [['a', 2], ['b', 1]]))
})

describe('BiWidgetRunner request building', () => {
  it('combines the page filter and the widget filter with AND and sends aggregate params', async () => {
    await mountRunner(widget({ groupBy: 'status', filter: 'qty>1', sort: '-count', limit: 5 }), 'x', { globalFilter: 'amount>0' })
    expect(api.db.aggregate).toHaveBeenCalledWith('p1', 'db1', 'orders', {
      metrics: ['count'], groupBy: 'status', series: undefined, filter: '(amount>0) && (qty>1)', sort: '-count', limit: 5
    })
  })

  it('sends only the widget filter or only the page filter when the other is empty, and none when both are', async () => {
    await mountRunner(widget({ groupBy: 'status', filter: 'qty>1' }))
    expect(api.db.aggregate.mock.calls[0][3].filter).toBe('qty>1')
    resetApiMocks()
    api.db.aggregate.mockResolvedValue(agg(['x', 'count'], [['a', 1]]))
    await mountRunner(widget({ groupBy: 'status' }), 'x', { globalFilter: 'amount>0' })
    expect(api.db.aggregate.mock.calls[0][3].filter).toBe('amount>0')
    resetApiMocks()
    api.db.aggregate.mockResolvedValue(agg(['x', 'count'], [['a', 1]]))
    await mountRunner(widget({ groupBy: 'status' }))
    expect(api.db.aggregate.mock.calls[0][3].filter).toBeUndefined()
  })

  it('a scatter widget requests raw points, not aggregates', async () => {
    api.db.aggregate.mockResolvedValue(agg(['x', 'y'], [[1, 2]]))
    await mountRunner(widget({ type: 'scatter', metrics: [], x: 'amount', y: 'qty', size: 'qty', series: 'status', limit: 300 }))
    expect(api.db.aggregate).toHaveBeenCalledWith('p1', 'db1', 'orders', {
      mode: 'points', x: 'amount', y: 'qty', size: 'qty', series: 'status', filter: undefined, limit: 300
    })
  })

  it('re-requests when the global filter or the refresh tick changes, but not when nothing changed', async () => {
    const wrapper = await mountRunner(widget({ groupBy: 'status' }))
    expect(api.db.aggregate).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ globalFilter: 'amount>1' })
    await flushPromises()
    expect(api.db.aggregate).toHaveBeenCalledTimes(2)
    await wrapper.setProps({ globalFilter: 'amount>1' })
    await flushPromises()
    expect(api.db.aggregate).toHaveBeenCalledTimes(2)
    await wrapper.setProps({ refreshTick: 1 })
    await flushPromises()
    expect(api.db.aggregate).toHaveBeenCalledTimes(3)
  })

  it('re-requests when the widget configuration changes (edited chart)', async () => {
    const wrapper = await mountRunner(widget({ groupBy: 'status' }))
    await wrapper.setProps({ widget: widget({ groupBy: 'title' }) })
    await flushPromises()
    expect(api.db.aggregate).toHaveBeenCalledTimes(2)
    expect(api.db.aggregate.mock.calls[1][3].groupBy).toBe('title')
  })

  it('re-requests when the collection changes', async () => {
    const wrapper = await mountRunner(widget({ groupBy: 'status' }))
    await wrapper.setProps({ target: { ...target, collection: 'users' } })
    await flushPromises()
    expect(api.db.aggregate.mock.calls[1].slice(0, 3)).toEqual(['p1', 'db1', 'users'])
  })

  it('cancels the in-flight request on unmount so a late response cannot update a dead card', async () => {
    let resolve!: (v: ReturnType<typeof agg>) => void
    api.db.aggregate.mockReturnValue(new Promise((r) => { resolve = r }))
    const wrapper = await mountRunner(widget({ groupBy: 'status' }))
    wrapper.unmount()
    resolve(agg(['x', 'count'], [['a', 1]]))
    await flushPromises()
    expect(api.db.aggregate).toHaveBeenCalledTimes(1)
  })
})

describe('BiWidgetRunner rendering', () => {
  it('renders a chart for grouped results and a KPI for ungrouped single metrics', async () => {
    const grouped = await mountRunner(widget({ type: 'column', groupBy: 'status' }))
    expect(grouped.find('.bi-chart-stub').exists()).toBe(true)
    expect(grouped.get('.bi-chart-stub').attributes('data-series')).toBe('bar')

    api.db.aggregate.mockResolvedValue(agg(['count'], [[42]]))
    const kpi = await mountRunner(widget())
    expect(kpi.get('[data-testid="kpi-value"]').text()).toBe('42')
  })

  it('"auto" resolves from the real result: few select categories become a pie', async () => {
    const wrapper = await mountRunner(widget({ type: 'auto', groupBy: 'status' }))
    expect(wrapper.get('.bi-chart-stub').attributes('data-series')).toBe('pie')
  })

  it('"auto" re-resolves once the real category count is known (many categories fall back to a bar)', async () => {
    api.db.aggregate.mockResolvedValue(agg(['x', 'count'], Array.from({ length: 15 }, (_, i) => [`long-category-name-${i}`, i + 1])))
    const wrapper = await mountRunner(widget({ type: 'auto', groupBy: 'title' }))
    expect(wrapper.get('.bi-chart-stub').attributes('data-series')).toBe('bar')
  })

  it('a time-bucket widget draws a line', async () => {
    api.db.aggregate.mockResolvedValue(agg(['x', 'count'], [['2026-11-16', 2], ['2026-11-17', 3]]))
    const wrapper = await mountRunner(widget({ type: 'auto', groupBy: 'placed:day' }))
    expect(wrapper.get('.bi-chart-stub').attributes('data-series')).toBe('line')
  })

  it('shows the backend error with a retry that re-queries', async () => {
    api.db.aggregate.mockRejectedValueOnce(new Error('scan limit exceeded'))
    const wrapper = await mountRunner(widget({ groupBy: 'status' }))
    expect(wrapper.text()).toContain('scan limit exceeded')
    api.db.aggregate.mockResolvedValue(agg(['x', 'count'], [['a', 1]]))
    const retry = wrapper.findAll('button').find((b) => b.text().includes('重试'))!
    await retry.trigger('click')
    await flushPromises()
    expect(api.db.aggregate).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('scan limit exceeded')
  })

  it('names metric columns in the table view with human labels', async () => {
    api.db.aggregate.mockResolvedValue(agg(['x', 'count', 'sum_amount', 'avg_qty', 'weird'], [['a', 1, 10, 2.5, 7]]))
    const wrapper = await mountRunner(widget({ type: 'table', metrics: ['count', 'sum:amount', 'avg:qty'], groupBy: 'status' }))
    const heads = wrapper.findAll('th').map((h) => h.text())
    expect(heads).toEqual(['status', '记录数', 'amount 合计', 'qty 平均', 'weird'])
  })

  it('exposes the query, reload and resolved type for the page', async () => {
    const wrapper = await mountRunner(widget({ groupBy: 'status' }))
    const vm = wrapper.vm as unknown as { query: { result: { value: unknown } }; reload: () => void; resolved: string }
    expect(vm.query.result.value).not.toBeNull()
    expect(typeof vm.resolved).toBe('string')
    vm.reload()
    await flushPromises()
    expect(api.db.aggregate).toHaveBeenCalledTimes(2)
  })
})

describe('BiWidgetRunner ranked requests (pie / bar)', () => {
  it('a category pie asks for the ranking, not the alphabetical head: descending by its metric with room for the long tail', async () => {
    await mountRunner(widget({ type: 'pie', groupBy: 'status', limit: 8 }))
    const q = api.db.aggregate.mock.calls[0][3]
    expect(q.sort).toBe('-count')
    // 取数上限与展示的 Top N 分开：limit 只是展示 N，取数要更多，才算得出「其他」。
    expect(q.limit).toBeGreaterThan(8)
  })

  it('keeps an explicit sort and ranks by the first metric', async () => {
    await mountRunner(widget({ type: 'bar', groupBy: 'title', metrics: ['sum:amount'] }))
    expect(api.db.aggregate.mock.calls[0][3].sort).toBe('-sum_amount')
    resetApiMocks()
    api.db.aggregate.mockResolvedValue(agg(['x', 'count'], [['a', 1]]))
    await mountRunner(widget({ type: 'bar', groupBy: 'title', sort: 'x' }))
    expect(api.db.aggregate.mock.calls[0][3].sort).toBe('x')
  })

  it('does not rewrite ordered dimensions, splits, or other chart types', async () => {
    await mountRunner(widget({ type: 'pie', groupBy: 'placed:day' }))
    expect(api.db.aggregate.mock.calls[0][3]).toMatchObject({ sort: undefined, limit: undefined })
    resetApiMocks()
    api.db.aggregate.mockResolvedValue(agg(['x', 'count'], [['a', 1]]))
    await mountRunner(widget({ type: 'column', groupBy: 'status', limit: 5 }))
    expect(api.db.aggregate.mock.calls[0][3]).toMatchObject({ sort: undefined, limit: 5 })
  })

  it('asks for the same-scope total only for a single additive pie, never for other charts', async () => {
    await mountRunner(widget({ type: 'pie', groupBy: 'status', metrics: ['count'] }))
    expect(api.db.aggregate.mock.calls[0][3]).toMatchObject({ withTotal: true })
    resetApiMocks(); api.db.aggregate.mockResolvedValue(agg(['x', 'count'], [['a', 1]]))
    // avg 不可累加：后端不会给总量，前端也不要。
    await mountRunner(widget({ type: 'pie', groupBy: 'status', metrics: ['avg:amount'] }))
    expect(api.db.aggregate.mock.calls[0][3].withTotal).toBeUndefined()
    resetApiMocks(); api.db.aggregate.mockResolvedValue(agg(['x', 'count'], [['a', 1]]))
    await mountRunner(widget({ type: 'bar', groupBy: 'status', metrics: ['count'] }))
    expect(api.db.aggregate.mock.calls[0][3].withTotal).toBeUndefined()
  })

  it('reports every settled request to the page, with the backend error code', async () => {
    const wrapper = await mountRunner(widget({ groupBy: 'status' }))
    expect(wrapper.emitted('outcome')?.[0]).toEqual([{ ok: true, code: '', message: '' }])
    resetApiMocks()
    api.db.aggregate.mockRejectedValueOnce(Object.assign(new Error('bad filter'), { code: 'invalid_filter' }))
    const failed = await mountRunner(widget({ groupBy: 'status' }))
    expect(failed.emitted('outcome')?.[0]).toEqual([{ ok: false, code: 'invalid_filter', message: 'bad filter' }])
  })

  it('never asks for more than the backend allows', async () => {
    await mountRunner(widget({ type: 'bar', groupBy: 'title', limit: 500 }))
    expect(api.db.aggregate.mock.calls[0][3].limit).toBeLessThanOrEqual(500)
  })
})

describe('BiWidgetRunner drill identity', () => {
  const many = Array.from({ length: 12 }, (_, i) => [`c${i}`, 100 - i])

  it('clicking the merged "other" slice does not build a bogus field="其他" chip', async () => {
    api.db.aggregate.mockResolvedValue(agg(['x', 'count'], many))
    const wrapper = await mountRunner(widget({ type: 'pie', groupBy: 'title', limit: 8 }), '其他')
    await wrapper.get('.bi-chart-click').trigger('click')
    expect(wrapper.emitted('chip')).toBeUndefined()
    // 告诉用户为什么没反应，而不是静默无事发生。
    expect(toast.info).toHaveBeenCalledWith(expect.stringContaining('合并'))
  })

  it('clicking an empty category does not produce a chip either', async () => {
    api.db.aggregate.mockResolvedValue(agg(['x', 'count'], [[null, 3], ['a', 1]]))
    const wrapper = await mountRunner(widget({ type: 'pie', groupBy: 'title' }), '（空）')
    await wrapper.get('.bi-chart-click').trigger('click')
    expect(wrapper.emitted('chip')).toBeUndefined()
  })

  it('a real category that merely looks special still drills when it is unambiguous', async () => {
    api.db.aggregate.mockResolvedValue(agg(['x', 'count'], [['其他', 3], ['a', 1]]))
    const wrapper = await mountRunner(widget({ type: 'pie', groupBy: 'title' }), '其他')
    await wrapper.get('.bi-chart-click').trigger('click')
    expect((wrapper.emitted('chip')![0][0] as { expr: string }).expr).toBe('title="其他"')
  })
})

describe('BiWidgetRunner click-to-filter', () => {
  it('a category click emits an equality chip typed by the field', async () => {
    const wrapper = await mountRunner(widget({ type: 'column', groupBy: 'status' }), 'paid')
    await wrapper.get('.bi-chart-click').trigger('click')
    expect(wrapper.emitted('chip')).toEqual([[{ id: 'status="paid"', label: 'status=paid', expr: 'status="paid"' }]])
  })

  it('a numeric category is compared as a number', async () => {
    const wrapper = await mountRunner(widget({ type: 'column', groupBy: 'qty' }), '3')
    await wrapper.get('.bi-chart-click').trigger('click')
    expect((wrapper.emitted('chip')![0][0] as { expr: string }).expr).toBe('qty=3')
  })

  it('a time bucket click emits a half-open time range', async () => {
    const wrapper = await mountRunner(widget({ type: 'line', groupBy: 'placed:day' }), '2026-11-16')
    await wrapper.get('.bi-chart-click').trigger('click')
    expect((wrapper.emitted('chip')![0][0] as { expr: string }).expr).toBe('placed>="2026-11-16T00:00:00Z" && placed<"2026-11-17T00:00:00Z"')
  })

  it('a bin click emits [start, start+step)', async () => {
    const wrapper = await mountRunner(widget({ type: 'column', groupBy: 'amount:bin=10' }), '20')
    await wrapper.get('.bi-chart-click').trigger('click')
    expect((wrapper.emitted('chip')![0][0] as { expr: string }).expr).toBe('amount>=20 && amount<30')
  })

  it('an unparsable time bucket produces no chip rather than a wrong range', async () => {
    const wrapper = await mountRunner(widget({ type: 'line', groupBy: 'placed:day' }), 'not-a-date')
    await wrapper.get('.bi-chart-click').trigger('click')
    expect(wrapper.emitted('chip')).toBeUndefined()
  })

  it('an ungrouped chart has nothing to filter on', async () => {
    api.db.aggregate.mockResolvedValue(agg(['x', 'y'], [[1, 2]]))
    const wrapper = await mountRunner(widget({ type: 'scatter', metrics: [], x: 'amount', y: 'qty' }))
    await wrapper.get('.bi-chart-click').trigger('click')
    expect(wrapper.emitted('chip')).toBeUndefined()
  })

  it('a hostile category value is escaped inside the chip expression', async () => {
    const wrapper = await mountRunner(widget({ type: 'column', groupBy: 'title' }), 'x" || title="y')
    await wrapper.get('.bi-chart-click').trigger('click')
    expect((wrapper.emitted('chip')![0][0] as { expr: string }).expr).toBe('title="x\\" || title=\\"y"')
  })
})

describe('BiWidgetRunner events', () => {
  it('board mode forwards edit / duplicate / remove', async () => {
    const w = widget({ groupBy: 'status' })
    const wrapper = await mountRunner(w)
    await wrapper.get('[data-action="edit"]').trigger('click')
    await wrapper.get('[data-action="duplicate"]').trigger('click')
    await wrapper.get('[data-action="remove"]').trigger('click')
    expect(wrapper.emitted('edit')).toEqual([[w]])
    expect(wrapper.emitted('duplicate')).toEqual([['w1']])
    expect(wrapper.emitted('remove')).toEqual([[w]])
  })

})
