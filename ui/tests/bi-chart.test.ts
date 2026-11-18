import { describe, expect, it } from 'vitest'
import type { AggregateResult, BiWidget } from '@/services/types'
import {
  DEFAULT_TOP_N,
  buildRender,
  compact,
  fillTimeBuckets,
  foldTopN,
  seriesColor,
  type BiLabels,
  type BiTheme,
  type Frame
} from '@/utils/bi-chart'

const theme: BiTheme = {
  palette: ['#2563eb', '#dc2626', '#386582', '#8d5b15', '#15803d'],
  muted: '#64748b', border: '#e2e8f0', card: '#ffffff', foreground: '#111111', sans: 'sans', dark: false
}
const labels: BiLabels = { other: '其他', empty: '（空）', metricName: (alias) => `M:${alias}` }
const w = (over: Partial<BiWidget>): BiWidget => ({ id: 'w', title: 't', type: 'auto', metrics: ['count'], span: 6, ...over })
const res = (columns: string[], rows: unknown[][]): AggregateResult => ({ columns, rows, truncated: false, scanned: rows.length, durationMs: 1, requestId: '' })

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Opt = Record<string, any>
function chart(render: ReturnType<typeof buildRender>): Opt {
  if (render.kind !== 'chart') throw new Error(`expected chart, got ${render.kind}`)
  return render.option as Opt
}

describe('axis orientation (confirmed: column vertical; bar / grouped_bar / stacked_bar horizontal)', () => {
  const rows = [['paid', 3], ['open', 2], ['void', 1]]
  const base = res(['x', 'count'], rows)

  it('column: category on xAxis', () => {
    const o = chart(buildRender({ widget: w({ groupBy: 'status' }), type: 'column', result: base, theme, labels }))
    expect(o.xAxis.type).toBe('category')
    expect(o.yAxis.type).toBe('value')
    expect(o.xAxis.inverse).toBe(false)
    expect(o.series[0].type).toBe('bar')
  })

  for (const type of ['bar', 'grouped_bar', 'stacked_bar'] as const) {
    it(`${type}: category on yAxis, inverse so the first item is on top`, () => {
      const result = type === 'stacked_bar' ? res(['x', 'series', 'count'], [['a', 'p', 1], ['a', 'q', 2], ['b', 'p', 3]]) : base
      const widget = w({ groupBy: 'status', series: type === 'stacked_bar' ? 'paid' : undefined })
      const o = chart(buildRender({ widget, type, result, theme, labels }))
      expect(o.yAxis.type).toBe('category')
      expect(o.yAxis.inverse).toBe(true)
      expect(o.xAxis.type).toBe('value')
      expect(o.series[0].type).toBe('bar')
    })
  }
})

