// bi-compat.ts — 数据看板图表配置的字段 / 度量 / 类型兼容性（planv6.7 §4.3、F4b）。
//
// 类型选择器置灰、编辑器校验、Agent 产出校验共用这一份口径，避免三处各写一套规则。
// 全部是纯函数，不依赖 Vue / i18n：原因用 reason key 返回，由调用方翻译。

import type { BiChartType, BiWidget, CollectionField } from '@/services/types'

/** 内置列：不依赖 schema 登记，恒可用（与后端 collection_aggregate_query.go 一致）。 */
export const BUILTIN_FIELDS: CollectionField[] = ['created_at', 'updated_at'].map((name) => ({
  name,
  type: 'date',
  required: false,
  unique: false,
  options: {},
  hidden: false,
  readonly: true
}))

export const TIME_UNITS = ['hour', 'day', 'week', 'month', 'year'] as const
export type TimeUnit = (typeof TIME_UNITS)[number]

/** 后端 limit 上限：聚合 500，散点 2000。 */
export const AGGREGATE_MAX_LIMIT = 500
export const POINTS_MAX_LIMIT = 2000
export const MAX_METRICS = 6

/** 可累加度量：堆积条形图的合法度量。 */
const ADDITIVE_KINDS = new Set(['count', 'sum'])
const METRIC_KINDS = new Set(['count', 'sum', 'avg', 'min', 'max', 'distinct'])
const NUMBER_ONLY = new Set(['sum', 'avg'])
const NUMBER_OR_DATE = new Set(['min', 'max'])
const CATEGORY_TYPES = new Set<CollectionField['type']>(['text', 'select', 'bool', 'email', 'url'])
const FIELD_NAME = /^[A-Za-z][A-Za-z0-9_]{0,62}$/

export interface ParsedMetric {
  kind: string
  field?: string
}

export interface ParsedGroupBy {
  field: string
  /** 时间桶粒度。 */
  unit?: TimeUnit
  /** 数值分箱步长。 */
  bin?: number
}

export type CompatResult = { ok: true } | { ok: false; reason: string }

const OK: CompatResult = { ok: true }
const no = (reason: string): CompatResult => ({ ok: false, reason })

/** 把 schema 字段表补上内置时间列，返回 name → field 的映射。 */
export function fieldMap(fields: CollectionField[], builtin: boolean = true): Map<string, CollectionField> {
  const map = new Map<string, CollectionField>()
  // 内置时间列是集合的概念（文档表固有的列）；SQL 表只有自己的真实列，不能凭空多出 created_at。
  if (builtin) for (const f of BUILTIN_FIELDS) map.set(f.name, f)
  for (const f of fields) if (!f.hidden) map.set(f.name, f)
  return map
}

export function parseMetric(raw: string): ParsedMetric | null {
  const [kind, field, ...rest] = raw.split(':')
  if (rest.length || !METRIC_KINDS.has(kind)) return null
  if (kind === 'count') return field === undefined ? { kind } : null
  if (field === undefined || !FIELD_NAME.test(field)) return null
  return { kind, field }
}

export function parseGroupBy(raw: string | undefined): ParsedGroupBy | null {
  if (!raw) return null
  const [field, mod, ...rest] = raw.split(':')
  if (rest.length || !FIELD_NAME.test(field)) return null
  if (mod === undefined) return { field }
  if (mod.startsWith('bin=')) {
    const step = Number(mod.slice(4))
    return Number.isFinite(step) && step > 0 ? { field, bin: step } : null
  }
  return (TIME_UNITS as readonly string[]).includes(mod) ? { field, unit: mod as TimeUnit } : null
}

export function formatGroupBy(g: ParsedGroupBy): string {
  if (g.unit) return `${g.field}:${g.unit}`
  if (g.bin !== undefined) return `${g.field}:bin=${g.bin}`
  return g.field
}

/** 度量别名：与后端响应列一致（count / sum_amount …）。 */
export function metricAlias(m: ParsedMetric): string {
  return m.field ? `${m.kind}_${m.field}` : m.kind
}

