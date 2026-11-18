// bi-defaults.ts — 按 schema 字段类型生成默认看板（planv6.7 §4.4）。
//
// 纯函数。零配置即可看：恒有「总记录」与「记录趋势」；其余按字段类型补充，
// 总数上限 8 个，避免首屏请求风暴。标题是文案，由调用方经 BiDefaultLabels 注入（本文件不依赖 i18n）。

import type { BiSpan, BiWidget, CollectionField } from '@/services/types'

export const MAX_DEFAULT_WIDGETS = 8
/** 每类字段最多取几个。 */
const MAX_NUMBER_FIELDS = 2
const MAX_CATEGORY_FIELDS = 2
const MAX_TEXT_FIELDS = 1
const MAX_DATE_FIELDS = 1
export const TOP_N = 10
export const PIE_TOP_N = 8

export interface BiDefaultLabels {
  total: () => string
  sum: (field: string) => string
  avg: (field: string) => string
  trend: () => string
  histogram: (field: string) => string
  share: (field: string) => string
  top: (field: string, n: number) => string
  dateTrend: (field: string) => string
}

export interface DefaultBoard {
  widgets: BiWidget[]
  /** schema 未登记时为 true：前端额外展示「登记 Schema」提示卡。 */
  needsSchema: boolean
}

function widget(id: string, title: string, over: Partial<BiWidget> & Pick<BiWidget, 'metrics'>, span: BiSpan): BiWidget {
  return { id, title, type: 'auto', span, ...over }
}

/** n 张图表补满 12 栅格的每张宽度：偶数 6 / 6，3 的倍数 4 / 4 / 4，其余奇数先 4×3 再 6 补齐。 */
function fillSpans(n: number): BiSpan[] {
  if (n <= 0) return []
  if (n === 1) return [12]
  if (n % 2 === 0) return Array<BiSpan>(n).fill(6)
  if (n % 3 === 0) return Array<BiSpan>(n).fill(4)
  return [4, 4, 4, ...Array<BiSpan>(n - 3).fill(6)]
}

/**
 * 排版：KPI 同一行 → 趋势图 → 其余图表逐行补满 12 栅格（不留空位）。
 * 只有「总记录」一个 KPI 时与趋势图同排（4 + 8）；顺序即阅读顺序，也是超出上限时的截断优先级。
 */
function layout(widgets: BiWidget[]): BiWidget[] {
  const kpis = widgets.filter((w) => w.id.startsWith('kpi-'))
  const trend = widgets.find((w) => w.id === 'trend')
  const rest = widgets.filter((w) => !w.id.startsWith('kpi-') && w.id !== 'trend')
  const ordered = [...kpis, ...(trend ? [trend] : []), ...rest].slice(0, MAX_DEFAULT_WIDGETS)
  const kpiCount = ordered.filter((w) => w.id.startsWith('kpi-')).length
  const beside = kpiCount === 1 && ordered.some((w) => w.id === 'trend')
  const kpiSpan = (beside ? 4 : kpiCount > 0 ? 12 / kpiCount : 12) as BiSpan
  const others = ordered.filter((w) => !w.id.startsWith('kpi-') && w.id !== 'trend')
  const spans = fillSpans(others.length)
  let i = 0
  return ordered.map((w) => {
    if (w.id.startsWith('kpi-')) return { ...w, span: kpiSpan }
    if (w.id === 'trend') return { ...w, span: beside ? 8 : 12 }
    return { ...w, span: spans[i++] }
  })
}

/**
 * 标识列：主键 / 外键 / 序号。对它们求和、求平均、画分布没有分析意义，默认看板里要避开。
 * SQL 表的主键通常是 BIGINT（会被当成数值字段）；集合没有这类字段，不受影响。
 * 只在默认看板生成时避开——用户仍可在编辑器里手动选它们。
 */
const IDENTIFIER_NAME = /^(id|uuid|guid|pk|rowid|seq|serial|index|idx)$|(_id|_uuid|_key|_no|_seq)$/i

function isIdentifier(f: CollectionField): boolean {
  return IDENTIFIER_NAME.test(f.name)
}

