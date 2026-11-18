import { describe, expect, it } from 'vitest'
import type { CollectionField } from '@/services/types'
import { MAX_DEFAULT_WIDGETS, buildDefaultWidgets, type BiDefaultLabels } from '@/utils/bi-defaults'
import { checkType, fieldMap } from '@/utils/bi-compat'

const f = (name: string, type: CollectionField['type'], hidden = false): CollectionField => ({
  name, type, required: false, unique: false, options: {}, hidden, readonly: false
})
const labels: BiDefaultLabels = {
  total: () => 'TOTAL',
  sum: (x) => `sum ${x}`,
  avg: (x) => `avg ${x}`,
  trend: () => 'TREND',
  histogram: (x) => `hist ${x}`,
  share: (x) => `share ${x}`,
  top: (x, n) => `top${n} ${x}`,
  dateTrend: (x) => `trend ${x}`
}
const ids = (fields: CollectionField[], registered = true) => buildDefaultWidgets(fields, registered, labels).widgets.map((w) => w.id)

describe('buildDefaultWidgets (planv6.7 §4.4)', () => {
  it('always has the total KPI and the created_at trend', () => {
    const { widgets, needsSchema } = buildDefaultWidgets([], true, labels)
    expect(needsSchema).toBe(true)
    expect(widgets.map((w) => w.id)).toEqual(['kpi-total', 'trend'])
    expect(widgets[0]).toMatchObject({ title: 'TOTAL', metrics: ['count'], type: 'auto' })
    expect(widgets[1]).toMatchObject({ title: 'TREND', groupBy: 'created_at:day', type: 'line', area: true })
  })

  it('an unregistered schema only gets the two basics even if fields are passed', () => {
    const board = buildDefaultWidgets([f('amount', 'number'), f('status', 'select')], false, labels)
    expect(board.needsSchema).toBe(true)
    expect(board.widgets.map((w) => w.id)).toEqual(['kpi-total', 'trend'])
  })

  it('number fields give sum / avg KPIs and a binned column (histogram)', () => {
    const { widgets, needsSchema } = buildDefaultWidgets([f('amount', 'number'), f('qty', 'number'), f('extra', 'number')], true, labels)
    expect(needsSchema).toBe(false)
    expect(widgets.map((w) => w.id)).toEqual(['kpi-total', 'kpi-sum-amount', 'kpi-avg-qty', 'trend', 'hist-amount', 'hist-qty'])
    expect(widgets.find((w) => w.id === 'kpi-sum-amount')!.metrics).toEqual(['sum:amount'])
    expect(widgets.find((w) => w.id === 'kpi-avg-qty')!.metrics).toEqual(['avg:qty'])
    const hist = widgets.find((w) => w.id === 'hist-amount')!
    expect(hist).toMatchObject({ type: 'column', groupBy: 'amount:bin=10', metrics: ['count'] })
  })

  it('select / bool fields give donut pies (top 8), at most two', () => {
    const { widgets } = buildDefaultWidgets([f('status', 'select'), f('paid', 'bool'), f('kind', 'select')], true, labels)
    const pies = widgets.filter((w) => w.type === 'pie')
    expect(pies.map((w) => w.id)).toEqual(['share-status', 'share-paid'])
    expect(pies[0]).toMatchObject({ donut: true, limit: 8, groupBy: 'status' })
  })

  it('the first text-like field gives a horizontal top-10 bar sorted by count', () => {
    const { widgets } = buildDefaultWidgets([f('title', 'text'), f('mail', 'email')], true, labels)
    const bars = widgets.filter((w) => w.type === 'bar')
    expect(bars).toHaveLength(1)
    expect(bars[0]).toMatchObject({ id: 'top-title', groupBy: 'title', sort: '-count', limit: 10, title: 'top10 title' })
  })

  it('the first non-builtin date field gives a day trend', () => {
    const { widgets } = buildDefaultWidgets([f('placed', 'date'), f('shipped', 'date')], true, labels)
    const trends = widgets.filter((w) => w.id.startsWith('trend-'))
    expect(trends.map((w) => w.id)).toEqual(['trend-placed'])
    expect(trends[0]).toMatchObject({ groupBy: 'placed:day', type: 'line' })
  })

  it('skips hidden fields', () => {
    expect(ids([f('secret', 'number', true), f('status', 'select', true)])).toEqual(['kpi-total', 'trend'])
  })

  it('caps the board at 8 widgets', () => {
    const many = [
      f('a', 'number'), f('b', 'number'), f('s1', 'select'), f('s2', 'bool'), f('t', 'text'), f('d', 'date')
    ]
    const { widgets } = buildDefaultWidgets(many, true, labels)
    expect(widgets).toHaveLength(MAX_DEFAULT_WIDGETS)
    // 基础两个永远在，上限截断的是后面的补充图表。
    expect(widgets[0].id).toBe('kpi-total')
    expect(widgets.map((w) => w.id)).toContain('trend')
  })

  /** 把一行行累加：每一行恰好 12 栅格，说明没有空位。 */
  function rows(spans: number[]): number[] {
    const out: number[] = []
    let acc = 0
    for (const s of spans) {
      if (acc + s > 12) { out.push(acc); acc = 0 }
      acc += s
      if (acc === 12) { out.push(acc); acc = 0 }
    }
    if (acc) out.push(acc)
    return out
  }

  it('lays the board out in full rows: KPIs together, trend full width, charts fill 12 columns', () => {
    const schema = [f('amount', 'number'), f('qty', 'number'), f('status', 'select'), f('paid', 'bool'), f('title', 'text'), f('placed', 'date')]
    const { widgets } = buildDefaultWidgets(schema, true, labels)
    expect(widgets.map((w) => w.id).slice(0, 4)).toEqual(['kpi-total', 'kpi-sum-amount', 'kpi-avg-qty', 'trend'])
    expect(widgets.slice(0, 3).map((w) => w.span)).toEqual([4, 4, 4])
    expect(widgets[3].span).toBe(12)
    expect(rows(widgets.map((w) => w.span)).every((r) => r === 12)).toBe(true)
  })

  it('a lone total KPI sits beside the trend (4 + 8); with no trend it is full width', () => {
    const basic = buildDefaultWidgets([], true, labels).widgets
    expect(basic.map((w) => [w.id, w.span])).toEqual([['kpi-total', 4], ['trend', 8]])
    const noTrend = buildDefaultWidgets([], true, labels, null).widgets
    expect(noTrend.map((w) => [w.id, w.span])).toEqual([['kpi-total', 12]])
  })

  it('every default board fills whole rows for any number of extra charts', () => {
    const pool = [f('a', 'number'), f('b', 'number'), f('s1', 'select'), f('s2', 'bool'), f('t', 'text'), f('d', 'date')]
    for (let n = 0; n <= pool.length; n++) {
      const { widgets } = buildDefaultWidgets(pool.slice(0, n), true, labels)
      expect(rows(widgets.map((w) => w.span)).every((r) => r === 12), `n=${n}: ${widgets.map((w) => w.span).join(',')}`).toBe(true)
    }
  })

  it('has unique ids and every generated widget is valid against its own schema', () => {
    const schema = [f('amount', 'number'), f('qty', 'number'), f('status', 'select'), f('paid', 'bool'), f('title', 'text'), f('placed', 'date')]
    const { widgets } = buildDefaultWidgets(schema, true, labels)
    const all = widgets.map((w) => w.id)
    expect(new Set(all).size).toBe(all.length)
    const fields = fieldMap(schema)
    for (const w of widgets) {
      const r = checkType(w.type, w, fields)
      expect(r.ok, `${w.id}: ${r.ok ? '' : r.reason}`).toBe(true)
      expect([4, 6, 8, 12]).toContain(w.span)
    }
  })
})

