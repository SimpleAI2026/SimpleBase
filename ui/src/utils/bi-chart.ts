// bi-chart.ts — 聚合结果 → ECharts option 的纯函数。
//
// 方向约定（已确认）：
//   column 纵向（类目轴在 xAxis）；bar / grouped_bar / stacked_bar 横向（类目轴在 yAxis，inverse）。
// 本文件不依赖 echarts 运行时（只引用类型），因此可以不挂载组件直接断言 option。
// 颜色、字体等主题值由调用方（BiChart.vue）读 CSS 变量后以 BiTheme 传入。

import type { EChartsOption, SeriesOption } from 'echarts'
import type { AggregateResult, BiChartType, BiWidget } from '@/services/types'
import type { ResolvedType } from '@/utils/bi-auto'
import { isAdditiveMetrics, metricAlias, parseGroupBy, parseMetric } from '@/utils/bi-compat'

export interface BiTheme {
  /** --chart-1..5；系列超过 5 个时在 palette 上循环并降低透明度。 */
  palette: string[]
  muted: string
  border: string
  card: string
  foreground: string
  sans: string
  dark: boolean
}

/** 文案与名称映射，由组件注入，保持本文件不依赖 i18n。 */
export interface BiLabels {
  other: string
  /** NULL / 空字符串类目的展示名；缺省用破折号。它与「其他」是两回事。 */
  empty?: string
  /** 度量别名 → 展示名，如 sum_amount → 「amount 合计」。 */
  metricName: (alias: string) => string
}

/** 饼图 / 条形图 Top N 之外合并为「其他」。 */
export const DEFAULT_TOP_N = 8

/**
 * 图元能否按「单个值」下钻：
 * - ok：真实的单个类目值；
 * - folded：Top N 合并出来的「其他」，对应的是一批值，没有单值表达式；
 * - empty：NULL / 空字符串，filter 语法里没有对应写法；
 * - ambiguous：展示名与别的类目重名（比如真实值就叫「其他」），无法确定点中的是谁。
 */
export type DrillState = 'ok' | 'folded' | 'empty' | 'ambiguous'

export interface BiTable {
  columns: string[]
  rows: unknown[][]
}

export type BiRender =
  | { kind: 'chart'; option: EChartsOption; drill?: Map<string, DrillState> }
  | { kind: 'table'; table: BiTable }
  | { kind: 'kpi'; value: number | string | null; label: string }
  | { kind: 'empty' }

