import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { toast } from 'vue-sonner'
import BiWidgetCard from '@/components/bi/BiWidgetCard.vue'
import BiDetailTable from '@/components/bi/BiDetailTable.vue'
import { uiStubs } from '@/test/helpers'
import type { AggregateResult, BiWidget } from '@/services/types'
import type { BiRender } from '@/utils/bi-chart'

// 图表容器依赖 canvas / ResizeObserver；这里只关心卡片传给它的 option、高度与点击透传。
vi.mock('@/components/bi/BiChart.vue', () => ({
  default: {
    name: 'BiChart',
    props: ['option', 'ariaLabel', 'heightClass'],
    emits: ['select'],
    template: '<div class="bi-chart-stub" :data-height="heightClass" :aria-label="ariaLabel"><button type="button" class="bi-chart-click" @click="$emit(\'select\', { name: \'a\', seriesName: \'s\', value: 1 })">click</button></div>'
  }
}))

const stubs = { ...uiStubs, Button: false, Spinner: false }
const widget = (over: Partial<BiWidget> = {}): BiWidget => ({ id: 'w1', title: '状态占比', type: 'pie', metrics: ['count'], groupBy: 'status', span: 6, ...over })
const result = (rows: unknown[][] = [['a', 1]], over: Partial<AggregateResult> = {}): AggregateResult => ({ columns: ['x', 'count'], rows, truncated: false, scanned: 1200, durationMs: 1, requestId: '', ...over })
const chartRender: BiRender = { kind: 'chart', option: { series: [] } }

function mountCard(props: Record<string, unknown> = {}) {
  return mount(BiWidgetCard, {
    props: { widget: widget(), result: result(), render: chartRender, resolved: 'pie', ...props } as never,
    global: { stubs }
  })
}

describe('BiWidgetCard board mode', () => {
  it('shows the title, scanned rows and passes the option to the chart', () => {
    const wrapper = mountCard()
    expect(wrapper.text()).toContain('状态占比')
    expect(wrapper.text()).toContain('参与统计 1,200 行')
    expect(wrapper.find('.bi-chart-stub').exists()).toBe(true)
    expect(wrapper.get('.bi-chart-stub').attributes('aria-label')).toBe('状态占比，饼图')
    expect(wrapper.find('[data-action="add"]').exists()).toBe(false)
    expect(wrapper.find('[data-action="fix"]').exists()).toBe(false)
  })

  it('forwards chart clicks', async () => {
    const wrapper = mountCard()
    await wrapper.get('.bi-chart-click').trigger('click')
    expect(wrapper.emitted('select')).toEqual([[{ name: 'a', seriesName: 's', value: 1 }]])
  })

  it('the more menu emits edit / duplicate / remove', async () => {
    const wrapper = mountCard()
    for (const action of ['edit', 'duplicate', 'remove']) await wrapper.get(`[data-action="${action}"]`).trigger('click')
    expect(wrapper.emitted('edit')).toHaveLength(1)
    expect(wrapper.emitted('duplicate')).toHaveLength(1)
    expect(wrapper.emitted('remove')).toHaveLength(1)
  })

  it('read-only hides the menu', () => {
    const wrapper = mountCard({ readonly: true })
    expect(wrapper.find('[data-action="menu"]').exists()).toBe(false)
  })

  it('renders a KPI value and label', () => {
    const wrapper = mountCard({ render: { kind: 'kpi', value: 12345.678, label: '金额合计' }, resolved: 'kpi' })
    expect(wrapper.get('[data-testid="kpi-value"]').text()).toBe('12,345.68')
    expect(wrapper.text()).toContain('金额合计')
    expect(wrapper.find('.bi-chart-stub').exists()).toBe(false)
    const none = mountCard({ render: { kind: 'kpi', value: null, label: 'x' }, resolved: 'kpi' })
    expect(none.get('[data-testid="kpi-value"]').text()).toBe('—')
  })

  it('renders a table render with headers and stringified cells', () => {
    const wrapper = mountCard({
      render: { kind: 'table', table: { columns: ['status', '记录数'], rows: [['open', 2], [null, { a: 1 }]] } },
      resolved: 'table'
    })
    const text = wrapper.get('table').text()
    expect(text).toContain('status')
    expect(text).toContain('记录数')
    expect(text).toContain('open')
    expect(text).toContain('{"a":1}')
  })

  it('shows the empty state when the result has no rows', () => {
    const wrapper = mountCard({ render: { kind: 'empty' }, result: result([]) })
    expect(wrapper.text()).toContain('当前条件下没有数据')
    expect(wrapper.find('.bi-chart-stub').exists()).toBe(false)
  })

  it('shows an error with retry', async () => {
    const wrapper = mountCard({ error: '查询失败', render: null, result: null })
    expect(wrapper.text()).toContain('查询失败')
    const retry = wrapper.findAll('button').find((b) => b.text().includes('重试'))
    await retry!.trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
  })

  it('notes when the result was truncated, distinguishing groups from points', () => {
    expect(mountCard({ result: result([['a', 1], ['b', 2]], { truncated: true }) }).text()).toContain('仅返回前 2 个分组，结果不完整')
    const points = mountCard({ result: { columns: ['x', 'y'], rows: [[1, 2], [3, 4]], truncated: true, scanned: 9, durationMs: 1, requestId: '' } })
    expect(points.text()).toContain('仅返回 2 个点')
    expect(mountCard().find('[data-testid="bi-truncated"]').exists()).toBe(false)
  })

  it('grows tall horizontal charts when there are many categories', () => {
    const many = result(Array.from({ length: 12 }, (_, i) => [`c${i}`, i]))
    expect(mountCard({ resolved: 'bar', result: many }).get('.bi-chart-stub').attributes('data-height')).toBe('h-[28rem]')
    expect(mountCard({ resolved: 'stacked_bar', result: many }).get('.bi-chart-stub').attributes('data-height')).toBe('h-[28rem]')
    // 竖向与少类目保持默认高度。
    expect(mountCard({ resolved: 'column', result: many }).get('.bi-chart-stub').attributes('data-height')).toBe('h-64 sm:h-72')
    expect(mountCard({ resolved: 'bar', result: result() }).get('.bi-chart-stub').attributes('data-height')).toBe('h-64 sm:h-72')
  })

  it('keeps showing the previous chart while refreshing', () => {
    const wrapper = mountCard({ pending: true })
    expect(wrapper.find('.bi-chart-stub').exists()).toBe(true)
  })
})