describe('series structure', () => {
  it('stacked_bar puts every series in one stack', () => {
    const result = res(['x', 'series', 'count'], [['a', 'p', 1], ['a', 'q', 2], ['b', 'p', 3], ['b', 'q', 4]])
    const o = chart(buildRender({ widget: w({ groupBy: 'status', series: 'paid' }), type: 'stacked_bar', result, theme, labels }))
    expect(o.series).toHaveLength(2)
    expect(new Set(o.series.map((s: Opt) => s.stack))).toEqual(new Set(['total']))
    // 按类目摊成矩阵：a 的 p=1 q=2，b 的 p=3 q=4（类目按合计降序，b=7 在前）。
    expect(o.yAxis.data).toEqual(['b', 'a'])
    expect(o.series.find((s: Opt) => s.name === 'p').data).toEqual([3, 1])
    expect(o.series.find((s: Opt) => s.name === 'q').data).toEqual([4, 2])
  })

  it('grouped_bar never stacks and has one series per metric', () => {
    const result = res(['x', 'count', 'sum_amount'], [['a', 1, 10], ['b', 2, 20]])
    const o = chart(buildRender({ widget: w({ groupBy: 'status', metrics: ['count', 'sum:amount'] }), type: 'grouped_bar', result, theme, labels }))
    expect(o.series).toHaveLength(2)
    for (const s of o.series) {
      expect(s.stack).toBeUndefined()
      expect(s.barGap).toBe('10%')
    }
    expect(o.series.map((s: Opt) => s.name)).toEqual(['M:count', 'M:sum_amount'])
    expect(o.legend).toBeDefined()
  })

  it('single series has no legend', () => {
    const o = chart(buildRender({ widget: w({ groupBy: 'status' }), type: 'column', result: res(['x', 'count'], [['a', 1]]), theme, labels }))
    expect(o.legend).toBeUndefined()
  })

  it('bar sorts categories by total descending', () => {
    const o = chart(buildRender({ widget: w({ groupBy: 'status' }), type: 'bar', result: res(['x', 'count'], [['a', 1], ['b', 5], ['c', 3]]), theme, labels }))
    expect(o.yAxis.data).toEqual(['b', 'c', 'a'])
    expect(o.series[0].data).toEqual([5, 3, 1])
  })

  it('explicit sort from the server order is kept', () => {
    const o = chart(buildRender({ widget: w({ groupBy: 'status', sort: 'x' }), type: 'bar', result: res(['x', 'count'], [['a', 1], ['b', 5]]), theme, labels }))
    expect(o.yAxis.data).toEqual(['a', 'b'])
  })

  it('bar folds categories beyond the limit into 其他', () => {
    const rows = Array.from({ length: 5 }, (_, i) => [`c${i}`, 10 - i])
    const o = chart(buildRender({ widget: w({ groupBy: 'title', limit: 3 }), type: 'bar', result: res(['x', 'count'], rows), theme, labels }))
    expect(o.yAxis.data).toEqual(['c0', 'c1', 'c2', '其他'])
    expect(o.series[0].data).toEqual([10, 9, 8, 7 + 6])
  })
})

describe('line', () => {
  const result = res(['x', 'count'], [['2026-11-16', 2], ['2026-11-17', 3]])

  it('is a smooth line on a category x axis', () => {
    const o = chart(buildRender({ widget: w({ groupBy: 'placed:day' }), type: 'line', result, theme, labels }))
    expect(o.xAxis.type).toBe('category')
    expect(o.series[0].type).toBe('line')
    expect(o.series[0].smooth).toBe(0.25)
    expect(o.series[0].areaStyle).toBeUndefined()
  })

  it('area fill is a style switch, not a separate type', () => {
    const o = chart(buildRender({ widget: w({ groupBy: 'placed:day', area: true }), type: 'line', result, theme, labels }))
    expect(o.series[0].areaStyle).toBeDefined()
  })

  it('multiple series become multiple lines', () => {
    const r = res(['x', 'series', 'count'], [['2026-11-16', 'a', 1], ['2026-11-16', 'b', 2], ['2026-11-17', 'a', 3]])
    const o = chart(buildRender({ widget: w({ groupBy: 'placed:day', series: 'status' }), type: 'line', result: r, theme, labels }))
    expect(o.series.map((s: Opt) => s.name)).toEqual(['a', 'b'])
    expect(o.series[1].data).toEqual([2, 0])
  })
})