describe('default board for SQL tables (identifier and category-like columns)', () => {
  const sqlLabels = { total: () => 'total', sum: (f: string) => `sum ${f}`, avg: (f: string) => `avg ${f}`, trend: () => 'trend', histogram: (f: string) => `hist ${f}`, share: (f: string) => `share ${f}`, top: (f: string) => `top ${f}`, dateTrend: (f: string) => `trend ${f}` }
  const fld = (name: string, type: 'text' | 'number' | 'date' | 'bool' | 'select' | 'json'): import('@/services/types').CollectionField => ({ name, type, required: false, unique: false, options: {}, hidden: false, readonly: true })
  const ids = (fields: ReturnType<typeof fld>[], trend: string | null = null) => buildDefaultWidgets(fields, true, sqlLabels, trend).widgets.map((w) => w.id)

  it('never sums, averages or bins an identifier column', () => {
    const out = ids([fld('id', 'number'), fld('user_id', 'number'), fld('order_no', 'number'), fld('amount', 'number'), fld('qty', 'number')])
    for (const id of ['kpi-sum-id', 'kpi-avg-id', 'hist-id', 'kpi-sum-user_id', 'hist-user_id', 'hist-order_no']) expect(out, id).not.toContain(id)
    expect(out).toContain('kpi-sum-amount')
    expect(out).toContain('kpi-avg-qty')
    expect(out).toContain('hist-amount')
  })

  it('a table of only identifier numbers still gets the total (no chart is better than a meaningless one)', () => {
    const out = ids([fld('id', 'number'), fld('parent_id', 'number')])
    expect(out).toEqual(['kpi-total'])
  })

  it('treats category-named text columns as categories (share chart), not as a Top N bar', () => {
    const out = ids([fld('status', 'text'), fld('channel', 'text'), fld('title', 'text')])
    expect(out).toContain('share-status')
    expect(out).toContain('share-channel')
    expect(out).toContain('top-title')
    expect(out).not.toContain('top-status')
    expect(out).not.toContain('top-channel')
  })

  it('collections keep their behavior: select is the category, plain text is Top N', () => {
    const out = ids([fld('state', 'select'), fld('title', 'text')])
    expect(out).toContain('share-state')
    expect(out).toContain('top-title')
  })

  it('does not double-generate a column that is both category-like and the only text column', () => {
    const out = ids([fld('status', 'text')])
    expect(out.filter((id) => id.endsWith('-status'))).toEqual(['share-status'])
  })

  it('names that merely contain a category word as a suffix or whole name match, not arbitrary substrings', () => {
    // "statusbar_label" 不是分类列：只有整名或 `_xxx` 后缀才算，避免误伤。
    const out = ids([fld('statusbar_label', 'text')])
    expect(out).not.toContain('share-statusbar_label')
    expect(out).toContain('top-statusbar_label')
    expect(ids([fld('order_status', 'text')])).toContain('share-order_status')
  })

  it('identifier-looking names do not hide real measures (id-like suffix only)', () => {
    const out = ids([fld('paid', 'number'), fld('valid', 'number'), fld('idle_seconds', 'number')])
    expect(out).toContain('kpi-sum-paid')
    expect(out).toContain('kpi-avg-valid')
  })
})
