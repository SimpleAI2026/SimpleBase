import { describe, expect, it } from 'vitest'
import type { BiChartType, BiWidget, CollectionField } from '@/services/types'
import {
  ALL_TYPES,
  CONCRETE_TYPES,
  checkFields,
  checkMetric,
  checkType,
  dimensionKind,
  fieldMap,
  formatGroupBy,
  hasDateMetric,
  checkSort,
  isAdditiveMetrics,
  metricAlias,
  parseGroupBy,
  parseMetric
} from '@/utils/bi-compat'

const f = (name: string, type: CollectionField['type'], hidden = false): CollectionField => ({
  name, type, required: false, unique: false, options: {}, hidden, readonly: false
})
const fields = fieldMap([
  f('amount', 'number'), f('qty', 'number'), f('status', 'select'), f('title', 'text'),
  f('paid', 'bool'), f('placed', 'date'), f('payload', 'json'), f('secret', 'text', true)
])
const w = (over: Partial<BiWidget>): BiWidget => ({ id: 'w', title: 't', type: 'auto', metrics: ['count'], span: 6, ...over })

describe('bi-compat parsing', () => {
  it('parses metrics', () => {
    expect(parseMetric('count')).toEqual({ kind: 'count' })
    expect(parseMetric('sum:amount')).toEqual({ kind: 'sum', field: 'amount' })
    for (const bad of ['count:amount', 'sum', 'median:amount', 'sum:1a', 'sum:a:b', '', 'sum:x y']) {
      expect(parseMetric(bad), bad).toBeNull()
    }
    expect(metricAlias({ kind: 'sum', field: 'amount' })).toBe('sum_amount')
    expect(metricAlias({ kind: 'count' })).toBe('count')
  })

  it('parses and formats group_by', () => {
    expect(parseGroupBy('status')).toEqual({ field: 'status' })
    expect(parseGroupBy('placed:day')).toEqual({ field: 'placed', unit: 'day' })
    expect(parseGroupBy('amount:bin=10')).toEqual({ field: 'amount', bin: 10 })
    expect(parseGroupBy('amount:bin=0.5')).toEqual({ field: 'amount', bin: 0.5 })
    for (const bad of ['placed:minute', 'amount:bin=0', 'amount:bin=-1', 'amount:bin=x', 'a:b:c', '1x', undefined, '']) {
      expect(parseGroupBy(bad as string | undefined), String(bad)).toBeNull()
    }
    for (const raw of ['status', 'placed:week', 'amount:bin=10']) {
      expect(formatGroupBy(parseGroupBy(raw)!)).toBe(raw)
    }
  })

  it('classifies dimension kind', () => {
    expect(dimensionKind(undefined)).toBe('none')
    expect(dimensionKind('')).toBe('none')
    expect(dimensionKind('placed:day')).toBe('time')
    expect(dimensionKind('amount:bin=5')).toBe('bin')
    expect(dimensionKind('status')).toBe('category')
  })

  it('knows additive metrics', () => {
    expect(isAdditiveMetrics(['count'])).toBe(true)
    expect(isAdditiveMetrics(['sum:amount'])).toBe(true)
    expect(isAdditiveMetrics(['avg:amount'])).toBe(false)
    expect(isAdditiveMetrics(['count', 'max:amount'])).toBe(false)
    expect(isAdditiveMetrics([])).toBe(false)
  })

  it('field map adds builtin time columns and drops hidden fields', () => {
    expect(fields.get('created_at')?.type).toBe('date')
    expect(fields.get('updated_at')?.type).toBe('date')
    expect(fields.has('secret')).toBe(false)
    expect(fields.has('amount')).toBe(true)
  })
})