describe('pie', () => {
  const result = res(['x', 'count'], [['paid', 5], ['open', 3], ['void', 1]])

  it('is a full pie by default and a donut on request', () => {
    const full = chart(buildRender({ widget: w({ groupBy: 'status' }), type: 'pie', result, theme, labels }))
    expect(full.series[0].type).toBe('pie')
    expect(full.series[0].radius).toEqual(['0%', '75%'])
    const donut = chart(buildRender({ widget: w({ groupBy: 'status', donut: true }), type: 'pie', result, theme, labels }))
    expect(donut.series[0].radius).toEqual(['55%', '75%'])
    expect(donut.series[0].data.map((d: Opt) => d.name)).toEqual(['paid', 'open', 'void'])
  })

  it('folds beyond Top N into 其他', () => {
    const rows = Array.from({ length: DEFAULT_TOP_N + 3 }, (_, i) => [`c${i}`, 100 - i])
    const o = chart(buildRender({ widget: w({ groupBy: 'title' }), type: 'pie', result: res(['x', 'count'], rows), theme, labels }))
    const names = o.series[0].data.map((d: Opt) => d.name)
    expect(names).toHaveLength(DEFAULT_TOP_N + 1)
    expect(names[names.length - 1]).toBe('其他')
    const other = o.series[0].data[DEFAULT_TOP_N].value
    expect(other).toBe(100 - 8 + (100 - 9) + (100 - 10))
  })

  it('labels a null category as empty, never as the merged "other"', () => {
    const o = chart(buildRender({ widget: w({ groupBy: 'status' }), type: 'pie', result: res(['x', 'count'], [[null, 2]]), theme, labels }))
    expect(o.series[0].data[0].name).toBe('（空）')
  })

  it('does not merge a truncated result or a non-additive metric into a misleading "other"', () => {
    const rows = Array.from({ length: DEFAULT_TOP_N + 3 }, (_, i) => [`c${i}`, 100 - i])
    const truncated = { ...res(['x', 'count'], rows), truncated: true }
    const a = chart(buildRender({ widget: w({ groupBy: 'title' }), type: 'pie', result: truncated, theme, labels }))
    expect(a.series[0].data.map((d: Opt) => d.name)).not.toContain('其他')
    expect(a.series[0].data).toHaveLength(DEFAULT_TOP_N)
    const avg = res(['x', 'avg_amount'], rows)
    const b = chart(buildRender({ widget: w({ groupBy: 'title', metrics: ['avg:amount'] }), type: 'pie', result: avg, theme, labels }))
    expect(b.series[0].data.map((d: Opt) => d.name)).not.toContain('其他')
  })

  it('marks the drill state of folded / empty / duplicated categories instead of trusting display names', () => {
    const rows: unknown[][] = Array.from({ length: DEFAULT_TOP_N + 2 }, (_, i) => [`c${i}`, 100 - i])
    const folded = buildRender({ widget: w({ groupBy: 'title' }), type: 'pie', result: res(['x', 'count'], rows), theme, labels })
    expect(folded.kind === 'chart' && folded.drill?.get('其他')).toBe('folded')
    const empty = buildRender({ widget: w({ groupBy: 'status' }), type: 'pie', result: res(['x', 'count'], [[null, 2], ['a', 1]]), theme, labels })
    expect(empty.kind === 'chart' && empty.drill?.get('（空）')).toBe('empty')
    expect(empty.kind === 'chart' && empty.drill?.get('a')).toBe('ok')
    // 真实值就叫「其他」，又合并出了「其他」：重名，无法确定点的是谁。
    const dup: unknown[][] = [['其他', 500], ...Array.from({ length: DEFAULT_TOP_N + 2 }, (_, i) => [`c${i}`, 100 - i])]
    const ambiguous = buildRender({ widget: w({ groupBy: 'title' }), type: 'pie', result: res(['x', 'count'], dup), theme, labels })
    expect(ambiguous.kind === 'chart' && ambiguous.drill?.get('其他')).toBe('ambiguous')
  })
})