describe('BiWidgetCard title safety', () => {
  it('renders the title as inert text, never as HTML', () => {
    const wrapper = mountCard({ widget: widget({ title: '<img src=x onerror=alert(1)>' }) })
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>')
  })
})

describe('BiDetailTable', () => {
  const mountTable = (props: Record<string, unknown> = {}) =>
    mount(BiDetailTable, {
      props: {
        columns: ['id', 'created_at', 'title', 'extra'],
        rows: [{ id: 'r1', created_at: '2026-01-01T00:00:00Z', title: 'One', extra: { a: 1 } }],
        mono: ['id', 'created_at'],
        loading: false, error: '', page: 1, pageSize: 20, total: 1,
        ...props
      } as never,
      global: { stubs: { ...uiStubs, Button: false, Switch: false } }
    })

  it('renders the given columns and rows; objects are stringified, missing values are marked as null', () => {
    const wrapper = mountTable({ rows: [{ id: 'r1', title: 'One', extra: { a: 1 } }, { id: 'r2' }] })
    expect(wrapper.findAll('th').map((h) => h.text())).toEqual(['id', 'created_at', 'title', 'extra'])
    const cells = wrapper.findAll('tbody td').map((c) => c.text())
    expect(cells).toContain('One')
    expect(cells).toContain('{"a":1}')
    // 缺失值与空字符串不是一回事：缺失显示破折号并带 data-cell="null"。
    const nulls = wrapper.findAll('[data-testid="cell-null"]')
    expect(nulls.length).toBeGreaterThan(0)
    expect(nulls[0].text()).toBe('—')
  })

  it('distinguishes an empty string from a missing value, and keeps 0 / false visible', () => {
    const wrapper = mountTable({ rows: [{ id: 'r1', created_at: 'c', title: '', extra: 0 }] })
    const cells = wrapper.findAll('tbody td')
    const titleText = cells.map((c) => c.text())
    expect(titleText).toContain('0')
    // 空字符串是真实值：不显示破折号。
    expect(wrapper.findAll('[data-testid="cell-null"]')).toHaveLength(0)
  })

  it('expands a long value on click and offers a copy button for it', async () => {
    const long = 'x'.repeat(120)
    const wrapper = mountTable({ columns: ['id', 'note'], rows: [{ id: 'r1', note: long }] })
    const cell = wrapper.get('[data-long]')
    expect(cell.attributes('data-expanded')).toBeUndefined()
    await cell.trigger('click')
    expect(wrapper.get('[data-long]').attributes('data-expanded')).toBeDefined()
    expect(wrapper.find('[data-action="copy-cell"]').exists()).toBe(true)
    // 短值不需要展开，也不显示复制按钮。
    await wrapper.setProps({ columns: ['id', 'note'], rows: [{ id: 'r1', note: 'short' }] } as never)
    expect(wrapper.find('[data-long]').exists()).toBe(false)
    expect(wrapper.find('[data-action="copy-cell"]').exists()).toBe(false)
  })

  it('copies a long value and reports the result', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    const wrapper = mountTable({ columns: ['id', 'note'], rows: [{ id: 'r1', note: 'y'.repeat(100) }] })
    await wrapper.get('[data-action="copy-cell"]').trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledWith('y'.repeat(100))
    expect(toast.success).toHaveBeenCalled()

    writeText.mockRejectedValueOnce(new Error('denied'))
    await wrapper.get('[data-action="copy-cell"]').trigger('click')
    await flushPromises()
    expect(toast.error).toHaveBeenCalled()
  })

  it('remembers hidden columns per data source instead of per component lifetime', async () => {
    window.localStorage.clear()
    const first = mountTable({ storageKey: 'sb_bi_cols_v1:p:db:orders' })
    await first.get('[data-column="extra"]').trigger('click')
    expect(first.findAll('th').map((h) => h.text())).not.toContain('extra')
    expect(window.localStorage.getItem('sb_bi_cols_v1:p:db:orders')).toContain('id')

    // 同一来源重新挂载：沿用保存的偏好。
    const again = mountTable({ storageKey: 'sb_bi_cols_v1:p:db:orders' })
    expect(again.findAll('th').map((h) => h.text())).not.toContain('extra')
    // 另一个来源不受影响。
    const other = mountTable({ storageKey: 'sb_bi_cols_v1:p:db:table:orders' })
    expect(other.findAll('th').map((h) => h.text())).toContain('extra')
  })

  it('uses a monospace font only for the listed columns', () => {
    const wrapper = mountTable()
    const idCell = wrapper.findAll('tbody td').find((c) => c.text() === 'r1')!
    const titleCell = wrapper.findAll('tbody td').find((c) => c.text() === 'One')!
    expect(idCell.find('.sb-mono').exists()).toBe(true)
    expect(titleCell.find('.sb-mono').exists()).toBe(false)
  })

  it('is read-only: no inputs, textareas or save buttons', () => {
    const wrapper = mountTable()
    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.find('textarea').exists()).toBe(false)
    expect(wrapper.findAll('button').some((b) => b.text() === '保存')).toBe(false)
  })

  it('toggles column visibility and keeps the choice when the column set changes', async () => {
    const wrapper = mountTable()
    await wrapper.get('[data-column="title"]').trigger('click')
    expect(wrapper.findAll('th').map((h) => h.text())).not.toContain('title')
    await wrapper.setProps({ columns: ['id', 'title', 'brandNew'], rows: [{ id: 'r2', title: 'Two', brandNew: 'x' }] })
    const heads = wrapper.findAll('th').map((h) => h.text())
    expect(heads).not.toContain('title')
    // 新出现的列默认显示。
    expect(heads).toContain('brandNew')
    await wrapper.get('[data-column="title"]').trigger('click')
    expect(wrapper.findAll('th').map((h) => h.text())).toContain('title')
  })

  it('hides the column picker when there are no columns', () => {
    expect(mountTable({ columns: [], rows: [] }).find('[data-action="columns"]').exists()).toBe(false)
  })

  it('emits page changes from the pager', async () => {
    const wrapper = mountTable({ total: 100 })
    await wrapper.get('.pager-next').trigger('click')
    expect(wrapper.emitted('update:page')).toEqual([[2]])
  })

  it('shows empty, loading and error states', async () => {
    expect(mountTable({ rows: [] }).text()).toContain('当前条件下没有数据')
    const err = mountTable({ rows: [], error: '加载失败' })
    expect(err.text()).toContain('加载失败')
    const retry = err.findAll('button').find((b) => b.text().includes('重试'))
    await retry!.trigger('click')
    expect(err.emitted('retry')).toHaveLength(1)
    await flushPromises()
  })
})