function withAlpha(color: string, alpha: number): string {
  const m = /^#([0-9a-f]{6})$/i.exec(color)
  if (!m) return color
  const n = parseInt(m[1], 16)
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`
}

/** 系列配色：前 5 色原样，之后循环并依次降低透明度，不引入新的色值字面量。 */
export function seriesColor(palette: string[], index: number): string {
  if (palette.length === 0) return '#64748b'
  const base = palette[index % palette.length]
  const round = Math.floor(index / palette.length)
  if (round === 0) return base
  return withAlpha(base, round === 1 ? 0.65 : 0.4)
}

/** 纵轴刻度：1200 → 1.2k。 */
export function compact(v: number): string {
  if (Math.abs(v) >= 1e6) return `${+(v / 1e6).toFixed(1)}M`
  if (Math.abs(v) >= 1e3) return `${+(v / 1e3).toFixed(1)}k`
  return String(v)
}

/** KPI 数值：千分位，小数最多 2 位；null / 非数字（如 avg 无数据）显示破折号；日期等文本原样显示。 */
export function formatKpi(value: unknown): string {
  if (value === null || value === undefined) return '—'
  if (typeof value === 'string' && value.trim() !== '' && !Number.isFinite(Number(value))) return value
  const n = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(n)) return '—'
  return n.toLocaleString(undefined, { maximumFractionDigits: 2 })
}

function num(v: unknown): number {
  const n = typeof v === 'number' ? v : Number(v)
  return Number.isFinite(n) ? n : 0
}

/** KPI 取值：数值（含 DuckDB 以字符串返回的 Decimal）转 number；日期等文本保持原样，绝不画成 0。 */
function kpiValue(raw: unknown): number | string | null {
  if (raw === null || raw === undefined) return null
  if (typeof raw === 'number') return raw
  if (typeof raw === 'string') {
    const text = raw.trim()
    if (text === '') return null
    const n = Number(text)
    return Number.isFinite(n) ? n : raw
  }
  return num(raw)
}

function label(v: unknown): string {
  return v === null || v === undefined ? '' : String(v)
}

export interface Frame {
  xs: string[]
  /**
   * 系列名 → 与 xs 对齐的数值。null 表示该位置没有观测（avg / min / max 的缺口），
   * 折线上留空、柱不画，不伪装成 0；count / sum 这类可累加度量缺口才是 0。
   */
  series: Map<string, Array<number | null>>
  /** 各系列的缺口取值：可累加度量（count / sum）为 0，其余为 null；补桶时用它填缺失的桶。 */
  gap?: Map<string, number | null>
  /** foldTopN 追加的「其他」在 xs 里的下标；没有合并时为 undefined。 */
  foldedAt?: number
}

function isAdditiveAlias(alias: string): boolean {
  return alias === 'count' || alias.startsWith('sum_')
}

function total(frame: Frame, i: number): number {
  let sum = 0
  for (const arr of frame.series.values()) sum += arr[i] ?? 0
  return sum
}

/**
 * 把聚合行摊成「x × 系列」矩阵。
 * - 有 series 列：系列名取自该列，数值取第一个度量；
 * - 否则每个度量一个系列。
 */
function toFrame(result: AggregateResult, labels: BiLabels): Frame {
  const xi = result.columns.indexOf('x')
  const si = result.columns.indexOf('series')
  const metricCols = result.columns.map((c, i) => ({ c, i })).filter(({ c }) => c !== 'x' && c !== 'series')
  const xs: string[] = []
  const xIndex = new Map<string, number>()
  const series = new Map<string, Array<number | null>>()
  /** 每个系列的缺口初值：可累加为 0，否则 null。 */
  const init = new Map<string, number | null>()
  const ensureX = (x: string): number => {
    let at = xIndex.get(x)
    if (at === undefined) {
      at = xs.length
      xIndex.set(x, at)
      xs.push(x)
      for (const [name, arr] of series) arr.push(init.has(name) ? init.get(name)! : 0)
    }
    return at
  }
  const ensureSeries = (name: string, additive: boolean): Array<number | null> => {
    let arr = series.get(name)
    if (!arr) {
      const fill = additive ? 0 : null
      init.set(name, fill)
      arr = xs.map(() => fill)
      series.set(name, arr)
    }
    return arr
  }
  const put = (arr: Array<number | null>, at: number, raw: unknown) => {
    if (raw === null || raw === undefined) return
    arr[at] = (arr[at] ?? 0) + num(raw)
  }
  for (const row of result.rows) {
    const x = xi >= 0 ? label(row[xi]) : ''
    const at = ensureX(x)
    if (si >= 0 && metricCols.length > 0) {
      // 系列名为空（NULL / 空串）归入「其他」，与 Top N 合并出来的「其他」不是一回事，但画图时同属无名系列。
      const name = label(row[si]) || labels.other
      put(ensureSeries(name, isAdditiveAlias(metricCols[0].c)), at, row[metricCols[0].i])
    } else {
      for (const { c, i } of metricCols) put(ensureSeries(labels.metricName(c), isAdditiveAlias(c)), at, row[i])
    }
  }
  return { xs, series, gap: init }
}

/**
 * Top N 合并：按系列合计（无 series 时按第一个度量）排序，保留前 n 个类目。
 * merge=true（可累加度量）时其余合并成「其他」；merge=false（avg / min / max / distinct 不能相加）时直接截断。
 * 只在类目数超过 n 时才处理；时间桶与分箱是有序维度，不做合并。
 */
export function foldTopN(frame: Frame, n: number, other: string, merge = true): Frame {
  if (frame.xs.length <= n) return frame
  const totals = frame.xs.map((_, i) => ({ i, sum: total(frame, i) }))
  totals.sort((a, b) => b.sum - a.sum || a.i - b.i)
  const keep = new Set(totals.slice(0, n).map((t) => t.i))
  const xs: string[] = []
  const series = new Map<string, Array<number | null>>()
  for (const name of frame.series.keys()) series.set(name, [])
  const rest = new Map<string, number>()
  frame.xs.forEach((x, i) => {
    if (keep.has(i)) {
      xs.push(x)
      for (const [name, arr] of frame.series) series.get(name)!.push(arr[i])
    } else {
      for (const [name, arr] of frame.series) rest.set(name, (rest.get(name) ?? 0) + (arr[i] ?? 0))
    }
  })
  if (!merge) return { xs, series, gap: frame.gap }
  xs.push(other)
  for (const name of frame.series.keys()) series.get(name)!.push(rest.get(name) ?? 0)
  return { xs, series, gap: frame.gap, foldedAt: xs.length - 1 }
}

/**
 * 类目的展示名与下钻状态。原始值为 NULL / 空串显示成 empty 文案（不再冒充「其他」）；
 * 展示名重名（含真实值就叫「其他」）时标记 ambiguous，避免点击后筛到错误的类目。
 */
export function describeCategories(frame: Frame, labels: BiLabels): { names: string[]; drill: Map<string, DrillState> } {
  const empty = labels.empty ?? '—'
  const names = frame.xs.map((x, i) => (i === frame.foldedAt ? labels.other : x === '' ? empty : x))
  const seen = new Map<string, number>()
  for (const name of names) seen.set(name, (seen.get(name) ?? 0) + 1)
  const drill = new Map<string, DrillState>()
  names.forEach((name, i) => {
    let state: DrillState = 'ok'
    if (i === frame.foldedAt) state = 'folded'
    else if (frame.xs[i] === '') state = 'empty'
    if ((seen.get(name) ?? 0) > 1) state = 'ambiguous'
    drill.set(name, state)
  })
  return { names, drill }
}

/**
 * 时间桶补缺口：缺失的桶按系列的缺口值填（count / sum 补 0，avg / min / max 留空），让折线 / 柱状在时间轴上连续。
 * 只处理 day / month / year；hour / week 不补，原样返回。
 */
export function fillTimeBuckets(frame: Frame, unit: string | undefined): Frame {
  if (!unit || frame.xs.length < 2) return frame
  const next = (s: string): string | null => {
    if (unit === 'hour') {
      // 后端桶标签形如 2026-11-16 09:00（UTC，与 DateBucket 的 %Y-%m-%d %H:00 一致）。
      const m = /^(\d{4})-(\d{2})-(\d{2}) (\d{2}):00$/.exec(s)
      if (!m) return null
      const d = new Date(Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4]))
      if (Number.isNaN(d.getTime())) return null
      d.setUTCHours(d.getUTCHours() + 1)
      return `${d.toISOString().slice(0, 10)} ${String(d.getUTCHours()).padStart(2, '0')}:00`
    }
    if (unit === 'week') {
      // 两个引擎的 week 桶都对齐到周一，标签就是那天的日期（%Y-%m-%d）：+7 天即下一个桶。
      const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s)
      if (!m) return null
      const d = new Date(Date.UTC(+m[1], +m[2] - 1, +m[3]))
      if (Number.isNaN(d.getTime())) return null
      d.setUTCDate(d.getUTCDate() + 7)
      return d.toISOString().slice(0, 10)
    }
    if (unit === 'day') {
      const d = new Date(`${s}T00:00:00Z`)
      if (Number.isNaN(d.getTime())) return null
      d.setUTCDate(d.getUTCDate() + 1)
      return d.toISOString().slice(0, 10)
    }
    if (unit === 'month') {
      const [y, m] = s.split('-').map(Number)
      if (!y || !m) return null
      return m === 12 ? `${y + 1}-01` : `${y}-${String(m + 1).padStart(2, '0')}`
    }
    if (unit === 'year') {
      const y = Number(s)
      return Number.isFinite(y) ? String(y + 1) : null
    }
    return null
  }
  const sorted = [...frame.xs].sort()
  const first = sorted[0]
  const last = sorted[sorted.length - 1]
  const full: string[] = []
  let cursor: string | null = first
  // 上限保护：异常数据不让循环失控。
  for (let guard = 0; cursor !== null && cursor <= last && guard < 2000; guard++) {
    full.push(cursor)
    cursor = next(cursor)
  }
  // 没能走到最后一个桶（粒度不支持补零如 hour / week，或桶标签无法解析）时必须放弃补零，
  // 否则会把已有的桶截断丢掉。
  if (full[full.length - 1] !== last || full.length === frame.xs.length) return frame
  const at = new Map(frame.xs.map((x, i) => [x, i]))
  const series = new Map<string, Array<number | null>>()
  for (const [name, arr] of frame.series) {
    // 注意 null 是合法的缺口值（avg / min / max 没有观测）：不能用 ?? 兜底成 0，只在没登记缺口时才默认 0。
    const gap = frame.gap?.has(name) ? frame.gap.get(name)! : 0
    series.set(name, full.map((x) => (at.has(x) ? arr[at.get(x)!] : gap)))
  }
  return { xs: full, series, gap: frame.gap }
}

interface Axes {
  category: 'x' | 'y'
}

function baseOption(theme: BiTheme): EChartsOption {
  return {
    animationDuration: 400,
    animationEasing: 'cubicOut',
    textStyle: { fontFamily: theme.sans, color: theme.muted, fontSize: 12 },
    tooltip: {
      backgroundColor: theme.card,
      borderColor: theme.border,
      borderWidth: 1,
      padding: [8, 12],
      extraCssText: 'border-radius: 8px; box-shadow: 0 4px 16px rgba(0,0,0,0.08);',
      textStyle: { color: theme.foreground, fontSize: 12 },
      valueFormatter: (v) => (typeof v === 'number' ? v.toLocaleString() : String(v ?? ''))
    }
  }
}

function legendOf(names: string[], theme: BiTheme): EChartsOption['legend'] {
  if (names.length < 2) return undefined
  return { top: 0, type: 'scroll', textStyle: { color: theme.muted, fontSize: 12 }, icon: 'roundRect', itemWidth: 10, itemHeight: 10 }
}

function categoryAxis(data: string[], theme: BiTheme, inverse: boolean) {
  return {
    type: 'category' as const,
    data,
    inverse,
    boundaryGap: true,
    axisLine: { lineStyle: { color: theme.border, width: 1 } },
    axisTick: { show: false },
    axisLabel: { color: theme.muted, fontSize: 12, margin: 12, hideOverlap: true }
  }
}

function valueAxis(theme: BiTheme) {
  return {
    type: 'value' as const,
    minInterval: 1,
    splitNumber: 4,
    splitLine: { lineStyle: { color: theme.border, type: [4, 4] as [number, number], width: 1 } },
    axisLine: { show: false },
    axisTick: { show: false },
    axisLabel: { color: theme.muted, fontSize: 12, margin: 12, formatter: (v: number) => compact(v) }
  }
}

/** 横向图：类目在 yAxis（inverse 让第一项在最上面），数值在 xAxis。names 是类目的展示名（已处理空值与「其他」）。 */
function cartesian(
  frame: Frame,
  names: string[],
  theme: BiTheme,
  axes: Axes,
  build: (name: string, data: Array<number | null>, color: string, index: number) => SeriesOption
): EChartsOption {
  const seriesNames = [...frame.series.keys()]
  const horizontal = axes.category === 'y'
  const series = seriesNames.map((name, i) => build(name, frame.series.get(name)!, seriesColor(theme.palette, i), i))
  const cat = categoryAxis(names, theme, horizontal)
  const val = valueAxis(theme)
  return {
    ...baseOption(theme),
    legend: legendOf(seriesNames, theme),
    tooltip: { ...(baseOption(theme).tooltip as object), trigger: 'axis', axisPointer: { type: 'shadow' } },
    grid: { left: 0, right: 8, top: seriesNames.length > 1 ? 32 : 16, bottom: 8, containLabel: true },
    xAxis: horizontal ? val : cat,
    yAxis: horizontal ? cat : val,
    series
  }
}

function barSeries(name: string, data: Array<number | null>, color: string, extra: Partial<SeriesOption> = {}, horizontal = false): SeriesOption {
  return {
    name,
    type: 'bar',
    data,
    barMaxWidth: 28,
    itemStyle: { color, borderRadius: horizontal ? [0, 4, 4, 0] : [4, 4, 0, 0] },
    emphasis: { itemStyle: { color: withAlpha(color, 0.85) } },
    ...extra
  } as SeriesOption
}

export interface BuildInput {
  widget: BiWidget
  /** 已解析的具体类型（auto 由 bi-auto 解析后传入）。 */
  type: ResolvedType
  result: AggregateResult
  theme: BiTheme
  labels: BiLabels
}

/**
 * 主入口：返回 ECharts option，或表格 / KPI / 空态的渲染描述。
 * 表格类型不产出 ECharts option（复用 ui/table）。
 */
export function buildRender(input: BuildInput): BiRender {
  const { widget, type, result, theme, labels } = input
  if (result.rows.length === 0) return { kind: 'empty' }

  if (type === 'table') {
    return {
      kind: 'table',
      table: {
        columns: result.columns.map((c) => (c === 'x' ? (widget.groupBy ?? c) : c === 'series' ? (widget.series ?? c) : labels.metricName(c))),
        rows: result.rows
      }
    }
  }
  if (type === 'kpi') {
    const idx = result.columns.findIndex((c) => c !== 'x' && c !== 'series')
    const raw = idx >= 0 ? result.rows[0][idx] : null
    const metric = widget.metrics[0] ? parseMetric(widget.metrics[0]) : null
    return {
      kind: 'kpi',
      value: kpiValue(raw),
      label: metric ? labels.metricName(metricAlias(metric)) : ''
    }
  }
  if (type === 'scatter') return { kind: 'chart', option: buildScatter(result, theme, labels) }
  // 可累加度量才允许合并「其他」。结果被截断时，只有拿到同口径总量（后端 with_total）才敢合并，
  // 否则「其他」只是已返回的尾部，会把部分分组当成全量占比。
  const mergeOther = isAdditiveMetrics(widget.metrics) && (!result.truncated || typeof result.total === 'number')
  if (type === 'pie') return buildPie(widget, result, theme, labels, mergeOther)

  const group = parseGroupBy(widget.groupBy)
  const ordered = Boolean(group?.unit) || group?.bin !== undefined
  let frame = toFrame(result, labels)
  if (group?.unit) frame = fillTimeBuckets(frame, group.unit)
  // 条形类（横向）类目多时按合计降序并合并「其他」；有序维度（时间 / 分箱）保持原序。
  if (!ordered && (type === 'bar' || type === 'grouped_bar' || type === 'stacked_bar') && !widget.sort) {
    frame = sortByTotal(frame)
  }
  if (!ordered && type === 'bar') frame = foldTopN(frame, widget.limit ?? 20, labels.other, mergeOther)
  const { names, drill } = describeCategories(frame, labels)
  const chart = (option: EChartsOption): BiRender => ({ kind: 'chart', option, drill })

  switch (type) {
    case 'line':
      return chart(buildLine(widget, frame, names, theme))
    case 'column':
      return chart(cartesian(frame, names, theme, { category: 'x' }, (n, d, c) => barSeries(n, d, c)))
    case 'bar':
      return chart(cartesian(frame, names, theme, { category: 'y' }, (n, d, c) => barSeries(n, d, c, {}, true)))
    case 'grouped_bar':
      return chart(cartesian(frame, names, theme, { category: 'y' }, (n, d, c) => barSeries(n, d, c, { barGap: '10%' }, true)))
    case 'stacked_bar':
      return chart(cartesian(frame, names, theme, { category: 'y' }, (n, d, c) => barSeries(n, d, c, { stack: 'total', barMaxWidth: 28 }, true)))
    default:
      return { kind: 'empty' }
  }
}

function sortByTotal(frame: Frame): Frame {
  const order = frame.xs.map((_, i) => ({ i, sum: total(frame, i) }))
  order.sort((a, b) => b.sum - a.sum || a.i - b.i)
  const series = new Map<string, Array<number | null>>()
  for (const [name, arr] of frame.series) series.set(name, order.map((o) => arr[o.i]))
  return { xs: order.map((o) => frame.xs[o.i]), series, gap: frame.gap }
}

function buildLine(widget: BiWidget, frame: Frame, names: string[], theme: BiTheme): EChartsOption {
  const seriesNames = [...frame.series.keys()]
  const dense = frame.xs.length > 1
  const series: SeriesOption[] = seriesNames.map((name, i) => {
    const color = seriesColor(theme.palette, i)
    return {
      name,
      type: 'line',
      data: frame.series.get(name)!,
      smooth: 0.25,
      symbol: 'circle',
      symbolSize: 8,
      showSymbol: !dense,
      lineStyle: { color, width: 2.5 },
      itemStyle: { color, borderColor: theme.card, borderWidth: 2 },
      ...(widget.area
        ? {
            areaStyle: {
              color: {
                type: 'linear',
                x: 0, y: 0, x2: 0, y2: 1,
                colorStops: [
                  { offset: 0, color: withAlpha(color, theme.dark ? 0.32 : 0.2) },
                  { offset: 1, color: withAlpha(color, theme.dark ? 0.04 : 0.02) }
                ]
              }
            }
          }
        : {}),
      emphasis: { focus: 'series' }
    } as SeriesOption
  })
  return {
    ...baseOption(theme),
    legend: legendOf(seriesNames, theme),
    tooltip: { ...(baseOption(theme).tooltip as object), trigger: 'axis', axisPointer: { type: 'line', lineStyle: { color: theme.muted, width: 1, type: 'dashed', opacity: 0.6 } } },
    grid: { left: 0, right: 8, top: seriesNames.length > 1 ? 32 : 16, bottom: 8, containLabel: true },
    xAxis: categoryAxis(names, theme, false),
    yAxis: valueAxis(theme),
    series
  }
}

function buildPie(widget: BiWidget, result: AggregateResult, theme: BiTheme, labels: BiLabels, merge: boolean): BiRender {
  const frame = foldTopN(toFrame(result, labels), widget.limit ?? DEFAULT_TOP_N, labels.other, merge)
  const { names, drill } = describeCategories(frame, labels)
  const first = [...frame.series.values()][0] ?? []
  const data = names.map((name, i) => ({
    name,
    value: first[i] ?? 0,
    itemStyle: { color: seriesColor(theme.palette, i) }
  }))
  // 有同口径总量时，「其他」用它减掉已返回分组，而不是只累加被返回的那几行：
  // 这样占比分母是全部数据，被 limit 截掉的尾部也不会凭空消失。
  if (typeof result.total === 'number' && Number.isFinite(result.total) && data.length > 0) {
    const shown = data.reduce((sum, d) => sum + (typeof d.value === 'number' ? d.value : 0), 0)
    const folded = frame.foldedAt === undefined ? undefined : data[frame.foldedAt]
    // 「其余」= 总量 − 保留下来的前 N 类（已含被合并的尾部与从未返回的部分）。
    const rest = Math.max(result.total - (folded ? shown - folded.value : shown), 0)
    if (folded) {
      // 不能把它加上已合并的尾部（那是重复计数）；总量比观察值还小时保留观察值，避免扇区反向缩小。
      folded.value = Math.max(rest, folded.value)
    } else if (rest > 0) {
      data.push({ name: labels.other, value: rest, itemStyle: { color: seriesColor(theme.palette, data.length) } })
      drill.set(labels.other, 'folded')
    }
  }
  const option: EChartsOption = {
    ...baseOption(theme),
    tooltip: { ...(baseOption(theme).tooltip as object), trigger: 'item' },
    legend: { bottom: 0, type: 'scroll', textStyle: { color: theme.muted, fontSize: 12 }, icon: 'roundRect', itemWidth: 10, itemHeight: 10 },
    series: [
      {
        type: 'pie',
        radius: widget.donut ? ['55%', '75%'] : ['0%', '75%'],
        center: ['50%', '45%'],
        data,
        label: { color: theme.muted, fontSize: 12 },
        itemStyle: { borderColor: theme.card, borderWidth: 2 },
        emphasis: { scale: true }
      }
    ]
  }
  return { kind: 'chart', option, drill }
}

function buildScatter(result: AggregateResult, theme: BiTheme, labels: BiLabels): EChartsOption {
  const xi = result.columns.indexOf('x')
  const yi = result.columns.indexOf('y')
  const zi = result.columns.indexOf('size')
  const si = result.columns.indexOf('series')
  const groups = new Map<string, number[][]>()
  for (const row of result.rows) {
    const key = si >= 0 ? label(row[si]) || labels.other : ''
    const point = zi >= 0 ? [num(row[xi]), num(row[yi]), num(row[zi])] : [num(row[xi]), num(row[yi])]
    const list = groups.get(key)
    if (list) list.push(point)
    else groups.set(key, [point])
  }
  const names = [...groups.keys()]
  const sizes = zi >= 0 ? result.rows.map((r) => num(r[zi])) : []
  const maxSize = sizes.length ? Math.max(...sizes) : 0
  const series: SeriesOption[] = names.map((name, i) => {
    const color = seriesColor(theme.palette, i)
    return {
      name,
      type: 'scatter',
      data: groups.get(name)!,
      symbolSize: zi >= 0 && maxSize > 0 ? (d: number[]) => 6 + (Math.max(d[2], 0) / maxSize) * 22 : 9,
      itemStyle: { color: withAlpha(color, 0.75), borderColor: color, borderWidth: 1 }
    } as SeriesOption
  })
  return {
    ...baseOption(theme),
    legend: names.length > 1 ? legendOf(names, theme) : undefined,
    tooltip: { ...(baseOption(theme).tooltip as object), trigger: 'item' },
    grid: { left: 0, right: 16, top: names.length > 1 ? 32 : 16, bottom: 8, containLabel: true },
    xAxis: { ...valueAxis(theme), minInterval: undefined, scale: true },
    yAxis: { ...valueAxis(theme), minInterval: undefined, scale: true },
    series
  }
}