describe('bi-compat field checks', () => {
  const reason = (r: ReturnType<typeof checkFields>) => (r.ok ? 'ok' : r.reason)

  it('checks metrics against field types', () => {
    expect(reason(checkMetric('count', fields))).toBe('ok')
    expect(reason(checkMetric('sum:amount', fields))).toBe('ok')
    expect(reason(checkMetric('min:placed', fields))).toBe('ok')
    expect(reason(checkMetric('distinct:status', fields))).toBe('ok')
    expect(reason(checkMetric('sum:status', fields))).toBe('metric_needs_number')
    expect(reason(checkMetric('avg:placed', fields))).toBe('metric_needs_number')
    expect(reason(checkMetric('max:title', fields))).toBe('metric_needs_number_or_date')
    expect(reason(checkMetric('distinct:payload', fields))).toBe('field_json')
    expect(reason(checkMetric('sum:nope', fields))).toBe('field_unknown')
    expect(reason(checkMetric('sum:secret', fields))).toBe('field_unknown')
    expect(reason(checkMetric('bogus', fields))).toBe('metric_invalid')
  })

  it('checks aggregate widgets', () => {
    expect(reason(checkFields(w({}), fields))).toBe('ok')
    expect(reason(checkFields(w({ metrics: [] }), fields))).toBe('metrics_required')
    expect(reason(checkFields(w({ metrics: Array(7).fill('count') }), fields))).toBe('metrics_too_many')
    expect(reason(checkFields(w({ groupBy: 'status:day' }), fields))).toBe('bucket_needs_date')
    expect(reason(checkFields(w({ groupBy: 'status:bin=5' }), fields))).toBe('bin_needs_number')
    expect(reason(checkFields(w({ groupBy: 'payload' }), fields))).toBe('field_json')
    expect(reason(checkFields(w({ groupBy: 'nope' }), fields))).toBe('field_unknown')
    expect(reason(checkFields(w({ groupBy: 'placed:minute' }), fields))).toBe('group_by_invalid')
    expect(reason(checkFields(w({ series: 'status' }), fields))).toBe('series_needs_group_by')
    expect(reason(checkFields(w({ groupBy: 'status', series: 'placed:day' }), fields))).toBe('series_no_modifier')
    expect(reason(checkFields(w({ groupBy: 'status', series: 'payload' }), fields))).toBe('field_json')
    expect(reason(checkFields(w({ groupBy: 'status', limit: 0 }), fields))).toBe('limit_out_of_range')
    expect(reason(checkFields(w({ groupBy: 'status', limit: 501 }), fields))).toBe('limit_out_of_range')
    expect(reason(checkFields(w({ groupBy: 'status', limit: 500 }), fields))).toBe('ok')
    expect(reason(checkFields(w({ groupBy: 'status', limit: 1.5 }), fields))).toBe('limit_out_of_range')
  })

  it('checks scatter widgets', () => {
    const scatter = (over: Partial<BiWidget>) => w({ metrics: [], x: 'amount', y: 'qty', ...over })
    expect(reason(checkFields(scatter({}), fields))).toBe('ok')
    expect(reason(checkFields(scatter({ size: 'amount', series: 'status' }), fields))).toBe('ok')
    expect(reason(checkFields(scatter({ x: 'status' }), fields))).toBe('scatter_needs_number')
    expect(reason(checkFields(scatter({ size: 'title' }), fields))).toBe('size_needs_number')
    expect(reason(checkFields(scatter({ series: 'placed' }), fields))).toBe('series_needs_category')
    expect(reason(checkFields(scatter({ x: 'nope' }), fields))).toBe('field_unknown')
    expect(reason(checkFields(scatter({ metrics: ['count'] }), fields))).toBe('scatter_no_aggregate')
    expect(reason(checkFields(scatter({ groupBy: 'status' }), fields))).toBe('scatter_no_aggregate')
    expect(reason(checkFields(scatter({ limit: 2000 }), fields))).toBe('ok')
    expect(reason(checkFields(scatter({ limit: 2001 }), fields))).toBe('limit_out_of_range')
  })
})