/** 取前 n 个满足条件的可见字段（保持 schema 顺序）；标识列不入选。 */
function pick(fields: CollectionField[], types: CollectionField['type'][], n: number): CollectionField[] {
  return fields.filter((f) => !f.hidden && !isIdentifier(f) && types.includes(f.type)).slice(0, n)
}

/**
 * 低基数文本：SQL 表里没有 `select` 类型，状态 / 类型 / 渠道这类列只是 VARCHAR。
 * 按列名判断它是否「更像分类」，让它和集合里的 select 一样生成占比图而不是 Top N。
 */
const CATEGORY_NAME = /(^|_)(status|state|type|kind|category|level|channel|source|stage|phase|role|gender|country|region|city|plan|tier|priority|result|method|platform|lang|language)$/i

function isCategoryLike(f: CollectionField): boolean {
  return f.type === 'text' && CATEGORY_NAME.test(f.name)
}

/**
 * 生成默认看板。fields 为已登记字段；registered=false 表示集合没有登记 schema
 * （此时只能看总量与时间趋势，字段级图表一律不生成）。
 */
export function buildDefaultWidgets(
  fields: CollectionField[],
  registered: boolean,
  labels: BiDefaultLabels,
  /** 「记录趋势」用的时间列：集合恒为 created_at；SQL 表取自己的时间列，没有则不生成趋势。 */
  trendField: string | null = 'created_at'
): DefaultBoard {
  const widgets: BiWidget[] = [widget('kpi-total', labels.total(), { metrics: ['count'], type: 'auto' }, trendField ? 4 : 12)]
  if (trendField) {
    widgets.push(widget('trend', labels.trend(), { metrics: ['count'], groupBy: `${trendField}:day`, type: 'line', area: true }, 8))
  }
  if (!registered || fields.length === 0) return { widgets: layout(widgets), needsSchema: true }

  const numbers = pick(fields, ['number'], MAX_NUMBER_FIELDS)
  // 集合：select / bool 是分类；SQL 表：名字像分类的文本列也算分类（否则状态列只会得到一张 Top N 条形图）。
  const categories = [
    ...pick(fields, ['select', 'bool'], MAX_CATEGORY_FIELDS),
    ...fields.filter((f) => !f.hidden && isCategoryLike(f))
  ].filter((f, i, all) => all.findIndex((x) => x.name === f.name) === i).slice(0, MAX_CATEGORY_FIELDS)
  const categoryNames = new Set(categories.map((f) => f.name))
  const texts = pick(fields, ['text', 'email', 'url'], MAX_TEXT_FIELDS + categoryNames.size).filter((f) => !categoryNames.has(f.name)).slice(0, MAX_TEXT_FIELDS)
  const dates = pick(fields, ['date'], MAX_DATE_FIELDS)

  const extra: BiWidget[] = []
  numbers.forEach((f, i) => {
    // 第 1 个数值字段给合计，第 2 个给平均，避免 KPI 行重复同一字段。
    extra.push(widget(`kpi-${i === 0 ? 'sum' : 'avg'}-${f.name}`, i === 0 ? labels.sum(f.name) : labels.avg(f.name), { metrics: [i === 0 ? `sum:${f.name}` : `avg:${f.name}`], type: 'auto' }, 4))
  })
  numbers.forEach((f) => {
    extra.push(widget(`hist-${f.name}`, labels.histogram(f.name), { metrics: ['count'], groupBy: `${f.name}:bin=10`, type: 'column' }, 6))
  })
  categories.forEach((f) => {
    extra.push(widget(`share-${f.name}`, labels.share(f.name), { metrics: ['count'], groupBy: f.name, type: 'pie', donut: true, limit: PIE_TOP_N }, 4))
  })
  texts.forEach((f) => {
    extra.push(widget(`top-${f.name}`, labels.top(f.name, TOP_N), { metrics: ['count'], groupBy: f.name, sort: '-count', type: 'bar', limit: TOP_N }, 6))
  })
  dates.filter((f) => f.name !== trendField).forEach((f) => {
    extra.push(widget(`trend-${f.name}`, labels.dateTrend(f.name), { metrics: ['count'], groupBy: `${f.name}:day`, type: 'line' }, 6))
  })

  return { widgets: layout([...widgets, ...extra]), needsSchema: false }
}