describe('pie with a same-scope total (BI-F01)', () => {
  it('uses the total to include the long tail that the group limit cut off', () => {
    const rows = Array.from({ length: 8 }, (_, i) => [`c${i}`, 10])
    const result = { ...res(['x', 'count'], rows), truncated: true, total: 100 }
    const o = chart(buildRender({ widget: w({ groupBy: 'title', limit: 8 }), type: 'pie', result, theme, labels }))
    const names = o.series[0].data.map((d: Opt) => d.name)
    expect(names).toContain('其他')
    // 8 类共 80，同口径总量 100：剩下的 20 归入「其他」，而不是只对 80 求占比。
    expect(o.series[0].data.find((d: Opt) => d.name === '其他').value).toBe(20)
  })

  it('adds the unreturned remainder to the folded slice when both exist', () => {
    const rows = Array.from({ length: 10 }, (_, i) => [`c${i}`, 10])
    const result = { ...res(['x', 'count'], rows), total: 140 }
    const o = chart(buildRender({ widget: w({ groupBy: 'title', limit: 8 }), type: 'pie', result, theme, labels }))
    const data = o.series[0].data
    expect(data).toHaveLength(9)
    // 已返回尾部 2 类各 10（合并成 20），总量 140、已返回 100 → 未返回 40 → 「其他」= 60。
    expect(data[8].name).toBe('其他')
    expect(data[8].value).toBe(60)
  })

  it('never shows a negative remainder when the total is stale', () => {
    const rows = Array.from({ length: 3 }, (_, i) => [`c${i}`, 10])
    const result = { ...res(['x', 'count'], rows), total: 5 }
    const o = chart(buildRender({ widget: w({ groupBy: 'title' }), type: 'pie', result, theme, labels }))
    expect(o.series[0].data.every((d: Opt) => d.value >= 0)).toBe(true)
    expect(o.series[0].data.map((d: Opt) => d.name)).not.toContain('其他')
  })
})

describe('scatter', () => {
  it('plots raw points with a size channel and series colour groups', () => {
    const r = res(['x', 'y', 'size', 'series'], [[1, 2, 10, 'a'], [3, 4, 20, 'a'], [5, 6, 5, 'b']])
    const o = chart(buildRender({ widget: w({ metrics: [], x: 'amount', y: 'qty', size: 'amount', series: 'status' }), type: 'scatter', result: r, theme, labels }))
    expect(o.series.map((s: Opt) => s.type)).toEqual(['scatter', 'scatter'])
    expect(o.series[0].data).toEqual([[1, 2, 10], [3, 4, 20]])
    expect(o.series[1].data).toEqual([[5, 6, 5]])
    expect(typeof o.series[0].symbolSize).toBe('function')
    // 点越大度量越大，且在 6..28 范围内。
    expect(o.series[0].symbolSize([0, 0, 20])).toBeCloseTo(28)
    expect(o.series[0].symbolSize([0, 0, 0])).toBe(6)
    expect(o.legend).toBeDefined()
    expect(o.xAxis.type).toBe('value')
    expect(o.yAxis.type).toBe('value')
  })

  it('uses a fixed size and no legend without size / series', () => {
    const r = res(['x', 'y'], [[1, 2], [3, 4]])
    const o = chart(buildRender({ widget: w({ metrics: [], x: 'amount', y: 'qty' }), type: 'scatter', result: r, theme, labels }))
    expect(o.series).toHaveLength(1)
    expect(o.series[0].symbolSize).toBe(9)
    expect(o.legend).toBeUndefined()
  })
})

describe('non-chart renders', () => {
  it('table does not produce an echarts option and maps column headers', () => {
    const r = res(['x', 'series', 'count'], [['a', 'p', 1]])
    const out = buildRender({ widget: w({ groupBy: 'status', series: 'paid' }), type: 'table', result: r, theme, labels })
    expect(out.kind).toBe('table')
    if (out.kind === 'table') {
      expect(out.table.columns).toEqual(['status', 'paid', 'M:count'])
      expect(out.table.rows).toEqual([['a', 'p', 1]])
    }
  })

  it('kpi reads the first metric of the first row', () => {
    const out = buildRender({ widget: w({ metrics: ['sum:amount'] }), type: 'kpi', result: res(['sum_amount'], [[60]]), theme, labels })
    expect(out).toEqual({ kind: 'kpi', value: 60, label: 'M:sum_amount' })
    const empty = buildRender({ widget: w({ metrics: ['avg:amount'] }), type: 'kpi', result: res(['avg_amount'], [[null]]), theme, labels })
    expect(empty).toEqual({ kind: 'kpi', value: null, label: 'M:avg_amount' })
  })

  it('kpi keeps a date result as text instead of turning it into 0', () => {
    const out = buildRender({ widget: w({ metrics: ['max:placed'] }), type: 'kpi', result: res(['max_placed'], [['2026-11-16T08:00:00Z']]), theme, labels })
    expect(out).toEqual({ kind: 'kpi', value: '2026-11-16T08:00:00Z', label: 'M:max_placed' })
    // DuckDB 以字符串返回的 Decimal 仍是数值。
    const dec = buildRender({ widget: w({ metrics: ['sum:amount'] }), type: 'kpi', result: res(['sum_amount'], [['12.50']]), theme, labels })
    expect(dec).toMatchObject({ value: 12.5 })
  })

  it('no rows is the empty state for every type', () => {
    for (const type of ['line', 'column', 'bar', 'pie', 'table', 'grouped_bar', 'stacked_bar', 'scatter', 'kpi'] as const) {
      expect(buildRender({ widget: w({ groupBy: 'status' }), type, result: res(['x', 'count'], []), theme, labels }).kind).toBe('empty')
    }
  })
})