/** 单个度量与字段类型是否匹配。 */
export function checkMetric(raw: string, fields: Map<string, CollectionField>): CompatResult {
  const m = parseMetric(raw)
  if (!m) return no('metric_invalid')
  if (!m.field) return OK
  const field = fields.get(m.field)
  if (!field) return no('field_unknown')
  if (field.type === 'json') return no('field_json')
  if (NUMBER_ONLY.has(m.kind) && field.type !== 'number') return no('metric_needs_number')
  if (NUMBER_OR_DATE.has(m.kind) && field.type !== 'number' && field.type !== 'date') return no('metric_needs_number_or_date')
  return OK
}

/** 主维度形态：决定「自动」推断与类型兼容性。 */
export type DimensionKind = 'none' | 'time' | 'bin' | 'category'

export function dimensionKind(groupBy: string | undefined): DimensionKind {
  if (!groupBy) return 'none'
  const g = parseGroupBy(groupBy)
  if (g?.unit) return 'time'
  if (g?.bin !== undefined) return 'bin'
  return 'category'
}

export function isAdditiveMetrics(metrics: string[]): boolean {
  return metrics.length > 0 && metrics.every((raw) => {
    const m = parseMetric(raw)
    return m !== null && ADDITIVE_KINDS.has(m.kind)
  })
}

function isScatter(w: BiWidget): boolean {
  return Boolean(w.x && w.y)
}

/** 是否含「日期字段的 min / max」：结果是日期而不是数值，不能画成数值轴，只能 KPI 文本或表格。 */
export function hasDateMetric(metrics: string[], fields: Map<string, CollectionField>): boolean {
  return metrics.some((raw) => {
    const m = parseMetric(raw)
    return m !== null && m.field !== undefined && NUMBER_OR_DATE.has(m.kind) && fields.get(m.field)?.type === 'date'
  })
}

/**
 * 校验字段层面的合法性（与图表类型无关）：字段存在、维度修饰符与字段类型匹配、
 * 度量与字段类型匹配、limit 范围、散点字段为 number。返回首个问题。
 */
export function checkFields(w: BiWidget, fields: Map<string, CollectionField>): CompatResult {
  if (isScatter(w)) {
    for (const [key, name] of [['x', w.x], ['y', w.y], ['size', w.size]] as const) {
      if (!name) continue
      const f = fields.get(name)
      if (!f) return no('field_unknown')
      if (f.type !== 'number') return no(key === 'size' ? 'size_needs_number' : 'scatter_needs_number')
    }
    if (w.series) {
      const f = fields.get(w.series)
      if (!f) return no('field_unknown')
      if (!CATEGORY_TYPES.has(f.type)) return no('series_needs_category')
    }
    if (w.metrics.length || w.groupBy) return no('scatter_no_aggregate')
    return limitCheck(w.limit, POINTS_MAX_LIMIT)
  }
  if (w.metrics.length === 0) return no('metrics_required')
  if (w.metrics.length > MAX_METRICS) return no('metrics_too_many')
  for (const raw of w.metrics) {
    const r = checkMetric(raw, fields)
    if (!r.ok) return r
  }
  if (w.groupBy) {
    const g = parseGroupBy(w.groupBy)
    if (!g) return no('group_by_invalid')
    const f = fields.get(g.field)
    if (!f) return no('field_unknown')
    if (f.type === 'json') return no('field_json')
    if (g.unit && f.type !== 'date') return no('bucket_needs_date')
    if (g.bin !== undefined && f.type !== 'number') return no('bin_needs_number')
  }
  if (w.series) {
    if (!w.groupBy) return no('series_needs_group_by')
    if (w.series.includes(':')) return no('series_no_modifier')
    const f = fields.get(w.series)
    if (!f) return no('field_unknown')
    if (f.type === 'json') return no('field_json')
  }
  const sort = checkSort(w)
  if (!sort.ok) return sort
  return limitCheck(w.limit, AGGREGATE_MAX_LIMIT)
}