describe('bi-compat type matrix', () => {
  const ok = (type: BiChartType, widget: BiWidget) => checkType(type, widget, fields).ok
  const why = (type: BiChartType, widget: BiWidget) => {
    const r = checkType(type, widget, fields)
    return r.ok ? 'ok' : r.reason
  }

  it('lists the nine selector items in prototype order', () => {
    expect(ALL_TYPES).toEqual(['auto', 'line', 'column', 'bar', 'pie', 'table', 'grouped_bar', 'stacked_bar', 'scatter'])
    expect(CONCRETE_TYPES).toHaveLength(8)
    expect(CONCRETE_TYPES).not.toContain('auto')
  })

  it('auto and table accept any valid widget but not invalid fields', () => {
    expect(ok('auto', w({}))).toBe(true)
    expect(ok('table', w({ groupBy: 'status', metrics: ['count', 'sum:amount', 'avg:amount', 'max:amount'] }))).toBe(true)
    expect(why('table', w({ metrics: ['sum:status'] }))).toBe('metric_needs_number')
    expect(why('auto', w({ metrics: [] }))).toBe('metrics_required')
  })

  it('charts need a dimension', () => {
    for (const t of ['line', 'column', 'bar', 'pie', 'grouped_bar', 'stacked_bar'] as const) {
      expect(why(t, w({})), t).toBe('chart_needs_group_by')
    }
  })

  it('line takes up to three metrics', () => {
    expect(ok('line', w({ groupBy: 'placed:day', metrics: ['count', 'sum:amount', 'avg:amount'] }))).toBe(true)
    expect(why('line', w({ groupBy: 'placed:day', metrics: ['count', 'sum:amount', 'avg:amount', 'max:amount'] }))).toBe('line_metrics_limit')
    expect(ok('line', w({ groupBy: 'placed:day', series: 'status' }))).toBe(true)
  })

  it('column and bar take a single metric without series', () => {
    for (const t of ['column', 'bar'] as const) {
      expect(ok(t, w({ groupBy: 'status' })), t).toBe(true)
      expect(why(t, w({ groupBy: 'status', metrics: ['count', 'sum:amount'] })), t).toBe('single_metric_only')
      expect(why(t, w({ groupBy: 'status', series: 'paid' })), t).toBe('single_metric_only')
    }
    expect(ok('column', w({ groupBy: 'amount:bin=10' }))).toBe(true)
  })

  it('pie needs a category dimension', () => {
    expect(ok('pie', w({ groupBy: 'status' }))).toBe(true)
    expect(why('pie', w({ groupBy: 'placed:day' }))).toBe('pie_needs_category')
    expect(why('pie', w({ groupBy: 'amount:bin=10' }))).toBe('pie_needs_category')
  })

  it('grouped_bar needs 2-3 metrics or series with one', () => {
    expect(ok('grouped_bar', w({ groupBy: 'status', metrics: ['count', 'sum:amount'] }))).toBe(true)
    expect(ok('grouped_bar', w({ groupBy: 'status', metrics: ['count', 'sum:amount', 'avg:amount'] }))).toBe(true)
    expect(ok('grouped_bar', w({ groupBy: 'status', series: 'paid', metrics: ['avg:amount'] }))).toBe(true)
    expect(why('grouped_bar', w({ groupBy: 'status' }))).toBe('grouped_needs_metrics')
    expect(why('grouped_bar', w({ groupBy: 'status', metrics: ['count', 'sum:amount', 'avg:amount', 'max:amount'] }))).toBe('grouped_needs_metrics')
    expect(why('grouped_bar', w({ groupBy: 'status', series: 'paid', metrics: ['count', 'sum:amount'] }))).toBe('grouped_series_one_metric')
  })

  it('stacked_bar needs series and an additive metric', () => {
    expect(ok('stacked_bar', w({ groupBy: 'status', series: 'paid' }))).toBe(true)
    expect(ok('stacked_bar', w({ groupBy: 'status', series: 'paid', metrics: ['sum:amount'] }))).toBe(true)
    expect(why('stacked_bar', w({ groupBy: 'status' }))).toBe('stacked_needs_series')
    expect(why('stacked_bar', w({ groupBy: 'status', series: 'paid', metrics: ['avg:amount'] }))).toBe('stacked_needs_additive')
    expect(why('stacked_bar', w({ groupBy: 'status', series: 'paid', metrics: ['min:amount'] }))).toBe('stacked_needs_additive')
    expect(why('stacked_bar', w({ groupBy: 'status', series: 'paid', metrics: ['distinct:title'] }))).toBe('stacked_needs_additive')
    expect(why('stacked_bar', w({ groupBy: 'status', series: 'paid', metrics: ['count', 'sum:amount'] }))).toBe('stacked_one_metric')
  })

  it('scatter and aggregate charts exclude each other', () => {
    const sc = w({ metrics: [], x: 'amount', y: 'qty' })
    expect(ok('scatter', sc)).toBe(true)
    expect(why('column', sc)).toBe('xy_needs_scatter')
    expect(ok('table', sc)).toBe(true)
    expect(why('scatter', w({ groupBy: 'status' }))).toBe('scatter_needs_xy')
  })

})