describe('helpers', () => {
  it('fills missing day buckets with zero', () => {
    const frame: Frame = { xs: ['2026-11-16', '2026-11-19'], series: new Map([['count', [2, 5]]]) }
    const out = fillTimeBuckets(frame, 'day')
    expect(out.xs).toEqual(['2026-11-16', '2026-11-17', '2026-11-18', '2026-11-19'])
    expect(out.series.get('count')).toEqual([2, 0, 0, 5])
  })

  it('fills month buckets across a year boundary and year buckets', () => {
    const months = fillTimeBuckets({ xs: ['2026-11', '2027-02'], series: new Map([['c', [1, 2]]]) }, 'month')
    expect(months.xs).toEqual(['2026-11', '2026-12', '2027-01', '2027-02'])
    expect(months.series.get('c')).toEqual([1, 0, 0, 2])
    const years = fillTimeBuckets({ xs: ['2024', '2026'], series: new Map([['c', [1, 2]]]) }, 'year')
    expect(years.xs).toEqual(['2024', '2025', '2026'])
  })

  it('fills weekly buckets aligned to the Monday label the backend emits', () => {
    const frame: Frame = { xs: ['2026-11-16', '2026-11-30'], series: new Map([['c', [1, 2]]]) }
    const out = fillTimeBuckets(frame, 'week')
    expect(out.xs).toEqual(['2026-11-16', '2026-11-23', '2026-11-30'])
    expect(out.series.get('c')).toEqual([1, 0, 2])
  })

  it('fills hourly buckets in the "YYYY-MM-DD HH:00" form', () => {
    const frame: Frame = {
      xs: ['2026-11-16 09:00', '2026-11-16 12:00'],
      series: new Map([['c', [1, 2]]]),
      gap: new Map([['c', 0]])
    }
    const out = fillTimeBuckets(frame, 'hour')
    expect(out.xs).toEqual(['2026-11-16 09:00', '2026-11-16 10:00', '2026-11-16 11:00', '2026-11-16 12:00'])
    expect(out.series.get('c')).toEqual([1, 0, 0, 2])
    // 跨天也必须正确进位。
    const across: Frame = { xs: ['2026-11-16 23:00', '2026-11-17 01:00'], series: new Map([['c', [1, 2]]]) }
    expect(fillTimeBuckets(across, 'hour').xs).toEqual(['2026-11-16 23:00', '2026-11-17 00:00', '2026-11-17 01:00'])
  })

  it('leaves an unknown unit and single-bucket frames untouched', () => {
    const frame: Frame = { xs: ['2026-11-16', '2026-11-30'], series: new Map([['c', [1, 2]]]) }
    expect(fillTimeBuckets(frame, undefined)).toBe(frame)
    expect(fillTimeBuckets(frame, 'quarter')).toBe(frame)
    const one: Frame = { xs: ['2026-11-16'], series: new Map([['c', [1]]]) }
    expect(fillTimeBuckets(one, 'day')).toBe(one)
  })

  it('does not loop forever or drop buckets on unparsable labels', () => {
    const frame: Frame = { xs: ['garbage', 'zzz'], series: new Map([['c', [1, 2]]]) }
    // 解析不了就原样返回，已有的桶一个都不能丢。
    expect(fillTimeBuckets(frame, 'day')).toBe(frame)
  })

  it('never truncates existing buckets when the gap is too large to fill', () => {
    const frame: Frame = { xs: ['1900-01-01', '2026-11-16'], series: new Map([['c', [1, 2]]]) }
    const out = fillTimeBuckets(frame, 'day')
    expect(out.xs).toContain('1900-01-01')
    expect(out.xs).toContain('2026-11-16')
    expect(out.series.get('c')!.reduce<number>((a, b) => a + (b ?? 0), 0)).toBe(3)
  })

  it('foldTopN keeps the largest and merges the rest per series', () => {
    const frame: Frame = { xs: ['a', 'b', 'c', 'd'], series: new Map([['p', [1, 10, 5, 2]], ['q', [0, 1, 0, 1]]]) }
    const out = foldTopN(frame, 2, 'rest')
    expect(out.xs).toEqual(['b', 'c', 'rest'])
    expect(out.series.get('p')).toEqual([10, 5, 3])
    expect(out.series.get('q')).toEqual([1, 0, 1])
    expect(out.foldedAt).toBe(2)
    expect(foldTopN(frame, 4, 'rest')).toBe(frame)
  })

  it('foldTopN can truncate without inventing a merged total (non-additive metrics)', () => {
    const frame: Frame = { xs: ['a', 'b', 'c'], series: new Map([['p', [1, 10, 5]]]) }
    const out = foldTopN(frame, 2, 'rest', false)
    expect(out.xs).toEqual(['b', 'c'])
    expect(out.foldedAt).toBeUndefined()
  })

  it('missing buckets: count / sum fill with 0, avg stays a gap (null) instead of a fake drop to 0', () => {
    const counts = chart(buildRender({
      widget: w({ groupBy: 'placed:day' }), type: 'line', theme, labels,
      result: res(['x', 'count'], [['2026-11-16', 2], ['2026-11-18', 5]])
    }))
    expect(counts.series[0].data).toEqual([2, 0, 5])
    const avgs = chart(buildRender({
      widget: w({ groupBy: 'placed:day', metrics: ['avg:amount'] }), type: 'line', theme, labels,
      result: res(['x', 'avg_amount'], [['2026-11-16', 2.5], ['2026-11-18', 4]])
    }))
    expect(avgs.series[0].data).toEqual([2.5, null, 4])
  })

  it('a NULL avg is a gap, not 0', () => {
    const o = chart(buildRender({
      widget: w({ groupBy: 'status', metrics: ['avg:amount'] }), type: 'column', theme, labels,
      result: res(['x', 'avg_amount'], [['a', null], ['b', 3]])
    }))
    expect(o.series[0].data).toEqual([null, 3])
  })

  it('cycles palette colours with lower opacity past five series', () => {
    expect(seriesColor(theme.palette, 0)).toBe('#2563eb')
    expect(seriesColor(theme.palette, 4)).toBe('#15803d')
    expect(seriesColor(theme.palette, 5)).toBe('rgba(37, 99, 235, 0.65)')
    expect(seriesColor(theme.palette, 10)).toBe('rgba(37, 99, 235, 0.4)')
    expect(seriesColor([], 0)).toBe('#64748b')
    // 非十六进制色值不做透明度变换。
    expect(seriesColor(['rgb(1,2,3)'], 1)).toBe('rgb(1,2,3)')
  })

  it('compacts large numbers', () => {
    expect(compact(950)).toBe('950')
    expect(compact(1200)).toBe('1.2k')
    expect(compact(3_400_000)).toBe('3.4M')
    expect(compact(-1500)).toBe('-1.5k')
  })
})