/** 可排序的列：维度（x）、系列列、以及每个度量的别名；后端只允许按响应列排序。 */
export function sortableKeys(w: BiWidget): string[] {
  const keys: string[] = []
  if (w.groupBy) keys.push('x')
  if (w.groupBy && w.series) keys.push('series')
  for (const raw of w.metrics) {
    const m = parseMetric(raw)
    if (m) keys.push(metricAlias(m))
  }
  return keys
}

/**
 * sort 必须引用本次响应里真实存在的列。前端提前拦住，避免保存成功、请求时才 400；
 * 也避免用户改了度量后留下一个指向已删除度量的排序。
 */
export function checkSort(w: BiWidget): CompatResult {
  const raw = (w.sort ?? '').trim()
  if (!raw) return OK
  const key = raw.startsWith('-') ? raw.slice(1) : raw
  return sortableKeys(w).includes(key) ? OK : no('sort_unknown')
}

function limitCheck(limit: number | undefined, max: number): CompatResult {
  if (limit === undefined) return OK
  return Number.isInteger(limit) && limit >= 1 && limit <= max ? OK : no('limit_out_of_range')
}

/**
 * 某个具体图表类型能否承载当前配置。
 * `auto` 与 `table` 总是可以（只要字段层面合法）。
 */
export function checkType(type: BiChartType, w: BiWidget, fields: Map<string, CollectionField>): CompatResult {
  // 选了散点却还没配齐 X / Y：直接说明缺什么，而不是落进聚合分支报「至少需要一个度量」。
  if (type === 'scatter' && !isScatter(w)) return no('scatter_needs_xy')
  const base = checkFields(w, fields)
  if (!base.ok) return base
  if (type === 'auto' || type === 'table') return OK
  if (type === 'scatter') return OK
  if (isScatter(w)) return no('xy_needs_scatter')
  const kind = dimensionKind(w.groupBy)
  if (kind === 'none') return no('chart_needs_group_by')
  // 日期的 min / max 结果是日期：画成数值轴会变成 0，只允许表格 / KPI。
  if (hasDateMetric(w.metrics, fields)) return no('metric_date_no_chart')
  switch (type) {
    case 'line':
      if (w.series && w.metrics.length > 1) return no('line_series_one_metric')
      return w.metrics.length <= 3 ? OK : no('line_metrics_limit')
    case 'column':
    case 'bar':
      return w.metrics.length === 1 && !w.series ? OK : no('single_metric_only')
    case 'pie':
      if (kind === 'time' || kind === 'bin') return no('pie_needs_category')
      return w.metrics.length === 1 && !w.series ? OK : no('single_metric_only')
    case 'grouped_bar':
      if (w.series) return w.metrics.length === 1 ? OK : no('grouped_series_one_metric')
      return w.metrics.length >= 2 && w.metrics.length <= 3 ? OK : no('grouped_needs_metrics')
    case 'stacked_bar':
      if (!w.series) return no('stacked_needs_series')
      if (w.metrics.length !== 1) return no('stacked_one_metric')
      return isAdditiveMetrics(w.metrics) ? OK : no('stacked_needs_additive')
    default:
      return OK
  }
}

/** 全部具体类型（不含 auto）按选择器顺序。 */
export const CONCRETE_TYPES: BiChartType[] = ['line', 'column', 'bar', 'pie', 'table', 'grouped_bar', 'stacked_bar', 'scatter']

/** 选择器里的全部 9 项，顺序固定（与产品原型一致）。 */
export const ALL_TYPES: BiChartType[] = ['auto', 'line', 'column', 'bar', 'pie', 'table', 'grouped_bar', 'stacked_bar', 'scatter']

/** 能承载当前配置的具体类型集合。 */
export function compatibleTypes(w: BiWidget, fields: Map<string, CollectionField>): BiChartType[] {
  return CONCRETE_TYPES.filter((t) => checkType(t, w, fields).ok)
}