describe('date metrics and series x multi-metric lines', () => {
  it('hasDateMetric flags min / max over a date field only', () => {
    expect(hasDateMetric(['max:placed'], fields)).toBe(true)
    expect(hasDateMetric(['min:placed', 'count'], fields)).toBe(true)
    expect(hasDateMetric(['max:amount', 'count'], fields)).toBe(false)
    expect(hasDateMetric(['count'], fields)).toBe(false)
    expect(hasDateMetric(['bad'], fields)).toBe(false)
  })

  it('a date min / max cannot be drawn on a chart, but a table still works', () => {
    const cfg = w({ groupBy: 'status', metrics: ['max:placed'] })
    for (const type of ['line', 'column', 'bar', 'pie', 'grouped_bar', 'stacked_bar'] as BiChartType[]) {
      expect(checkType(type, cfg, fields), type).toEqual({ ok: false, reason: expect.any(String) })
    }
    expect(checkType('column', cfg, fields)).toEqual({ ok: false, reason: 'metric_date_no_chart' })
    expect(checkType('table', cfg, fields)).toEqual({ ok: true })
    expect(checkType('auto', cfg, fields)).toEqual({ ok: true })
  })

  it('a line split by series takes exactly one metric (extra metrics used to be dropped silently)', () => {
    const base = { groupBy: 'placed:day', series: 'status' }
    expect(checkType('line', w({ ...base, metrics: ['count'] }), fields)).toEqual({ ok: true })
    expect(checkType('line', w({ ...base, metrics: ['count', 'sum:amount'] }), fields)).toEqual({ ok: false, reason: 'line_series_one_metric' })
    // 没有 series 时多度量折线仍然合法。
    expect(checkType('line', w({ groupBy: 'placed:day', metrics: ['count', 'sum:amount'] }), fields)).toEqual({ ok: true })
  })
})

describe('sort must reference a returned column', () => {
  it('accepts x / series / metric aliases only', () => {
    const ok = w({ groupBy: 'status', metrics: ['count', 'sum:amount'], sort: '-count' })
    expect(checkFields(ok, fields)).toEqual({ ok: true })
    expect(checkFields(w({ groupBy: 'status', metrics: ['sum:amount'], sort: '-sum_amount' }), fields)).toEqual({ ok: true })
    expect(checkFields(w({ groupBy: 'status', metrics: ['count'], sort: 'x' }), fields)).toEqual({ ok: true })
    expect(checkFields(w({ groupBy: 'placed:day', series: 'status', metrics: ['count'], sort: 'series' }), fields)).toEqual({ ok: true })
  })

  it('rejects a stale or unknown sort column instead of failing at request time', () => {
    expect(checkFields(w({ groupBy: 'status', metrics: ['count'], sort: '-sum_amount' }), fields)).toEqual({ ok: false, reason: 'sort_unknown' })
    expect(checkFields(w({ metrics: ['count'], sort: 'x' }), fields)).toEqual({ ok: false, reason: 'sort_unknown' })
    expect(checkSort(w({ groupBy: 'status', series: 'status', metrics: ['count'], sort: 'sum_amount' }))).toEqual({ ok: false, reason: 'sort_unknown' })
  })
})
