import { describe, expect, it } from 'vitest'
import type { BiWidget, CollectionField } from '@/services/types'
import { fieldMap } from '@/utils/bi-compat'
import { COLUMN_MAX_CATEGORIES, COLUMN_MAX_LABEL, PIE_MAX_CATEGORIES, hintsFromRows, resolveAutoType } from '@/utils/bi-auto'

const f = (name: string, type: CollectionField['type']): CollectionField => ({
  name, type, required: false, unique: false, options: {}, hidden: false, readonly: false
})
const fields = fieldMap([f('amount', 'number'), f('qty', 'number'), f('status', 'select'), f('title', 'text'), f('paid', 'bool'), f('placed', 'date')])
const w = (over: Partial<BiWidget>): BiWidget => ({ id: 'w', title: 't', type: 'auto', metrics: ['count'], span: 6, ...over })
const auto = (over: Partial<BiWidget>, hints = {}) => resolveAutoType(w(over), fields, hints)

describe('resolveAutoType (planv6.7 §4.3.1 rules 1-10 in priority order)', () => {
  it('1: x and y without metrics is a scatter', () => {
    expect(auto({ metrics: [], x: 'amount', y: 'qty' })).toBe('scatter')
    // 一旦选了度量，规则 1 不再命中。
    expect(auto({ metrics: ['count'], x: 'amount', y: 'qty' })).toBe('kpi')
  })

  it('2: no dimension and a single metric is a KPI', () => {
    expect(auto({})).toBe('kpi')
    expect(auto({ metrics: ['sum:amount'] })).toBe('kpi')
  })

  it('3: a time bucket is a line', () => {
    expect(auto({ groupBy: 'placed:day' })).toBe('line')
    expect(auto({ groupBy: 'placed:month', series: 'status' })).toBe('line')
    expect(auto({ groupBy: 'placed:hour', metrics: ['count', 'sum:amount'] })).toBe('line')
  })

  it('4: a numeric bin is a column (histogram)', () => {
    expect(auto({ groupBy: 'amount:bin=10' })).toBe('column')
  })

  it('5: category + series + one additive metric is stacked_bar', () => {
    expect(auto({ groupBy: 'status', series: 'paid' })).toBe('stacked_bar')
    expect(auto({ groupBy: 'status', series: 'paid', metrics: ['sum:amount'] })).toBe('stacked_bar')
  })

  it('6: non-additive metric with series, or 2-3 metrics, is grouped_bar', () => {
    expect(auto({ groupBy: 'status', series: 'paid', metrics: ['avg:amount'] })).toBe('grouped_bar')
    expect(auto({ groupBy: 'status', series: 'paid', metrics: ['distinct:title'] })).toBe('grouped_bar')
    expect(auto({ groupBy: 'status', metrics: ['count', 'sum:amount'] })).toBe('grouped_bar')
    expect(auto({ groupBy: 'status', metrics: ['count', 'sum:amount', 'avg:amount'] })).toBe('grouped_bar')
  })

  it('7: few select/bool categories is a pie', () => {
    expect(auto({ groupBy: 'status' })).toBe('pie')
    expect(auto({ groupBy: 'paid' })).toBe('pie')
    expect(auto({ groupBy: 'status' }, { categoryCount: PIE_MAX_CATEGORIES })).toBe('pie')
  })

  it('7 boundary: more than 6 categories leaves the pie', () => {
    expect(auto({ groupBy: 'status' }, { categoryCount: PIE_MAX_CATEGORIES + 1, avgLabelLength: 3 })).toBe('column')
  })

  it('8: up to 12 short categories is a column', () => {
    expect(auto({ groupBy: 'title' }, { categoryCount: 5, avgLabelLength: 4 })).toBe('column')
    expect(auto({ groupBy: 'title' }, { categoryCount: COLUMN_MAX_CATEGORIES, avgLabelLength: COLUMN_MAX_LABEL })).toBe('column')
  })

  it('9: otherwise a horizontal bar', () => {
    // 类目数超过 12。
    expect(auto({ groupBy: 'title' }, { categoryCount: COLUMN_MAX_CATEGORIES + 1, avgLabelLength: 3 })).toBe('bar')
    // 标签平均长度超过 8。
    expect(auto({ groupBy: 'title' }, { categoryCount: 5, avgLabelLength: COLUMN_MAX_LABEL + 0.1 })).toBe('bar')
    // 还没有真实类目数时，非 select/bool 的文本维度先走条形图。
    expect(auto({ groupBy: 'title' })).toBe('bar')
  })

  it('10: too many metrics falls back to a table', () => {
    expect(auto({ groupBy: 'status', metrics: ['count', 'sum:amount', 'avg:amount', 'max:amount'] })).toBe('table')
    expect(auto({ metrics: ['count', 'sum:amount'] })).toBe('table')
    expect(auto({ groupBy: 'status', series: 'paid', metrics: ['count', 'sum:amount'] })).toBe('table')
  })

  it('priority: rule 3 beats 5, rule 5 beats 7', () => {
    expect(auto({ groupBy: 'placed:day', series: 'status' })).toBe('line')
    expect(auto({ groupBy: 'status', series: 'paid' })).toBe('stacked_bar')
  })

  it('re-resolves once real category counts arrive', () => {
    const before = auto({ groupBy: 'status' })
    const after = auto({ groupBy: 'status' }, { categoryCount: 9, avgLabelLength: 4 })
    expect(before).toBe('pie')
    expect(after).toBe('column')
  })
})

describe('hintsFromRows', () => {
  it('counts distinct x labels and their average length', () => {
    expect(hintsFromRows(['x', 'count'], [['ab', 1], ['abcd', 2], ['ab', 3]])).toEqual({ categoryCount: 2, avgLabelLength: (2 + 4 + 2) / 3 })
  })
  it('counts characters, not bytes', () => {
    expect(hintsFromRows(['x', 'count'], [['已付款', 1]]).avgLabelLength).toBe(3)
  })
  it('treats null as an empty label', () => {
    expect(hintsFromRows(['x', 'count'], [[null, 1]])).toEqual({ categoryCount: 1, avgLabelLength: 0 })
  })
  it('returns no hints without an x column or rows', () => {
    expect(hintsFromRows(['count'], [[1]])).toEqual({})
    expect(hintsFromRows(['x', 'count'], [])).toEqual({})
  })
})

describe('resolveAutoType never picks a chart that would drop or distort data', () => {
  it('a date min / max with a dimension falls back to a table (it is a date, not a number)', () => {
    expect(auto({ groupBy: 'status', metrics: ['max:placed'] })).toBe('table')
    expect(auto({ groupBy: 'placed:day', metrics: ['min:placed'] })).toBe('table')
  })

  it('series x several metrics on a time bucket is a table, not a line that drops metrics', () => {
    expect(auto({ groupBy: 'placed:day', series: 'status', metrics: ['count', 'sum:amount'] })).toBe('table')
    expect(auto({ groupBy: 'placed:day', series: 'status', metrics: ['count'] })).toBe('line')
  })

  it('a bin with several metrics or a series is a table (a column holds one metric)', () => {
    expect(auto({ groupBy: 'amount:bin=10', metrics: ['count', 'sum:qty'] })).toBe('table')
    expect(auto({ groupBy: 'amount:bin=10', series: 'status' })).toBe('table')
    expect(auto({ groupBy: 'amount:bin=10' })).toBe('column')
  })
})
